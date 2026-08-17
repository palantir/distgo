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
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/mholt/archiver/v3"
	"github.com/palantir/distgo/distgo"
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
