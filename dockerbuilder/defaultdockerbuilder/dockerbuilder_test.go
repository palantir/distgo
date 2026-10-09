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

package defaultdockerbuilder

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/mholt/archiver/v3"
	"github.com/palantir/distgo/distgo"
	"github.com/palantir/pkg/gittest"
	"github.com/stretchr/testify/require"
)

// TestExtractToOCILayoutIsRerunnable verifies that extracting into an output directory that already contains the
// artifacts of a previous extraction succeeds. This guards against the failure seen when re-running "docker build" at
// the same version (e.g. a "-dirty" version, whose output directory name does not change between runs), where the tar
// extractor would otherwise refuse to overwrite the existing OCI layout files.
func TestExtractToOCILayoutIsRerunnable(t *testing.T) {
	// Build a minimal but valid OCI image layout and pack it into a tarball shaped like buildx's OCI output.
	srcLayoutDir := filepath.Join(t.TempDir(), "src")
	_, err := layout.Write(srcLayoutDir, mutate.AppendManifests(empty.Index, mutate.IndexAddendum{Add: empty.Image}))
	require.NoError(t, err)

	destDir := t.TempDir()
	tarball := filepath.Join(destDir, "image.tar")
	require.NoError(t, archiver.DefaultTar.Archive([]string{
		filepath.Join(srcLayoutDir, "oci-layout"),
		filepath.Join(srcLayoutDir, "index.json"),
		filepath.Join(srcLayoutDir, "blobs"),
	}, tarball))

	b := &DefaultDockerBuilder{}
	require.NoError(t, b.extractToOCILayout(destDir, tarball), "first extraction should succeed")
	require.NoError(t, b.extractToOCILayout(destDir, tarball), "re-extraction into a populated directory should succeed")

	// The source tarball must be preserved across re-extraction (it is the input, not stale output).
	_, err = os.Stat(tarball)
	require.NoError(t, err)
}

// TestExtractToOCILayout_ProducesConformantIndexForSinglePlatformBuild verifies that a single-platform build's OCI
// layout gets a conformant image index at the top-level index.json, not a bare image manifest.
func TestExtractToOCILayout_ProducesConformantIndexForSinglePlatformBuild(t *testing.T) {
	// Build a minimal OCI image layout shaped like buildx's raw "--output=type=oci" output for a single-platform/single-tag build.
	srcLayoutDir := filepath.Join(t.TempDir(), "src")
	srcPath, err := layout.Write(srcLayoutDir, mutate.AppendManifests(empty.Index, mutate.IndexAddendum{Add: empty.Image}))
	require.NoError(t, err)

	srcIndex, err := srcPath.ImageIndex()
	require.NoError(t, err)
	srcIdxManifest, err := srcIndex.IndexManifest()
	require.NoError(t, err)
	require.Len(t, srcIdxManifest.Manifests, 1)
	wantDigest := srcIdxManifest.Manifests[0].Digest

	destDir := t.TempDir()
	tarball := filepath.Join(destDir, "image.tar")
	require.NoError(t, archiver.DefaultTar.Archive([]string{
		filepath.Join(srcLayoutDir, "oci-layout"),
		filepath.Join(srcLayoutDir, "index.json"),
		filepath.Join(srcLayoutDir, "blobs"),
	}, tarball))

	b := &DefaultDockerBuilder{}
	require.NoError(t, b.extractToOCILayout(destDir, tarball))

	destIndex, err := layout.ImageIndexFromPath(destDir)
	require.NoError(t, err, "resulting OCI layout must remain readable as an image index")
	destIdxManifest, err := destIndex.IndexManifest()
	require.NoError(t, err)
	require.Equal(t, types.OCIImageIndex, destIdxManifest.MediaType, "top-level index.json must always be an image index, never a bare manifest")
	require.Len(t, destIdxManifest.Manifests, 1)
	require.Equal(t, wantDigest, destIdxManifest.Manifests[0].Digest)

	// The referenced manifest's blob must still be resolvable by digest, not just inlined into index.json.
	image, err := destIndex.Image(wantDigest)
	require.NoError(t, err, "manifest blob must remain addressable under blobs/ after extraction")
	gotDigest, err := image.Digest()
	require.NoError(t, err)
	require.Equal(t, wantDigest, gotDigest)
}

// the distgo running the task resolves the output directory; one that predates either that field or the Docker output
// directory sends less, and the location is derived from what it did send
func TestOCIOutputDir(t *testing.T) {
	projectDir := t.TempDir()
	info := func(resolvedDir, dockerOutputDir string, dist *distgo.DistOutputInfos) distgo.ProductTaskOutputInfo {
		return distgo.ProductTaskOutputInfo{
			Project: distgo.ProjectInfo{ProjectDir: projectDir, Version: "1.0.0"},
			Product: distgo.ProductOutputInfo{
				ID:              "product",
				DistOutputInfos: dist,
				DockerOutputInfos: &distgo.DockerOutputInfos{
					DockerOutputDir: dockerOutputDir,
					DockerBuilderOutputInfos: map[distgo.DockerID]distgo.DockerBuilderOutputInfo{
						"builder": {OutputDir: resolvedDir},
					},
				},
			},
		}
	}
	distOutputInfos := &distgo.DistOutputInfos{DistOutputDir: "out/dist"}

	// the resolved directory wins over anything this builder would derive
	dir, err := ociOutputDir(info("elsewhere/product/1.0.0/builder", "out/docker", distOutputInfos), "builder")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(projectDir, "elsewhere", "product", "1.0.0", "builder"), dir)

	dir, err = ociOutputDir(info("", "out/docker", distOutputInfos), "builder")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(projectDir, "out", "docker", "product", "1.0.0", "builder"), dir)

	// a distgo that predates the Docker output directory looks only in the legacy dist-based location
	dir, err = ociOutputDir(info("", "", distOutputInfos), "builder")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(projectDir, "out", "dist", "product", "1.0.0", "oci-builder"), dir)

	_, err = ociOutputDir(info("", "", nil), "builder")
	require.EqualError(t, err, "no output directory is available for OCI output for configuration builder: the product declares neither a Docker nor a dist output directory")
}

