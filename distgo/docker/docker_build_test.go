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

package docker_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"testing"
	"time"

	"github.com/nmiyake/pkg/dirs"
	"github.com/palantir/distgo/dister/disterfactory"
	"github.com/palantir/distgo/dister/manual"
	"github.com/palantir/distgo/dister/osarchbin"
	"github.com/palantir/distgo/distgo"
	distgoconfig "github.com/palantir/distgo/distgo/config"
	"github.com/palantir/distgo/distgo/dist"
	"github.com/palantir/distgo/distgo/docker"
	"github.com/palantir/distgo/dockerbuilder"
	"github.com/palantir/distgo/dockerbuilder/dockerbuilderfactory"
	"github.com/palantir/distgo/projectversioner/projectversionerfactory"
	"github.com/palantir/distgo/publisher/publisherfactory"
	"github.com/palantir/godel/v2/pkg/osarch"
	"github.com/palantir/pkg/gittest"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

const (
	testMain       = `package main; func main(){}`
	testDockerfile = `FROM alpine:3.5
`
)

const printDockerfileDockerBuilderTypeName = "print-dockerfile"

func newPrintDockerfileBuilder(cfgYML []byte) (distgo.DockerBuilder, error) {
	return &printDockerfileDockerBuilder{}, nil
}

type printDockerfileDockerBuilder struct{}

func (b *printDockerfileDockerBuilder) TypeName() (string, error) {
	return printDockerfileDockerBuilderTypeName, nil
}

func (b *printDockerfileDockerBuilder) RunDockerBuild(dockerID distgo.DockerID, productTaskOutputInfo distgo.ProductTaskOutputInfo, verbose, dryRun bool, stdout io.Writer) error {
	dockerBuilderOutputInfo := productTaskOutputInfo.Product.DockerOutputInfos.DockerBuilderOutputInfos[dockerID]
	fullDockerfilePath := path.Join(productTaskOutputInfo.Project.ProjectDir, dockerBuilderOutputInfo.ContextDir, dockerBuilderOutputInfo.DockerfilePath)
	bytes, err := os.ReadFile(fullDockerfilePath)
	if err != nil {
		return errors.Wrapf(err, "failed to read Dockerfile at %s", fullDockerfilePath)
	}
	_, _ = fmt.Fprint(stdout, string(bytes))
	return nil
}

