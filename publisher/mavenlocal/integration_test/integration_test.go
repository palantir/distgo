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

package integration_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/nmiyake/pkg/gofiles"
	"github.com/palantir/godel/framework/pluginapitester"
	"github.com/palantir/godel/pkg/osarch"
	"github.com/palantir/godel/pkg/products/v2/products"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/palantir/distgo/publisher/publishertester"
)

func TestMavenLocalPublish(t *testing.T) {
	const godelYML = `exclude:
  names:
    - "\\..+"
    - "vendor"
  paths:
    - "godel"
`

	pluginPath, err := products.Bin("dist-plugin")
	require.NoError(t, err)

	publishertester.RunAssetPublishTest(t,
		pluginapitester.NewPluginProvider(pluginPath),
		nil,
		"maven-local",
		[]publishertester.TestCase{
			{
				Name: "publishes artifact and POM to M2 home by default",
				Specs: []gofiles.GoFileSpec{
					{
						RelPath: "go.mod",
						Src:     `module foo`,
					},
					{
						RelPath: "foo/foo.go",
						Src:     `package main; func main() {}`,
					},
				},
				ConfigFiles: map[string]string{
					"godel/config/godel.yml": godelYML,
					"godel/config/dist-plugin.yml": `
products:
  foo:
    build:
      main-pkg: ./foo
    dist:
      disters:
        type: os-arch-bin
    publish:
      group-id: com.test.group
`,
				},
				Args: []string{
					"--dry-run",
				},
				WantOutput: func(projectDir string) string {
					baseDir := path.Join(os.Getenv("HOME"), ".m2", "repository")
					return fmt.Sprintf(`[DRY RUN] Writing POM to %s/com/test/group/foo/1.0.0/foo-1.0.0.pom
[DRY RUN] Copying artifact from out/dist/foo/1.0.0/os-arch-bin/foo-1.0.0-%s.tgz to %s/com/test/group/foo/1.0.0/foo-1.0.0-%s.tgz
`, baseDir, osarch.Current().String(), baseDir, osarch.Current().String())
				},
			},
			{
				Name: "skips POM publish based on flag",
				Specs: []gofiles.GoFileSpec{
					{
						RelPath: "go.mod",
						Src:     `module foo`,
					},
					{
						RelPath: "foo/foo.go",
						Src:     `package main; func main() {}`,
					},
				},
				ConfigFiles: map[string]string{
					"godel/config/godel.yml": godelYML,
					"godel/config/dist-plugin.yml": `
products:
  foo:
    build:
      main-pkg: ./foo
    dist:
      disters:
        type: os-arch-bin
    publish:
      group-id: com.test.group
`,
				},
				Args: []string{
					"--dry-run",
					"--no-pom",
				},
				WantOutput: func(projectDir string) string {
					baseDir := path.Join(os.Getenv("HOME"), ".m2", "repository")
					return fmt.Sprintf(`[DRY RUN] Copying artifact from out/dist/foo/1.0.0/os-arch-bin/foo-1.0.0-%s.tgz to %s/com/test/group/foo/1.0.0/foo-1.0.0-%s.tgz
`, osarch.Current().String(), baseDir, osarch.Current().String())
				},
			},
			{
				Name: "skips POM publish based on configuration",
				Specs: []gofiles.GoFileSpec{
					{
						RelPath: "go.mod",
						Src:     `module foo`,
					},
					{
						RelPath: "foo/foo.go",
						Src:     `package main; func main() {}`,
					},
				},
				ConfigFiles: map[string]string{
					"godel/config/godel.yml": godelYML,
					"godel/config/dist-plugin.yml": `
products:
  foo:
    build:
      main-pkg: ./foo
    dist:
      disters:
        type: os-arch-bin
    publish:
      group-id: com.test.group
      info:
        maven-local:
          config:
            no-pom: true
`,
				},
				Args: []string{
					"--dry-run",
				},
				WantOutput: func(projectDir string) string {
					baseDir := path.Join(os.Getenv("HOME"), ".m2", "repository")
					return fmt.Sprintf(`[DRY RUN] Copying artifact from out/dist/foo/1.0.0/os-arch-bin/foo-1.0.0-%s.tgz to %s/com/test/group/foo/1.0.0/foo-1.0.0-%s.tgz
`, osarch.Current().String(), baseDir, osarch.Current().String())
				},
			},
			{
				Name: "publishes artifact and POM to local directory",
				Specs: []gofiles.GoFileSpec{
					{
						RelPath: "go.mod",
						Src:     `module foo`,
					},
					{
						RelPath: "foo/foo.go",
						Src:     `package main; func main() {}`,
					},
				},
				ConfigFiles: map[string]string{
					"godel/config/godel.yml": godelYML,
					"godel/config/dist-plugin.yml": `
products:
  foo:
    build:
      main-pkg: ./foo
    dist:
      disters:
        type: os-arch-bin
    publish:
      group-id: com.test.group
      info:
        maven-local:
          config:
            base-dir: out/publish
`,
				},
				Args: []string{
					"--dry-run",
				},
				WantOutput: func(projectDir string) string {
					return fmt.Sprintf(`[DRY RUN] Writing POM to out/publish/com/test/group/foo/1.0.0/foo-1.0.0.pom
[DRY RUN] Copying artifact from out/dist/foo/1.0.0/os-arch-bin/foo-1.0.0-%s.tgz to out/publish/com/test/group/foo/1.0.0/foo-1.0.0-%s.tgz
`, osarch.Current().String(), osarch.Current().String())
				},
			},
			{
				Name: "verify content of published POM",
				Specs: []gofiles.GoFileSpec{
					{
						RelPath: "go.mod",
						Src:     `module foo`,
					},
					{
						RelPath: "foo/foo.go",
						Src:     `package main; func main() {}`,
					},
				},
				ConfigFiles: map[string]string{
					"godel/config/godel.yml": godelYML,
					"godel/config/dist-plugin.yml": `
products:
  foo:
    build:
      main-pkg: ./foo
    dist:
      disters:
        type: os-arch-bin
    publish:
      group-id: com.test.group
      info:
        maven-local:
          config:
            base-dir: out/publish
`,
				},
				Args: nil,
				WantOutput: func(projectDir string) string {
					return fmt.Sprintf(`Writing POM to out/publish/com/test/group/foo/1.0.0/foo-1.0.0.pom
Copying artifact from out/dist/foo/1.0.0/os-arch-bin/foo-1.0.0-%s.tgz to out/publish/com/test/group/foo/1.0.0/foo-1.0.0-%s.tgz
`, osarch.Current().String(), osarch.Current().String())
				},
				Validate: func(projectDir string) {
					pomFile := path.Join(projectDir, "out", "publish", "com", "test", "group", "foo", "1.0.0", "foo-1.0.0.pom")
					bytes, err := ioutil.ReadFile(pomFile)
					require.NoError(t, err)
					want := `<project xmlns="http://maven.apache.org/POM/4.0.0" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
  xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 http://maven.apache.org/xsd/maven-4.0.0.xsd">
  <modelVersion>4.0.0</modelVersion>

  <groupId>com.test.group</groupId>
  <artifactId>foo</artifactId>
  <version>1.0.0</version>
  <packaging>tgz</packaging>
</project>
`
					assert.Equal(t, want, string(bytes))
				},
			},
			{
				Name: "publishes artifact and POM to current directory using '.'",
				Specs: []gofiles.GoFileSpec{
					{
						RelPath: "go.mod",
						Src:     `module foo`,
					},
					{
						RelPath: "foo/foo.go",
						Src:     `package main; func main() {}`,
					},
				},
				ConfigFiles: map[string]string{
					"godel/config/godel.yml": godelYML,
					"godel/config/dist-plugin.yml": `
products:
  foo:
    build:
      main-pkg: ./foo
    dist:
      disters:
        type: os-arch-bin
    publish:
      group-id: com.test.group
      info:
        maven-local:
          config:
            base-dir: .
`,
				},
				Args: []string{
					"--dry-run",
				},
				WantOutput: func(projectDir string) string {
					return fmt.Sprintf(`[DRY RUN] Writing POM to com/test/group/foo/1.0.0/foo-1.0.0.pom
[DRY RUN] Copying artifact from out/dist/foo/1.0.0/os-arch-bin/foo-1.0.0-%s.tgz to com/test/group/foo/1.0.0/foo-1.0.0-%s.tgz
`, osarch.Current().String(), osarch.Current().String())
				},
			},
		},
	)
}

