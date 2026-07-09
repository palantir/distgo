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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/palantir/distgo/distgo"
	"github.com/pkg/errors"
)

// buildxContextLayoutSubdir holds the buildx-consumable "wrapper" OCI layout within an image's OCI dist output dir.
const buildxContextLayoutSubdir = "docker-build-context"

// dependencyImageBuildContextArgs returns "--build-context <renderedTag>=oci-layout://<layout>" args for each declared
// dependency Docker image, so a "FROM <dep tag>" resolves from the dependency's on-disk OCI layout instead of a
// registry. buildx validates every --build-context eagerly, so outside of dry runs an entry is emitted only when the
// layout exists on disk: dependencies build before dependents, so a multi-arch or SLS dependency's layout is present,
// while a daemon-only "default" builder produces none and is skipped.
func dependencyImageBuildContextArgs(productTaskOutputInfo distgo.ProductTaskOutputInfo, dryRun bool) []string {
	depIDs := make([]string, 0, len(productTaskOutputInfo.Deps))
	for depID := range productTaskOutputInfo.Deps {
		depIDs = append(depIDs, string(depID))
	}
	sort.Strings(depIDs)

	var args []string
	for _, depID := range depIDs {
		depOutputInfo := productTaskOutputInfo.Deps[distgo.ProductID(depID)]
		if depOutputInfo.DockerOutputInfos == nil {
			continue
		}
		for _, dockerID := range depOutputInfo.DockerOutputInfos.DockerIDs {
			builderOutputInfo, ok := depOutputInfo.DockerOutputInfos.DockerBuilderOutputInfos[dockerID]
			if !ok || len(builderOutputInfo.RenderedTags) == 0 {
				continue
			}
			ociDir := distgo.ProductDistOutputDir(productTaskOutputInfo.Project, depOutputInfo, distgo.DistID(fmt.Sprintf("oci-%s", dockerID)))
			if ociDir == "" {
				continue
			}
			layoutDir := filepath.Join(ociDir, buildxContextLayoutSubdir)
			if !dryRun {
				if _, err := os.Stat(filepath.Join(layoutDir, "index.json")); err != nil {
					continue
				}
			}
			for _, tag := range builderOutputInfo.RenderedTags {
				args = append(args, "--build-context", fmt.Sprintf("%s=oci-layout://%s", tag, layoutDir))
			}
		}
	}
	return args
}

// writeBuildxContextLayout writes a buildx-consumable "wrapper" OCI layout into buildxContextLayoutSubdir. The
// publish-shaped index.json from extractToOCILayout has the multi-arch index's per-platform manifests at the top
// level, which buildx cannot resolve as a "FROM" base; the wrapper re-adds a top-level index that names the image via
// its rendered tag(s). Layer blobs are shared with the parent layout via a "blobs" symlink (nothing is copied); only
// the small image-index blob that extractToOCILayout moved to index.json is restored so the wrapper descriptor resolves.
func writeBuildxContextLayout(ociLayoutDir string, indexDesc v1.Descriptor, renderedTags []string) error {
	// Restore the image-index blob that extractToOCILayout moved out to index.json.
	indexData, err := os.ReadFile(filepath.Join(ociLayoutDir, "index.json"))
	if err != nil {
		return errors.Wrap(err, "failed to read OCI layout index.json")
	}
	indexBlobPath := filepath.Join(ociLayoutDir, "blobs", indexDesc.Digest.Algorithm, indexDesc.Digest.Hex)
	if err := os.WriteFile(indexBlobPath, indexData, 0644); err != nil {
		return errors.Wrapf(err, "failed to restore image-index blob %s", indexBlobPath)
	}

	layoutDir := filepath.Join(ociLayoutDir, buildxContextLayoutSubdir)
	if err := os.MkdirAll(filepath.Join(layoutDir, "blobs"), 0755); err != nil {
		return errors.Wrapf(err, "failed to create buildx context layout dir %s", layoutDir)
	}
	// Share the parent layout's blobs via symlink so no layer data is copied.
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
		desc := indexDesc
		desc.Annotations = map[string]string{"org.opencontainers.image.ref.name": tag}
		manifests = append(manifests, desc)
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
