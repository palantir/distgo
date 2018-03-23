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
	"io"
	"os/exec"

	"github.com/palantir/distgo/distgo"
)

const TypeName = "default"

type DefaultDockerBuilder struct {
	BuildArgs []string
}

func NewDefaultDockerBuilder(buildArgs []string) distgo.DockerBuilder {
	return &DefaultDockerBuilder{
		BuildArgs: buildArgs,
	}
}

func (d *DefaultDockerBuilder) TypeName() (string, error) {
	return TypeName, nil
}

func (d *DefaultDockerBuilder) RunDockerBuild(dockerID distgo.DockerID, productTaskOutputInfo distgo.ProductTaskOutputInfo, verbose, dryRun bool, stdout io.Writer) error {
	dockerBuilderOutputInfo := productTaskOutputInfo.Product.DockerOutputInfos.DockerBuilderOutputInfos[dockerID]
	args := []string{
		"build",
		"--file", dockerBuilderOutputInfo.DockerfilePath,
	}
	for _, tag := range dockerBuilderOutputInfo.RenderedTags {
		args = append(args,
			"-t", tag,
		)
	}
	args = append(args, dockerBuilderOutputInfo.ContextDir)
	args = append(args, d.BuildArgs...)

	cmd := exec.Command("docker", args...)
	if err := distgo.RunCommandWithVerboseOption(cmd, verbose, dryRun, stdout); err != nil {
		return err
	}
	return nil
}
