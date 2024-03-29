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

package artifacts_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/nmiyake/pkg/dirs"
	"github.com/nmiyake/pkg/gofiles"
	"github.com/palantir/distgo/dister/disterfactory"
	"github.com/palantir/distgo/dister/osarchbin"
	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/distgo/artifacts"
	"github.com/palantir/distgo/distgo/build"
	distgoconfig "github.com/palantir/distgo/distgo/config"
	"github.com/palantir/distgo/distgo/testfuncs"
	"github.com/palantir/distgo/dockerbuilder/defaultdockerbuilder"
	"github.com/palantir/distgo/dockerbuilder/dockerbuilderfactory"
	"github.com/palantir/distgo/internal/files"
	"github.com/palantir/distgo/projectversioner/projectversionerfactory"
	"github.com/palantir/distgo/publisher/publisherfactory"
	"github.com/palantir/godel/v2/pkg/osarch"
	"github.com/palantir/pkg/gittest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildArtifactsDefaultOutput(t *testing.T) {
	tmpDir, cleanup, err := dirs.TempDir("", "")
	defer cleanup()
	require.NoError(t, err)

	for i, tc := range []struct {
		name            string
		projectConfig   distgoconfig.ProjectConfig
		setupProjectDir func(projectDir string)
		wantAbsFalse    func(projectDir string) string
		wantAbsTrue     func(projectDir string) string
	}{
		{
			"if param is empty, prints main packages in build output directory",
			distgoconfig.ProjectConfig{},
			func(projectDir string) {
				err := files.WriteGoFiles(projectDir, []gofiles.GoFileSpec{
					{
						RelPath: "go.mod",
						Src:     `module foo`,
					},
					{
						RelPath: "main.go",
						Src:     `package main`,
					},
					{
						RelPath: "bar/bar.go",
						Src:     `package bar`,
					},
					{
						RelPath: "foo/foo.go",
						Src:     `package main`,
					},
				})
				require.NoError(t, err)
			},
			func(projectDir string) string {
				return fmt.Sprintf(`out/build/%s/unspecified/%v/%s
out/build/foo/unspecified/%v/foo
`, path.Base(projectDir), osarch.Current(), path.Base(projectDir), osarch.Current())
			},
			func(projectDir string) string {
				return fmt.Sprintf(`%s/out/build/%s/unspecified/%v/%s
%s/out/build/foo/unspecified/%v/foo
`, projectDir, path.Base(projectDir), osarch.Current(), path.Base(projectDir), projectDir, osarch.Current())
			},
		},
		{
			"output directory specified in param is used",
			distgoconfig.ProjectConfig{
				Products: distgoconfig.ToProductsMap(map[distgo.ProductID]distgoconfig.ProductConfig{
					"foo": {
						Build: distgoconfig.ToBuildConfig(&distgoconfig.BuildConfig{
							OutputDir: stringPtr("build-output"),
							OSArchs: &[]osarch.OSArch{
								osarch.Current(),
							},
						}),
					},
				}),
			},
			nil,
			func(projectDir string) string {
				return fmt.Sprintf(`build-output/foo/unspecified/%v/foo
`, osarch.Current())
			},
			func(projectDir string) string {
				return fmt.Sprintf(`%s/build-output/foo/unspecified/%v/foo
`, projectDir, osarch.Current())
			},
		},
	} {
		projectDir, err := ioutil.TempDir(tmpDir, "")
		require.NoError(t, err, "Case %d: %s", i, tc.name)

		gittest.InitGitDir(t, projectDir)
		if tc.setupProjectDir != nil {
			tc.setupProjectDir(projectDir)
		}

		projectParam := testfuncs.NewProjectParam(t, tc.projectConfig, projectDir, fmt.Sprintf("Case %d: %s", i, tc.name))
		projectInfo, err := projectParam.ProjectInfo(projectDir)
		require.NoError(t, err, "Case %d: %s", i, tc.name)

		buf := &bytes.Buffer{}
		err = artifacts.PrintBuildArtifacts(projectInfo, projectParam, nil, false, false, buf)
		require.NoError(t, err, "Case %d: %s", i, tc.name)
		assert.Equal(t, tc.wantAbsFalse(projectDir), buf.String(), "Case %d: %s", i, tc.name)

		buf = &bytes.Buffer{}
		err = artifacts.PrintBuildArtifacts(projectInfo, projectParam, nil, true, false, buf)
		require.NoError(t, err, "Case %d: %s", i, tc.name)
		assert.Equal(t, tc.wantAbsTrue(projectDir), buf.String(), "Case %d: %s", i, tc.name)
	}
}

