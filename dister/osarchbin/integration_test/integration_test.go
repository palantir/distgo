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
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/mholt/archiver"
	"github.com/nmiyake/pkg/gofiles"
	"github.com/palantir/godel/v2/framework/pluginapitester"
	"github.com/palantir/godel/v2/pkg/products"
	"github.com/palantir/pkg/specdir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/palantir/distgo/dister/distertester"
)

func TestOSArchBinDist(t *testing.T) {
	const godelYML = `exclude:
  names:
    - "\\..+"
    - "vendor"
  paths:
    - "godel"
`

	pluginPath, err := products.Bin("dist-plugin")
	require.NoError(t, err)

	distertester.RunAssetDistTest(t,
		pluginapitester.NewPluginProvider(pluginPath),
		nil,
		[]distertester.TestCase{
			{
				Name: "os-arch-bin creates expected output",
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
`,
				},
				WantOutput: func(projectDir string) string {
					return `Creating distribution for foo at out/dist/foo/1.0.0/os-arch-bin/foo-1.0.0-darwin-amd64.tgz, out/dist/foo/1.0.0/os-arch-bin/foo-1.0.0-linux-amd64.tgz
Finished creating os-arch-bin distribution for foo
`
				},
				Validate: func(projectDir string) {
					wantLayout := specdir.NewLayoutSpec(
						specdir.Dir(specdir.LiteralName("1.0.0"), "",
							specdir.Dir(specdir.LiteralName("os-arch-bin"), "",
								specdir.Dir(specdir.LiteralName("foo-1.0.0"), "",
									specdir.Dir(specdir.LiteralName("darwin-amd64"), "",
										specdir.File(specdir.LiteralName("foo"), ""),
									),
									specdir.Dir(specdir.LiteralName("linux-amd64"), "",
										specdir.File(specdir.LiteralName("foo"), ""),
									),
								),
								specdir.File(specdir.LiteralName("foo-1.0.0-darwin-amd64.tgz"), ""),
								specdir.File(specdir.LiteralName("foo-1.0.0-linux-amd64.tgz"), ""),
							),
						), true,
					)
					assert.NoError(t, wantLayout.Validate(path.Join(projectDir, "out", "dist", "foo", "1.0.0"), nil))
				},
			},
			{
				Name: "os-arch-bin allows customized output",
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
        script: |
          #!/bin/bash
          for (( i=0; i<${BUILD_OS_ARCH_COUNT}; i++ )); do
            OS_ARCH_VAR="BUILD_OS_ARCH_${i}"
            echo "${!OS_ARCH_VAR}" > "${DIST_WORK_DIR}/${!OS_ARCH_VAR}/os-arch"
          done
`,
				},
				WantOutput: func(projectDir string) string {
					return `Creating distribution for foo at out/dist/foo/1.0.0/os-arch-bin/foo-1.0.0-darwin-amd64.tgz, out/dist/foo/1.0.0/os-arch-bin/foo-1.0.0-linux-amd64.tgz
Finished creating os-arch-bin distribution for foo
`
				},
				Validate: func(projectDir string) {
					wantLayout := specdir.NewLayoutSpec(
						specdir.Dir(specdir.LiteralName("1.0.0"), "",
							specdir.Dir(specdir.LiteralName("os-arch-bin"), "",
								specdir.Dir(specdir.LiteralName("foo-1.0.0"), "",
									specdir.Dir(specdir.LiteralName("darwin-amd64"), "",
										specdir.File(specdir.LiteralName("foo"), ""),
										specdir.File(specdir.LiteralName("os-arch"), ""),
									),
									specdir.Dir(specdir.LiteralName("linux-amd64"), "",
										specdir.File(specdir.LiteralName("foo"), ""),
										specdir.File(specdir.LiteralName("os-arch"), ""),
									),
								),
								specdir.File(specdir.LiteralName("foo-1.0.0-darwin-amd64.tgz"), ""),
								specdir.File(specdir.LiteralName("foo-1.0.0-linux-amd64.tgz"), ""),
							),
						), true,
					)
					require.NoError(t, wantLayout.Validate(path.Join(projectDir, "out", "dist", "foo", "1.0.0"), nil))

					dir, err := ioutil.TempDir("", "")
					require.NoError(t, err)

					defer func() {
						require.NoError(t, os.RemoveAll(dir))
					}()

					dist := filepath.Join(projectDir, "out", "dist", "foo", "1.0.0", "os-arch-bin", "foo-1.0.0-linux-amd64.tgz")
					require.NoError(t, archiver.TarGz.Open(dist, dir))

					arch, err := ioutil.ReadFile(filepath.Join(dir, "os-arch"))
					require.NoError(t, err)

					binInfo, err := os.Stat(filepath.Join(dir, "foo"))
					require.NoError(t, err)

					assert.Equal(t, "linux-amd64\n", string(arch))
					assert.Equal(t, os.FileMode(0111), binInfo.Mode()&0111)
				},
			},
		},
	)
}

func TestOSArchBinUpgradeConfig(t *testing.T) {
	pluginPath, err := products.Bin("dist-plugin")
	require.NoError(t, err)

	pluginapitester.RunUpgradeConfigTest(t,
		pluginapitester.NewPluginProvider(pluginPath),
		nil,
		[]pluginapitester.UpgradeConfigTestCase{
			{
				Name: `legacy configuration is upgraded`,
				ConfigFiles: map[string]string{
					"godel/config/dist.yml": `products:
  foo:
    build:
      main-pkg: ./foo
      os-archs:
        - os: darwin
          arch: amd64
        - os: linux
          arch: amd64
    dist:
      dist-type:
        type: os-arch-bin
        info:
          os-archs:
            - os: darwin
              arch: amd64
            - os: linux
              arch: amd64
`,
				},
				Legacy: true,
				WantOutput: `Upgraded configuration for dist-plugin.yml
`,
				WantFiles: map[string]string{
					"godel/config/dist-plugin.yml": `products:
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
        os-arch-bin:
          type: os-arch-bin
          config:
            os-archs:
            - os: darwin
              arch: amd64
            - os: linux
              arch: amd64
`,
				},
			},
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
            # comment
            - os: darwin
              arch: amd64
            - os: linux
              arch: amd64
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
            # comment
            - os: darwin
              arch: amd64
            - os: linux
              arch: amd64
`,
				},
			},
		},
	)
}
