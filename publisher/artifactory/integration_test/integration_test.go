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
	"os"
	"testing"

	"github.com/nmiyake/pkg/gofiles"
	"github.com/palantir/distgo/publisher/publishertester"
	"github.com/palantir/godel/v2/framework/pluginapitester"
	"github.com/palantir/godel/v2/pkg/osarch"
	"github.com/palantir/godel/v2/pkg/products"
	"github.com/stretchr/testify/require"
)

func TestArtifactoryPublish(t *testing.T) {
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
		"artifactory",
		[]publishertester.TestCase{
			{
				Name: "publishes artifact and POM to Artifactory",
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
        artifactory:
          config:
            url: http://artifactory.domain.com
            username: testUsername
            password: testPassword
            repository: testRepo
`,
				},
				Args: []string{
					"--dry-run",
				},
				WantOutput: func(projectDir string) string {
					return fmt.Sprintf(`[DRY RUN] Uploading out/dist/foo/1.0.0/os-arch-bin/foo-1.0.0-%s.tgz to http://artifactory.domain.com/artifactory/testRepo/com/test/group/foo/1.0.0/foo-1.0.0-%s.tgz
[DRY RUN] Uploading to http://artifactory.domain.com/artifactory/testRepo/com/test/group/foo/1.0.0/foo-1.0.0.pom
`, osarch.Current().String(), osarch.Current().String())
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
        artifactory:
          config:
            url: http://artifactory.domain.com
            username: testUsername
            password: testPassword
            repository: testRepo
            no-pom: true
`,
				},
				Args: []string{
					"--dry-run",
				},
				WantOutput: func(projectDir string) string {
					return fmt.Sprintf(`[DRY RUN] Uploading out/dist/foo/1.0.0/os-arch-bin/foo-1.0.0-%s.tgz to http://artifactory.domain.com/artifactory/testRepo/com/test/group/foo/1.0.0/foo-1.0.0-%s.tgz
`, osarch.Current().String(), osarch.Current().String())
				},
			},
			{
				Name: "skips POM publish if no artifacts are uploaded",
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
        artifactory:
          config:
            url: http://artifactory.domain.com
            username: testUsername
            password: testPassword
            repository: testRepo
`,
				},
				Args: []string{
					"--dry-run",
					`--exclude-artifact-names=^.+\.tgz$`,
				},
				WantOutput: func(projectDir string) string {
					return ""
				},
			},
			{
				Name: "can use flags to specify values",
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
      info:
        artifactory:
`,
				},
				Args: []string{
					"--dry-run",
					"--group-id", "com.test.group",
					"--url", "http://artifactory.domain.com",
					"--username", "testUsername",
					"--password", "testPassword",
					"--repository", "testRepo",
				},
				WantOutput: func(projectDir string) string {
					return fmt.Sprintf(`[DRY RUN] Uploading out/dist/foo/1.0.0/os-arch-bin/foo-1.0.0-%s.tgz to http://artifactory.domain.com/artifactory/testRepo/com/test/group/foo/1.0.0/foo-1.0.0-%s.tgz
[DRY RUN] Uploading to http://artifactory.domain.com/artifactory/testRepo/com/test/group/foo/1.0.0/foo-1.0.0.pom
`, osarch.Current().String(), osarch.Current().String())
				},
			},
			{
				Name: "can use flags to specify values including no-pom",
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
      info:
        artifactory:
`,
				},
				Args: []string{
					"--dry-run",
					"--group-id", "com.test.group",
					"--url", "http://artifactory.domain.com",
					"--username", "testUsername",
					"--password", "testPassword",
					"--repository", "testRepo",
					"--no-pom",
				},
				WantOutput: func(projectDir string) string {
					return fmt.Sprintf(`[DRY RUN] Uploading out/dist/foo/1.0.0/os-arch-bin/foo-1.0.0-%s.tgz to http://artifactory.domain.com/artifactory/testRepo/com/test/group/foo/1.0.0/foo-1.0.0-%s.tgz
`, osarch.Current().String(), osarch.Current().String())
				},
			},
			{
				Name: "publishes multiple artifacts and POM to Artifactory",
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
        artifactory:
          config:
            url: http://artifactory.domain.com
            username: testUsername
            password: testPassword
            repository: testRepo
`,
				},
				Args: []string{
					"--dry-run",
				},
				WantOutput: func(projectDir string) string {
					return `[DRY RUN] Uploading out/dist/foo/1.0.0/os-arch-bin/foo-1.0.0-darwin-amd64.tgz to http://artifactory.domain.com/artifactory/testRepo/com/test/group/foo/1.0.0/foo-1.0.0-darwin-amd64.tgz
[DRY RUN] Uploading out/dist/foo/1.0.0/os-arch-bin/foo-1.0.0-linux-amd64.tgz to http://artifactory.domain.com/artifactory/testRepo/com/test/group/foo/1.0.0/foo-1.0.0-linux-amd64.tgz
[DRY RUN] Uploading to http://artifactory.domain.com/artifactory/testRepo/com/test/group/foo/1.0.0/foo-1.0.0.pom
`
				},
			},
			{
				Name: "appends properties to publish URL",
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
        artifactory:
          config:
            url: http://artifactory.domain.com
            username: testUsername
            password: testPassword
            repository: testRepo
            properties:
              key1: value1
              key2: value2
`,
				},
				Args: []string{
					"--dry-run",
				},
				WantOutput: func(projectDir string) string {
					return fmt.Sprintf(`[DRY RUN] Uploading out/dist/foo/1.0.0/os-arch-bin/foo-1.0.0-%s.tgz to http://artifactory.domain.com/artifactory/testRepo;key1=value1;key2=value2/com/test/group/foo/1.0.0/foo-1.0.0-%s.tgz
[DRY RUN] Uploading to http://artifactory.domain.com/artifactory/testRepo;key1=value1;key2=value2/com/test/group/foo/1.0.0/foo-1.0.0.pom
`, osarch.Current().String(), osarch.Current().String())
				},
			},
			{
				Name: "URL encodes and appends properties to the publish URL",
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
        artifactory:
          config:
            url: http://artifactory.domain.com
            username: testUsername
            password: testPassword
            repository: testRepo
            properties:
              key1: some/branch
              key2: value2
`,
				},
				Args: []string{
					"--dry-run",
				},
				WantOutput: func(projectDir string) string {
					return fmt.Sprintf(`[DRY RUN] Uploading out/dist/foo/1.0.0/os-arch-bin/foo-1.0.0-%s.tgz to http://artifactory.domain.com/artifactory/testRepo;key1=some%%2Fbranch;key2=value2/com/test/group/foo/1.0.0/foo-1.0.0-%s.tgz
[DRY RUN] Uploading to http://artifactory.domain.com/artifactory/testRepo;key1=some%%2Fbranch;key2=value2/com/test/group/foo/1.0.0/foo-1.0.0.pom
`, osarch.Current().String(), osarch.Current().String())
				},
			},
			{
				Name: "appends properties with flat-dir",
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
        artifactory:
          config:
            url: http://artifactory.domain.com
            username: testUsername
            password: testPassword
            repository: testRepo
            flat-dir: true
            no-pom: true
            properties:
              key1: value1
              key2: value2
`,
				},
				Args: []string{
					"--dry-run",
				},
				WantOutput: func(projectDir string) string {
					return fmt.Sprintf(`[DRY RUN] Uploading out/dist/foo/1.0.0/os-arch-bin/foo-1.0.0-%s.tgz to http://artifactory.domain.com/artifactory/testRepo;key1=value1;key2=value2/foo-1.0.0-%s.tgz
`, osarch.Current().String(), osarch.Current().String())
				},
			},
		},
	)
}

func TestArtifactoryPublishRendersPropertiesGoTemplates(t *testing.T) {
	const godelYML = `exclude:
  names:
    - "\\..+"
    - "vendor"
  paths:
    - "godel"
`

	pluginPath, err := products.Bin("dist-plugin")
	require.NoError(t, err)

	err = os.Setenv("TEST_ENV_VAR", "testValue")
	require.NoError(t, err)
	publishertester.RunAssetPublishTest(t,
		pluginapitester.NewPluginProvider(pluginPath),
		nil,
		"artifactory",
		[]publishertester.TestCase{
			{
				Name: "appends properties with env vars to publish URL",
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
        artifactory:
          config:
            url: http://artifactory.domain.com
            username: testUsername
            password: testPassword
            repository: testRepo
            properties:
              key1: value1
              env-key: '{{ env "TEST_ENV_VAR" }}'
`,
				},
				Args: []string{
					"--dry-run",
				},
				WantOutput: func(projectDir string) string {
					return fmt.Sprintf(`[DRY RUN] Uploading out/dist/foo/1.0.0/os-arch-bin/foo-1.0.0-%s.tgz to http://artifactory.domain.com/artifactory/testRepo;env-key=testValue;key1=value1/com/test/group/foo/1.0.0/foo-1.0.0-%s.tgz
[DRY RUN] Uploading to http://artifactory.domain.com/artifactory/testRepo;env-key=testValue;key1=value1/com/test/group/foo/1.0.0/foo-1.0.0.pom
`, osarch.Current().String(), osarch.Current().String())
				},
			},
		},
	)

	err = os.Unsetenv("TEST_ENV_VAR")
	require.NoError(t, err)
}

func TestArtifactoryUpgradeConfig(t *testing.T) {
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
        artifactory:
          config:
            # comment
            url: http://artifactory.domain.com
            username: testUsername
            password: testPassword
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
        artifactory:
          config:
            # comment
            url: http://artifactory.domain.com
            username: testUsername
            password: testPassword
            repository: testRepo
`,
				},
			},
		},
	)
}