func TestDockerBuild(t *testing.T) {
	tmp, cleanup, err := dirs.TempDir("", "")
	defer cleanup()
	require.NoError(t, err)

	for i, tc := range []struct {
		name            string
		projectCfg      distgoconfig.ProjectConfig
		tagKeys         []string
		preDockerAction func(t *testing.T, projectDir string, projectCfg distgoconfig.ProjectConfig)
		wantErrorRegexp string
		wantStdout      string
		validate        func(t *testing.T, caseNum int, name, projectDir string)
	}{
		{
			"build and dist output artifacts are hard-linked into context directory",
			distgoconfig.ProjectConfig{
				Products: distgoconfig.ToProductsMap(map[distgo.ProductID]distgoconfig.ProductConfig{
					"foo": {
						Build: distgoconfig.ToBuildConfig(&distgoconfig.BuildConfig{
							MainPkg: stringPtr("./foo"),
						}),
						Dist: distgoconfig.ToDistConfig(&distgoconfig.DistConfig{
							Disters: distgoconfig.ToDistersConfig(&distgoconfig.DistersConfig{
								osarchbin.TypeName: distgoconfig.ToDisterConfig(distgoconfig.DisterConfig{
									Type: stringPtr(osarchbin.TypeName),
								}),
							}),
						}),
						Docker: distgoconfig.ToDockerConfig(&distgoconfig.DockerConfig{
							DockerBuildersConfig: distgoconfig.ToDockerBuildersConfig(&distgoconfig.DockerBuildersConfig{
								printDockerfileDockerBuilderTypeName: distgoconfig.ToDockerBuilderConfig(distgoconfig.DockerBuilderConfig{
									Type:       stringPtr(printDockerfileDockerBuilderTypeName),
									ContextDir: stringPtr("docker-context-dir"),
									InputBuilds: &[]distgo.ProductBuildID{
										"foo",
									},
									InputDists: &[]distgo.ProductDistID{
										"foo",
									},
									TagTemplates: distgoconfig.ToTagTemplatesMap(mustTagTemplatesMap(
										"default", "foo:latest",
									)),
								}),
							}),
						}),
					},
				}),
			},
			nil,
			func(t *testing.T, projectDir string, projectCfg distgoconfig.ProjectConfig) {
				contextDir := path.Join(projectDir, "docker-context-dir")
				err := os.Mkdir(contextDir, 0755)
				require.NoError(t, err)
				dockerfile := path.Join(contextDir, "Dockerfile")
				err = os.WriteFile(dockerfile, []byte(testDockerfile), 0644)
				require.NoError(t, err)
				gittest.CommitAllFiles(t, projectDir, "Commit files")
				gittest.CreateGitTag(t, projectDir, "0.1.0")
			},
			"",
			"",
			func(t *testing.T, caseNum int, name, projectDir string) {
				_, err := os.Stat(path.Join(projectDir, "docker-context-dir", "foo", "build", osarch.Current().String(), "foo"))
				require.NoError(t, err)
				_, err = os.Stat(path.Join(projectDir, "docker-context-dir", "foo", "dist", "os-arch-bin", fmt.Sprintf("foo-0.1.0-%v.tgz", osarch.Current())))
				require.NoError(t, err)
			},
		},
		{
			"dist output artifacts uses override path if specified",
			distgoconfig.ProjectConfig{
				Products: distgoconfig.ToProductsMap(map[distgo.ProductID]distgoconfig.ProductConfig{
					"foo": {
						Build: distgoconfig.ToBuildConfig(&distgoconfig.BuildConfig{
							MainPkg: stringPtr("./foo"),
						}),
						Dist: distgoconfig.ToDistConfig(&distgoconfig.DistConfig{
							Disters: distgoconfig.ToDistersConfig(&distgoconfig.DistersConfig{
								osarchbin.TypeName: distgoconfig.ToDisterConfig(distgoconfig.DisterConfig{
									Type: stringPtr(osarchbin.TypeName),
								}),
							}),
						}),
						Docker: distgoconfig.ToDockerConfig(&distgoconfig.DockerConfig{
							DockerBuildersConfig: distgoconfig.ToDockerBuildersConfig(&distgoconfig.DockerBuildersConfig{
								printDockerfileDockerBuilderTypeName: distgoconfig.ToDockerBuilderConfig(distgoconfig.DockerBuilderConfig{
									Type:       stringPtr(printDockerfileDockerBuilderTypeName),
									ContextDir: stringPtr("docker-context-dir"),
									InputDists: &[]distgo.ProductDistID{
										"foo",
									},
									InputDistsOutputPaths: &map[distgo.ProductDistID][]string{
										"foo.os-arch-bin": {
											"docker/dist-latest.tgz",
										},
									},
									TagTemplates: distgoconfig.ToTagTemplatesMap(mustTagTemplatesMap(
										"default", "foo:latest",
									)),
								}),
							}),
						}),
					},
				}),
			},
			nil,
			func(t *testing.T, projectDir string, projectCfg distgoconfig.ProjectConfig) {
				contextDir := path.Join(projectDir, "docker-context-dir")
				err := os.Mkdir(contextDir, 0755)
				require.NoError(t, err)
				dockerfile := path.Join(contextDir, "Dockerfile")
				err = os.WriteFile(dockerfile, []byte(testDockerfile), 0644)
				require.NoError(t, err)
				gittest.CommitAllFiles(t, projectDir, "Commit files")
				gittest.CreateGitTag(t, projectDir, "0.1.0")
			},
			"",
			"",
			func(t *testing.T, caseNum int, name, projectDir string) {
				_, err := os.Stat(path.Join(projectDir, "docker-context-dir", "foo", "build", osarch.Current().String(), "foo"))
				require.NoError(t, err)
				_, err = os.Stat(path.Join(projectDir, "docker-context-dir", "docker", "dist-latest.tgz"))
				require.NoError(t, err)
			},
		},
		{
			"Dockerfile renders template variables",
			distgoconfig.ProjectConfig{
				Products: distgoconfig.ToProductsMap(map[distgo.ProductID]distgoconfig.ProductConfig{
					"foo": {
						Build: distgoconfig.ToBuildConfig(&distgoconfig.BuildConfig{
							MainPkg: stringPtr("./foo"),
						}),
						Dist: distgoconfig.ToDistConfig(&distgoconfig.DistConfig{
							Disters: distgoconfig.ToDistersConfig(&distgoconfig.DistersConfig{
								osarchbin.TypeName: distgoconfig.ToDisterConfig(distgoconfig.DisterConfig{
									Type: stringPtr(osarchbin.TypeName),
								}),
							}),
						}),
						Docker: distgoconfig.ToDockerConfig(&distgoconfig.DockerConfig{
							Repository: stringPtr("registry-host:5000"),
							DockerBuildersConfig: distgoconfig.ToDockerBuildersConfig(&distgoconfig.DockerBuildersConfig{
								printDockerfileDockerBuilderTypeName: distgoconfig.ToDockerBuilderConfig(distgoconfig.DockerBuilderConfig{
									Type:             stringPtr(printDockerfileDockerBuilderTypeName),
									ContextDir:       stringPtr("docker-context-dir"),
									InputProductsDir: stringPtr("input-products"),
									InputBuilds: &[]distgo.ProductBuildID{
										"foo",
									},
									InputDists: &[]distgo.ProductDistID{
										"foo",
									},
									TagTemplates: distgoconfig.ToTagTemplatesMap(mustTagTemplatesMap(
										"default", "{{Repository}}foo:latest",
									)),
								}),
							}),
						}),
					},
				}),
			},
			nil,
			func(t *testing.T, projectDir string, projectCfg distgoconfig.ProjectConfig) {
				contextDir := path.Join(projectDir, "docker-context-dir")
				err := os.Mkdir(contextDir, 0755)
				require.NoError(t, err)

				dockerfile := path.Join(contextDir, "Dockerfile")
				err = os.WriteFile(dockerfile, []byte(fmt.Sprintf(`FROM alpine:3.5
RUN echo 'Product: {{Product}}'
RUN echo 'Version: {{Version}}'
RUN echo 'Repository: {{Repository}}'
RUN echo 'RepositoryLiteral: {{RepositoryLiteral}}'
RUN echo 'InputBuildArtifact for foo: {{InputBuildArtifact "foo" %q}}'
RUN echo 'InputDistArtifacts for foo: {{InputDistArtifacts "foo" "os-arch-bin"}}'
RUN echo 'Tag for foo: {{Tag "foo" "print-dockerfile" "default"}}'
RUN echo 'Tags for foo: {{Tags "foo" "print-dockerfile"}}'
`, osarch.Current().String())), 0644)

				require.NoError(t, err)
				gittest.CommitAllFiles(t, projectDir, "Commit files")
				gittest.CreateGitTag(t, projectDir, "0.1.0")
			},
			"",
			fmt.Sprintf(`Running Docker build for configuration print-dockerfile of product foo...
FROM alpine:3.5
RUN echo 'Product: foo'
RUN echo 'Version: 0.1.0'
RUN echo 'Repository: registry-host:5000/'
RUN echo 'RepositoryLiteral: registry-host:5000'
RUN echo 'InputBuildArtifact for foo: input-products/foo/build/%s/foo'
RUN echo 'InputDistArtifacts for foo: [input-products/foo/dist/os-arch-bin/foo-0.1.0-%s.tgz]'
RUN echo 'Tag for foo: registry-host:5000/foo:latest'
RUN echo 'Tags for foo: [registry-host:5000/foo:latest]'
`, osarch.Current().String(), osarch.Current().String()),
			func(t *testing.T, caseNum int, name, projectDir string) {
				bytes, err := os.ReadFile(path.Join(projectDir, "docker-context-dir", "Dockerfile"))
				require.NoError(t, err)
				originalDockerfileContent := fmt.Sprintf(`FROM alpine:3.5
RUN echo 'Product: {{Product}}'
RUN echo 'Version: {{Version}}'
RUN echo 'Repository: {{Repository}}'
RUN echo 'RepositoryLiteral: {{RepositoryLiteral}}'
RUN echo 'InputBuildArtifact for foo: {{InputBuildArtifact "foo" %q}}'
RUN echo 'InputDistArtifacts for foo: {{InputDistArtifacts "foo" "os-arch-bin"}}'
RUN echo 'Tag for foo: {{Tag "foo" "print-dockerfile" "default"}}'
RUN echo 'Tags for foo: {{Tags "foo" "print-dockerfile"}}'
`, osarch.Current().String())
				assert.Equal(t, originalDockerfileContent, string(bytes))
			},
		},
		{
			"tags template function renders tags in order of rendered output",
			distgoconfig.ProjectConfig{
				Products: distgoconfig.ToProductsMap(map[distgo.ProductID]distgoconfig.ProductConfig{
					"foo": {
						Docker: distgoconfig.ToDockerConfig(&distgoconfig.DockerConfig{
							DockerBuildersConfig: distgoconfig.ToDockerBuildersConfig(&distgoconfig.DockerBuildersConfig{
								printDockerfileDockerBuilderTypeName: distgoconfig.ToDockerBuilderConfig(distgoconfig.DockerBuilderConfig{
									Type:       stringPtr(printDockerfileDockerBuilderTypeName),
									ContextDir: stringPtr("docker-context-dir"),
									TagTemplates: distgoconfig.ToTagTemplatesMap(mustTagTemplatesMap(
										"0", "foo:5",
										"2", "foo:3",
										"3", "foo:2",
										"5", "foo:0",
										"1", "foo:1",
										"4", "foo:4",
									)),
								}),
							}),
						}),
					},
				}),
			},
			nil,
			func(t *testing.T, projectDir string, projectCfg distgoconfig.ProjectConfig) {
				contextDir := path.Join(projectDir, "docker-context-dir")
				err := os.Mkdir(contextDir, 0755)
				require.NoError(t, err)

				dockerfile := path.Join(contextDir, "Dockerfile")
				err = os.WriteFile(dockerfile, []byte(`FROM alpine:3.5
RUN echo 'Tags for foo: {{Tags "foo" "print-dockerfile"}}'
`), 0644)

				require.NoError(t, err)
				gittest.CommitAllFiles(t, projectDir, "Commit files")
				gittest.CreateGitTag(t, projectDir, "0.1.0")
			},
			"",
			`Running Docker build for configuration print-dockerfile of product foo...
FROM alpine:3.5
RUN echo 'Tags for foo: [foo:5 foo:3 foo:2 foo:0 foo:1 foo:4]'
`,
			func(t *testing.T, caseNum int, name, projectDir string) {
				bytes, err := os.ReadFile(path.Join(projectDir, "docker-context-dir", "Dockerfile"))
				require.NoError(t, err)
				originalDockerfileContent := `FROM alpine:3.5
RUN echo 'Tags for foo: {{Tags "foo" "print-dockerfile"}}'
`
				assert.Equal(t, originalDockerfileContent, string(bytes))
			},
		},
		{
			"Dockerfile renders template variables for cross-product build dependencies",
			distgoconfig.ProjectConfig{
				Products: distgoconfig.ToProductsMap(map[distgo.ProductID]distgoconfig.ProductConfig{
					"bar": {
						Docker: distgoconfig.ToDockerConfig(&distgoconfig.DockerConfig{
							Repository: stringPtr("registry-host:5000"),
							DockerBuildersConfig: distgoconfig.ToDockerBuildersConfig(&distgoconfig.DockerBuildersConfig{
								printDockerfileDockerBuilderTypeName: distgoconfig.ToDockerBuilderConfig(distgoconfig.DockerBuilderConfig{
									Type:             stringPtr(printDockerfileDockerBuilderTypeName),
									ContextDir:       stringPtr("docker-context-dir"),
									InputProductsDir: stringPtr("input-products"),
									InputBuilds: &[]distgo.ProductBuildID{
										"foo",
									},
									TagTemplates: distgoconfig.ToTagTemplatesMap(mustTagTemplatesMap(
										"default", "{{Repository}}bar:latest",
									)),
								}),
							}),
						}),
						Dependencies: &[]distgo.ProductID{
							"foo",
						},
					},
					"foo": {
						Build: distgoconfig.ToBuildConfig(&distgoconfig.BuildConfig{
							MainPkg: stringPtr("./foo"),
						}),
					},
				}),
			},
			nil,
			func(t *testing.T, projectDir string, projectCfg distgoconfig.ProjectConfig) {
				contextDir := path.Join(projectDir, "docker-context-dir")
				err := os.Mkdir(contextDir, 0755)
				require.NoError(t, err)

				dockerfile := path.Join(contextDir, "Dockerfile")
				err = os.WriteFile(dockerfile, []byte(fmt.Sprintf(`FROM alpine:3.5
RUN echo 'Product: {{Product}}'
RUN echo 'Version: {{Version}}'
RUN echo 'Repository: {{Repository}}'
RUN echo 'RepositoryLiteral: {{RepositoryLiteral}}'
RUN echo 'InputBuildArtifact for bar: {{InputBuildArtifact "foo" %q}}'
`, osarch.Current().String())), 0644)

				require.NoError(t, err)
				gittest.CommitAllFiles(t, projectDir, "Commit files")
				gittest.CreateGitTag(t, projectDir, "0.1.0")
			},
			"",
			fmt.Sprintf(`foo does not have Docker outputs; skipping build
Running Docker build for configuration print-dockerfile of product bar...
FROM alpine:3.5
RUN echo 'Product: bar'
RUN echo 'Version: 0.1.0'
RUN echo 'Repository: registry-host:5000/'
RUN echo 'RepositoryLiteral: registry-host:5000'
RUN echo 'InputBuildArtifact for bar: input-products/foo/build/%s/foo'
`, osarch.Current().String()),
			func(t *testing.T, caseNum int, name, projectDir string) {
				bytes, err := os.ReadFile(path.Join(projectDir, "docker-context-dir", "Dockerfile"))
				require.NoError(t, err)
				originalDockerfileContent := fmt.Sprintf(`FROM alpine:3.5
RUN echo 'Product: {{Product}}'
RUN echo 'Version: {{Version}}'
RUN echo 'Repository: {{Repository}}'
RUN echo 'RepositoryLiteral: {{RepositoryLiteral}}'
RUN echo 'InputBuildArtifact for bar: {{InputBuildArtifact "foo" %q}}'
`, osarch.Current().String())
				assert.Equal(t, originalDockerfileContent, string(bytes))
			},
		},
		{
			"Dockerfile renders template variables for cross-product dist dependencies",
			distgoconfig.ProjectConfig{
				Products: distgoconfig.ToProductsMap(map[distgo.ProductID]distgoconfig.ProductConfig{
					"bar": {
						Docker: distgoconfig.ToDockerConfig(&distgoconfig.DockerConfig{
							Repository: stringPtr("registry-host:5000"),
							DockerBuildersConfig: distgoconfig.ToDockerBuildersConfig(&distgoconfig.DockerBuildersConfig{
								printDockerfileDockerBuilderTypeName: distgoconfig.ToDockerBuilderConfig(distgoconfig.DockerBuilderConfig{
									Type:             stringPtr(printDockerfileDockerBuilderTypeName),
									ContextDir:       stringPtr("docker-context-dir"),
									InputProductsDir: stringPtr("input-products"),
									InputDists: &[]distgo.ProductDistID{
										"foo.manual",
									},
									TagTemplates: distgoconfig.ToTagTemplatesMap(mustTagTemplatesMap(
										"default", "{{Repository}}bar:latest",
									)),
								}),
							}),
						}),
						Dependencies: &[]distgo.ProductID{
							"foo",
						},
					},
					"foo": {
						Dist: distgoconfig.ToDistConfig(&distgoconfig.DistConfig{
							Disters: distgoconfig.ToDistersConfig(&distgoconfig.DistersConfig{
								manual.TypeName: distgoconfig.ToDisterConfig(distgoconfig.DisterConfig{
									Type: stringPtr(manual.TypeName),
									Config: &yaml.MapSlice{
										{
											Key:   "extension",
											Value: "tar",
										},
									},
									Script: stringPtr(`#!/usr/bin/env bash
echo "hello" > $DIST_WORK_DIR/out.txt
tar -cf "$DIST_DIR/$DIST_NAME".tar -C "$DIST_WORK_DIR" out.txt`),
								}),
							}),
						}),
					},
				}),
			},
			nil,
			func(t *testing.T, projectDir string, projectCfg distgoconfig.ProjectConfig) {
				contextDir := path.Join(projectDir, "docker-context-dir")
				err := os.Mkdir(contextDir, 0755)
				require.NoError(t, err)

				dockerfile := path.Join(contextDir, "Dockerfile")
				err = os.WriteFile(dockerfile, []byte(fmt.Sprintf(`FROM alpine:3.5
RUN echo 'Product: {{Product}}'
RUN echo 'Version: {{Version}}'
RUN echo 'Repository: {{Repository}}'
RUN echo 'RepositoryLiteral: {{RepositoryLiteral}}'
RUN echo 'InputDistArtifacts for bar: {{InputDistArtifacts "foo" "manual"}}'
`)), 0644)

				require.NoError(t, err)
				gittest.CommitAllFiles(t, projectDir, "Commit files")
				gittest.CreateGitTag(t, projectDir, "0.1.0")
			},
			"",
			`foo does not have Docker outputs; skipping build
Running Docker build for configuration print-dockerfile of product bar...
FROM alpine:3.5
RUN echo 'Product: bar'
RUN echo 'Version: 0.1.0'
RUN echo 'Repository: registry-host:5000/'
RUN echo 'RepositoryLiteral: registry-host:5000'
RUN echo 'InputDistArtifacts for bar: [input-products/foo/dist/manual/foo-0.1.0.tar]'
`,
			func(t *testing.T, caseNum int, name, projectDir string) {
				bytes, err := os.ReadFile(path.Join(projectDir, "docker-context-dir", "Dockerfile"))
				require.NoError(t, err)
				originalDockerfileContent := `FROM alpine:3.5
RUN echo 'Product: {{Product}}'
RUN echo 'Version: {{Version}}'
RUN echo 'Repository: {{Repository}}'
RUN echo 'RepositoryLiteral: {{RepositoryLiteral}}'
RUN echo 'InputDistArtifacts for bar: {{InputDistArtifacts "foo" "manual"}}'
`
				assert.Equal(t, originalDockerfileContent, string(bytes))
			},
		},
		{
			"Dockerfile renders empty repository",
			distgoconfig.ProjectConfig{
				Products: distgoconfig.ToProductsMap(map[distgo.ProductID]distgoconfig.ProductConfig{
					"foo": {
						Build: distgoconfig.ToBuildConfig(&distgoconfig.BuildConfig{
							MainPkg: stringPtr("./foo"),
						}),
						Dist: distgoconfig.ToDistConfig(&distgoconfig.DistConfig{
							Disters: distgoconfig.ToDistersConfig(&distgoconfig.DistersConfig{
								osarchbin.TypeName: distgoconfig.ToDisterConfig(distgoconfig.DisterConfig{
									Type: stringPtr(osarchbin.TypeName),
								}),
							}),
						}),
						Docker: distgoconfig.ToDockerConfig(&distgoconfig.DockerConfig{
							DockerBuildersConfig: distgoconfig.ToDockerBuildersConfig(&distgoconfig.DockerBuildersConfig{
								printDockerfileDockerBuilderTypeName: distgoconfig.ToDockerBuilderConfig(distgoconfig.DockerBuilderConfig{
									Type:             stringPtr(printDockerfileDockerBuilderTypeName),
									ContextDir:       stringPtr("docker-context-dir"),
									InputProductsDir: stringPtr("input-products"),
									InputBuilds: &[]distgo.ProductBuildID{
										"foo",
									},
									InputDists: &[]distgo.ProductDistID{
										"foo",
									},
									TagTemplates: distgoconfig.ToTagTemplatesMap(mustTagTemplatesMap(
										"default", "{{Repository}}foo:latest",
									)),
								}),
							}),
						}),
					},
				}),
			},
			nil,
			func(t *testing.T, projectDir string, projectCfg distgoconfig.ProjectConfig) {
				contextDir := path.Join(projectDir, "docker-context-dir")
				err := os.Mkdir(contextDir, 0755)
				require.NoError(t, err)

				dockerfile := path.Join(contextDir, "Dockerfile")
				err = os.WriteFile(dockerfile, []byte(fmt.Sprintf(`FROM alpine:3.5
RUN echo 'Product: {{Product}}'
RUN echo 'Version: {{Version}}'
RUN echo 'Repository: {{Repository}}'
RUN echo 'RepositoryLiteral: {{RepositoryLiteral}}'
RUN echo 'InputBuildArtifact for foo: {{InputBuildArtifact "foo" %q}}'
RUN echo 'InputDistArtifacts for foo: {{InputDistArtifacts "foo" "os-arch-bin"}}'
RUN echo 'Tags for foo: {{Tags "foo" "print-dockerfile"}}'
`, osarch.Current().String())), 0644)

				require.NoError(t, err)
				gittest.CommitAllFiles(t, projectDir, "Commit files")
				gittest.CreateGitTag(t, projectDir, "0.1.0")
			},
			"",
			fmt.Sprintf(`Running Docker build for configuration print-dockerfile of product foo...
FROM alpine:3.5
RUN echo 'Product: foo'
RUN echo 'Version: 0.1.0'
RUN echo 'Repository: '
RUN echo 'RepositoryLiteral: '
RUN echo 'InputBuildArtifact for foo: input-products/foo/build/%s/foo'
RUN echo 'InputDistArtifacts for foo: [input-products/foo/dist/os-arch-bin/foo-0.1.0-%s.tgz]'
RUN echo 'Tags for foo: [foo:latest]'
`, osarch.Current().String(), osarch.Current().String()),
			func(t *testing.T, caseNum int, name, projectDir string) {
				bytes, err := os.ReadFile(path.Join(projectDir, "docker-context-dir", "Dockerfile"))
				require.NoError(t, err)
				originalDockerfileContent := fmt.Sprintf(`FROM alpine:3.5
RUN echo 'Product: {{Product}}'
RUN echo 'Version: {{Version}}'
RUN echo 'Repository: {{Repository}}'
RUN echo 'RepositoryLiteral: {{RepositoryLiteral}}'
RUN echo 'InputBuildArtifact for foo: {{InputBuildArtifact "foo" %q}}'
RUN echo 'InputDistArtifacts for foo: {{InputDistArtifacts "foo" "os-arch-bin"}}'
RUN echo 'Tags for foo: {{Tags "foo" "print-dockerfile"}}'
`, osarch.Current().String())
				assert.Equal(t, originalDockerfileContent, string(bytes))
			},
		},
		{
			"Dockerfile skips rendering template variables when rendering is disabled",
			distgoconfig.ProjectConfig{
				Products: distgoconfig.ToProductsMap(map[distgo.ProductID]distgoconfig.ProductConfig{
					"foo": {
						Build: distgoconfig.ToBuildConfig(&distgoconfig.BuildConfig{
							MainPkg: stringPtr("./foo"),
						}),
						Dist: distgoconfig.ToDistConfig(&distgoconfig.DistConfig{
							Disters: distgoconfig.ToDistersConfig(&distgoconfig.DistersConfig{
								osarchbin.TypeName: distgoconfig.ToDisterConfig(distgoconfig.DisterConfig{
									Type: stringPtr(osarchbin.TypeName),
								}),
							}),
						}),
						Docker: distgoconfig.ToDockerConfig(&distgoconfig.DockerConfig{
							Repository: stringPtr("registry-host:5000"),
							DockerBuildersConfig: distgoconfig.ToDockerBuildersConfig(&distgoconfig.DockerBuildersConfig{
								printDockerfileDockerBuilderTypeName: distgoconfig.ToDockerBuilderConfig(distgoconfig.DockerBuilderConfig{
									Type:                     stringPtr(printDockerfileDockerBuilderTypeName),
									DisableTemplateRendering: boolPtr(true),
									ContextDir:               stringPtr("docker-context-dir"),
									InputProductsDir:         stringPtr("input-products"),
									InputBuilds: &[]distgo.ProductBuildID{
										"foo",
									},
									InputDists: &[]distgo.ProductDistID{
										"foo",
									},
									TagTemplates: distgoconfig.ToTagTemplatesMap(mustTagTemplatesMap(
										"default", "{{Repository}}foo:latest",
									)),
								}),
							}),
						}),
					},
				}),
			},
			nil,
			func(t *testing.T, projectDir string, projectCfg distgoconfig.ProjectConfig) {
				contextDir := path.Join(projectDir, "docker-context-dir")
				err := os.Mkdir(contextDir, 0755)
				require.NoError(t, err)

				dockerfile := path.Join(contextDir, "Dockerfile")
				err = os.WriteFile(dockerfile, []byte(fmt.Sprintf(`FROM alpine:3.5
RUN echo 'Product: {{Product}}'
RUN echo 'Version: {{Version}}'
RUN echo 'Repository: {{Repository}}'
RUN echo 'InputBuildArtifact for foo: {{InputBuildArtifact "foo" %q}}'
RUN echo 'InputDistArtifacts for foo: {{InputDistArtifacts "foo" "os-arch-bin"}}'
RUN echo 'Tags for foo: {{Tags "foo" "print-dockerfile"}}'
`, osarch.Current().String())), 0644)

				require.NoError(t, err)
				gittest.CommitAllFiles(t, projectDir, "Commit files")
				gittest.CreateGitTag(t, projectDir, "0.1.0")
			},
			"",
			fmt.Sprintf(`Running Docker build for configuration print-dockerfile of product foo...
FROM alpine:3.5
RUN echo 'Product: {{Product}}'
RUN echo 'Version: {{Version}}'
RUN echo 'Repository: {{Repository}}'
RUN echo 'InputBuildArtifact for foo: {{InputBuildArtifact "foo" %q}}'
RUN echo 'InputDistArtifacts for foo: {{InputDistArtifacts "foo" "os-arch-bin"}}'
RUN echo 'Tags for foo: {{Tags "foo" "print-dockerfile"}}'
`, osarch.Current().String()),
			func(t *testing.T, caseNum int, name, projectDir string) {
				bytes, err := os.ReadFile(path.Join(projectDir, "docker-context-dir", "Dockerfile"))
				require.NoError(t, err)
				originalDockerfileContent := fmt.Sprintf(`FROM alpine:3.5
RUN echo 'Product: {{Product}}'
RUN echo 'Version: {{Version}}'
RUN echo 'Repository: {{Repository}}'
RUN echo 'InputBuildArtifact for foo: {{InputBuildArtifact "foo" %q}}'
RUN echo 'InputDistArtifacts for foo: {{InputDistArtifacts "foo" "os-arch-bin"}}'
RUN echo 'Tags for foo: {{Tags "foo" "print-dockerfile"}}'
`, osarch.Current().String())
				assert.Equal(t, originalDockerfileContent, string(bytes))
			},
		},
		{
			"Docker build only runs for matching tag keys when tag keys are provided",
			distgoconfig.ProjectConfig{
				Products: distgoconfig.ToProductsMap(map[distgo.ProductID]distgoconfig.ProductConfig{
					"foo": {
						Build: distgoconfig.ToBuildConfig(&distgoconfig.BuildConfig{
							MainPkg: stringPtr("./foo"),
						}),
						Dist: distgoconfig.ToDistConfig(&distgoconfig.DistConfig{
							Disters: distgoconfig.ToDistersConfig(&distgoconfig.DistersConfig{
								osarchbin.TypeName: distgoconfig.ToDisterConfig(distgoconfig.DisterConfig{
									Type: stringPtr(osarchbin.TypeName),
								}),
							}),
						}),
						Docker: distgoconfig.ToDockerConfig(&distgoconfig.DockerConfig{
							Repository: stringPtr("registry-host:5000"),
							DockerBuildersConfig: distgoconfig.ToDockerBuildersConfig(&distgoconfig.DockerBuildersConfig{
								printDockerfileDockerBuilderTypeName: distgoconfig.ToDockerBuilderConfig(distgoconfig.DockerBuilderConfig{
									Type:             stringPtr(printDockerfileDockerBuilderTypeName),
									ContextDir:       stringPtr("docker-context-dir"),
									InputProductsDir: stringPtr("input-products"),
									InputBuilds: &[]distgo.ProductBuildID{
										"foo",
									},
									InputDists: &[]distgo.ProductDistID{
										"foo",
									},
									TagTemplates: distgoconfig.ToTagTemplatesMap(mustTagTemplatesMap(
										"snapshot", "{{Repository}}foo:latest",
										"release", "{{Repository}}foo:{{Version}}",
									)),
								}),
							}),
						}),
					},
				}),
			},
			[]string{
				"snapshot",
			},
			func(t *testing.T, projectDir string, projectCfg distgoconfig.ProjectConfig) {
				contextDir := path.Join(projectDir, "docker-context-dir")
				err := os.Mkdir(contextDir, 0755)
				require.NoError(t, err)

				dockerfile := path.Join(contextDir, "Dockerfile")
				err = os.WriteFile(dockerfile, []byte(fmt.Sprintf(`FROM alpine:3.5
RUN echo 'Product: {{Product}}'
RUN echo 'Version: {{Version}}'
RUN echo 'Repository: {{Repository}}'
RUN echo 'RepositoryLiteral: {{RepositoryLiteral}}'
RUN echo 'InputBuildArtifact for foo: {{InputBuildArtifact "foo" %q}}'
RUN echo 'InputDistArtifacts for foo: {{InputDistArtifacts "foo" "os-arch-bin"}}'
RUN echo 'Tags for foo: {{Tags "foo" "print-dockerfile"}}'
`, osarch.Current().String())), 0644)

				require.NoError(t, err)
				gittest.CommitAllFiles(t, projectDir, "Commit files")
				gittest.CreateGitTag(t, projectDir, "0.1.0")
			},
			"",
			fmt.Sprintf(`Running Docker build for configuration print-dockerfile of product foo...
FROM alpine:3.5
RUN echo 'Product: foo'
RUN echo 'Version: 0.1.0'
RUN echo 'Repository: registry-host:5000/'
RUN echo 'RepositoryLiteral: registry-host:5000'
RUN echo 'InputBuildArtifact for foo: input-products/foo/build/%s/foo'
RUN echo 'InputDistArtifacts for foo: [input-products/foo/dist/os-arch-bin/foo-0.1.0-%s.tgz]'
RUN echo 'Tags for foo: [registry-host:5000/foo:latest]'
`, osarch.Current().String(), osarch.Current().String()),
			func(t *testing.T, caseNum int, name, projectDir string) {
				bytes, err := os.ReadFile(path.Join(projectDir, "docker-context-dir", "Dockerfile"))
				require.NoError(t, err)
				originalDockerfileContent := fmt.Sprintf(`FROM alpine:3.5
RUN echo 'Product: {{Product}}'
RUN echo 'Version: {{Version}}'
RUN echo 'Repository: {{Repository}}'
RUN echo 'RepositoryLiteral: {{RepositoryLiteral}}'
RUN echo 'InputBuildArtifact for foo: {{InputBuildArtifact "foo" %q}}'
RUN echo 'InputDistArtifacts for foo: {{InputDistArtifacts "foo" "os-arch-bin"}}'
RUN echo 'Tags for foo: {{Tags "foo" "print-dockerfile"}}'
`, osarch.Current().String())
				assert.Equal(t, originalDockerfileContent, string(bytes))
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			projectDir, err := os.MkdirTemp(tmp, "")
			require.NoError(t, err, "Case %d: %s", i, tc.name)

			gittest.InitGitDir(t, projectDir)
			err = os.MkdirAll(path.Join(projectDir, "foo"), 0755)
			require.NoError(t, err, "Case %d: %s", i, tc.name)
			err = os.WriteFile(path.Join(projectDir, "foo", "main.go"), []byte(testMain), 0644)
			require.NoError(t, err, "Case %d: %s", i, tc.name)
			err = os.WriteFile(path.Join(projectDir, "go.mod"), []byte("module foo"), 0644)
			require.NoError(t, err, "Case %d: %s", i, tc.name)
			gittest.CommitAllFiles(t, projectDir, "Commit")

			if tc.preDockerAction != nil {
				tc.preDockerAction(t, projectDir, tc.projectCfg)
			}

			projectVersionerFactory, err := projectversionerfactory.New(nil, nil)
			require.NoError(t, err, "Case %d: %s", i, tc.name)
			disterFactory, err := disterfactory.New(nil, nil)
			require.NoError(t, err, "Case %d: %s", i, tc.name)
			defaultDisterCfg, err := disterfactory.DefaultConfig()
			require.NoError(t, err, "Case %d: %s", i, tc.name)
			dockerBuilderFactory, err := dockerbuilderfactory.New([]dockerbuilder.Creator{dockerbuilder.NewCreator(printDockerfileDockerBuilderTypeName, newPrintDockerfileBuilder)}, nil)
			require.NoError(t, err, "Case %d: %s", i, tc.name)
			publisherFactory, err := publisherfactory.New(nil, nil)
			require.NoError(t, err, "Case %d: %s", i, tc.name)

			projectParam, err := tc.projectCfg.ToParam(projectDir, projectVersionerFactory, disterFactory, defaultDisterCfg, dockerBuilderFactory, publisherFactory)
			require.NoError(t, err, "Case %d: %s", i, tc.name)

			projectInfo, err := projectParam.ProjectInfo(projectDir)
			require.NoError(t, err, "Case %d: %s", i, tc.name)

			preDistTime := time.Now().Truncate(time.Second).Add(-1 * time.Second)
			err = dist.Products(projectInfo, projectParam, nil, nil, false, true, io.Discard)
			require.NoError(t, err, "Case %d: %s", i, tc.name)

			buffer := &bytes.Buffer{}
			err = docker.BuildProducts(projectInfo, projectParam, &preDistTime, nil, tc.tagKeys, false, false, buffer)
			if tc.wantErrorRegexp == "" {
				require.NoError(t, err, "Case %d: %s", i, tc.name)
			} else {
				require.Error(t, err, fmt.Sprintf("Case %d: %s", i, tc.name))
				assert.Regexp(t, regexp.MustCompile(tc.wantErrorRegexp), err.Error(), "Case %d: %s", i, tc.name)
			}

			if tc.wantStdout != "" {
				assert.Equal(t, tc.wantStdout, buffer.String(), "Case %d: %s\nOutput:\n%s", i, tc.name, buffer.String())
			}

			if tc.validate != nil {
				tc.validate(t, i, tc.name, projectDir)
			}
		})

	}
}

func stringPtr(in string) *string {
	return &in
}

func boolPtr(in bool) *bool {
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
