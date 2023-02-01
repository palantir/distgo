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
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"

	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/mholt/archiver/v3"
	"github.com/palantir/distgo/distgo"
)

const TypeName = "default"

type BuildxOutput uint

const (
	OCITarball BuildxOutput = 1 << iota
	DockerDaemon
)

type Option func(*DefaultDockerBuilder)

type DefaultDockerBuilder struct {
	BuildArgs        []string
	BuildArgsScript  string
	BuildxOutput     BuildxOutput
	BuildxDriverOpts []string
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

func (d *DefaultDockerBuilder) RunDockerBuild(dockerID distgo.DockerID, productTaskOutputInfo distgo.ProductTaskOutputInfo, verbose, dryRun bool, stdout io.Writer) error {
	dockerBuilderOutputInfo := productTaskOutputInfo.Product.DockerOutputInfos.DockerBuilderOutputInfos[dockerID]
	contextDirPath := path.Join(productTaskOutputInfo.Project.ProjectDir, dockerBuilderOutputInfo.ContextDir)
	args := []string{
		"build",
		"--file", path.Join(contextDirPath, dockerBuilderOutputInfo.DockerfilePath),
	}
	for _, tag := range dockerBuilderOutputInfo.RenderedTags {
		args = append(args,
			"-t", tag,
		)
	}
	args = append(args, d.BuildArgs...)
	if d.BuildArgsScript != "" {
		buildArgsFromScript, err := distgo.DockerBuildArgsFromScript(dockerID, productTaskOutputInfo, d.BuildArgsScript)
		if err != nil {
			return err
		}
		args = append(args, buildArgsFromScript...)
	}

	if d.BuildxOutput != 0 {
		return d.buildX(dockerID, productTaskOutputInfo, args, contextDirPath, verbose, dryRun, stdout)
	}

	args = append(args, contextDirPath)

	cmd := exec.Command("docker", args...)
	return distgo.RunCommandWithVerboseOption(cmd, verbose, dryRun, stdout)
}

func (d *DefaultDockerBuilder) buildX(dockerID distgo.DockerID, productTaskOutputInfo distgo.ProductTaskOutputInfo, args []string, contextDirPath string, verbose, dryRun bool, stdout io.Writer) error {
	if err := d.ensureDockerContainerDriver(verbose, dryRun, stdout); err != nil {
		return err
	}
	args = append([]string{"buildx"}, args...)
	if d.BuildxOutput&OCITarball != 0 {
		destDir := productTaskOutputInfo.ProductDockerDistOutputDir(dockerID)
		destFile := fmt.Sprintf("%s/image.tar", destDir)
		ociArgs := append(args, fmt.Sprintf("--output=type=oci,dest=%s", destFile), contextDirPath)
		cmd := exec.Command("docker", ociArgs...)
		if err := distgo.RunCommandWithVerboseOption(cmd, verbose, dryRun, stdout); err != nil {
			return err
		}
		if err := d.extractToOCILayout(destDir, destFile); err != nil {
			return err
		}
	}
	if d.BuildxOutput&DockerDaemon != 0 {
		dockerArgs := append(args, "--output=type=docker", contextDirPath)
		cmd := exec.Command("docker", dockerArgs...)
		if err := distgo.RunCommandWithVerboseOption(cmd, verbose, dryRun, stdout); err != nil {
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
	if err := archiver.DefaultTar.Unarchive(destOCILayoutDir, sourceOCITarball); err != nil {
		return err
	}
	index, err := layout.ImageIndexFromPath(destOCILayoutDir)
	if err != nil {
		return err
	}
	idxManifest, err := index.IndexManifest()
	if err != nil {
		return err
	}
	if len(idxManifest.Manifests) == 0 {
		return errors.New("Top-level OCI image index does not contain any manifests. While this is a valid image index, it is unexpected and likely means something erroneous happened earlier in the build")
	}
	if err := os.Rename(path.Join(destOCILayoutDir, "blobs", idxManifest.Manifests[0].Digest.Algorithm, idxManifest.Manifests[0].Digest.Hex), path.Join(destOCILayoutDir, "index.json")); err != nil {
		return err
	}
	return nil
}

// ensureDockerContainerDriver ensures there is a buildx builder that uses the docker-container driver, currently
// required for multi-arch support, until docker finishes supporting multi-arch containers in the daemon. If one does
// not exist, create one and set it to the default.
func (d *DefaultDockerBuilder) ensureDockerContainerDriver(verbose, dryRun bool, stdout io.Writer) error {
	cmd := exec.Command("docker", "buildx", "inspect")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}

	if bytes.Contains(out, []byte("docker-container")) {
		return nil
	}
	var driverOptArgs []string
	for _, opt := range d.BuildxDriverOpts {
		driverOptArgs = append(driverOptArgs, "--driver-opt", opt)
	}
	args := []string{"buildx", "create", "--use", "--driver", "docker-container"}
	cmd = exec.Command("docker", append(args, driverOptArgs...)...)
	if err := distgo.RunCommandWithVerboseOption(cmd, verbose, dryRun, stdout); err != nil {
		return err
	}

	// The version of docker/buildx on circle does not have --bootstrap, so we run an empty build to make sure it's
	// ready and is working.
	cmd = exec.Command("docker", "buildx", "--file", "-", ".")
	cmd.Stdin = bytes.NewBufferString("FROM scratch")
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

func WithBuildxOutput(output BuildxOutput) Option {
	return func(d *DefaultDockerBuilder) {
		d.BuildxOutput = output
	}
}
