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

package osarchbin_test

import (
	"fmt"
	"testing"

	"github.com/palantir/godel/pkg/osarch"
	"github.com/palantir/pkg/specdir"

	"github.com/palantir/distgo/dister/distertest"
	"github.com/palantir/distgo/dister/osarchbin"
	"github.com/palantir/distgo/dister/osarchbin/config"
	"github.com/palantir/distgo/distgo"
	distgoconfig "github.com/palantir/distgo/distgo/config"
	"github.com/palantir/distgo/publisher/publishertest"
)

func TestDister(t *testing.T) {
	distertest.Run(t, false,
		distertest.TestCase{
			Name: "os-arch-bin creates expected output",
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
								osarchbin.TypeName: {
									Type: publishertest.StringPtr(osarchbin.TypeName),
									Config: distertest.MustMapSlicePtr(
										config.OSArchBin{
											OSArchs: []osarch.OSArch{
												distertest.MustOSArch("darwin-amd64"),
												distertest.MustOSArch("linux-amd64"),
											},
										},
									),
								},
							}),
						}),
					},
				}),
			},
			WantOutput: func(projectDir string) string {
				return fmt.Sprintf(`Creating distribution for foo at %s/out/dist/foo/1.0.0/os-arch-bin/foo-1.0.0-darwin-amd64.tgz, %s/out/dist/foo/1.0.0/os-arch-bin/foo-1.0.0-linux-amd64.tgz
Finished creating os-arch-bin distribution for foo
`, projectDir, projectDir)
			},
			WantLayout: specdir.NewLayoutSpec(
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
			),
		},
	)
}

//s.Dir(s.CompositeName(s.LiteralName(AppName+"-"), s.TemplateName(versionTemplate)), AppDir,
//	s.Dir(s.LiteralName("bin"), "",
//		s.Dir(s.CompositeName(s.TemplateName(osTemplate), s.LiteralName("-"), s.TemplateName(archTemplate)), "",
//			s.File(s.LiteralName(AppName), AppExecutable),
//		),
//	),
//	WrapperSpec(),
//),
