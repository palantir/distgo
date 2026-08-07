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
