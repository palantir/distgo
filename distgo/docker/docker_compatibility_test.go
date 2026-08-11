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

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/palantir/distgo/distgo"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type legacyOutputDockerBuilder struct{}

func (legacyOutputDockerBuilder) TypeName() (string, error) {
	return "legacy", nil
}

func (legacyOutputDockerBuilder) RunDockerBuild(dockerID distgo.DockerID, info distgo.ProductTaskOutputInfo, _, _ bool, _ io.Writer) error {
	return writeOCILayout(info.ProductDockerOCIDistOutputDir(dockerID))
}

// daemonOnlyDockerBuilder stands in for a builder that loads an image into the Docker daemon and writes no OCI output
type daemonOnlyDockerBuilder struct{}

func (daemonOnlyDockerBuilder) TypeName() (string, error) {
	return "daemon", nil
}

func (daemonOnlyDockerBuilder) RunDockerBuild(distgo.DockerID, distgo.ProductTaskOutputInfo, bool, bool, io.Writer) error {
	return nil
}

func writeOCILayout(outputDir string) error {
	if err := os.MkdirAll(filepath.Join(outputDir, "blobs"), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outputDir, "oci-layout"), []byte(`{"imageLayoutVersion":"1.0.0"}`), 0644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outputDir, "index.json"), []byte(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[]}`), 0644)
}

// outputDirDockerBuilder writes its OCI layout to the directory the distgo running the task resolved, as an asset
// vendoring a current distgo does
type outputDirDockerBuilder struct{}

func (outputDirDockerBuilder) TypeName() (string, error) {
	return "outputdir", nil
}

func (outputDirDockerBuilder) RunDockerBuild(dockerID distgo.DockerID, info distgo.ProductTaskOutputInfo, _, _ bool, _ io.Writer) error {
	relDir := info.Product.DockerOutputInfos.DockerBuilderOutputInfos[dockerID].OutputDir
	if relDir == "" {
		return errors.Errorf("no output directory was provided for configuration %s", dockerID)
	}
	return writeOCILayout(filepath.Join(info.Project.ProjectDir, relDir))
}

// testOutputInfo returns the output info for a product with a single Docker configuration, as ToProductTaskOutputInfo
// produces it
func testOutputInfo(t *testing.T, dockerID distgo.DockerID) distgo.ProductTaskOutputInfo {
	projectDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "context"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "context", "Dockerfile"), []byte("FROM scratch\n"), 0644))

	return distgo.ProductTaskOutputInfo{
		Project: distgo.ProjectInfo{ProjectDir: projectDir, Version: "1.0.0"},
		Product: distgo.ProductOutputInfo{
			ID:              "product",
			DistOutputInfos: &distgo.DistOutputInfos{DistOutputDir: "out/dist"},
			DockerOutputInfos: &distgo.DockerOutputInfos{
				DockerOutputDir: "out/docker",
				DockerBuilderOutputInfos: map[distgo.DockerID]distgo.DockerBuilderOutputInfo{
					dockerID: {
						OutputDir:    filepath.Join("out", "docker", "product", "1.0.0", string(dockerID)),
						RenderedTags: []string{"product:1.0.0"},
					},
				},
			},
		},
	}
}

// runTestBuild runs the Docker build task for a single Docker configuration built by the given DockerBuilder
func runTestBuild(t *testing.T, info distgo.ProductTaskOutputInfo, dockerID distgo.DockerID, builder distgo.DockerBuilder) {
	t.Helper()
	param := distgo.DockerBuilderParam{
		DockerBuilder:  builder,
		ContextDir:     "context",
		DockerfilePath: "Dockerfile",
	}
	require.NoError(t, runSingleDockerBuild(info.Project, "product", "product", dockerID, param, info, nil, nil, false, false, io.Discard))
}

func TestLegacyDockerBuilderOutput(t *testing.T) {
	info := testOutputInfo(t, "legacy")
	runTestBuild(t, info, "legacy", legacyOutputDockerBuilder{})

	legacyOutputDir := info.ProductDockerOCIDistOutputDir("legacy")
	_, err := os.Stat(filepath.Join(legacyOutputDir, distgo.DockerBuildContextLayoutSubdir, "index.json"))
	require.NoError(t, err, "host should write the wrapper beside a legacy asset's OCI layout")
	assert.Equal(t, legacyOutputDir, dockerOCIOutputDir(info, "legacy"), "push should discover a legacy asset's OCI layout")
}

// a builder that writes to the directory it was given must be discovered there
func TestOutputDirDockerBuilderOutput(t *testing.T) {
	info := testOutputInfo(t, "outputdir")
	runTestBuild(t, info, "outputdir", outputDirDockerBuilder{})

	outputDir := distgo.ProductDockerOutputDir(info.Project, info.Product, "outputdir")
	_, err := os.Stat(filepath.Join(outputDir, distgo.DockerBuildContextLayoutSubdir, "index.json"))
	require.NoError(t, err, "host should write the wrapper beside the layout")
	assert.Equal(t, outputDir, dockerOCIOutputDir(info, "outputdir"), "push should discover the layout")
}

// nothing migrated the layouts an older distgo wrote to the legacy location, so a build that produces no OCI output
// must clear one: otherwise "docker push" publishes that earlier build's image instead of pushing from the daemon
func TestStaleLegacyDockerBuilderOutputRemoved(t *testing.T) {
	info := testOutputInfo(t, "daemon")
	legacyOutputDir := info.ProductDockerOCIDistOutputDir("daemon")
	require.NoError(t, writeOCILayout(legacyOutputDir))

	runTestBuild(t, info, "daemon", daemonOnlyDockerBuilder{})

	assert.NoDirExists(t, legacyOutputDir)
	assert.Empty(t, dockerOCIOutputDir(info, "daemon"), "push should fall back to the Docker daemon")
}

// a dist output that happens to occupy the legacy Docker output location is not Docker output and must be left alone
func TestNonLayoutLegacyDirNotRemoved(t *testing.T) {
	info := testOutputInfo(t, "daemon")
	legacyOutputDir := info.ProductDockerOCIDistOutputDir("daemon")
	require.NoError(t, os.MkdirAll(legacyOutputDir, 0755))
	distArtifact := filepath.Join(legacyOutputDir, "product-1.0.0.tgz")
	require.NoError(t, os.WriteFile(distArtifact, []byte("dist"), 0644))

	runTestBuild(t, info, "daemon", daemonOnlyDockerBuilder{})

	assert.FileExists(t, distArtifact)
}

// Before the OCI layout was made conformant, a single-platform build wrote a bare image manifest as index.json and
// kept the image only in image.tar. Probing the legacy location makes those layouts reachable again.
func TestLegacyBareManifestLayoutPush(t *testing.T) {
	info := testOutputInfo(t, "legacy")
	outputDir := info.ProductDockerOCIDistOutputDir("legacy")
	require.NoError(t, os.MkdirAll(filepath.Join(outputDir, "blobs"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(outputDir, "oci-layout"), []byte(`{"imageLayoutVersion":"1.0.0"}`), 0644))

	image := mutate.ConfigMediaType(mutate.MediaType(empty.Image, types.OCIManifestSchema1), types.OCIConfigJSON)
	rawManifest, err := image.RawManifest()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(outputDir, "index.json"), rawManifest, 0644))
	require.NoError(t, tarball.WriteToFile(filepath.Join(outputDir, "image.tar"), name.MustParseReference("product:1.0.0"), image))

	require.Equal(t, outputDir, dockerOCIOutputDir(info, "legacy"))
	require.NoError(t, runSingleDockerPush("product", "legacy", info, true, false, io.Discard))
}