func TestOCIImageMetadataArgs(t *testing.T) {
	// Only buildx inspect executes during a dry run. Stub it so these tests need no Docker daemon.
	binDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "docker"), []byte("#!/bin/sh\nif [ \"$1 $2\" = \"buildx inspect\" ]; then echo 'Driver: docker-container'; else exit 1; fi\n"), 0755))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	for _, tc := range []struct {
		name      string
		platforms []string
		scope     string
		origin    string
		source    string
		metadata  bool
	}{
		{name: "host", scope: "manifest", origin: "git@github.com:example/project.git", source: "https://github.com/example/project", metadata: true},
		{name: "single", platforms: []string{"linux/amd64"}, scope: "manifest", origin: "git@github.com:example/project.git", source: "https://github.com/example/project", metadata: true},
		{name: "multi", platforms: []string{"linux/amd64", "linux/arm64"}, scope: "index,manifest,manifest-descriptor", origin: "git@github.com:example/project.git", source: "https://github.com/example/project", metadata: true},
		{name: "no origin", platforms: []string{"linux/amd64", "linux/arm64"}, scope: "index,manifest,manifest-descriptor", metadata: true},
		{name: "local origin", scope: "manifest", origin: "/tmp/project.git", source: "https:///tmp/project", metadata: true},
		{name: "opt in required", platforms: []string{"linux/amd64", "linux/arm64"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			projectDir := t.TempDir()
			gittest.RunGitCommand(t, projectDir, "init")
			if tc.origin != "" {
				gittest.RunGitCommand(t, projectDir, "remote", "add", "origin", tc.origin)
			}
			info := distgo.ProductTaskOutputInfo{
				Project: distgo.ProjectInfo{ProjectDir: projectDir, Version: "1.2.3"},
				Product: distgo.ProductOutputInfo{
					ID: "product",
					DockerOutputInfos: &distgo.DockerOutputInfos{
						DockerOutputDir: "out/docker",
						DockerBuilderOutputInfos: map[distgo.DockerID]distgo.DockerBuilderOutputInfo{
							"image": {ContextDir: "context", DockerfilePath: "Dockerfile"},
						},
					},
				},
			}
			options := []Option{WithBuildxOutput(OCILayout | DockerDaemon), WithBuildxPlatforms(tc.platforms)}
			var userArgs []string
			if tc.metadata {
				options = append(options, WithOCIImageMetadata())
				userArgs = []string{"--label", distgo.OCIImageVersionAnnotation + "=user-version"}
				options = append(options, WithBuildArgs(userArgs), WithBuildArgsScript("#!/bin/sh\nprintf '%s\\n' '--label' '"+distgo.OCIImageVersionAnnotation+"=script-version'\n"))
			}
			builder := NewDefaultDockerBuilderWithOptions(options...)
			var stdout bytes.Buffer
			require.NoError(t, builder.RunDockerBuild("image", info, true, true, &stdout))
			lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
			require.Len(t, lines, 2, stdout.String())
			oci, daemon := lines[0], lines[1]
			require.Contains(t, oci, "--output=type=oci,")
			require.Contains(t, daemon, "--output=type=docker,")
			require.NotContains(t, daemon, "--annotation")
			require.NotContains(t, daemon, "--platform")
			if len(tc.platforms) == 0 {
				require.NotContains(t, oci, "--platform")
			} else {
				require.Contains(t, oci, "--platform="+strings.Join(tc.platforms, ","))
			}
			if !tc.metadata {
				require.NotContains(t, stdout.String(), "--label")
				require.NotContains(t, stdout.String(), "--annotation")
				return
			}
			labels := fmt.Sprintf("--label %s=%s --label %s=1.2.3", distgo.OCIImageSourceAnnotation, tc.source, distgo.OCIImageVersionAnnotation)
			for _, line := range lines {
				require.Contains(t, line, labels)
				require.Contains(t, line, "--label "+distgo.OCIImageVersionAnnotation+"=user-version --label "+distgo.OCIImageVersionAnnotation+"=script-version")
				require.Less(t, strings.Index(line, labels), strings.Index(line, "=user-version"))
			}
			annotations := "--annotation " + tc.scope + ":" + distgo.OCIImageVersionAnnotation + "=1.2.3"
			if tc.source != "" {
				annotations = "--annotation " + tc.scope + ":" + distgo.OCIImageSourceAnnotation + "=" + tc.source + " " + annotations
			} else {
				require.NotContains(t, oci, ":"+distgo.OCIImageSourceAnnotation+"=")
			}
			require.Contains(t, oci, labels+" "+annotations+" "+strings.Join(userArgs, " "))
		})
	}
}
