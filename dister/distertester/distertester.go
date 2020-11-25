// Copyright 2016 Palantir Technologies, Inc.
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

package distertester

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"testing"

	"github.com/nmiyake/pkg/dirs"
	"github.com/nmiyake/pkg/gofiles"
	"github.com/palantir/distgo/distgo"
	distgoconfig "github.com/palantir/distgo/distgo/config"
	"github.com/palantir/godel/v2/framework/pluginapitester"
	"github.com/palantir/godel/v2/pkg/osarch"
	"github.com/palantir/pkg/gittest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

type TestCase struct {
	Name        string
	Specs       []gofiles.GoFileSpec
	ConfigFiles map[string]string
	WantError   bool
	WantOutput  func(projectDir string) string
	Validate    func(projectDir string)
}

var builtinSpecs = []gofiles.GoFileSpec{
	{
		RelPath: "godelw",
		Src:     `// placeholder`,
	},
	{
		RelPath: ".gitignore",
		Src: `/out
`,
	},
}

// RunAssetDistTest tests the "dist" operation using the provided asset. Uses the provided plugin provider and asset
// provider to resolve the plugin and asset and invokes the "dist" command.
func RunAssetDistTest(t *testing.T,
	pluginProvider pluginapitester.PluginProvider,
	assetProvider pluginapitester.AssetProvider,
	testCases []TestCase,
) {
	wd, err := os.Getwd()
	require.NoError(t, err)

	tmpDir, cleanup, err := dirs.TempDir("", "")
	require.NoError(t, err)
	if !filepath.IsAbs(tmpDir) {
		tmpDir = path.Join(wd, tmpDir)
	}
	defer cleanup()

	tmpDir, err = filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	for i, tc := range testCases {
		projectDir, err := ioutil.TempDir(tmpDir, "")
		require.NoError(t, err)

		gittest.InitGitDir(t, projectDir)
		require.NoError(t, err)

		var sortedKeys []string
		for k := range tc.ConfigFiles {
			sortedKeys = append(sortedKeys, k)
		}
		sort.Strings(sortedKeys)

		for _, k := range sortedKeys {
			err = os.MkdirAll(path.Dir(path.Join(projectDir, k)), 0755)
			require.NoError(t, err)
			err = ioutil.WriteFile(path.Join(projectDir, k), []byte(tc.ConfigFiles[k]), 0644)
			require.NoError(t, err)
		}

		// write files required for test framework
		_, err = gofiles.Write(projectDir, builtinSpecs)
		require.NoError(t, err)
		// write provided specs
		_, err = gofiles.Write(projectDir, tc.Specs)
		require.NoError(t, err)

		// commit all files and tag project as v1.0.0
		gittest.CommitAllFiles(t, projectDir, "Commit all files")
		gittest.CreateGitTag(t, projectDir, "v1.0.0")

		outputBuf := &bytes.Buffer{}
		func() {
			wantWd := projectDir
			err = os.Chdir(wantWd)
			require.NoError(t, err)
			defer func() {
				err = os.Chdir(wd)
				require.NoError(t, err)
			}()

			var assetProviders []pluginapitester.AssetProvider
			if assetProvider != nil {
				assetProviders = append(assetProviders, assetProvider)
			}

			// run build task first
			func() {
				runPluginCleanup, err := pluginapitester.RunPlugin(
					pluginProvider,
					assetProviders,
					"build", nil,
					projectDir, false, outputBuf)
				defer runPluginCleanup()
				require.NoError(t, err, "Case %d: %s\nBuild operation failed with output:\n%s", i, tc.Name, outputBuf.String())
				outputBuf = &bytes.Buffer{}
			}()

			runPluginCleanup, err := pluginapitester.RunPlugin(
				pluginProvider,
				assetProviders,
				"dist", nil,
				projectDir, false, outputBuf)
			defer runPluginCleanup()
			if tc.WantError {
				require.EqualError(t, err, "", "Case %d: %s", i, tc.Name)
			} else {
				require.NoError(t, err, "Case %d: %s\nOutput:\n%s", i, tc.Name, outputBuf.String())
			}
			if tc.WantOutput != nil {
				assert.Equal(t, tc.WantOutput(projectDir), outputBuf.String(), "Case %d: %s", i, tc.Name)
			}
			if tc.Validate != nil {
				tc.Validate(projectDir)
			}
		}()
	}
}

