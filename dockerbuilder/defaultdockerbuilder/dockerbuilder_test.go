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
	"fmt"
	"os"
	"path/filepath"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/require"
)

// TestFinalizeOCILayoutIsRerunnable verifies that finalizing a layout that has already been finalized succeeds. Output
// directory names do not change between runs at an unchanged version (e.g. a "-dirty" version), so re-running
// "docker build" finalizes over the previous run's output.
func TestFinalizeOCILayoutIsRerunnable(t *testing.T) {
	destDir := writeBuildxOCILayout(t, 1)

	require.NoError(t, finalizeOCILayout(destDir), "first finalize should succeed")
	require.NoError(t, finalizeOCILayout(destDir), "finalizing an already-finalized layout should succeed")
}

// TestFinalizeOCILayout_ProducesConformantIndexForSinglePlatformBuild verifies that a single-platform build's OCI layout
// gets a conformant image index at the top-level index.json, not a bare image manifest.
func TestFinalizeOCILayout_ProducesConformantIndexForSinglePlatformBuild(t *testing.T) {
	destDir := writeBuildxOCILayout(t, 1)
	wantDigest := topLevelDescriptors(t, destDir)[0].Digest

	require.NoError(t, finalizeOCILayout(destDir))

	destIndex, err := layout.ImageIndexFromPath(destDir)
	require.NoError(t, err, "resulting OCI layout must remain readable as an image index")
	destIdxManifest, err := destIndex.IndexManifest()
	require.NoError(t, err)
	require.Equal(t, types.OCIImageIndex, destIdxManifest.MediaType, "top-level index.json must always be an image index, never a bare manifest")
	require.Len(t, destIdxManifest.Manifests, 1)
	require.Equal(t, wantDigest, destIdxManifest.Manifests[0].Digest)

	// The referenced manifest's blob must still be resolvable by digest, not just inlined into index.json.
	image, err := destIndex.Image(wantDigest)
	require.NoError(t, err, "manifest blob must remain addressable under blobs/ after finalizing")
	gotDigest, err := image.Digest()
	require.NoError(t, err)
	require.Equal(t, wantDigest, gotDigest)
}

// TestFinalizeOCILayout_CollapsesPerTagDescriptors verifies that the one-descriptor-per-tag index buildx writes for a
// multi-tag build is reduced to a single descriptor, so publishing it cannot produce a manifest list whose entries are
// all the same image.
func TestFinalizeOCILayout_CollapsesPerTagDescriptors(t *testing.T) {
	destDir := writeBuildxOCILayout(t, 3)
	srcDescriptors := topLevelDescriptors(t, destDir)
	require.Len(t, srcDescriptors, 3, "fixture must reproduce buildx's per-tag descriptors")

	require.NoError(t, finalizeOCILayout(destDir))

	require.Len(t, topLevelDescriptors(t, destDir), 1)
	require.Equal(t, srcDescriptors[0].Digest, topLevelDescriptors(t, destDir)[0].Digest)
}

// TestFinalizeOCILayout_RemovesStagingDir verifies that buildx's content-store staging directory does not survive into
// the published layout.
func TestFinalizeOCILayout_RemovesStagingDir(t *testing.T) {
	destDir := writeBuildxOCILayout(t, 1)
	require.NoError(t, os.MkdirAll(filepath.Join(destDir, "ingest"), 0755))

	require.NoError(t, finalizeOCILayout(destDir))

	_, err := os.Stat(filepath.Join(destDir, "ingest"))
	require.True(t, os.IsNotExist(err), "staging directory must be removed")
}

// writeBuildxOCILayout writes a minimal OCI layout shaped like buildx's "--output=type=oci,tar=false" output: one
// top-level descriptor per tag, each pointing at the same image manifest.
func writeBuildxOCILayout(t *testing.T, tags int) string {
	t.Helper()
	addenda := make([]mutate.IndexAddendum, 0, tags)
	for i := range tags {
		addenda = append(addenda, mutate.IndexAddendum{
			Add: empty.Image,
			Descriptor: v1.Descriptor{
				Annotations: map[string]string{"org.opencontainers.image.ref.name": fmt.Sprintf("tag-%d", i)},
			},
		})
	}
	destDir := filepath.Join(t.TempDir(), "oci")
	_, err := layout.Write(destDir, mutate.AppendManifests(empty.Index, addenda...))
	require.NoError(t, err)
	return destDir
}

func topLevelDescriptors(t *testing.T, layoutDir string) []v1.Descriptor {
	t.Helper()
	index, err := layout.ImageIndexFromPath(layoutDir)
	require.NoError(t, err)
	idxManifest, err := index.IndexManifest()
	require.NoError(t, err)
	return idxManifest.Manifests
}
