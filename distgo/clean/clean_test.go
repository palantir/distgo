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

package clean_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/nmiyake/pkg/dirs"
	"github.com/nmiyake/pkg/gofiles"
	"github.com/palantir/distgo/dister/disterfactory"
	"github.com/palantir/distgo/dister/osarchbin"
	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/distgo/build"
	"github.com/palantir/distgo/distgo/clean"
	distgoconfig "github.com/palantir/distgo/distgo/config"
	"github.com/palantir/distgo/distgo/dist"
	"github.com/palantir/distgo/distgo/testfuncs"
	"github.com/palantir/distgo/internal/files"
	"github.com/palantir/godel/v2/pkg/osarch"
	"github.com/palantir/pkg/gittest"
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

func TestClean(t *testing.T) {
	defaultDisterConfig, err := disterfactory.DefaultConfig()
	require.NoError(t, err)

	tmp, cleanup, err := dirs.TempDir("", "")
	defer cleanup()
	require.NoError(t, err)

	for i, tc := range []struct {
		name          string
		projectConfig distgoconfig.ProjectConfig
		preAction     func(projectDir string)
		action        func(projectInfo distgo.ProjectInfo, projectParam distgo.ProjectParam)
		validate      func(caseNum int, name string, projectInfo distgo.ProjectInfo, projectParam distgo.ProjectParam)
	}{
		{
			"cleans build output",
			distgoconfig.ProjectConfig{
				Products: distgoconfig.ToProductsMap(map[distgo.ProductID]distgoconfig.ProductConfig{
					"foo": {
						Build: distgoconfig.ToBuildConfig(&distgoconfig.BuildConfig{
							MainPkg: stringPtr("foo"),
						}),
					},
				}),
			},
			func(projectDir string) {
				err := files.WriteGoFiles(projectDir, []gofiles.GoFileSpec{
					{
						RelPath: "go.mod",
						Src:     `module foo`,
					},
					{
						RelPath: "foo/main.go",
						Src:     "package main; func main(){}",
					},
				})
				require.NoError(t, err)
				gittest.CommitAllFiles(t, projectDir, "Add foo")

				gittest.CreateGitTag(t, projectDir, "0.1.0")
			},
			func(projectInfo distgo.ProjectInfo, projectParam distgo.ProjectParam) {
				err := build.Products(projectInfo, projectParam, nil, build.Options{}, ioutil.Discard)
				require.NoError(t, err)

				productTaskOutputInfo, err := distgo.ToProductTaskOutputInfo(projectInfo, projectParam.Products["foo"])
				require.NoError(t, err)

				buildOutput := path.Join(productTaskOutputInfo.ProductBuildOutputDir(), osarch.Current().String(), productTaskOutputInfo.Product.BuildOutputInfo.BuildNameTemplateRendered)
				_, err = os.Stat(buildOutput)
				require.NoError(t, err, "expected build output to exist at %s", buildOutput)
			},
			func(caseNum int, name string, projectInfo distgo.ProjectInfo, projectParam distgo.ProjectParam) {
				productTaskOutputInfo, err := distgo.ToProductTaskOutputInfo(projectInfo, projectParam.Products["foo"])
				require.NoError(t, err)

				buildOutput := path.Join(productTaskOutputInfo.ProductBuildOutputDir(), osarch.Current().String(), productTaskOutputInfo.Product.BuildOutputInfo.BuildNameTemplateRendered)
				_, err = os.Stat(buildOutput)
				assert.True(t, os.IsNotExist(err))

				outputDir := path.Join(projectInfo.ProjectDir, "out")
				_, err = os.Stat(outputDir)
				assert.True(t, os.IsNotExist(err))
			},
		},
		{
			"cleans build output for multiple versions",
			distgoconfig.ProjectConfig{
				Products: distgoconfig.ToProductsMap(map[distgo.ProductID]distgoconfig.ProductConfig{
					"foo": {
						Build: distgoconfig.ToBuildConfig(&distgoconfig.BuildConfig{
							MainPkg: stringPtr("foo"),
						}),
					},
				}),
			},
			func(projectDir string) {
				err := files.WriteGoFiles(projectDir, []gofiles.GoFileSpec{
					{
						RelPath: "go.mod",
						Src:     `module foo`,
					},
					{
						RelPath: "foo/main.go",
						Src:     "package main; func main(){}",
					},
				})
				require.NoError(t, err)
				gittest.CommitAllFiles(t, projectDir, "Add foo")

				gittest.CreateGitTag(t, projectDir, "0.1.0")
			},
			func(projectInfo distgo.ProjectInfo, projectParam distgo.ProjectParam) {
				err := build.Products(projectInfo, projectParam, nil, build.Options{}, ioutil.Discard)
				require.NoError(t, err)

				productTaskOutputInfo, err := distgo.ToProductTaskOutputInfo(projectInfo, projectParam.Products["foo"])
				require.NoError(t, err)

				buildOutput := path.Join(productTaskOutputInfo.ProductBuildOutputDir(), osarch.Current().String(), productTaskOutputInfo.Product.BuildOutputInfo.BuildNameTemplateRendered)
				_, err = os.Stat(buildOutput)
				require.NoError(t, err, "expected build output to exist at %s", buildOutput)

				projectInfo.Version = "0.1.0-dirty"
				err = build.Products(projectInfo, projectParam, nil, build.Options{}, ioutil.Discard)
				require.NoError(t, err)

				productTaskOutputInfo, err = distgo.ToProductTaskOutputInfo(projectInfo, projectParam.Products["foo"])
				require.NoError(t, err)

				buildOutput = path.Join(productTaskOutputInfo.ProductBuildOutputDir(), osarch.Current().String(), productTaskOutputInfo.Product.BuildOutputInfo.BuildNameTemplateRendered)
				_, err = os.Stat(buildOutput)
				require.NoError(t, err, "expected build output to exist at %s", buildOutput)
			},
			func(caseNum int, name string, projectInfo distgo.ProjectInfo, projectParam distgo.ProjectParam) {
				productTaskOutputInfo, err := distgo.ToProductTaskOutputInfo(projectInfo, projectParam.Products["foo"])
				require.NoError(t, err)

				buildOutput := path.Join(productTaskOutputInfo.ProductBuildOutputDir(), osarch.Current().String(), productTaskOutputInfo.Product.BuildOutputInfo.BuildNameTemplateRendered)
				_, err = os.Stat(buildOutput)
				assert.True(t, os.IsNotExist(err))

				outputDir := path.Join(projectInfo.ProjectDir, "out")
				_, err = os.Stat(outputDir)
				assert.True(t, os.IsNotExist(err))
			},
		},
		{
			"cleans dist output",
			distgoconfig.ProjectConfig{
				Products: distgoconfig.ToProductsMap(map[distgo.ProductID]distgoconfig.ProductConfig{
					"foo": {
						Build: distgoconfig.ToBuildConfig(&distgoconfig.BuildConfig{
							MainPkg: stringPtr("foo"),
						}),
						Dist: distgoconfig.ToDistConfig(&distgoconfig.DistConfig{
							Disters: distgoconfig.ToDistersConfig(&distgoconfig.DistersConfig{
								osarchbin.TypeName: distgoconfig.ToDisterConfig(defaultDisterConfig),
							}),
						}),
					},
				}),
			},
			func(projectDir string) {
				err := files.WriteGoFiles(projectDir, []gofiles.GoFileSpec{
					{
						RelPath: "go.mod",
						Src:     `module foo`,
					},
					{
						RelPath: "foo/main.go",
						Src:     "package main; func main(){}",
					},
				})
				require.NoError(t, err)
				gittest.CommitAllFiles(t, projectDir, "Add foo")

				gittest.CreateGitTag(t, projectDir, "0.1.0")
			},
			func(projectInfo distgo.ProjectInfo, projectParam distgo.ProjectParam) {
				err := dist.Products(projectInfo, projectParam, nil, nil, false, true, ioutil.Discard)
				require.NoError(t, err)

				productTaskOutputInfo, err := distgo.ToProductTaskOutputInfo(projectInfo, projectParam.Products["foo"])
				require.NoError(t, err)

				outputPaths := productTaskOutputInfo.ProductDistArtifactPaths()
				require.Equal(t, 1, len(outputPaths))

				distArtifactPath := outputPaths[productTaskOutputInfo.Product.DistOutputInfos.DistIDs[0]][0]
				_, err = os.Stat(distArtifactPath)
				require.NoError(t, err, "expected dist output to exist at %s", distArtifactPath)
			},
			func(caseNum int, name string, projectInfo distgo.ProjectInfo, projectParam distgo.ProjectParam) {
				productTaskOutputInfo, err := distgo.ToProductTaskOutputInfo(projectInfo, projectParam.Products["foo"])
				require.NoError(t, err)

				outputPaths := productTaskOutputInfo.ProductDistArtifactPaths()
				require.Equal(t, 1, len(outputPaths))

				distArtifactPath := outputPaths[productTaskOutputInfo.Product.DistOutputInfos.DistIDs[0]][0]
				_, err = os.Stat(distArtifactPath)
				assert.True(t, os.IsNotExist(err))

				outputDir := path.Join(projectInfo.ProjectDir, "out")
				_, err = os.Stat(outputDir)
				assert.True(t, os.IsNotExist(err))
			},
		},
		{
			"cleans dist output for multiple versions",
			distgoconfig.ProjectConfig{
				Products: distgoconfig.ToProductsMap(map[distgo.ProductID]distgoconfig.ProductConfig{
					"foo": {
						Build: distgoconfig.ToBuildConfig(&distgoconfig.BuildConfig{
							MainPkg: stringPtr("foo"),
						}),
						Dist: distgoconfig.ToDistConfig(&distgoconfig.DistConfig{
							Disters: distgoconfig.ToDistersConfig(&distgoconfig.DistersConfig{
								osarchbin.TypeName: distgoconfig.ToDisterConfig(defaultDisterConfig),
							}),
						}),
					},
				}),
			},
			func(projectDir string) {
				err := files.WriteGoFiles(projectDir, []gofiles.GoFileSpec{
					{
						RelPath: "go.mod",
						Src:     `module foo`,
					},
					{
						RelPath: "foo/main.go",
						Src:     "package main; func main(){}",
					},
				})
				require.NoError(t, err)
				gittest.CommitAllFiles(t, projectDir, "Add foo")

				gittest.CreateGitTag(t, projectDir, "0.1.0")
			},
			func(projectInfo distgo.ProjectInfo, projectParam distgo.ProjectParam) {
				err := dist.Products(projectInfo, projectParam, nil, nil, false, true, ioutil.Discard)
				require.NoError(t, err)

				productTaskOutputInfo, err := distgo.ToProductTaskOutputInfo(projectInfo, projectParam.Products["foo"])
				require.NoError(t, err)

				outputPaths := productTaskOutputInfo.ProductDistArtifactPaths()
				require.Equal(t, 1, len(outputPaths))

				distArtifactPath := outputPaths[productTaskOutputInfo.Product.DistOutputInfos.DistIDs[0]][0]
				_, err = os.Stat(distArtifactPath)
				require.NoError(t, err, "expected dist output to exist at %s", distArtifactPath)

				projectInfo.Version = "0.1.0-dirty"
				err = dist.Products(projectInfo, projectParam, nil, nil, false, true, ioutil.Discard)
				require.NoError(t, err)

				productTaskOutputInfo, err = distgo.ToProductTaskOutputInfo(projectInfo, projectParam.Products["foo"])
				require.NoError(t, err)

				outputPaths = productTaskOutputInfo.ProductDistArtifactPaths()
				require.Equal(t, 1, len(outputPaths))

				distArtifactPath = outputPaths[productTaskOutputInfo.Product.DistOutputInfos.DistIDs[0]][0]
				_, err = os.Stat(distArtifactPath)
				require.NoError(t, err, "expected dist output to exist at %s", distArtifactPath)
			},
			func(caseNum int, name string, projectInfo distgo.ProjectInfo, projectParam distgo.ProjectParam) {
				productTaskOutputInfo, err := distgo.ToProductTaskOutputInfo(projectInfo, projectParam.Products["foo"])
				require.NoError(t, err)

				outputPaths := productTaskOutputInfo.ProductDistArtifactPaths()
				require.Equal(t, 1, len(outputPaths))

				distArtifactPath := outputPaths[productTaskOutputInfo.Product.DistOutputInfos.DistIDs[0]][0]
				_, err = os.Stat(distArtifactPath)
				assert.True(t, os.IsNotExist(err))

				outputDir := path.Join(projectInfo.ProjectDir, "out")
				_, err = os.Stat(outputDir)
				assert.True(t, os.IsNotExist(err))
			},
		},
		{
			"clean works even if output does not exist",
			distgoconfig.ProjectConfig{
				Products: distgoconfig.ToProductsMap(map[distgo.ProductID]distgoconfig.ProductConfig{
					"foo": {
						Build: distgoconfig.ToBuildConfig(&distgoconfig.BuildConfig{
							MainPkg: stringPtr("foo"),
						}),
						Dist: distgoconfig.ToDistConfig(&distgoconfig.DistConfig{
							Disters: distgoconfig.ToDistersConfig(&distgoconfig.DistersConfig{
								osarchbin.TypeName: distgoconfig.ToDisterConfig(defaultDisterConfig),
							}),
						}),
					},
				}),
			},
			func(projectDir string) {
				err := files.WriteGoFiles(projectDir, []gofiles.GoFileSpec{
					{
						RelPath: "go.mod",
						Src:     `module foo`,
					},
					{
						RelPath: "foo/main.go",
						Src:     "package main; func main(){}",
					},
				})
				require.NoError(t, err)
				gittest.CommitAllFiles(t, projectDir, "Add foo")

				gittest.CreateGitTag(t, projectDir, "0.1.0")
			},
			func(projectInfo distgo.ProjectInfo, projectParam distgo.ProjectParam) {
			},
			func(caseNum int, name string, projectInfo distgo.ProjectInfo, projectParam distgo.ProjectParam) {
				productTaskOutputInfo, err := distgo.ToProductTaskOutputInfo(projectInfo, projectParam.Products["foo"])
				require.NoError(t, err)

				outputPaths := productTaskOutputInfo.ProductDistArtifactPaths()
				require.Equal(t, 1, len(outputPaths))

				distArtifactPath := outputPaths[productTaskOutputInfo.Product.DistOutputInfos.DistIDs[0]][0]
				_, err = os.Stat(distArtifactPath)
				assert.True(t, os.IsNotExist(err))

				outputDir := path.Join(projectInfo.ProjectDir, "out")
				_, err = os.Stat(outputDir)
				assert.True(t, os.IsNotExist(err))
			},
		},
	} {
		projectDir, err := ioutil.TempDir(tmp, "")
		require.NoError(t, err, "Case %d: %s", i, tc.name)

		gittest.InitGitDir(t, projectDir)
		err = ioutil.WriteFile(path.Join(projectDir, "main.go"), []byte(testMain), 0644)
		require.NoError(t, err, "Case %d: %s", i, tc.name)
		gittest.CommitAllFiles(t, projectDir, "Commit")

		tc.preAction(projectDir)

		projectParam := testfuncs.NewProjectParam(t, tc.projectConfig, projectDir, fmt.Sprintf("Case %d: %s", i, tc.name))
		projectInfo, err := projectParam.ProjectInfo(projectDir)
		require.NoError(t, err, "Case %d: %s", i, tc.name)

		tc.action(projectInfo, projectParam)

		err = clean.Products(projectInfo, projectParam, nil, false, ioutil.Discard)
		require.NoError(t, err, "Case %d: %s", i, tc.name)

		tc.validate(i, tc.name, projectInfo, projectParam)
	}
}

func stringPtr(in string) *string {
	return &in
}