func TestBuildArtifacts(t *testing.T) {
	tmpDir, cleanup, err := dirs.TempDir("", "")
	defer cleanup()
	require.NoError(t, err)

	for i, tc := range []struct {
		params []distgo.ProductParam
		want   func(projectDir string) map[distgo.ProductID][]string
	}{
		// empty spec
		{
			params: []distgo.ProductParam{},
			want: func(projectDir string) map[distgo.ProductID][]string {
				return map[distgo.ProductID][]string{}
			},
		},
		// returns paths for all OS/arch combinations if requested osArchs is empty
		{
			params: []distgo.ProductParam{
				createBuildSpec("foo", "foo", []osarch.OSArch{
					{OS: "darwin", Arch: "amd64"},
					{OS: "darwin", Arch: "arm64"},
					{OS: "linux", Arch: "amd64"},
				}),
			},
			want: func(projectDir string) map[distgo.ProductID][]string {
				return map[distgo.ProductID][]string{
					"foo": {
						path.Join(projectDir, "out", "build", "foo", "0.1.0", "darwin-amd64", "foo"),
						path.Join(projectDir, "out", "build", "foo", "0.1.0", "darwin-arm64", "foo"),
						path.Join(projectDir, "out", "build", "foo", "0.1.0", "linux-amd64", "foo"),
					},
				}
			},
		},
		// returns paths for all OS/arch combinations if requested osArchs is empty, with name override
		{
			params: []distgo.ProductParam{
				createBuildSpec("foo", "bar", []osarch.OSArch{
					{OS: "darwin", Arch: "amd64"},
					{OS: "darwin", Arch: "arm64"},
					{OS: "linux", Arch: "amd64"},
				}),
			},
			want: func(projectDir string) map[distgo.ProductID][]string {
				return map[distgo.ProductID][]string{
					"foo": {
						path.Join(projectDir, "out", "build", "foo", "0.1.0", "darwin-amd64", "bar"),
						path.Join(projectDir, "out", "build", "foo", "0.1.0", "darwin-arm64", "bar"),
						path.Join(projectDir, "out", "build", "foo", "0.1.0", "linux-amd64", "bar"),
					},
				}
			},
		},
		// path to windows executable includes ".exe"
		{
			params: []distgo.ProductParam{
				createBuildSpec("foo", "foo", []osarch.OSArch{
					{OS: "windows", Arch: "amd64"},
				}),
			},
			want: func(projectDir string) map[distgo.ProductID][]string {
				return map[distgo.ProductID][]string{
					"foo": {
						path.Join(projectDir, "out", "build", "foo", "0.1.0", "windows-amd64", "foo.exe"),
					},
				}
			},
		},
		// path to windows executable includes ".exe", with name override
		{
			params: []distgo.ProductParam{
				createBuildSpec("foo", "bar", []osarch.OSArch{
					{OS: "windows", Arch: "amd64"},
				}),
			},
			want: func(projectDir string) map[distgo.ProductID][]string {
				return map[distgo.ProductID][]string{
					"foo": {
						path.Join(projectDir, "out", "build", "foo", "0.1.0", "windows-amd64", "bar.exe"),
					},
				}
			},
		},
	} {
		currProjectDir, err := ioutil.TempDir(tmpDir, "")
		require.NoError(t, err)

		projectInfo := distgo.ProjectInfo{
			ProjectDir: currProjectDir,
			Version:    "0.1.0",
		}
		got, err := artifacts.Build(projectInfo, tc.params, false)
		require.NoError(t, err, "Case %d", i)
		assert.Equal(t, tc.want(currProjectDir), got, "Case %d", i)
	}
}

