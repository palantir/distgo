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
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/mholt/archiver/v3"
	"github.com/palantir/distgo/distgo"
	"github.com/pkg/errors"
)

const TypeName = "default"

// OutputType is a bitmask which specifies which artifacts to produce as part of the docker build. At least one build
// type must be specified, but multiple build types can be combined
type OutputType uint

const (
	// OCILayout output type indicates that the build should produce an OCI-compliant filesystem layout as an output
	OCILayout OutputType = 1 << iota
	// DockerDaemon output type indicates that the build should produce an image in the local docker daemon
	DockerDaemon

	allOutputs = OCILayout | DockerDaemon
)

type Option func(*DefaultDockerBuilder)

type DefaultDockerBuilder struct {
	BuildArgs         []string
	BuildArgsScript   string
	BuildxDriverOpts  []string
	BuildxPlatformArg string
	OutputType        OutputType
}

func NewDefaultDockerBuilder(buildArgs []string, buildArgsScript string) distgo.DockerBuilder {
	return &DefaultDockerBuilder{
		BuildArgs:       buildArgs,
		BuildArgsScript: buildArgsScript,
		// Produce an OCI layout (the reproducible published artifact, and the on-disk base a dependent product's FROM
		// resolves against) and load the host-arch image into the local daemon for `docker run`.
		OutputType: OCILayout | DockerDaemon,
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

func (d *DefaultDockerBuilder) RunDockerBuild(dockerID distgo.DockerID, productTaskOutputInfo distgo.ProductTaskOutputInfo, verbose, dryRun bool, stdout io.Writer) error {
	if d.OutputType&allOutputs == 0 {
		return errors.New("a valid output type of docker builder must be specified")
	}

	dockerBuilderOutputInfo := productTaskOutputInfo.Product.DockerOutputInfos.DockerBuilderOutputInfos[dockerID]
	contextDirPath := filepath.Join(productTaskOutputInfo.Project.ProjectDir, dockerBuilderOutputInfo.ContextDir)

	// baseArgs are common to every output type. Each output below builds its "docker buildx build" invocation by
	// cloning baseArgs and appending its own "--output=..." plus the context dir.
	baseArgs := []string{
		"buildx",
		"build",
		"--file", filepath.Join(contextDirPath, dockerBuilderOutputInfo.DockerfilePath),
		"--build-arg", "SOURCE_DATE_EPOCH=0",
	}
	for _, tag := range dockerBuilderOutputInfo.RenderedTags {
		baseArgs = append(baseArgs, "-t", tag)
	}
	baseArgs = append(baseArgs, d.BuildArgs...)
	if d.BuildArgsScript != "" {
		buildArgsFromScript, err := distgo.DockerBuildArgsFromScript(dockerID, productTaskOutputInfo, d.BuildArgsScript)
		if err != nil {
			return err
		}
		baseArgs = append(baseArgs, buildArgsFromScript...)
	}
	// Resolve a "FROM <dependency image tag>" from the dependency's on-disk OCI layout instead of a registry.
	baseArgs = append(baseArgs, dependencyImageBuildContextArgs(productTaskOutputInfo, dryRun)...)

	if err := d.ensureDockerContainerDriver(dockerID, verbose, dryRun, stdout); err != nil {
		return err
	}

	if d.OutputType&OCILayout != 0 {
		destDir := productTaskOutputInfo.ProductDockerOCIDistOutputDir(dockerID)
		if err := os.MkdirAll(destDir, 0755); err != nil {
			return errors.Wrapf(err, "failed to create directory %s for OCI output", destDir)
		}
		destFile := filepath.Join(destDir, "image.tar")

		ociArgs := slices.Clone(baseArgs)
		// BuildxPlatformArg is empty in the default "host arch only" mode; appending it then would pass an empty
		// positional argument, which `docker buildx build` rejects.
		if d.BuildxPlatformArg != "" {
			ociArgs = append(ociArgs, d.BuildxPlatformArg)
		}
		ociArgs = append(ociArgs, fmt.Sprintf("--output=type=oci,rewrite-timestamp=true,dest=%s", destFile), contextDirPath)
		if err := distgo.RunCommandWithVerboseOption(exec.Command("docker", ociArgs...), verbose, dryRun, stdout); err != nil {
			return err
		}
		if !dryRun {
			if err := d.extractToOCILayout(destDir, destFile); err != nil {
				return err
			}
		}
	}
	if d.OutputType&DockerDaemon != 0 {
		daemonArgs := append(slices.Clone(baseArgs), "--output=type=docker,rewrite-timestamp=true", contextDirPath)
		if err := distgo.RunCommandWithVerboseOption(exec.Command("docker", daemonArgs...), verbose, dryRun, stdout); err != nil {
			return err
		}
	}
	return nil
}

// extractToOCILayout is responsible for converting the buildx OCI tarball output to a compatible OCI layout on disk.
// The buildx tarball adds a layer of indirection which doesn't seem to play nicely with some registries; the top-level
// image index produced contains a manifest per-tag, which point to the "actual" image index we want to publish. Since
// we know all the rendered tags at publish time, we can move the "actual" image index back to the top level and do a
// publish per-tag.
func (d *DefaultDockerBuilder) extractToOCILayout(destOCILayoutDir, sourceOCITarball string) error {
	// Clear any prior extraction output (keeping the source tarball) first: the tar extractor won't overwrite existing
	// files, so re-running "docker build" at an unchanged version (e.g. a "-dirty" version) would otherwise fail.
	entries, err := os.ReadDir(destOCILayoutDir)
	if err != nil {
		return errors.Wrapf(err, "failed to read OCI output directory %s", destOCILayoutDir)
	}
	sourceBase := filepath.Base(sourceOCITarball)
	for _, entry := range entries {
		if entry.Name() == sourceBase {
			continue
		}
		if err := os.RemoveAll(filepath.Join(destOCILayoutDir, entry.Name())); err != nil {
			return errors.Wrapf(err, "failed to remove stale OCI layout artifact %s", entry.Name())
		}
	}
	if err := archiver.DefaultTar.Unarchive(sourceOCITarball, destOCILayoutDir); err != nil {
		return errors.Wrapf(err, "failed to extract OCI tarball %s to location %s", sourceOCITarball, destOCILayoutDir)
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
	indexDesc := idxManifest.Manifests[0]
	if err := os.Rename(filepath.Join(destOCILayoutDir, "blobs", indexDesc.Digest.Algorithm, indexDesc.Digest.Hex), filepath.Join(destOCILayoutDir, "index.json")); err != nil {
		return err
	}
	// The buildx-consumable wrapper layout that lets a dependent's "FROM" resolve this image from disk is written by
	// the Docker build task after the builder runs (distgo.WriteDockerBuildContextLayout), not here, so it survives
	// re-layering builders that rewrite this layout.
	return nil
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

func WithBuildxOutput(output OutputType) Option {
	return func(d *DefaultDockerBuilder) {
		d.OutputType = output
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
