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

package buildxdockerbuilder

import (
	"bytes"
	"io"
	"os/exec"
	"path"

	"github.com/palantir/distgo/distgo"
)

const TypeName = "buildx"

type BuildxDockerBuilder struct {
	DriverOpts      []string
	BuildArgs       []string
	BuildArgsScript string
}

func NewBuildxDockerBuilder(driverOpts []string, buildArgs []string, buildArgsScript string) distgo.DockerBuilder {
	return &BuildxDockerBuilder{
		DriverOpts:      driverOpts,
		BuildArgs:       buildArgs,
		BuildArgsScript: buildArgsScript,
	}
}

func (b *BuildxDockerBuilder) TypeName() (string, error) {
	return TypeName, nil
}

func (b *BuildxDockerBuilder) RunDockerBuild(dockerID distgo.DockerID, productTaskOutputInfo distgo.ProductTaskOutputInfo, verbose, dryRun bool, stdout io.Writer) error {
	// if we actually need to build the image, we need to execute against a working buildx environment
	if !dryRun {
		if err := b.ensureDockerContainerDriver(); err != nil {
			return err
		}
	}

	dockerBuilderOutputInfo := productTaskOutputInfo.Product.DockerOutputInfos.DockerBuilderOutputInfos[dockerID]
	contextDirPath := path.Join(productTaskOutputInfo.Project.ProjectDir, dockerBuilderOutputInfo.ContextDir)
	args := []string{
		"buildx",
		"build",
		"--file", path.Join(contextDirPath, dockerBuilderOutputInfo.DockerfilePath),
	}
	for _, tag := range dockerBuilderOutputInfo.RenderedTags {
		args = append(args,
			"-t", tag,
		)
	}
	args = append(args, b.BuildArgs...)
	if b.BuildArgsScript != "" {
		buildArgsFromScript, err := distgo.DockerBuildArgsFromScript(dockerID, productTaskOutputInfo, b.BuildArgsScript)
		if err != nil {
			return err
		}
		args = append(args, buildArgsFromScript...)
	}
	args = append(args, contextDirPath)

	cmd := exec.Command("docker", args...)
	return distgo.RunCommandWithVerboseOption(cmd, verbose, dryRun, stdout)
}

// ensureDockerContainerDriver ensures there is a buildx builder that uses the docker-container driver, currently
// required for multi-arch support, until docker finishes supporting multi-arch containers in the daemon. If one does
// not exist, create one and set it to the default.
func (b *BuildxDockerBuilder) ensureDockerContainerDriver() error {
	cmd := exec.Command("docker", "buildx", "inspect")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}

	if bytes.Contains(out, []byte("docker-container")) {
		return nil
	}
	var driverOptArgs []string
	for _, opt := range b.DriverOpts {
		driverOptArgs = append(driverOptArgs, "--driver-opt", opt)
	}
	args := []string{"docker", "buildx", "create", "--use", "--driver", "docker-container"}
	cmd = exec.Command("docker", append(args, driverOptArgs...)...)
	if err := cmd.Run(); err != nil {
		return err
	}

	// The version of docker/buildx on circle does not have --bootstrap, so we run an empty build to make sure it's
	// ready and is working.
	cmd = exec.Command("docker", "buildx", "--file", "-", ".")
	cmd.Stdin = bytes.NewBufferString("FROM scratch")
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}