func TestBuildArtifactsRequiresBuild(t *testing.T) {
	tmpDir, cleanup, err := dirs.TempDir(".", "")
	require.NoError(t, err)
	defer cleanup()
	err = ioutil.WriteFile(path.Join(tmpDir, ".gitignore"), []byte(`*
*/
`), 0644)
	require.NoError(t, err)

	tmpDir, err = filepath.Abs(tmpDir)
	require.NoError(t, err)

	for i, tc := range []struct {
		params        []distgo.ProductParam
		requiresBuild bool
		beforeAction  func(projectInfo distgo.ProjectInfo, productParams []distgo.ProductParam)
		want          func(projectDir string) map[distgo.ProductID][]string
	}{
		// returns paths to all artifacts if build has not happened
		{
			params: []distgo.ProductParam{
				createBuildSpec("foo", "foo", []osarch.OSArch{
					{OS: "darwin", Arch: "amd64"},
					{OS: "darwin", Arch: "arm64"},
					{OS: "linux", Arch: "amd64"},
				}),
			},
			want: func(projectDir string) map[distgo.ProductID][]string {
				return map[distgo.ProductID][]string{
					"foo": {
						path.Join(projectDir, "out", "build", "foo", "0.1.0", "darwin-amd64", "foo"),
						path.Join(projectDir, "out", "build", "foo", "0.1.0", "darwin-arm64", "foo"),
						path.Join(projectDir, "out", "build", "foo", "0.1.0", "linux-amd64", "foo"),
					},
				}
			},
		},
		// returns paths to all artifacts if build has not happened, no name collision with name override
		{
			params: []distgo.ProductParam{
				createBuildSpec("foo", "foo", []osarch.OSArch{
					{OS: "darwin", Arch: "amd64"},
					{OS: "darwin", Arch: "arm64"},
					{OS: "linux", Arch: "amd64"},
				}),
				createBuildSpec("bar", "foo", []osarch.OSArch{
					{OS: "darwin", Arch: "amd64"},
					{OS: "darwin", Arch: "arm64"},
					{OS: "linux", Arch: "amd64"},
				}),
			},
			want: func(projectDir string) map[distgo.ProductID][]string {
				return map[distgo.ProductID][]string{
					"foo": {
						path.Join(projectDir, "out", "build", "foo", "0.1.0", "darwin-amd64", "foo"),
						path.Join(projectDir, "out", "build", "foo", "0.1.0", "darwin-arm64", "foo"),
						path.Join(projectDir, "out", "build", "foo", "0.1.0", "linux-amd64", "foo"),
					},
					"bar": {
						path.Join(projectDir, "out", "build", "bar", "0.1.0", "darwin-amd64", "foo"),
						path.Join(projectDir, "out", "build", "bar", "0.1.0", "darwin-arm64", "foo"),
						path.Join(projectDir, "out", "build", "bar", "0.1.0", "linux-amd64", "foo"),
					},
				}
			},
		},
		// returns empty if all artifacts exist and are up-to-date
		{
			params: []distgo.ProductParam{
				createBuildSpec("foo", "foo", []osarch.OSArch{
					{OS: "darwin", Arch: "amd64"},
					{OS: "darwin", Arch: "arm64"},
					{OS: "linux", Arch: "amd64"},
				}),
			},
			beforeAction: func(projectInfo distgo.ProjectInfo, productParams []distgo.ProductParam) {
				// build products
				err := build.Run(projectInfo, productParams, build.Options{
					Parallel: false,
				}, ioutil.Discard)
				require.NoError(t, err)
			},
			want: func(projectDir string) map[distgo.ProductID][]string {
				return map[distgo.ProductID][]string{}
			},
		},
		// returns paths to all artifacts if input source file has been modified
		{
			params: []distgo.ProductParam{
				createBuildSpec("foo", "foo", []osarch.OSArch{
					{OS: "darwin", Arch: "amd64"},
					{OS: "darwin", Arch: "arm64"},
					{OS: "linux", Arch: "amd64"},
				}),
			},
			beforeAction: func(projectInfo distgo.ProjectInfo, params []distgo.ProductParam) {
				// build products
				err := build.Run(projectInfo, params, build.Options{
					Parallel: false,
				}, ioutil.Discard)
				require.NoError(t, err)

				// sleep to ensure that modification time will differ
				time.Sleep(time.Second)

				// update source file
				err = ioutil.WriteFile(path.Join(projectInfo.ProjectDir, "main.go"), []byte("package main; func main(){}"), 0644)
				require.NoError(t, err)
			},
			want: func(projectDir string) map[distgo.ProductID][]string {
				return map[distgo.ProductID][]string{
					"foo": {
						path.Join(projectDir, "out", "build", "foo", "0.1.0", "darwin-amd64", "foo"),
						path.Join(projectDir, "out", "build", "foo", "0.1.0", "darwin-arm64", "foo"),
						path.Join(projectDir, "out", "build", "foo", "0.1.0", "linux-amd64", "foo"),
					},
				}
			},
		},
	} {
		currProjectDir, err := ioutil.TempDir(tmpDir, "")
		require.NoError(t, err)

		err = ioutil.WriteFile(path.Join(currProjectDir, "main.go"), []byte("package main; func main(){}"), 0644)
		require.NoError(t, err)

		projectInfo := distgo.ProjectInfo{
			ProjectDir: currProjectDir,
			Version:    "0.1.0",
		}
		if tc.beforeAction != nil {
			tc.beforeAction(projectInfo, tc.params)
		}

		got, err := artifacts.Build(projectInfo, tc.params, true)
		require.NoError(t, err, "Case %d", i)
		assert.Equal(t, tc.want(currProjectDir), got, "Case %d", i)
	}
}

