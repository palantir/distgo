// Copyright 2016 Palantir Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package defaultdockerbuilder

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/palantir/distgo/distgo"
	"github.com/pkg/errors"
)

const TypeName = "default"

// dockerDaemonOutputArg loads the built image into the local Docker daemon so it is usable with "docker run". A
// cross-platform build additionally requires the daemon to run the containerd image store, since the classic image
// store cannot hold a manifest list and rejects the export.
const dockerDaemonOutputArg = "--output=type=docker,rewrite-timestamp=true"

type Option func(*DefaultDockerBuilder)

type DefaultDockerBuilder struct {
	BuildArgs         []string
	BuildArgsScript   string
	BuildxDriverOpts  []string
	BuildxPlatformArg string
}

func NewDefaultDockerBuilder(buildArgs []string, buildArgsScript string) distgo.DockerBuilder {
	return &DefaultDockerBuilder{
		BuildArgs:       buildArgs,
		BuildArgsScript: buildArgsScript,
	}
}

func NewDefaultDockerBuilderWithOptions(options ...Option) distgo.DockerBuilder {
	builder := &DefaultDockerBuilder{}
	for _, opt := range options {
		opt(builder)
	}
	return builder
}

func (d *DefaultDockerBuilder) TypeName() (string, error) {
	return TypeName, nil
}

// RunDockerBuild runs a single buildx invocation that produces both an OCI layout -- the reproducible published
// artifact, and the on-disk base that a dependent product's "FROM" resolves against -- and a load into the local Docker
// daemon for "docker run". Each output is dropped when the environment cannot accept it, and dropping one is reported;
// dropping both is an error rather than a build that produces nothing.
func (d *DefaultDockerBuilder) RunDockerBuild(dockerID distgo.DockerID, productTaskOutputInfo distgo.ProductTaskOutputInfo, verbose, dryRun bool, stdout io.Writer) error {
	dockerBuilderOutputInfo := productTaskOutputInfo.Product.DockerOutputInfos.DockerBuilderOutputInfos[dockerID]
	contextDirPath := filepath.Join(productTaskOutputInfo.Project.ProjectDir, dockerBuilderOutputInfo.ContextDir)

	args := []string{
		"buildx",
		"build",
		"--file", filepath.Join(contextDirPath, dockerBuilderOutputInfo.DockerfilePath),
		"--build-arg", "SOURCE_DATE_EPOCH=0",
	}
	for _, tag := range dockerBuilderOutputInfo.RenderedTags {
		args = append(args, "-t", tag)
	}
	args = append(args, d.BuildArgs...)
	if d.BuildArgsScript != "" {
		buildArgsFromScript, err := distgo.DockerBuildArgsFromScript(dockerID, productTaskOutputInfo, d.BuildArgsScript)
		if err != nil {
			return err
		}
		args = append(args, buildArgsFromScript...)
	}
	// Resolve a "FROM <dependency image tag>" from the dependency's on-disk OCI layout instead of a registry.
	args = append(args, dependencyImageBuildContextArgs(productTaskOutputInfo, dryRun)...)

	if err := d.ensureDockerContainerDriver(dockerID, verbose, dryRun, stdout); err != nil {
		return err
	}

	destDir, err := prepareOCIOutputDir(dockerID, productTaskOutputInfo, dryRun, stdout)
	if err != nil {
		return err
	}
	var outputArgs []string
	if destDir != "" {
		outputArgs = append(outputArgs, fmt.Sprintf("--output=type=oci,rewrite-timestamp=true,tar=false,dest=%s", destDir))
	}

	// A cross-platform build produces a manifest list, which only the containerd image store can hold. Asking a
	// classic-store daemon for it fails the whole build, taking any OCI layout down with it, so drop the daemon output
	// instead and say so: the layout is the artifact that matters, and "docker run" is the convenience.
	loadIntoDaemon := true
	if d.BuildxPlatformArg != "" {
		usesContainerdStore, err := UsesContainerdImageStore()
		if err != nil {
			return err
		}
		loadIntoDaemon = usesContainerdStore
		if !loadIntoDaemon {
			distgo.PrintlnOrDryRunPrintln(stdout, fmt.Sprintf("Docker daemon does not use the containerd image store: not loading the cross-platform image for configuration %s into the local daemon", dockerID), dryRun)
		}
	}
	if loadIntoDaemon {
		outputArgs = append(outputArgs, dockerDaemonOutputArg)
	}

	if len(outputArgs) == 0 {
		return errors.Errorf("cannot build Docker configuration %s of product %s: it declares no dist output to hold an OCI layout, and its cross-platform image cannot be loaded into a Docker daemon that does not use the containerd image store", dockerID, productTaskOutputInfo.Product.ID)
	}
	args = append(args, outputArgs...)
	// BuildxPlatformArg is empty in the default "host arch only" mode; appending it then would pass an empty positional
	// argument, which `docker buildx build` rejects.
	if d.BuildxPlatformArg != "" {
		args = append(args, d.BuildxPlatformArg)
	}
	if err := distgo.RunCommandWithVerboseOption(exec.Command("docker", append(args, contextDirPath)...), verbose, dryRun, stdout); err != nil {
		return err
	}
	if dryRun || destDir == "" {
		return nil
	}
	return finalizeOCILayout(destDir)
}

