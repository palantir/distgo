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
	"os"
	"path/filepath"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/palantir/distgo/distgo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDependencyImageBuildContextArgs(t *testing.T) {
	projectDir := t.TempDir()
	info := distgo.ProductTaskOutputInfo{
		Project: distgo.ProjectInfo{ProjectDir: projectDir, Version: "1.0.0"},
		Deps: map[distgo.ProductID]distgo.ProductOutputInfo{
			"base": {
				ID:              "base",
				DistOutputInfos: &distgo.DistOutputInfos{DistOutputDir: "out/dist"},
				DockerOutputInfos: &distgo.DockerOutputInfos{
					DockerIDs: []distgo.DockerID{"base-docker"},
					DockerBuilderOutputInfos: map[distgo.DockerID]distgo.DockerBuilderOutputInfo{
						"base-docker": {RenderedTags: []string{"registry/base:1.0.0", "registry/base:latest"}},
					},
				},
			},
		},
	}
	layoutDir := filepath.Join(projectDir, "out", "dist", "base", "1.0.0", "oci-base-docker", distgo.DockerBuildContextLayoutSubdir)
	unpinnedArgs := []string{
		"--build-context", "registry/base:1.0.0=oci-layout://" + layoutDir,
		"--build-context", "registry/base:latest=oci-layout://" + layoutDir,
	}

	// dry run before the dependency is built: emitted unpinned (buildx is not invoked, so the digest-less form is fine)
	assert.Equal(t, unpinnedArgs, dependencyImageBuildContextArgs(info, true))

	// non-dry-run: skipped while the dependency's buildx layout does not exist
	assert.Nil(t, dependencyImageBuildContextArgs(info, false))

	// once the dependency's buildx layout exists, each ref is pinned with the digest from its index.json entry so
	// buildx v0.35+ can resolve the oci-layout:// reference.
	const digest = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	writeWrapperIndex(t, layoutDir, digest, "registry/base:1.0.0", "registry/base:latest")
	pinnedArgs := []string{
		"--build-context", "registry/base:1.0.0=oci-layout://" + layoutDir + "@" + digest,
		"--build-context", "registry/base:latest=oci-layout://" + layoutDir + "@" + digest,
	}
	assert.Equal(t, pinnedArgs, dependencyImageBuildContextArgs(info, false))
	assert.Equal(t, pinnedArgs, dependencyImageBuildContextArgs(info, true))

	// no dependencies: no args
	assert.Nil(t, dependencyImageBuildContextArgs(distgo.ProductTaskOutputInfo{
		Project: distgo.ProjectInfo{ProjectDir: projectDir, Version: "1.0.0"},
	}, true))
}

// writeWrapperIndex writes an index.json shaped like distgo.WriteDockerBuildContextLayout's output: one manifest per
// rendered tag, each naming the tag via distgo.OCIRefNameAnnotation and sharing the wrapping image-index digest.
func writeWrapperIndex(t *testing.T, layoutDir, digest string, tags ...string) {
	require.NoError(t, os.MkdirAll(layoutDir, 0755))
	h, err := v1.NewHash(digest)
	require.NoError(t, err)
	manifests := make([]v1.Descriptor, 0, len(tags))
	for _, tag := range tags {
		manifests = append(manifests, v1.Descriptor{
			MediaType:   types.OCIImageIndex,
			Digest:      h,
			Annotations: map[string]string{distgo.OCIRefNameAnnotation: tag},
		})
	}
	data, err := json.Marshal(v1.IndexManifest{
		SchemaVersion: 2,
		MediaType:     types.OCIImageIndex,
		Manifests:     manifests,
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(layoutDir, "index.json"), data, 0644))
}
