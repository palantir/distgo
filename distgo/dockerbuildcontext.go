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

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/pkg/errors"
)

// DockerBuildContextLayoutSubdir is the subdirectory within a Docker image's OCI dist output directory that holds a
// buildx-consumable "wrapper" OCI layout. A dependent product's "FROM <dep tag>" resolves from this layout (passed to
// buildx as "--build-context <tag>=oci-layout://<subdir>") instead of from a registry.
const DockerBuildContextLayoutSubdir = "docker-build-context"

// OCIRefNameAnnotation is the OCI annotation buildx matches against an oci-layout build-context name (the rendered FROM
// tag) to resolve a manifest in the layout's index.json. WriteDockerBuildContextLayout stamps it on the wrapper
// descriptor; the build-context consumer reads it back to pin each reference to its digest.
const OCIRefNameAnnotation = "org.opencontainers.image.ref.name"

// WriteDockerBuildContextLayout writes the buildx-consumable wrapper OCI layout (see DockerBuildContextLayoutSubdir)
// into the OCI layout at ociLayoutDir, exposing the image under each of renderedTags so a dependent product's
// "FROM <tag>" can resolve it from disk with no registry.
//
// It is builder-agnostic: it re-exposes whatever top-level descriptor the layout's index.json points at (a multi-arch
// image-index or a single image manifest). The Docker build task calls this after the builder runs -- not the builder
// itself -- so it works for any builder that leaves an OCI layout on disk, including re-layering builders (e.g. the
// chunkah asset) that rewrite the layout after building and would clobber a wrapper the builder had written.
//
// buildx resolves an oci-layout build context by matching the "org.opencontainers.image.ref.name" annotation on a
// manifest in the layout's index.json against the build-context name (the rendered FROM tag), so the wrapper annotates
// the descriptor with each rendered tag. Layer/manifest blobs are shared with the parent layout via a "blobs" symlink
// (nothing is copied); index.json's own content is (re-)written into the parent's blobs so the wrapper descriptor
// resolves by digest -- go-containerregistry's layout.Write stores it only as index.json, not as a blob.
//
// It is a no-op when renderedTags is empty (nothing to name the image by).
func WriteDockerBuildContextLayout(ociLayoutDir string, renderedTags []string) error {
	if len(renderedTags) == 0 {
		return nil
	}

	indexData, err := os.ReadFile(filepath.Join(ociLayoutDir, "index.json"))
	if err != nil {
		return errors.Wrap(err, "failed to read OCI layout index.json")
	}
	var probe struct {
		MediaType types.MediaType `json:"mediaType"`
	}
	if err := json.Unmarshal(indexData, &probe); err != nil {
		return errors.Wrap(err, "failed to parse OCI layout index.json mediaType")
	}
	digest, size, err := v1.SHA256(bytes.NewReader(indexData))
	if err != nil {
		return errors.Wrap(err, "failed to hash OCI layout index.json")
	}

	// Store index.json's content as a blob so the wrapper descriptor (which points at it by digest) resolves.
	indexBlobPath := filepath.Join(ociLayoutDir, "blobs", digest.Algorithm, digest.Hex)
	if err := os.MkdirAll(filepath.Dir(indexBlobPath), 0755); err != nil {
		return errors.Wrap(err, "failed to create blobs directory")
	}
	if err := os.WriteFile(indexBlobPath, indexData, 0644); err != nil {
		return errors.Wrapf(err, "failed to write image-index blob %s", indexBlobPath)
	}

	layoutDir := filepath.Join(ociLayoutDir, DockerBuildContextLayoutSubdir)
	if err := os.MkdirAll(filepath.Join(layoutDir, "blobs"), 0755); err != nil {
		return errors.Wrapf(err, "failed to create buildx context layout dir %s", layoutDir)
	}
	// Share the parent layout's blobs via a symlink so no layer data is copied.
	blobsLink := filepath.Join(layoutDir, "blobs")
	if err := os.Remove(blobsLink); err != nil && !os.IsNotExist(err) {
		return errors.Wrapf(err, "failed to reset buildx context blobs path %s", blobsLink)
	}
	if err := os.Symlink(filepath.Join("..", "blobs"), blobsLink); err != nil {
		return errors.Wrapf(err, "failed to symlink buildx context blobs %s", blobsLink)
	}
	if err := os.WriteFile(filepath.Join(layoutDir, "oci-layout"), []byte(`{"imageLayoutVersion":"1.0.0"}`), 0644); err != nil {
		return errors.Wrap(err, "failed to write buildx context oci-layout")
	}

	manifests := make([]v1.Descriptor, 0, len(renderedTags))
	for _, tag := range renderedTags {
		manifests = append(manifests, v1.Descriptor{
			MediaType:   probe.MediaType,
			Digest:      digest,
			Size:        size,
			Annotations: map[string]string{OCIRefNameAnnotation: tag},
		})
	}
	wrapperBytes, err := json.Marshal(v1.IndexManifest{
		SchemaVersion: 2,
		MediaType:     types.OCIImageIndex,
		Manifests:     manifests,
	})
	if err != nil {
		return errors.Wrap(err, "failed to marshal buildx context index")
	}
	if err := os.WriteFile(filepath.Join(layoutDir, "index.json"), wrapperBytes, 0644); err != nil {
		return errors.Wrap(err, "failed to write buildx context index.json")
	}
	return nil
}
