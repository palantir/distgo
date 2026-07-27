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

package mavenlocal_test

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/publisher/mavenlocal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunPublish_UsesEachInputsOwnConfig(t *testing.T) {
	projectDir := t.TempDir()
	fooBaseDir := filepath.Join(t.TempDir(), "foo-repo")
	barBaseDir := filepath.Join(t.TempDir(), "bar-repo")

	inputs := []distgo.ProductPublishInfo{
		testProductInput(t, projectDir, "foo", fooBaseDir),
		testProductInput(t, projectDir, "bar", barBaseDir),
	}

	publisher := mavenlocal.PublisherCreator().Publisher()
	err := publisher.RunPublish(inputs, nil, false, io.Discard)
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(fooBaseDir, "com/test/group/foo/1.0.0/foo-1.0.0-linux-amd64.tgz"))
	assert.FileExists(t, filepath.Join(barBaseDir, "com/test/group/bar/1.0.0/bar-1.0.0-linux-amd64.tgz"))
	assert.NoFileExists(t, filepath.Join(fooBaseDir, "com/test/group/bar/1.0.0/bar-1.0.0-linux-amd64.tgz"))
	assert.NoFileExists(t, filepath.Join(barBaseDir, "com/test/group/foo/1.0.0/foo-1.0.0-linux-amd64.tgz"))
}

func TestRunPublish_StopsOnFirstError(t *testing.T) {
	projectDir := t.TempDir()
	barBaseDir := filepath.Join(t.TempDir(), "bar-repo")

	badInput := testProductInput(t, projectDir, "foo", "")
	badInput.ProductTaskOutputInfo.Product.PublishOutputInfo = nil // no group ID
	goodInput := testProductInput(t, projectDir, "bar", barBaseDir)

	publisher := mavenlocal.PublisherCreator().Publisher()
	err := publisher.RunPublish([]distgo.ProductPublishInfo{badInput, goodInput}, nil, false, io.Discard)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "foo")

	assert.NoFileExists(t, filepath.Join(barBaseDir, "com/test/group/bar/1.0.0/bar-1.0.0-linux-amd64.tgz"))
}

func testProductInput(t *testing.T, projectDir, productID, baseDir string) distgo.ProductPublishInfo {
	artifactName := fmt.Sprintf("%s-1.0.0-linux-amd64.tgz", productID)
	artifactPath := filepath.Join(projectDir, "out", "dist", productID, "1.0.0", "os-arch-bin", artifactName)
	require.NoError(t, os.MkdirAll(filepath.Dir(artifactPath), 0755))
	require.NoError(t, os.WriteFile(artifactPath, []byte("fake tgz content for "+productID), 0644))

	return distgo.ProductPublishInfo{
		ProductTaskOutputInfo: distgo.ProductTaskOutputInfo{
			Project: distgo.ProjectInfo{
				ProjectDir: projectDir,
				Version:    "1.0.0",
			},
			Product: distgo.ProductOutputInfo{
				ID:                distgo.ProductID(productID),
				Name:              productID,
				PublishOutputInfo: &distgo.PublishOutputInfo{GroupID: "com.test.group"},
				DistOutputInfos: &distgo.DistOutputInfos{
					DistOutputDir: "out/dist",
					DistIDs:       []distgo.DistID{"os-arch-bin"},
					DistInfos: map[distgo.DistID]distgo.DistOutputInfo{
						"os-arch-bin": {
							DistNameTemplateRendered: fmt.Sprintf("%s-1.0.0", productID),
							DistArtifactNames:        []string{artifactName},
							PackagingExtension:       "tgz",
						},
					},
				},
			},
		},
		PublisherConfigYML: fmt.Appendf(nil, "base-dir: %s\nno-pom: true\n", baseDir),
	}
}
