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

package bin_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/mholt/archiver"
	"github.com/palantir/godel/pkg/osarch"
	"github.com/palantir/pkg/specdir"
	"github.com/stretchr/testify/require"

	"github.com/palantir/distgo/dister/bin"
	"github.com/palantir/distgo/dister/distertest"
	"github.com/palantir/distgo/distgo"
	distgoconfig "github.com/palantir/distgo/distgo/config"
	"github.com/palantir/distgo/publisher/publishertest"
)

func TestDister(t *testing.T) {
	distertest.Run(t, false,
		distertest.TestCase{
			Name: "bin creates expected output",
			ProjectCfg: distgoconfig.ProjectConfig{
				Products: distgoconfig.ToProductsMap(map[distgo.ProductID]distgoconfig.ProductConfig{
					"foo": {
						Build: distgoconfig.ToBuildConfig(&distgoconfig.BuildConfig{
							MainPkg: publishertest.StringPtr("foo"),
							OSArchs: &[]osarch.OSArch{
								distertest.MustOSArch("darwin-amd64"),
								distertest.MustOSArch("linux-amd64"),
							},
						}),
						Dist: distgoconfig.ToDistConfig(&distgoconfig.DistConfig{
							Disters: distgoconfig.ToDistersConfig(&distgoconfig.DistersConfig{
								bin.TypeName: {
									Type: publishertest.StringPtr(bin.TypeName),
								},
							}),
						}),
					},
				}),
			},
			WantOutput: func(projectDir string) string {
				return fmt.Sprintf(`Creating distribution for foo at %s/out/dist/foo/1.0.0/bin/foo-1.0.0.tgz
Finished creating bin distribution for foo
`, projectDir)
			},
			WantLayout: specdir.NewLayoutSpec(
				specdir.Dir(specdir.LiteralName("1.0.0"), "",
					specdir.Dir(specdir.LiteralName("bin"), "",
						specdir.Dir(specdir.LiteralName("foo-1.0.0"), "",
							specdir.Dir(specdir.LiteralName("bin"), "",
								specdir.Dir(specdir.LiteralName("darwin-amd64"), "",
									specdir.File(specdir.LiteralName("foo"), ""),
								),
								specdir.Dir(specdir.LiteralName("linux-amd64"), "",
									specdir.File(specdir.LiteralName("foo"), ""),
								),
							),
						),
						specdir.File(specdir.LiteralName("foo-1.0.0.tgz"), ""),
					),
				), true,
			),
			Validate: func(projectDir string) {
				tmpDir, err := ioutil.TempDir(projectDir, "expanded")
				require.NoError(t, err)
				require.NoError(t, archiver.TarGz.Open(path.Join(projectDir, "out", "dist", "foo", "1.0.0", "bin", "foo-1.0.0.tgz"), tmpDir))

				wantLayout := specdir.NewLayoutSpec(
					specdir.Dir(specdir.LiteralName("foo-1.0.0"), "",
						specdir.Dir(specdir.LiteralName("bin"), "",
							specdir.Dir(specdir.LiteralName("darwin-amd64"), "",
								specdir.File(specdir.LiteralName("foo"), ""),
							),
							specdir.Dir(specdir.LiteralName("linux-amd64"), "",
								specdir.File(specdir.LiteralName("foo"), ""),
							),
						),
					), true,
				)
				require.NoError(t, wantLayout.Validate(path.Join(tmpDir, "foo-1.0.0"), nil))
			},
		},
		distertest.TestCase{
			Name: "bin compresses work directory and includes output created by script",
			ProjectCfg: distgoconfig.ProjectConfig{
				Products: distgoconfig.ToProductsMap(map[distgo.ProductID]distgoconfig.ProductConfig{
					"foo": {
						Build: distgoconfig.ToBuildConfig(&distgoconfig.BuildConfig{
							MainPkg: publishertest.StringPtr("foo"),
							OSArchs: &[]osarch.OSArch{
								distertest.MustOSArch("darwin-amd64"),
								distertest.MustOSArch("linux-amd64"),
							},
						}),
						Dist: distgoconfig.ToDistConfig(&distgoconfig.DistConfig{
							Disters: distgoconfig.ToDistersConfig(&distgoconfig.DistersConfig{
								bin.TypeName: {
									Type: publishertest.StringPtr(bin.TypeName),
									Script: publishertest.StringPtr(`#!/usr/bin/env bash
# move bin directory into service directory
mkdir $DIST_WORK_DIR/service
mv $DIST_WORK_DIR/bin $DIST_WORK_DIR/service/bin
echo "hello" > $DIST_WORK_DIR/foo.txt
`),
								},
							}),
						}),
					},
				}),
			},
			WantOutput: func(projectDir string) string {
				return fmt.Sprintf(`Creating distribution for foo at %s/out/dist/foo/1.0.0/bin/foo-1.0.0.tgz
Finished creating bin distribution for foo
`, projectDir)
			},
			WantLayout: specdir.NewLayoutSpec(
				specdir.Dir(specdir.LiteralName("1.0.0"), "",
					specdir.Dir(specdir.LiteralName("bin"), "",
						specdir.Dir(specdir.LiteralName("foo-1.0.0"), "",
							specdir.Dir(specdir.LiteralName("service"), "",
								specdir.Dir(specdir.LiteralName("bin"), "",
									specdir.Dir(specdir.LiteralName("darwin-amd64"), "",
										specdir.File(specdir.LiteralName("foo"), ""),
									),
									specdir.Dir(specdir.LiteralName("linux-amd64"), "",
										specdir.File(specdir.LiteralName("foo"), ""),
									),
								),
							),
							specdir.File(specdir.LiteralName("foo.txt"), ""),
						),
						specdir.File(specdir.LiteralName("foo-1.0.0.tgz"), ""),
					),
				), true,
			),
			Validate: func(projectDir string) {
				tmpDir, err := ioutil.TempDir(projectDir, "expanded")
				require.NoError(t, err)
				require.NoError(t, archiver.TarGz.Open(path.Join(projectDir, "out", "dist", "foo", "1.0.0", "bin", "foo-1.0.0.tgz"), tmpDir))

				wantLayout := specdir.NewLayoutSpec(
					specdir.Dir(specdir.LiteralName("foo-1.0.0"), "",
						specdir.Dir(specdir.LiteralName("service"), "",
							specdir.Dir(specdir.LiteralName("bin"), "",
								specdir.Dir(specdir.LiteralName("darwin-amd64"), "",
									specdir.File(specdir.LiteralName("foo"), ""),
								),
								specdir.Dir(specdir.LiteralName("linux-amd64"), "",
									specdir.File(specdir.LiteralName("foo"), ""),
								),
							),
						),
						specdir.File(specdir.LiteralName("foo.txt"), ""),
					), true,
				)
				require.NoError(t, wantLayout.Validate(path.Join(tmpDir, "foo-1.0.0"), nil))
			},
		},
		distertest.TestCase{
			Name: "is able to create a valid TGZ archive containing long paths",
			ProjectCfg: distgoconfig.ProjectConfig{
				Products: distgoconfig.ToProductsMap(map[distgo.ProductID]distgoconfig.ProductConfig{
					"foo": {
						Build: distgoconfig.ToBuildConfig(&distgoconfig.BuildConfig{
							MainPkg: publishertest.StringPtr("foo"),
							OSArchs: &[]osarch.OSArch{
								distertest.MustOSArch("darwin-amd64"),
								distertest.MustOSArch("linux-amd64"),
							},
						}),
						Dist: distgoconfig.ToDistConfig(&distgoconfig.DistConfig{
							Disters: distgoconfig.ToDistersConfig(&distgoconfig.DistersConfig{
								bin.TypeName: {
									Type: publishertest.StringPtr(bin.TypeName),
									Script: publishertest.StringPtr(`#!/usr/bin/env bash
mkdir -p $DIST_WORK_DIR/0/1/2/3/4/5/6/7/8/9/10/11/12/13/14/15/16/17/18/19/20/21/22/23/24/25/26/27/28/29/30/31/32/33/
touch $DIST_WORK_DIR/0/1/2/3/4/5/6/7/8/9/10/11/12/13/14/15/16/17/18/19/20/21/22/23/24/25/26/27/28/29/30/31/32/33/file.txt
`),
								},
							}),
						}),
					},
				}),
			},
			WantOutput: func(projectDir string) string {
				return fmt.Sprintf(`Creating distribution for foo at %s/out/dist/foo/1.0.0/bin/foo-1.0.0.tgz
Finished creating bin distribution for foo
`, projectDir)
			},
			Validate: func(projectDir string) {
				tmpDir, err := ioutil.TempDir(projectDir, "expanded")
				require.NoError(t, err)
				require.NoError(t, archiver.TarGz.Open(path.Join(projectDir, "out", "dist", "foo", "1.0.0", "bin", "foo-1.0.0.tgz"), tmpDir))

				// long file in tgz should be expanded properly
				_, err = os.Stat(path.Join(tmpDir, "foo-1.0.0", "0/1/2/3/4/5/6/7/8/9/10/11/12/13/14/15/16/17/18/19/20/21/22/23/24/25/26/27/28/29/30/31/32/33/file.txt"))
				require.NoError(t, err, "unable to locate expected file")

				// stray file should not exist
				_, err = os.Stat(path.Join(tmpDir, "file.txt"))
				require.Error(t, err, "stray file exists")
			},
		},
	)
}