func TestDistArtifacts(t *testing.T) {
	tmpDir, cleanup, err := dirs.TempDir("", "")
	defer cleanup()
	require.NoError(t, err)

	for i, tc := range []struct {
		params []distgo.ProductParam
		want   func(projectDir string) map[distgo.ProductID][]string
	}{
		// empty spec
		{
			params: []distgo.ProductParam{},
			want: func(projectDir string) map[distgo.ProductID][]string {
				return map[distgo.ProductID][]string{}
			},
		},
		// returns dist artifact outputs
		{
			params: []distgo.ProductParam{
				createDistSpec("foo", "foo", osarchbin.New(
					osarch.OSArch{OS: "darwin", Arch: "amd64"},
					osarch.OSArch{OS: "linux", Arch: "amd64"},
				)),
			},
			want: func(projectDir string) map[distgo.ProductID][]string {
				return map[distgo.ProductID][]string{
					"foo": {
						path.Join(projectDir, "out", "dist", "foo", "0.1.0", "os-arch-bin", "foo-0.1.0-darwin-amd64.tgz"),
						path.Join(projectDir, "out", "dist", "foo", "0.1.0", "os-arch-bin", "foo-0.1.0-linux-amd64.tgz"),
					},
				}
			},
		},
		// returns dist artifact outputs with name override
		{
			params: []distgo.ProductParam{
				createDistSpec("foo", "foo", osarchbin.New(
					osarch.OSArch{OS: "darwin", Arch: "amd64"},
					osarch.OSArch{OS: "linux", Arch: "amd64"},
				)),
				createDistSpec("bar", "foo", osarchbin.New(
					osarch.OSArch{OS: "darwin", Arch: "amd64"},
					osarch.OSArch{OS: "linux", Arch: "amd64"},
				)),
			},
			want: func(projectDir string) map[distgo.ProductID][]string {
				return map[distgo.ProductID][]string{
					"foo": {
						path.Join(projectDir, "out", "dist", "foo", "0.1.0", "os-arch-bin", "foo-0.1.0-darwin-amd64.tgz"),
						path.Join(projectDir, "out", "dist", "foo", "0.1.0", "os-arch-bin", "foo-0.1.0-linux-amd64.tgz"),
					},
					"bar": {
						path.Join(projectDir, "out", "dist", "bar", "0.1.0", "os-arch-bin", "foo-0.1.0-darwin-amd64.tgz"),
						path.Join(projectDir, "out", "dist", "bar", "0.1.0", "os-arch-bin", "foo-0.1.0-linux-amd64.tgz"),
					},
				}
			},
		},
	} {
		currProjectDir, err := ioutil.TempDir(tmpDir, "")
		require.NoError(t, err)

		projectInfo := distgo.ProjectInfo{
			ProjectDir: currProjectDir,
			Version:    "0.1.0",
		}
		got, err := artifacts.Dist(projectInfo, tc.params)
		require.NoError(t, err, "Case %d", i)
		assert.Equal(t, tc.want(currProjectDir), got, "Case %d", i)
	}
}