// prepareOCIOutputDir returns an empty directory for buildx to write the OCI layout into, or "" when the product
// declares no dist output. The layout lives under the product's dist output namespace, so a product that declares a
// Docker image but no dist has nowhere to put one, and the caller falls back to producing the daemon image alone.
//
// TODO: Remove after merging https://github.com/palantir/distgo/pull/937. The Docker output directory it introduces
// defaults to "out/docker" and is set for every product with Docker configuration, so this returns a directory
// unconditionally: fold it back into RunDockerBuild and drop the "" handling there, including the daemon-only fallback
// and the error for a build left with no outputs at all.
func prepareOCIOutputDir(dockerID distgo.DockerID, productTaskOutputInfo distgo.ProductTaskOutputInfo, dryRun bool, stdout io.Writer) (string, error) {
	destDir := productTaskOutputInfo.ProductDockerOCIDistOutputDir(dockerID)
	if destDir == "" {
		distgo.PrintlnOrDryRunPrintln(stdout, fmt.Sprintf("Product %s declares no dist output: building the local Docker daemon image for configuration %s only, and not writing an OCI layout", productTaskOutputInfo.Product.ID, dockerID), dryRun)
		return "", nil
	}
	if dryRun {
		return destDir, nil
	}
	// buildx writes the layout into destDir without clearing it, so wipe first: re-running "docker build" at an
	// unchanged version (e.g. a "-dirty" version, whose output directory name does not change between runs) would
	// otherwise leave blobs from the previous build behind.
	if err := os.RemoveAll(destDir); err != nil {
		return "", errors.Wrapf(err, "failed to clear directory %s for OCI output", destDir)
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", errors.Wrapf(err, "failed to create directory %s for OCI output", destDir)
	}
	return destDir, nil
}

// finalizeOCILayout reduces the layout buildx just wrote to the single descriptor we publish. buildx emits one
// index.json entry per tag: for a multi-tag build those entries all carry the same digest and differ only in their ref
// annotations, so publishing the index verbatim would yield a manifest list with duplicate entries. Since all rendered
// tags are known at publish time, keep the first descriptor and do a publish per-tag.
func finalizeOCILayout(destOCILayoutDir string) error {
	// buildx leaves its content-store staging directory behind; it is not part of a conformant layout.
	if err := os.RemoveAll(filepath.Join(destOCILayoutDir, "ingest")); err != nil {
		return errors.Wrapf(err, "failed to remove staging directory from OCI layout %s", destOCILayoutDir)
	}
	index, err := layout.ImageIndexFromPath(destOCILayoutDir)
	if err != nil {
		return errors.Wrap(err, "failed to read OCI layout from path")
	}
	idxManifest, err := index.IndexManifest()
	if err != nil {
		return errors.Wrap(err, "failed to read index manifest")
	}
	if len(idxManifest.Manifests) == 0 {
		return errors.New("top-level OCI image index does not contain any manifests. While this is a valid image index, it is unexpected and likely means something erroneous happened earlier in the build")
	}
	// Replace the top-level index with one containing only the "actual" per-tag descriptor, rather than promoting its
	// blob to be index.json directly. A conformant OCI image layout's index.json must always be an image index (never
	// a bare manifest), and the descriptor's blob must stay in blobs/ addressable by digest so the docker publish
	// path and any OCI-layout consumer can resolve it.
	indexDesc := idxManifest.Manifests[0]
	newIndexBytes, err := json.Marshal(v1.IndexManifest{
		SchemaVersion: 2,
		MediaType:     types.OCIImageIndex,
		Manifests:     []v1.Descriptor{indexDesc},
	})
	if err != nil {
		return errors.Wrap(err, "failed to marshal top-level OCI image index")
	}
	if err := os.WriteFile(filepath.Join(destOCILayoutDir, "index.json"), newIndexBytes, 0644); err != nil {
		return errors.Wrap(err, "failed to write top-level OCI image index")
	}
	// The buildx-consumable wrapper layout that lets a dependent's "FROM" resolve this image from disk is written by
	// the Docker build task after the builder runs (distgo.WriteDockerBuildContextLayout), not here, so it survives
	// re-layering builders that rewrite this layout.
	return nil
}

