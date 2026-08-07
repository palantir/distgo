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

package docker

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/palantir/distgo/distgo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type legacyOutputDockerBuilder struct{}

func (legacyOutputDockerBuilder) TypeName() (string, error) {
	return "legacy", nil
}

func (legacyOutputDockerBuilder) RunDockerBuild(dockerID distgo.DockerID, info distgo.ProductTaskOutputInfo, _, _ bool, _ io.Writer) error {
	outputDir := info.ProductDockerOCIDistOutputDir(dockerID)
	if err := os.MkdirAll(filepath.Join(outputDir, "blobs"), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outputDir, "oci-layout"), []byte(`{"imageLayoutVersion":"1.0.0"}`), 0644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outputDir, "index.json"), []byte(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[]}`), 0644)
}

func TestLegacyDockerBuilderOutput(t *testing.T) {
	projectDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "context"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "context", "Dockerfile"), []byte("FROM scratch\n"), 0644))

	info := distgo.ProductTaskOutputInfo{
		Project: distgo.ProjectInfo{ProjectDir: projectDir, Version: "1.0.0"},
		Product: distgo.ProductOutputInfo{
			ID:              "product",
			DistOutputInfos: &distgo.DistOutputInfos{DistOutputDir: "out/dist"},
			DockerOutputInfos: &distgo.DockerOutputInfos{
				DockerOutputDir: "out/docker",
				DockerBuilderOutputInfos: map[distgo.DockerID]distgo.DockerBuilderOutputInfo{
					"legacy": {RenderedTags: []string{"product:1.0.0"}},
				},
			},
		},
	}
	param := distgo.DockerBuilderParam{
		DockerBuilder:  legacyOutputDockerBuilder{},
		ContextDir:     "context",
		DockerfilePath: "Dockerfile",
	}
	require.NoError(t, runSingleDockerBuild(info.Project, "product", "product", "legacy", param, info, nil, nil, false, false, io.Discard))

	legacyOutputDir := info.ProductDockerOCIDistOutputDir("legacy")
	_, err := os.Stat(filepath.Join(legacyOutputDir, distgo.DockerBuildContextLayoutSubdir, "index.json"))
	require.NoError(t, err, "host should write the wrapper beside a legacy asset's OCI layout")
	assert.Equal(t, legacyOutputDir, dockerOCIOutputDir(info, "legacy"), "push should discover a legacy asset's OCI layout")
}