func TestPrintDockerArtifacts(t *testing.T) {
	tmpDir, cleanup, err := dirs.TempDir("", "")
	defer cleanup()
	require.NoError(t, err)

	cfg := distgoconfig.ProjectConfig{
		Products: distgoconfig.ToProductsMap(map[distgo.ProductID]distgoconfig.ProductConfig{
			"foo": {
				Docker: distgoconfig.ToDockerConfig(&distgoconfig.DockerConfig{
					Repository: stringPtr("repo"),
					DockerBuildersConfig: distgoconfig.ToDockerBuildersConfig(&distgoconfig.DockerBuildersConfig{
						"docker-builder-1": distgoconfig.ToDockerBuilderConfig(distgoconfig.DockerBuilderConfig{
							Type:       stringPtr(defaultdockerbuilder.TypeName),
							ContextDir: stringPtr("dockerContextDir"),
							TagTemplates: distgoconfig.ToTagTemplatesMap(mustTagTemplatesMap(
								"latest", "{{Repository}}foo-db-1:latest",
								"release", "{{Repository}}foo-db-1:release",
							)),
						}),
						"docker-builder-2": distgoconfig.ToDockerBuilderConfig(distgoconfig.DockerBuilderConfig{
							Type:       stringPtr(defaultdockerbuilder.TypeName),
							ContextDir: stringPtr("dockerContextDir-2"),
							TagTemplates: distgoconfig.ToTagTemplatesMap(mustTagTemplatesMap(
								"latest", "{{Repository}}foo-db-2:latest",
							)),
						}),
					}),
				}),
			},
			"bar": {
				Docker: distgoconfig.ToDockerConfig(&distgoconfig.DockerConfig{
					Repository: stringPtr("repo"),
					DockerBuildersConfig: distgoconfig.ToDockerBuildersConfig(&distgoconfig.DockerBuildersConfig{
						"docker-builder-1": distgoconfig.ToDockerBuilderConfig(distgoconfig.DockerBuilderConfig{
							Type:       stringPtr(defaultdockerbuilder.TypeName),
							ContextDir: stringPtr("dockerContextDir"),
							TagTemplates: distgoconfig.ToTagTemplatesMap(mustTagTemplatesMap(
								"latest", "{{Repository}}bar-db-1:latest",
								"release", "{{Repository}}bar-db-1:release",
							)),
						}),
					}),
				}),
			},
			"baz": {},
		}),
	}

	projectDir, err := ioutil.TempDir(tmpDir, "")
	require.NoError(t, err)
	gittest.InitGitDir(t, projectDir)
	gittest.CreateGitTag(t, projectDir, "0.1.0")

	projectVersionerFactory, err := projectversionerfactory.New(nil, nil)
	require.NoError(t, err)
	disterFactory, err := disterfactory.New(nil, nil)
	require.NoError(t, err)
	defaultDisterCfg, err := disterfactory.DefaultConfig()
	require.NoError(t, err)
	dockerBuilderFactory, err := dockerbuilderfactory.New(nil, nil)
	require.NoError(t, err)
	publisherFactory, err := publisherfactory.New(nil, nil)
	require.NoError(t, err)

	projectParam, err := cfg.ToParam(projectDir, projectVersionerFactory, disterFactory, defaultDisterCfg, dockerBuilderFactory, publisherFactory)
	require.NoError(t, err)

	projectInfo, err := projectParam.ProjectInfo(projectDir)
	require.NoError(t, err)

	for i, tc := range []struct {
		name             string
		productDockerIDs []distgo.ProductDockerID
		want             string
	}{
		{
			"prints all docker artifacts",
			nil,
			`repo/bar-db-1:latest
repo/bar-db-1:release
repo/foo-db-1:latest
repo/foo-db-1:release
repo/foo-db-2:latest
`,
		},
		{
			"prints docker artifacts for specified product",
			[]distgo.ProductDockerID{
				"foo",
			},
			`repo/foo-db-1:latest
repo/foo-db-1:release
repo/foo-db-2:latest
`,
		},
		{
			"prints docker artifacts for specified docker image",
			[]distgo.ProductDockerID{
				"foo.docker-builder-1",
			},
			`repo/foo-db-1:latest
repo/foo-db-1:release
`,
		},
		{
			"prints docker artifacts for specified tag",
			[]distgo.ProductDockerID{
				"foo.docker-builder-1.latest",
			},
			`repo/foo-db-1:latest
`,
		},
	} {
		buf := &bytes.Buffer{}
		err := artifacts.PrintDockerArtifacts(projectInfo, projectParam, tc.productDockerIDs, buf)
		require.NoError(t, err, "Case %d: %s", i, tc.name)
		assert.Equal(t, tc.want, buf.String(), "Case %d: %s\nGot:\n%s", i, tc.name, buf.String())
	}
}