func TestMavenLocalUpgradeConfig(t *testing.T) {
	pluginPath, err := products.Bin("dist-plugin")
	require.NoError(t, err)

	pluginapitester.RunUpgradeConfigTest(t,
		pluginapitester.NewPluginProvider(pluginPath),
		nil,
		[]pluginapitester.UpgradeConfigTestCase{
			{
				Name: `valid v0 config works`,
				ConfigFiles: map[string]string{
					"godel/config/dist-plugin.yml": `
products:
  foo:
    build:
      main-pkg: ./foo
      os-archs:
        - os: darwin
          arch: amd64
        - os: linux
          arch: amd64
    dist:
      disters:
        type: os-arch-bin
        config:
          os-archs:
            - os: darwin
              arch: amd64
            - os: linux
              arch: amd64
    publish:
      group-id: com.test.group
      info:
        github:
          config:
            api-url: http://github.domain.com
            user: testUser
            token: testToken
            # comment
            owner: testOwner
            repository: testRepo
`,
				},
				WantOutput: ``,
				WantFiles: map[string]string{
					"godel/config/dist-plugin.yml": `
products:
  foo:
    build:
      main-pkg: ./foo
      os-archs:
        - os: darwin
          arch: amd64
        - os: linux
          arch: amd64
    dist:
      disters:
        type: os-arch-bin
        config:
          os-archs:
            - os: darwin
              arch: amd64
            - os: linux
              arch: amd64
    publish:
      group-id: com.test.group
      info:
        github:
          config:
            api-url: http://github.domain.com
            user: testUser
            token: testToken
            # comment
            owner: testOwner
            repository: testRepo
`,
				},
			},
		},
	)
}
