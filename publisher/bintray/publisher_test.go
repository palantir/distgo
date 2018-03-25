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

package bintray_test

import (
	"fmt"
	"testing"

	"github.com/palantir/godel/pkg/osarch"

	"github.com/palantir/distgo/dister/osarchbin"
	"github.com/palantir/distgo/distgo"
	distgoconfig "github.com/palantir/distgo/distgo/config"
	"github.com/palantir/distgo/publisher"
	"github.com/palantir/distgo/publisher/bintray"
	bintrayconfig "github.com/palantir/distgo/publisher/bintray/config"
	"github.com/palantir/distgo/publisher/publishertest"
)

func TestPublisher(t *testing.T) {
	publishertest.Run(t, bintray.PublisherCreator().Publisher(), true,
		publishertest.TestCase{
			Name: "publishes artifact to Bintray",
			ProjectCfg: distgoconfig.ProjectConfig{
				Products: distgoconfig.ToProductsMap(map[distgo.ProductID]distgoconfig.ProductConfig{
					"foo": {
						Build: distgoconfig.ToBuildConfig(&distgoconfig.BuildConfig{
							MainPkg: publishertest.StringPtr("foo"),
						}),
						Dist: distgoconfig.ToDistConfig(&distgoconfig.DistConfig{
							Disters: distgoconfig.ToDistersConfig(&distgoconfig.DistersConfig{
								osarchbin.TypeName: {
									Type: publishertest.StringPtr(osarchbin.TypeName),
								},
							}),
						}),
						Publish: distgoconfig.ToPublishConfig(&distgoconfig.PublishConfig{
							GroupID: publishertest.StringPtr("com.test.group"),
							PublishInfo: distgoconfig.ToPublishInfo(
								&map[distgo.PublisherTypeID]distgoconfig.PublisherConfig{
									bintray.TypeName: {
										Config: publishertest.MustMapSlicePtr(bintrayconfig.Bintray{
											BasicConnectionInfo: publisher.BasicConnectionInfo{
												URL:      "http://bintray.domain.com",
												Username: "testUsername",
												Password: "testPassword",
											},
											Subject:       "testSubject",
											Repository:    "testRepo",
											Product:       "testProduct",
											Publish:       true,
											DownloadsList: true,
										}),
									},
								},
							),
						}),
					},
				}),
			},
			WantOutput: func(projectDir string) string {
				return fmt.Sprintf(`[DRY RUN] Uploading %s/out/dist/foo/1.0.0/os-arch-bin/foo-1.0.0-%s.tgz to http://bintray.domain.com/content/testSubject/testRepo/testProduct/1.0.0/com/test/group/foo/1.0.0/foo-1.0.0-%s.tgz
[DRY RUN] Running Bintray publish for uploaded artifacts...done
[DRY RUN] Adding artifact to Bintray downloads list for package...done
`, projectDir, osarch.Current().String(), osarch.Current().String())
			},
		},
		publishertest.TestCase{
			Name: "omitting product configuration defaults to ProductID",
			ProjectCfg: distgoconfig.ProjectConfig{
				Products: distgoconfig.ToProductsMap(map[distgo.ProductID]distgoconfig.ProductConfig{
					"foo": {
						Build: distgoconfig.ToBuildConfig(&distgoconfig.BuildConfig{
							MainPkg: publishertest.StringPtr("foo"),
						}),
						Dist: distgoconfig.ToDistConfig(&distgoconfig.DistConfig{
							Disters: distgoconfig.ToDistersConfig(&distgoconfig.DistersConfig{
								osarchbin.TypeName: {
									Type: publishertest.StringPtr(osarchbin.TypeName),
								},
							}),
						}),
						Publish: distgoconfig.ToPublishConfig(&distgoconfig.PublishConfig{
							GroupID: publishertest.StringPtr("com.test.group"),
							PublishInfo: distgoconfig.ToPublishInfo(
								&map[distgo.PublisherTypeID]distgoconfig.PublisherConfig{
									bintray.TypeName: {
										Config: publishertest.MustMapSlicePtr(bintrayconfig.Bintray{
											BasicConnectionInfo: publisher.BasicConnectionInfo{
												URL:      "http://bintray.domain.com",
												Username: "testUsername",
												Password: "testPassword",
											},
											Subject:       "testSubject",
											Repository:    "testRepo",
											Publish:       true,
											DownloadsList: true,
										}),
									},
								},
							),
						}),
					},
				}),
			},
			WantOutput: func(projectDir string) string {
				return fmt.Sprintf(`[DRY RUN] Uploading %s/out/dist/foo/1.0.0/os-arch-bin/foo-1.0.0-%s.tgz to http://bintray.domain.com/content/testSubject/testRepo/foo/1.0.0/com/test/group/foo/1.0.0/foo-1.0.0-%s.tgz
[DRY RUN] Running Bintray publish for uploaded artifacts...done
[DRY RUN] Adding artifact to Bintray downloads list for package...done
`, projectDir, osarch.Current().String(), osarch.Current().String())
			},
		},
	)
}
