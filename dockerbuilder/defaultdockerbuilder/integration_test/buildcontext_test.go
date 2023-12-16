// Copyright 2026 Palantir Technologies, Inc.
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

package integration_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/dockerbuilder/defaultdockerbuilder"
	"github.com/stretchr/testify/require"
)

// TestCrossProductBuildContextRegistryFree proves the registry-free behavior with real builds: a base image built with
// OCI-layout output plus the build-context wrapper, then a dependent leaf's "FROM itbase:tag" (a tag in no registry)
// builds only by resolving the base from that on-disk wrapper. Skipped without a docker-container builder.
//
// The wrapper is written by the Docker build task (distgo.WriteDockerBuildContextLayout) after the builder runs, not by
// the builder itself; this test drives the builder directly, so it calls WriteDockerBuildContextLayout itself to stand
// in for that task step.
func TestCrossProductBuildContextRegistryFree(t *testing.T) {
	if uses, err := defaultdockerbuilder.UsesDockerContainerDriver(); err != nil || !uses {
		t.Skip("requires an active docker-container buildx builder")
	}

	projectDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "base"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "base", "Dockerfile"), []byte("FROM alpine:latest\nRUN echo base-layer > /base-marker\n"), 0644))
	// distgo renders {{Tag ...}} before the builder runs, so the leaf Dockerfile uses the base's rendered tag directly.
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "leaf"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "leaf", "Dockerfile"), []byte("FROM itbase:tag\nRUN cat /base-marker\n"), 0644))

	project := distgo.ProjectInfo{ProjectDir: projectDir, Version: "1.0.0"}
	baseProduct := distgo.ProductOutputInfo{
		ID: "base",
		DockerOutputInfos: &distgo.DockerOutputInfos{
			DockerOutputDir: "out/docker",
			DockerIDs:       []distgo.DockerID{"base-docker"},
			DockerBuilderOutputInfos: map[distgo.DockerID]distgo.DockerBuilderOutputInfo{
				"base-docker": {ContextDir: "base", DockerfilePath: "Dockerfile", RenderedTags: []string{"itbase:tag"}},
			},
		},
	}
	baseInfo := distgo.ProductTaskOutputInfo{Project: project, Product: baseProduct}
	leafInfo := distgo.ProductTaskOutputInfo{
		Project: project,
		Product: distgo.ProductOutputInfo{
			ID: "leaf",
			DockerOutputInfos: &distgo.DockerOutputInfos{
				DockerOutputDir: "out/docker",
				DockerIDs:       []distgo.DockerID{"leaf-docker"},
				DockerBuilderOutputInfos: map[distgo.DockerID]distgo.DockerBuilderOutputInfo{
					"leaf-docker": {ContextDir: "leaf", DockerfilePath: "Dockerfile", RenderedTags: []string{"itleaf:tag"}},
				},
			},
		},
		Deps: map[distgo.ProductID]distgo.ProductOutputInfo{"base": baseProduct},
	}

	// OCI-layout output (as the managed builder uses) produces the layout the wrapper is written into.
	builder := defaultdockerbuilder.NewDefaultDockerBuilderWithOptions(
		defaultdockerbuilder.WithBuildxOutput(defaultdockerbuilder.OCILayout),
		defaultdockerbuilder.WithBuildxPlatforms([]string{"linux/amd64", "linux/arm64"}),
	)
	require.NoError(t, builder.RunDockerBuild("base-docker", baseInfo, false, false, io.Discard))
	// Stand in for the Docker build task's post-build wrapper write.
	require.NoError(t, distgo.WriteDockerBuildContextLayout(distgo.ProductDockerOutputDir(baseInfo.Project, baseInfo.Product, "base-docker"), []string{"itbase:tag"}))
	require.NoError(t, builder.RunDockerBuild("leaf-docker", leafInfo, false, false, io.Discard),
		"leaf must resolve FROM itbase:tag from the base's local layout (registry-free)")
}