// UsesContainerdImageStore reports whether the local Docker daemon runs the containerd image store, which is required
// to load a cross-platform image into the daemon. The daemon reports the store through the storage driver: the
// containerd store is a containerd snapshotter, whereas the classic store names a graph driver such as "overlay2".
func UsesContainerdImageStore() (bool, error) {
	cmd := exec.Command("docker", "info", "--format", "{{.DriverStatus}}")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, errors.Wrapf(err, "failed to determine the image store used by the Docker daemon: %s", string(out))
	}
	return bytes.Contains(out, []byte("io.containerd.snapshotter.v1")), nil
}

func UsesDockerContainerDriver() (bool, error) {
	cmd := exec.Command("docker", "buildx", "inspect")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, errors.Wrapf(err, "failed to check for existence of buildx drivers: %s", string(out))
	}
	return bytes.Contains(out, []byte("docker-container")), nil
}

// ensureDockerContainerDriver ensures there is a buildx builder that uses the docker-container driver, which is
// required for building multi-arch images. If a buildx builder does not exist, creates one and sets it as the default.
// This is required until docker finishes supporting multi-arch containers in the daemon.
// https://docs.docker.com/engine/reference/commandline/buildx_create/#driver
func (d *DefaultDockerBuilder) ensureDockerContainerDriver(dockerID distgo.DockerID, verbose, dryRun bool, stdout io.Writer) error {
	if usesDockerContainerDriver, err := UsesDockerContainerDriver(); err != nil {
		return err
	} else if usesDockerContainerDriver {
		return nil
	}
	var driverOptArgs []string
	for _, opt := range d.BuildxDriverOpts {
		driverOptArgs = append(driverOptArgs, "--driver-opt", opt)
	}
	// Some CI environments have compatibility issues running with the TLS data in the default context. Creating a new
	// named context copies the TLS data correctly, and allows for a buildx builder to be created.
	// https://support.circleci.com/hc/en-us/articles/360058095471-How-To-Use-Docker-Buildx-in-Rem have compatibility issues running with the TLS data in the default context. Creating a new
	// named context copies the TLS data correctly, and allows for a buildx builder to be created.ote-Docker-
	createContextArgs := []string{"context", "create", string(dockerID)}
	createContextCmd := exec.Command("docker", createContextArgs...)
	if err := distgo.RunCommandWithVerboseOption(createContextCmd, verbose, dryRun, stdout); err != nil {
		return err
	}

	args := []string{"buildx", "create", string(dockerID), "--bootstrap", "--use", "--driver", "docker-container"}
	cmd := exec.Command("docker", append(args, driverOptArgs...)...)
	if err := distgo.RunCommandWithVerboseOption(cmd, verbose, dryRun, stdout); err != nil {
		return err
	}
	return nil
}

func WithBuildArgs(buildArgs []string) Option {
	return func(d *DefaultDockerBuilder) {
		d.BuildArgs = buildArgs
	}
}

func WithBuildArgsScript(buildArgsScript string) Option {
	return func(d *DefaultDockerBuilder) {
		d.BuildArgsScript = buildArgsScript
	}
}

func WithBuildxDriverOptions(buildxDriverOptions []string) Option {
	return func(d *DefaultDockerBuilder) {
		d.BuildxDriverOpts = buildxDriverOptions
	}
}

// WithBuildxPlatforms allows buildx builds to produce multi-platform images. The formatting for the platform specifier
// is defined in the containerd source code.
// https://github.com/containerd/containerd/blob/v1.4.3/platforms/platforms.go#L63
func WithBuildxPlatforms(buildxPlatforms []string) Option {
	return func(d *DefaultDockerBuilder) {
		if len(buildxPlatforms) != 0 {
			d.BuildxPlatformArg = fmt.Sprintf("--platform=%s", strings.Join(buildxPlatforms, ","))
		}
	}
}
