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
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/dockerbuilder/defaultdockerbuilder"
	"github.com/stretchr/testify/require"
)

// TestBuildProducesLayoutAndDaemonImage proves with real builds that the one buildx invocation both writes a conformant
// OCI layout and loads the image into the local daemon, cross-platform included. Loading a cross-platform image needs
// the containerd image store, so against a classic-store daemon the build must still produce the layout and skip only
// the daemon load; this asserts whichever of the two applies to the daemon running the test. Skipped without a
// docker-container builder.
func TestBuildProducesLayoutAndDaemonImage(t *testing.T) {
	if uses, err := defaultdockerbuilder.UsesDockerContainerDriver(); err != nil || !uses {
		t.Skip("requires an active docker-container buildx builder")
	}
	usesContainerdStore, err := defaultdockerbuilder.UsesContainerdImageStore()
	require.NoError(t, err)

	for _, tc := range []struct {
		name      string
		tag       string
		platforms []string
	}{
		{name: "host platform", tag: "distgo-outputs-it:host"},
		{name: "cross platform", tag: "distgo-outputs-it:cross", platforms: []string{"linux/amd64", "linux/arm64"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() {
				_ = exec.Command("docker", "rmi", "-f", tc.tag).Run()
			})

			outputInfo := newDockerProductInfo(t, tc.tag)

			builder := defaultdockerbuilder.NewDefaultDockerBuilderWithOptions(defaultdockerbuilder.WithBuildxPlatforms(tc.platforms))
			require.NoError(t, builder.RunDockerBuild("img", outputInfo, false, false, io.Discard))

			layoutDir := outputInfo.ProductDockerOCIDistOutputDir("img")
			index, err := layout.ImageIndexFromPath(layoutDir)
			require.NoError(t, err, "OCI output must be a readable layout directory")
			idxManifest, err := index.IndexManifest()
			require.NoError(t, err)
			require.Equal(t, types.OCIImageIndex, idxManifest.MediaType, "top-level index.json must be an image index")
			require.Len(t, idxManifest.Manifests, 1)

			// The layout is written directly, so neither the intermediate tarball nor buildx's staging dir should survive.
			for _, name := range []string{"image.tar", "ingest"} {
				_, err := os.Stat(filepath.Join(layoutDir, name))
				require.True(t, os.IsNotExist(err), "%s must not be present in the published layout", name)
			}

			// The daemon load is dropped only for a cross-platform build against a classic-store daemon, which cannot
			// hold the manifest list; the OCI layout asserted above is produced either way.
			daemonLoadExpected := tc.platforms == nil || usesContainerdStore
			inspectErr := exec.Command("docker", "image", "inspect", tc.tag).Run()
			if daemonLoadExpected {
				require.NoError(t, inspectErr, "image must be loaded into the local daemon")
			} else {
				require.Error(t, inspectErr, "cross-platform image must not be loaded into a classic-store daemon")
			}
		})
	}
}

// TestCrossPlatformDaemonOutputTracksImageStore asserts the command a cross-platform build emits: the daemon exporter
// is requested only when the daemon can hold a manifest list, and dropping it is reported rather than silent. Asking a
// classic-store daemon for it fails the whole build ("docker exporter does not currently support exporting manifest
// lists"), which would take the OCI layout down with it.
func TestCrossPlatformDaemonOutputTracksImageStore(t *testing.T) {
	if uses, err := defaultdockerbuilder.UsesDockerContainerDriver(); err != nil || !uses {
		t.Skip("requires an active docker-container buildx builder")
	}
	usesContainerdStore, err := defaultdockerbuilder.UsesContainerdImageStore()
	require.NoError(t, err)

	builder := defaultdockerbuilder.NewDefaultDockerBuilderWithOptions(
		defaultdockerbuilder.WithBuildxPlatforms([]string{"linux/amd64", "linux/arm64"}),
	)
	out := &bytes.Buffer{}
	require.NoError(t, builder.RunDockerBuild("img", newDockerProductInfo(t, "distgo-outputs-it:dryrun"), false, true, out))

	require.Contains(t, out.String(), "--output=type=oci", "the OCI layout is produced regardless of image store")
	if usesContainerdStore {
		require.Contains(t, out.String(), "--output=type=docker")
	} else {
		require.NotContains(t, out.String(), "--output=type=docker", "must not ask a classic-store daemon to hold a manifest list")
		require.Contains(t, out.String(), "does not use the containerd image store", "skipping the daemon load must be reported")
	}
}

// TestProductWithoutDistBuildsDaemonImageOnly covers a product that declares a Docker image but no dist: there is no
// dist output directory to hold an OCI layout, so the build produces the daemon image alone rather than failing.
func TestProductWithoutDistBuildsDaemonImageOnly(t *testing.T) {
	if uses, err := defaultdockerbuilder.UsesDockerContainerDriver(); err != nil || !uses {
		t.Skip("requires an active docker-container buildx builder")
	}

	const tag = "distgo-outputs-it:nodist"
	t.Cleanup(func() {
		_ = exec.Command("docker", "rmi", "-f", tag).Run()
	})
	outputInfo := newDockerProductInfo(t, tag)
	outputInfo.Product.DistOutputInfos = nil
	require.Empty(t, outputInfo.ProductDockerOCIDistOutputDir("img"), "a product with no dist must have no OCI output dir")
	builder := defaultdockerbuilder.NewDefaultDockerBuilder(nil, "")

	// The build command is echoed only on a dry run, so assert the emitted flags there.
	dryRunOut := &bytes.Buffer{}
	require.NoError(t, builder.RunDockerBuild("img", outputInfo, false, true, dryRunOut))
	require.NotContains(t, dryRunOut.String(), "--output=type=oci", "there is nowhere to write an OCI layout")
	require.Contains(t, dryRunOut.String(), "--output=type=docker")
	require.Contains(t, dryRunOut.String(), "declares no dist output", "skipping the OCI layout must be reported")

	require.NoError(t, builder.RunDockerBuild("img", outputInfo, false, false, io.Discard))
	require.NoError(t, exec.Command("docker", "image", "inspect", tag).Run(), "image must still be loaded into the local daemon")
	_, err := os.Stat(filepath.Join(outputInfo.Project.ProjectDir, "out"))
	require.True(t, os.IsNotExist(err), "no output directory should be created for a product with no dist")
}

func newDockerProductInfo(t *testing.T, tag string) distgo.ProductTaskOutputInfo {
	t.Helper()
	projectDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "ctx"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "ctx", "Dockerfile"), []byte("FROM alpine:latest\nRUN echo marker > /marker\n"), 0644))

	return distgo.ProductTaskOutputInfo{
		Project: distgo.ProjectInfo{ProjectDir: projectDir, Version: "1.0.0"},
		Product: distgo.ProductOutputInfo{
			ID:              "prod",
			DistOutputInfos: &distgo.DistOutputInfos{DistOutputDir: "out/dist"},
			DockerOutputInfos: &distgo.DockerOutputInfos{
				DockerIDs: []distgo.DockerID{"img"},
				DockerBuilderOutputInfos: map[distgo.DockerID]distgo.DockerBuilderOutputInfo{
					"img": {ContextDir: "ctx", DockerfilePath: "Dockerfile", RenderedTags: []string{tag}},
				},
			},
		},
	}
}
