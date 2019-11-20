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

package dist_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"testing"

	"github.com/nmiyake/pkg/dirs"
	"github.com/nmiyake/pkg/gofiles"
	"github.com/palantir/distgo/dister/disterfactory"
	"github.com/palantir/distgo/dister/osarchbin"
	"github.com/palantir/distgo/distgo"
	distgoconfig "github.com/palantir/distgo/distgo/config"
	"github.com/palantir/distgo/distgo/dist"
	"github.com/palantir/distgo/distgo/testfuncs"
	"github.com/palantir/godel/v2/pkg/osarch"
	"github.com/palantir/pkg/gittest"
	"github.com/palantir/pkg/matcher"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testMain = `package main

import "fmt"

var testVersionVar = "defaultVersion"

func main() {
	fmt.Println(testVersionVar)
}
`
)

func TestDist(t *testing.T) {
	tmp, cleanup, err := dirs.TempDir("", "")
	defer cleanup()
	require.NoError(t, err)

	defaultDisterCfg, err := disterfactory.DefaultConfig()
	require.NoError(t, err)

	for i, tc := range []struct {
		name            string
		projectCfg      distgoconfig.ProjectConfig
		preDistAction   func(projectDir string, projectCfg distgoconfig.ProjectConfig)
		productDistIDs  []distgo.ProductDistID
		wantErrorRegexp string
		validate        func(caseNum int, name, projectDir string)
	}{
		{
			name: "default dist is os-arch-bin",
			preDistAction: func(projectDir string, projectCfg distgoconfig.ProjectConfig) {
				gittest.CreateGitTag(t, projectDir, "0.1.0")
			},
			validate: func(caseNum int, name, projectDir string) {
				info, err := os.Stat(path.Join(projectDir, "out", "dist", "foo", "0.1.0", "os-arch-bin", fmt.Sprintf("foo-0.1.0-%s.tgz", osarch.Current().String())))
				require.NoError(t, err)
				assert.False(t, info.IsDir(), "Case %d: %s", caseNum, name)
			},
		},
		{
			name: "runs custom dist script",
			projectCfg: distgoconfig.ProjectConfig{
				ProductDefaults: *distgoconfig.ToProductConfig(&distgoconfig.ProductConfig{
					Dist: distgoconfig.ToDistConfig(&distgoconfig.DistConfig{
						Disters: distgoconfig.ToDistersConfig(&distgoconfig.DistersConfig{
							osarchbin.TypeName: {
								Type:   defaultDisterCfg.Type,
								Config: defaultDisterCfg.Config,
								Script: stringPtr(`#!/usr/bin/env bash
touch $DIST_DIR/test-file.txt`),
							},
						}),
					}),
				}),
			},
			preDistAction: func(projectDir string, projectCfg distgoconfig.ProjectConfig) {
				gittest.CreateGitTag(t, projectDir, "0.1.0")
			},
			validate: func(caseNum int, name, projectDir string) {
				info, err := os.Stat(path.Join(projectDir, "out", "dist", "foo", "0.1.0", "os-arch-bin", "test-file.txt"))
				require.NoError(t, err)
				assert.False(t, info.IsDir(), "Case %d: %s", caseNum, name)
			},
		},
		{
			name: "custom dist script inherits process environment variables",
			projectCfg: distgoconfig.ProjectConfig{
				ProductDefaults: *distgoconfig.ToProductConfig(&distgoconfig.ProductConfig{
					Dist: distgoconfig.ToDistConfig(&distgoconfig.DistConfig{
						Disters: distgoconfig.ToDistersConfig(&distgoconfig.DistersConfig{
							osarchbin.TypeName: {
								Type:   defaultDisterCfg.Type,
								Config: defaultDisterCfg.Config,
								Script: stringPtr(`#!/usr/bin/env bash
touch $DIST_DIR/$DIST_TEST_KEY.txt`),
							},
						}),
					}),
				}),
			},
			preDistAction: func(projectDir string, projectCfg distgoconfig.ProjectConfig) {
				gittest.CreateGitTag(t, projectDir, "0.1.0")
				err := os.Setenv("DIST_TEST_KEY", "distTestVal")
				require.NoError(t, err)
			},
			validate: func(caseNum int, name, projectDir string) {
				info, err := os.Stat(path.Join(projectDir, "out", "dist", "foo", "0.1.0", "os-arch-bin", "distTestVal.txt"))
				require.NoError(t, err)
				assert.False(t, info.IsDir(), "Case %d: %s", caseNum, name)
				err = os.Unsetenv("DIST_TEST_KEY")
				require.NoError(t, err)
			},
		},
		{
			name: "custom dist script uses script includes",
			projectCfg: distgoconfig.ProjectConfig{
				ScriptIncludes: `touch $DIST_DIR/foo.txt
helper_func() {
	touch $DIST_DIR/baz.txt
}`,
				ProductDefaults: *distgoconfig.ToProductConfig(&distgoconfig.ProductConfig{
					Dist: distgoconfig.ToDistConfig(&distgoconfig.DistConfig{
						Disters: distgoconfig.ToDistersConfig(&distgoconfig.DistersConfig{
							osarchbin.TypeName: {
								Type:   defaultDisterCfg.Type,
								Config: defaultDisterCfg.Config,
								Script: stringPtr(`#!/usr/bin/env bash
touch $DIST_DIR/$VERSION
helper_func`),
							},
						}),
					}),
				}),
			},
			preDistAction: func(projectDir string, projectCfg distgoconfig.ProjectConfig) {
				gittest.CreateGitTag(t, projectDir, "0.1.0")
			},
			validate: func(caseNum int, name, projectDir string) {
				info, err := os.Stat(path.Join(projectDir, "out", "dist", "foo", "0.1.0", "os-arch-bin", "foo.txt"))
				require.NoError(t, err)
				assert.False(t, info.IsDir(), "Case %d: %s", caseNum, name)

				info, err = os.Stat(path.Join(projectDir, "out", "dist", "foo", "0.1.0", "os-arch-bin", "baz.txt"))
				require.NoError(t, err)
				assert.False(t, info.IsDir(), "Case %d: %s", caseNum, name)

				info, err = os.Stat(path.Join(projectDir, "out", "dist", "foo", "0.1.0", "os-arch-bin", "0.1.0"))
				require.NoError(t, err)
				assert.False(t, info.IsDir(), "Case %d: %s", caseNum, name)
			},
		},
		{
			name: "script includes not executed if custom script not specified",
			projectCfg: distgoconfig.ProjectConfig{
				ScriptIncludes: `touch $DIST_DIR/foo.txt
helper_func() {
	touch $DIST_DIR/baz.txt
}`,
			},
			preDistAction: func(projectDir string, projectCfg distgoconfig.ProjectConfig) {
				gittest.CreateGitTag(t, projectDir, "0.1.0")
			},
			validate: func(caseNum int, name, projectDir string) {
				_, err := os.Stat(path.Join(projectDir, "out", "dist", "foo", "0.1.0", "foo.txt"))
				assert.True(t, os.IsNotExist(err), "Case %d: %s", caseNum, name)
			},
		},
		{
			name: "dependent products and dists are available",
			projectCfg: distgoconfig.ProjectConfig{
				Products: distgoconfig.ToProductsMap(map[distgo.ProductID]distgoconfig.ProductConfig{
					"foo": {
						Build: distgoconfig.ToBuildConfig(&distgoconfig.BuildConfig{
							MainPkg: stringPtr("foo"),
						}),
						Dist: distgoconfig.ToDistConfig(&distgoconfig.DistConfig{
							Disters: distgoconfig.ToDistersConfig(&distgoconfig.DistersConfig{
								osarchbin.TypeName: {
									Type:   defaultDisterCfg.Type,
									Config: defaultDisterCfg.Config,
									Script: stringPtr(`#!/usr/bin/env bash
echo $DEP_PRODUCT_ID_COUNT $DEP_PRODUCT_ID_0 > $DIST_DIR/dep-product-ids.txt
echo $DEP_PRODUCT_ID_0_BUILD_DIR > $DIST_DIR/bar-build-dir.txt
echo $DEP_PRODUCT_ID_0_DIST_ID_0_DIST_DIR > $DIST_DIR/bar-dist-dir.txt
echo $DEP_PRODUCT_ID_0_DIST_ID_0_DIST_ARTIFACT_0 > $DIST_DIR/bar-dist-artifacts.txt
`),
								},
							}),
						}),
						Dependencies: &[]distgo.ProductID{
							"bar",
						},
					},
					"bar": {
						Build: distgoconfig.ToBuildConfig(&distgoconfig.BuildConfig{
							MainPkg: stringPtr("bar"),
						}),
						Dist: distgoconfig.ToDistConfig(&distgoconfig.DistConfig{
							Disters: distgoconfig.ToDistersConfig(&distgoconfig.DistersConfig{
								osarchbin.TypeName: {
									Type:   defaultDisterCfg.Type,
									Config: defaultDisterCfg.Config,
								},
							}),
						}),
					},
				}),
			},
			preDistAction: func(projectDir string, projectCfg distgoconfig.ProjectConfig) {
				_, err := gofiles.Write(projectDir, []gofiles.GoFileSpec{
					{
						RelPath: "bar/main.go",
						Src: `package main

func main() {}
`,
					},
				})
				require.NoError(t, err)
				gittest.CommitAllFiles(t, projectDir, "Add bar")
				gittest.CreateGitTag(t, projectDir, "0.1.0")
			},
			validate: func(caseNum int, name, projectDir string) {
				bytes, err := ioutil.ReadFile(path.Join(projectDir, "out", "dist", "foo", "0.1.0", "os-arch-bin", "dep-product-ids.txt"))
				assert.NoError(t, err, "Case %d: %s", caseNum, name)
				assert.Equal(t, "1 bar\n", string(bytes), "Case %d: %s", caseNum, name)

				bytes, err = ioutil.ReadFile(path.Join(projectDir, "out", "dist", "foo", "0.1.0", "os-arch-bin", "bar-build-dir.txt"))
				assert.NoError(t, err, "Case %d: %s", caseNum, name)
				assert.Equal(t, fmt.Sprintf("%s\n", path.Join(projectDir, "out", "build", "bar", "0.1.0")), string(bytes), "Case %d: %s", caseNum, name)

				bytes, err = ioutil.ReadFile(path.Join(projectDir, "out", "dist", "foo", "0.1.0", "os-arch-bin", "bar-dist-artifacts.txt"))
				assert.NoError(t, err, "Case %d: %s", caseNum, name)
				assert.Equal(t, fmt.Sprintf("bar-0.1.0-%v.tgz\n", osarch.Current()), string(bytes), "Case %d: %s", caseNum, name)

				bytes, err = ioutil.ReadFile(path.Join(projectDir, "out", "dist", "foo", "0.1.0", "os-arch-bin", "bar-dist-dir.txt"))
				assert.NoError(t, err, "Case %d: %s", caseNum, name)
				assert.Equal(t, fmt.Sprintf("%s\n", path.Join(projectDir, "out", "dist", "bar", "0.1.0", "os-arch-bin")), string(bytes), "Case %d: %s", caseNum, name)
			},
		},
		{
			name: "dependent products and are filtered properly",
			projectCfg: distgoconfig.ProjectConfig{
				Products: distgoconfig.ToProductsMap(map[distgo.ProductID]distgoconfig.ProductConfig{
					"product-1": {
						Build: distgoconfig.ToBuildConfig(&distgoconfig.BuildConfig{
							MainPkg: stringPtr("foo"),
						}),
						Dist: distgoconfig.ToDistConfig(&distgoconfig.DistConfig{
							Disters: distgoconfig.ToDistersConfig(&distgoconfig.DistersConfig{
								"dister-1-1": {
									Type:   defaultDisterCfg.Type,
									Config: defaultDisterCfg.Config,
								},
								"dister-1-2": {
									Type:   defaultDisterCfg.Type,
									Config: defaultDisterCfg.Config,
								},
							}),
						}),
						Dependencies: &[]distgo.ProductID{
							"product-2",
						},
					},
					"product-2": {
						Build: distgoconfig.ToBuildConfig(&distgoconfig.BuildConfig{
							MainPkg: stringPtr("foo"),
						}),
						Dist: distgoconfig.ToDistConfig(&distgoconfig.DistConfig{
							Disters: distgoconfig.ToDistersConfig(&distgoconfig.DistersConfig{
								"dister-2-1": {
									Type:   defaultDisterCfg.Type,
									Config: defaultDisterCfg.Config,
								},
								"dister-2-2": {
									Type:   defaultDisterCfg.Type,
									Config: defaultDisterCfg.Config,
								},
							}),
						}),
					},
				}),
			},
			preDistAction: func(projectDir string, projectCfg distgoconfig.ProjectConfig) {
				gittest.CreateGitTag(t, projectDir, "0.1.0")
			},
			productDistIDs: []distgo.ProductDistID{
				"product-1.dister-1-1",
				"product-2.dister-2-1",
			},
			validate: func(caseNum int, name, projectDir string) {
				_, err := os.Stat(path.Join(projectDir, "out", "dist", "product-1", "0.1.0", "dister-1-1", fmt.Sprintf("product-1-0.1.0-%v.tgz", osarch.Current())))
				assert.NoError(t, err, "Case %d: %s", caseNum, name)
				_, err = os.Stat(path.Join(projectDir, "out", "dist", "product-1", "0.1.0", "dister-1-2", fmt.Sprintf("product-1-0.1.0-%v.tgz", osarch.Current())))
				assert.True(t, os.IsNotExist(err), "Case %d: %s", caseNum, name)

				_, err = os.Stat(path.Join(projectDir, "out", "dist", "product-2", "0.1.0", "dister-2-1", fmt.Sprintf("product-2-0.1.0-%v.tgz", osarch.Current())))
				assert.NoError(t, err, "Case %d: %s", caseNum, name)
				_, err = os.Stat(path.Join(projectDir, "out", "dist", "product-2", "0.1.0", "dister-2-2", fmt.Sprintf("product-2-0.1.0-%v.tgz", osarch.Current())))
				assert.NoError(t, err, "Case %d: %s", caseNum, name)
			},
		},
		{
			name: "input-dir files and directories copied",
			projectCfg: distgoconfig.ProjectConfig{
				ProductDefaults: *distgoconfig.ToProductConfig(&distgoconfig.ProductConfig{
					Dist: distgoconfig.ToDistConfig(&distgoconfig.DistConfig{
						Disters: distgoconfig.ToDistersConfig(&distgoconfig.DistersConfig{
							osarchbin.TypeName: {
								Type:   defaultDisterCfg.Type,
								Config: defaultDisterCfg.Config,
								InputDir: distgoconfig.ToInputDirConfig(&distgoconfig.InputDirConfig{
									Path: "input-dir",
								}),
							},
						}),
					}),
				}),
			},
			preDistAction: func(projectDir string, projectCfg distgoconfig.ProjectConfig) {
				inputFile := path.Join(projectDir, "input-dir", "bar.txt")
				err = os.MkdirAll(path.Dir(inputFile), 0755)
				require.NoError(t, err)
				err = ioutil.WriteFile(inputFile, []byte("bar\n"), 0644)
				require.NoError(t, err)

				inputFile = path.Join(projectDir, "input-dir", "foo-dir", "foo.txt")
				err := os.MkdirAll(path.Dir(inputFile), 0755)
				require.NoError(t, err)
				err = ioutil.WriteFile(inputFile, []byte("foo\n"), 0644)
				require.NoError(t, err)

				gittest.CommitAllFiles(t, projectDir, "Commit input directory")
				gittest.CreateGitTag(t, projectDir, "0.1.0")
			},
			validate: func(caseNum int, name, projectDir string) {
				info, err := os.Stat(path.Join(projectDir, "out", "dist", "foo", "0.1.0", "os-arch-bin", "foo-0.1.0", "bar.txt"))
				require.NoError(t, err)
				assert.False(t, info.IsDir(), "Case %d: %s", caseNum, name)

				info, err = os.Stat(path.Join(projectDir, "out", "dist", "foo", "0.1.0", "os-arch-bin", "foo-0.1.0", "foo-dir", "foo.txt"))
				require.NoError(t, err)
				assert.False(t, info.IsDir(), "Case %d: %s", caseNum, name)
			},
		},
		{
			name: "input-dir excludes work",
			projectCfg: distgoconfig.ProjectConfig{
				ProductDefaults: *distgoconfig.ToProductConfig(&distgoconfig.ProductConfig{
					Dist: distgoconfig.ToDistConfig(&distgoconfig.DistConfig{
						Disters: distgoconfig.ToDistersConfig(&distgoconfig.DistersConfig{
							osarchbin.TypeName: {
								Type:   defaultDisterCfg.Type,
								Config: defaultDisterCfg.Config,
								InputDir: distgoconfig.ToInputDirConfig(&distgoconfig.InputDirConfig{
									Path: "input-dir",
									Exclude: matcher.NamesPathsCfg{
										Names: []string{`\.gitkeep`},
										Paths: []string{"bar/foo"},
									},
								}),
							},
						}),
					}),
				}),
			},
			preDistAction: func(projectDir string, projectCfg distgoconfig.ProjectConfig) {
				inputDir := path.Join(projectDir, "input-dir")
				err = os.MkdirAll(path.Dir(inputDir), 0755)
				require.NoError(t, err)

				_, err = gofiles.Write(inputDir, []gofiles.GoFileSpec{
					{
						RelPath: "foo/.gitkeep",
					},
					{
						RelPath: "foo/bar/bar.txt",
					},
					{
						RelPath: "foo/bar/foo/foo.txt",
					},
					{
						RelPath: "bar/foo/foo.txt",
					},
					{
						RelPath: "bar/baz/.gitkeep",
					},
				})
				require.NoError(t, err)

				gittest.CommitAllFiles(t, projectDir, "Commit input directory")
				gittest.CreateGitTag(t, projectDir, "0.1.0")
			},
			validate: func(caseNum int, name, projectDir string) {
				info, err := os.Stat(path.Join(projectDir, "out", "dist", "foo", "0.1.0", "os-arch-bin", "foo-0.1.0", "foo", ".gitkeep"))
				assert.True(t, os.IsNotExist(err), "Case %d: %s", caseNum, name)

				info, err = os.Stat(path.Join(projectDir, "out", "dist", "foo", "0.1.0", "os-arch-bin", "foo-0.1.0", "foo", "bar", "bar.txt"))
				require.NoError(t, err)
				assert.False(t, info.IsDir(), "Case %d: %s", caseNum, name)

				info, err = os.Stat(path.Join(projectDir, "out", "dist", "foo", "0.1.0", "os-arch-bin", "foo-0.1.0", "foo", "bar", "foo", "foo.txt"))
				require.NoError(t, err)
				assert.False(t, info.IsDir(), "Case %d: %s", caseNum, name)

				info, err = os.Stat(path.Join(projectDir, "out", "dist", "foo", "0.1.0", "os-arch-bin", "foo-0.1.0", "bar", "baz", ".gitkeep"))
				assert.True(t, os.IsNotExist(err), "Case %d: %s", caseNum, name)

				info, err = os.Stat(path.Join(projectDir, "out", "dist", "foo", "0.1.0", "os-arch-bin", "foo-0.1.0", "bar", "baz"))
				require.NoError(t, err)
				assert.True(t, info.IsDir(), "Case %d: %s", caseNum, name)
			},
		},
	} {
		projectDir, err := ioutil.TempDir(tmp, "")
		require.NoError(t, err, "Case %d: %s", i, tc.name)

		gittest.InitGitDir(t, projectDir)
		err = os.MkdirAll(path.Join(projectDir, "foo"), 0755)
		require.NoError(t, err, "Case %d: %s", i, tc.name)
		err = ioutil.WriteFile(path.Join(projectDir, "foo", "main.go"), []byte(testMain), 0644)
		require.NoError(t, err, "Case %d: %s", i, tc.name)
		err = ioutil.WriteFile(path.Join(projectDir, "go.mod"), []byte("module foo"), 0644)
		require.NoError(t, err, "Case %d: %s", i, tc.name)
		gittest.CommitAllFiles(t, projectDir, "Commit")

		if tc.preDistAction != nil {
			tc.preDistAction(projectDir, tc.projectCfg)
		}

		projectParam := testfuncs.NewProjectParam(t, tc.projectCfg, projectDir, fmt.Sprintf("Case %d: %s", i, tc.name))
		projectInfo, err := projectParam.ProjectInfo(projectDir)
		require.NoError(t, err, "Case %d: %s", i, tc.name)

		err = dist.Products(projectInfo, projectParam, nil, tc.productDistIDs, false, ioutil.Discard)
		if tc.wantErrorRegexp == "" {
			require.NoError(t, err, "Case %d: %s", i, tc.name)
		} else {
			require.Error(t, err, fmt.Sprintf("Case %d: %s", i, tc.name))
			assert.Regexp(t, regexp.MustCompile(tc.wantErrorRegexp), err.Error(), "Case %d: %s", i, tc.name)
		}

		if tc.validate != nil {
			tc.validate(i, tc.name, projectDir)
		}
	}
}

func stringPtr(in string) *string {
	return &in
}
