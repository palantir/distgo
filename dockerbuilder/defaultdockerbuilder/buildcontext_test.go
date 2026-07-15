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
	wantArgs := []string{
		"--build-context", "registry/base:1.0.0=oci-layout://" + layoutDir,
		"--build-context", "registry/base:latest=oci-layout://" + layoutDir,
	}

	// dry run: emitted regardless of whether the layout exists on disk yet
	assert.Equal(t, wantArgs, dependencyImageBuildContextArgs(info, true))

	// non-dry-run: skipped while the dependency's buildx layout does not exist
	assert.Nil(t, dependencyImageBuildContextArgs(info, false))

	// non-dry-run: emitted once the dependency's buildx layout exists on disk
	require.NoError(t, os.MkdirAll(layoutDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(layoutDir, "index.json"), []byte("{}"), 0644))
	assert.Equal(t, wantArgs, dependencyImageBuildContextArgs(info, false))

	// no dependencies: no args
	assert.Nil(t, dependencyImageBuildContextArgs(distgo.ProductTaskOutputInfo{
		Project: distgo.ProjectInfo{ProjectDir: projectDir, Version: "1.0.0"},
	}, true))
}