func TestDockerArtifacts(t *testing.T) {
	tmpDir, cleanup, err := dirs.TempDir("", "")
	defer cleanup()
	require.NoError(t, err)

	for i, tc := range []struct {
		name string
		cfg  distgoconfig.ProjectConfig
		want map[distgo.ProductID][]string
	}{
		{
			"prints docker artifacts",
			distgoconfig.ProjectConfig{
				Products: distgoconfig.ToProductsMap(map[distgo.ProductID]distgoconfig.ProductConfig{
					"foo": {
						Docker: distgoconfig.ToDockerConfig(&distgoconfig.DockerConfig{
							Repository: stringPtr("repo"),
							DockerBuildersConfig: distgoconfig.ToDockerBuildersConfig(&distgoconfig.DockerBuildersConfig{
								defaultdockerbuilder.TypeName: distgoconfig.ToDockerBuilderConfig(distgoconfig.DockerBuilderConfig{
									Type:       stringPtr(defaultdockerbuilder.TypeName),
									ContextDir: stringPtr("dockerContextDir"),
									TagTemplates: distgoconfig.ToTagTemplatesMap(mustTagTemplatesMap(
										"latest", "{{Repository}}foo:latest",
									)),
								}),
							}),
						}),
					},
				}),
			},
			map[distgo.ProductID][]string{
				"foo": {
					"repo/foo:latest",
				},
			},
		},
	} {
		projectDir, err := ioutil.TempDir(tmpDir, "")
		require.NoError(t, err)
		gittest.InitGitDir(t, projectDir)
		gittest.CreateGitTag(t, projectDir, "0.1.0")

		projectVersionerFactory, err := projectversionerfactory.New(nil, nil)
		require.NoError(t, err, "Case %d: %s", i, tc.name)
		disterFactory, err := disterfactory.New(nil, nil)
		require.NoError(t, err, "Case %d: %s", i, tc.name)
		defaultDisterCfg, err := disterfactory.DefaultConfig()
		require.NoError(t, err, "Case %d: %s", i, tc.name)
		dockerBuilderFactory, err := dockerbuilderfactory.New(nil, nil)
		require.NoError(t, err, "Case %d: %s", i, tc.name)
		publisherFactory, err := publisherfactory.New(nil, nil)
		require.NoError(t, err, "Case %d: %s", i, tc.name)

		projectParam, err := tc.cfg.ToParam(projectDir, projectVersionerFactory, disterFactory, defaultDisterCfg, dockerBuilderFactory, publisherFactory)
		require.NoError(t, err, "Case %d: %s", i, tc.name)

		projectInfo, err := projectParam.ProjectInfo(projectDir)
		require.NoError(t, err, "Case %d: %s", i, tc.name)

		products, err := distgo.ProductParamsForProductArgs(projectParam.Products)
		require.NoError(t, err, "Case %d: %s", i, tc.name)

		dockerArtifacts, err := artifacts.Docker(projectInfo, products)
		require.NoError(t, err, "Case %d: %s", i, tc.name)

		assert.Equal(t, tc.want, dockerArtifacts, "Case %d: %s", i, tc.name)
	}
}

func createBuildSpec(productID, productName string, osArchs []osarch.OSArch) distgo.ProductParam {
	return distgo.ProductParam{
		ID:   distgo.ProductID(productID),
		Name: productName,
		Build: &distgo.BuildParam{
			NameTemplate: "{{Product}}",
			OutputDir:    "out/build",
			MainPkg:      ".",
			OSArchs:      osArchs,
		},
	}
}

func createDistSpec(productID, productName string, dister distgo.Dister) distgo.ProductParam {
	disterName, err := dister.TypeName()
	if err != nil {
		panic(err)
	}

	return distgo.ProductParam{
		ID:   distgo.ProductID(productID),
		Name: productName,
		Dist: &distgo.DistParam{
			OutputDir: "out/dist",
			DistParams: map[distgo.DistID]distgo.DisterParam{
				distgo.DistID(disterName): {
					NameTemplate: "{{Product}}-{{Version}}",
					Dister:       dister,
				},
			},
		},
	}
}

func stringPtr(in string) *string {
	return &in
}

func mustTagTemplatesMap(nameAndVal ...string) *distgoconfig.TagTemplatesMap {
	out := &distgoconfig.TagTemplatesMap{
		Templates: make(map[distgo.DockerTagID]string),
	}
	for i := 0; i < len(nameAndVal); i += 2 {
		tagID := distgo.DockerTagID(nameAndVal[i])
		out.Templates[tagID] = nameAndVal[i+1]
		out.OrderedKeys = append(out.OrderedKeys, tagID)
	}
	return out
}
