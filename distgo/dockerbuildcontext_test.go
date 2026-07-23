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

package distgo

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteDockerBuildContextLayout(t *testing.T) {
	ociDir := t.TempDir()
	indexJSON := []byte(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[]}`)
	require.NoError(t, os.WriteFile(filepath.Join(ociDir, "index.json"), indexJSON, 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(ociDir, "blobs", "sha256"), 0755))

	tags := []string{"registry/base:1.0.0", "registry/base:latest"}
	require.NoError(t, WriteDockerBuildContextLayout(ociDir, tags))

	wrapperDir := filepath.Join(ociDir, DockerBuildContextLayoutSubdir)

	_, err := os.Stat(filepath.Join(wrapperDir, "oci-layout"))
	require.NoError(t, err, "wrapper must have an oci-layout marker")

	// blobs is a symlink to the parent layout's blobs, so no layer data is copied.
	linkTarget, err := os.Readlink(filepath.Join(wrapperDir, "blobs"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("..", "blobs"), linkTarget)

	// index.json's content is stored as a blob so the wrapper descriptor resolves by digest.
	wantDigest, wantSize, err := v1.SHA256(bytes.NewReader(indexJSON))
	require.NoError(t, err)
	blob, err := os.ReadFile(filepath.Join(ociDir, "blobs", wantDigest.Algorithm, wantDigest.Hex))
	require.NoError(t, err)
	assert.Equal(t, indexJSON, blob)

	// wrapper index.json re-exposes that descriptor once per rendered tag.
	wrapperData, err := os.ReadFile(filepath.Join(wrapperDir, "index.json"))
	require.NoError(t, err)
	var wrapper v1.IndexManifest
	require.NoError(t, json.Unmarshal(wrapperData, &wrapper))
	assert.Equal(t, types.OCIImageIndex, wrapper.MediaType)
	require.Len(t, wrapper.Manifests, len(tags))
	for i, tag := range tags {
		desc := wrapper.Manifests[i]
		assert.Equal(t, wantDigest, desc.Digest)
		assert.Equal(t, wantSize, desc.Size)
		assert.Equal(t, types.OCIImageIndex, desc.MediaType)
		assert.Equal(t, tag, desc.Annotations["org.opencontainers.image.ref.name"])
	}
}

func TestWriteDockerBuildContextLayoutNoTagsIsNoOp(t *testing.T) {
	ociDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(ociDir, "index.json"),
		[]byte(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[]}`), 0644))

	require.NoError(t, WriteDockerBuildContextLayout(ociDir, nil))

	_, err := os.Stat(filepath.Join(ociDir, DockerBuildContextLayoutSubdir))
	assert.True(t, os.IsNotExist(err), "no wrapper should be written when there are no rendered tags")
}
