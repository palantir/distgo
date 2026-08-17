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

package distgo_test

import (
	"path/filepath"
	"testing"

	"github.com/palantir/distgo/distgo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProductDockerOutputDirCandidates(t *testing.T) {
	project := distgo.ProjectInfo{ProjectDir: t.TempDir(), Version: "1.2.3"}
	product := distgo.ProductOutputInfo{
		ID:                "product",
		DistOutputInfos:   &distgo.DistOutputInfos{DistOutputDir: "out/dist"},
		DockerOutputInfos: &distgo.DockerOutputInfos{DockerOutputDir: "out/docker"},
	}
	want := []string{
		filepath.Join(project.ProjectDir, "out", "docker", "product", "1.2.3", "builder"),
		filepath.Join(project.ProjectDir, "out", "dist", "product", "1.2.3", "oci-builder"),
	}
	assert.Equal(t, want, distgo.ProductDockerOutputDirCandidates(project, product, "builder"))

	info := distgo.ProductTaskOutputInfo{Project: project, Product: product}
	assert.Equal(t, want[1], info.ProductDockerOCIDistOutputDir("builder"))

	product.DistOutputInfos = nil
	assert.Equal(t, want[:1], distgo.ProductDockerOutputDirCandidates(project, product, "builder"))
}

// output info from a distgo that predates the Docker output directory carries none: joining it would resolve output
// into the source tree, so it must degrade to the legacy location instead
func TestProductDockerOutputDirUnset(t *testing.T) {
	project := distgo.ProjectInfo{ProjectDir: t.TempDir(), Version: "1.2.3"}
	product := distgo.ProductOutputInfo{
		ID:                "product",
		DistOutputInfos:   &distgo.DistOutputInfos{DistOutputDir: "out/dist"},
		DockerOutputInfos: &distgo.DockerOutputInfos{},
	}
	assert.Empty(t, distgo.ProductDockerOutputDir(project, product, "builder"))
	assert.Equal(t,
		[]string{filepath.Join(project.ProjectDir, "out", "dist", "product", "1.2.3", "oci-builder")},
		distgo.ProductDockerOutputDirCandidates(project, product, "builder"))

	product.DistOutputInfos = nil
	assert.Empty(t, distgo.ProductDockerOutputDirCandidates(project, product, "builder"))
}

// the distgo running the task resolves the output directory and passes it to the DockerBuilder as data, so the two
// agree even when built from different versions of distgo
func TestToProductTaskOutputInfoResolvesDockerOutputDir(t *testing.T) {
	dockerParam := func() *distgo.DockerParam {
		return &distgo.DockerParam{
			OutputDir:           "out/docker",
			DockerBuilderParams: map[distgo.DockerID]distgo.DockerBuilderParam{"builder": {}},
		}
	}
	// the product name differs from the product ID, as a product-name override makes it: the directory keys off the ID
	param := distgo.ProductParam{
		ID:     "product-id",
		Name:   "product-name",
		Docker: dockerParam(),
		AllDependencies: map[distgo.ProductID]distgo.ProductParam{
			"dep-id": {ID: "dep-id", Name: "dep-name", Docker: dockerParam()},
		},
	}

	info, err := distgo.ToProductTaskOutputInfo(distgo.ProjectInfo{ProjectDir: "/project", Version: "1.2.3"}, param)
	require.NoError(t, err)

	assert.Equal(t, "out/docker/product-id/1.2.3/builder", info.Product.DockerOutputInfos.DockerBuilderOutputInfos["builder"].OutputDir)
	assert.Equal(t, "out/docker/dep-id/1.2.3/builder", info.Deps["dep-id"].DockerOutputInfos.DockerBuilderOutputInfos["builder"].OutputDir)

	// a builder joining the resolved directory with the project directory must land where the task looks for output
	assert.Equal(t,
		distgo.ProductDockerOutputDir(info.Project, info.Product, "builder"),
		filepath.Join(info.Project.ProjectDir, info.Product.DockerOutputInfos.DockerBuilderOutputInfos["builder"].OutputDir))
}
