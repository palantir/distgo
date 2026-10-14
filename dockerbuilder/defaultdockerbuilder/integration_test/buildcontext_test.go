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
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/dockerbuilder/defaultdockerbuilder"
	"github.com/palantir/pkg/gittest"
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

// TestOCIImageMetadataRegistryFree verifies metadata in real buildx output without pulling or pushing images.
func TestOCIImageMetadataRegistryFree(t *testing.T) {
	if uses, err := defaultdockerbuilder.UsesDockerContainerDriver(); err != nil || !uses {
		t.Skip("requires an active docker-container buildx builder")
	}
	projectDir := t.TempDir()
	gittest.RunGitCommand(t, projectDir, "init")
	gittest.RunGitCommand(t, projectDir, "remote", "add", "origin", "git@github.com:example/project.git")
	contextDir := filepath.Join(projectDir, "context")
	require.NoError(t, os.MkdirAll(contextDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(contextDir, "Dockerfile"), []byte("FROM scratch\nCOPY marker /marker\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(contextDir, "marker"), []byte("metadata test\n"), 0644))

	want := map[string]string{
		distgo.OCIImageSourceAnnotation:  "https://github.com/example/project",
		distgo.OCIImageVersionAnnotation: "1.2.3",
	}
	checkMetadata := func(t *testing.T, got map[string]string) {
		t.Helper()
		for key, value := range want {
			require.Equal(t, value, got[key], "metadata key %s", key)
		}
	}
	checkImage := func(t *testing.T, index v1.ImageIndex, desc v1.Descriptor) {
		t.Helper()
		img, err := index.Image(desc.Digest)
		require.NoError(t, err)
		manifest, err := img.Manifest()
		require.NoError(t, err)
		checkMetadata(t, manifest.Annotations)
		config, err := img.ConfigFile()
		require.NoError(t, err)
		checkMetadata(t, config.Config.Labels)
	}
	for _, tc := range []struct {
		name      string
		platforms []string
	}{
		{name: "single", platforms: []string{"linux/amd64"}},
		{name: "multi", platforms: []string{"linux/amd64", "linux/arm64"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := distgo.ProductTaskOutputInfo{
				Project: distgo.ProjectInfo{ProjectDir: projectDir, Version: "1.2.3"},
				Product: distgo.ProductOutputInfo{
					ID: distgo.ProductID(tc.name),
					DockerOutputInfos: &distgo.DockerOutputInfos{
						DockerOutputDir: "out/docker",
						DockerBuilderOutputInfos: map[distgo.DockerID]distgo.DockerBuilderOutputInfo{
							"image": {ContextDir: "context", DockerfilePath: "Dockerfile", RenderedTags: []string{"metadata-test:" + tc.name}},
						},
					},
				},
			}
			builder := defaultdockerbuilder.NewDefaultDockerBuilderWithOptions(
				defaultdockerbuilder.WithBuildxOutput(defaultdockerbuilder.OCILayout),
				defaultdockerbuilder.WithBuildxPlatforms(tc.platforms),
				defaultdockerbuilder.WithOCIImageMetadata(),
				// Disable attestations so a single platform exports a bare manifest, exercising its scope restriction.
				defaultdockerbuilder.WithBuildArgs([]string{"--provenance=false", "--sbom=false"}),
			)
			var stdout bytes.Buffer
			require.NoError(t, builder.RunDockerBuild("image", info, false, false, &stdout), stdout.String())
			index, err := layout.ImageIndexFromPath(distgo.ProductDockerOutputDir(info.Project, info.Product, "image"))
			require.NoError(t, err)
			manifest, err := index.IndexManifest()
			require.NoError(t, err)
			require.Len(t, manifest.Manifests, 1)
			desc := manifest.Manifests[0]
			if len(tc.platforms) == 1 {
				require.True(t, desc.MediaType.IsImage())
				for key := range want {
					require.NotContains(t, manifest.Annotations, key)
					require.NotContains(t, desc.Annotations, key)
				}
				checkImage(t, index, desc)
				return
			}
			require.True(t, desc.MediaType.IsIndex())
			publishedIndex, err := index.ImageIndex(desc.Digest)
			require.NoError(t, err)
			publishedManifest, err := publishedIndex.IndexManifest()
			require.NoError(t, err)
			checkMetadata(t, publishedManifest.Annotations)
			require.Len(t, publishedManifest.Manifests, len(tc.platforms))
			var platforms []string
			for _, imageDesc := range publishedManifest.Manifests {
				checkMetadata(t, imageDesc.Annotations)
				checkImage(t, publishedIndex, imageDesc)
				require.NotNil(t, imageDesc.Platform)
				platforms = append(platforms, imageDesc.Platform.OS+"/"+imageDesc.Platform.Architecture)
			}
			require.ElementsMatch(t, tc.platforms, platforms)
		})
	}
}