// RunRepeatedDistTest verifies that running the "dist" task multiple times with
// the provided DistersConfig will succeed without error.
// This test generates a single build artifact and runs the "dist" task in a way that ignores the build cache
// to verify the behavior of strictly running "dist" multiple times.
func RunRepeatedDistTest(t *testing.T,
	pluginProvider pluginapitester.PluginProvider,
	assetProvider pluginapitester.AssetProvider,
	distersCfg distgoconfig.DistersConfig,
) {
	const productName = "dist-overwrite-test-product"
	osarches := []osarch.OSArch{osarch.Current()}

	projectCfg := distgoconfig.ProjectConfig{
		Products: distgoconfig.ToProductsMap(map[distgo.ProductID]distgoconfig.ProductConfig{
			productName: {
				Build: distgoconfig.ToBuildConfig(&distgoconfig.BuildConfig{
					OSArchs: &osarches,
				}),
				Dist: distgoconfig.ToDistConfig(&distgoconfig.DistConfig{
					Disters: distgoconfig.ToDistersConfig(&distersCfg),
				}),
				Publish: distgoconfig.ToPublishConfig(&distgoconfig.PublishConfig{
					GroupID:  stringPtr("com.palantir.test"),
				}),
			},
		}),
	}
	projectCfgBytes, err := yaml.Marshal(projectCfg)
	require.NoError(t, err)
	internalFiles := map[string][]byte{
		"godel/config/dist-plugin.yml": projectCfgBytes,
		"main.go":                      []byte(`package main; func main() {}`),
		"go.mod":                       []byte(fmt.Sprintf("module %s\ngo 1.15", productName)),
	}
	wd, err := os.Getwd()
	require.NoError(t, err)

	tmpDir, cleanup, err := dirs.TempDir("", "")
	require.NoError(t, err)
	if !filepath.IsAbs(tmpDir) {
		tmpDir = path.Join(wd, tmpDir)
	}
	defer cleanup()

	tmpDir, err = filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	projectDir, err := ioutil.TempDir(tmpDir, "")
	require.NoError(t, err)

	for filename, contents := range internalFiles {
		err = os.MkdirAll(path.Dir(path.Join(projectDir, filename)), 0755)
		require.NoError(t, err)
		err = ioutil.WriteFile(path.Join(projectDir, filename), contents, 0644)
		require.NoError(t, err)
	}
	// write files required for test framework
	_, err = gofiles.Write(projectDir, builtinSpecs)
	require.NoError(t, err)

	wantWd := projectDir
	err = os.Chdir(wantWd)
	require.NoError(t, err)
	defer func() {
		err = os.Chdir(wd)
		require.NoError(t, err)
	}()

	var assetProviders []pluginapitester.AssetProvider
	if assetProvider != nil {
		assetProviders = append(assetProviders, assetProvider)
	}

	// run build task once
	buildBuf := new(bytes.Buffer)
	_, err = pluginapitester.RunPlugin(
		pluginProvider,
		assetProviders,
		"build", nil,
		projectDir, false, buildBuf)
	require.NoError(t, err, buildBuf.String())

	for j := 0; j < 2; j++ {
		// run dist task twice and ensure no errors
		distBuf := new(bytes.Buffer)
		_, err = pluginapitester.RunPlugin(
			pluginProvider,
			assetProviders,
			"dist", []string{"--force"}, // use --force to skip build caching
			projectDir, false, distBuf)
		require.NoError(t, err, distBuf.String())
	}
}

func stringPtr(s string) *string {
	return &s
}