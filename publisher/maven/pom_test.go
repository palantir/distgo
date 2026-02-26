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

package maven

import (
	"testing"

	"github.com/palantir/distgo/distgo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderPOM(t *testing.T) {
	for i, tc := range []struct {
		name          string
		productID     string
		version       string
		groupID       string
		packagingType string
		git           gitParams
		want          string
	}{
		{
			"render POM without packaging",
			"foo",
			"1.0.0",
			"com.palantir",
			"",
			gitParams{},
			`<project xmlns="http://maven.apache.org/POM/4.0.0" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
  xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 http://maven.apache.org/xsd/maven-4.0.0.xsd">
  <modelVersion>4.0.0</modelVersion>

  <groupId>com.palantir</groupId>
  <artifactId>foo</artifactId>
  <version>1.0.0</version>
</project>
`,
		},
		{
			"render POM with packaging",
			"foo",
			"1.0.0",
			"com.palantir",
			"tgz",
			gitParams{},
			`<project xmlns="http://maven.apache.org/POM/4.0.0" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
  xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 http://maven.apache.org/xsd/maven-4.0.0.xsd">
  <modelVersion>4.0.0</modelVersion>

  <groupId>com.palantir</groupId>
  <artifactId>foo</artifactId>
  <version>1.0.0</version>
  <packaging>tgz</packaging>
</project>
`,
		},
		{
			"render POM with URLs",
			"foo",
			"1.0.0",
			"com.palantir",
			"",
			gitParams{
				gitURL: "git@github.com:palantir/distgo.git",
				webURL: "https://github.com/palantir/distgo",
			},
			`<project xmlns="http://maven.apache.org/POM/4.0.0" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
  xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 http://maven.apache.org/xsd/maven-4.0.0.xsd">
  <modelVersion>4.0.0</modelVersion>

  <groupId>com.palantir</groupId>
  <artifactId>foo</artifactId>
  <version>1.0.0</version>
  <scm><url>git@github.com:palantir/distgo.git</url></scm>
  <url>https://github.com/palantir/distgo</url>
</project>
`,
		},
		{
			"render POM with only git URL",
			"foo",
			"1.0.0",
			"com.palantir",
			"",
			gitParams{
				gitURL: "git@github.com:palantir/distgo.git",
			},
			`<project xmlns="http://maven.apache.org/POM/4.0.0" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
  xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 http://maven.apache.org/xsd/maven-4.0.0.xsd">
  <modelVersion>4.0.0</modelVersion>

  <groupId>com.palantir</groupId>
  <artifactId>foo</artifactId>
  <version>1.0.0</version>
  <scm><url>git@github.com:palantir/distgo.git</url></scm>
</project>
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := renderPOM(tc.productID, tc.version, tc.groupID, tc.packagingType, tc.git)
			require.NoError(t, err, "Case %d", i)
			assert.Equal(t, tc.want, got, "Case %d: %s\nOutput:\n%s", i, tc.name, got)
		})
	}
}

// TestPOM tests the functionality of the POM function, including specifying a packaging extension. Written by Claude
// with prompting to base on existing tests in the file.
func TestPOM(t *testing.T) {
	for _, tc := range []struct {
		name               string
		outputInfo         distgo.ProductTaskOutputInfo
		packagingExtension string
		wantErrorStr       string
	}{
		{
			"succeed if there is no dist output",
			distgo.ProductTaskOutputInfo{
				Product: distgo.ProductOutputInfo{
					ID:              "ProdID",
					Name:            "foo",
					DistOutputInfos: nil,
				},
				Project: distgo.ProjectInfo{
					Version: "1.0.0",
				},
			},
			"",
			"",
		},
		{
			"succeed if there is only a single dist",
			distgo.ProductTaskOutputInfo{
				Product: distgo.ProductOutputInfo{
					ID:   "ProdID",
					Name: "foo",
					DistOutputInfos: &distgo.DistOutputInfos{
						DistIDs: []distgo.DistID{"A"},
						DistInfos: map[distgo.DistID]distgo.DistOutputInfo{
							"A": {
								PackagingExtension: "tgz",
							},
						},
					},
				},
				Project: distgo.ProjectInfo{
					Version: "1.0.0",
				},
			},
			"",
			"",
		},
		{
			"succeed if there are multiple dists with the same packaging extension",
			distgo.ProductTaskOutputInfo{
				Product: distgo.ProductOutputInfo{
					ID:   "ProdID",
					Name: "foo",
					DistOutputInfos: &distgo.DistOutputInfos{
						DistIDs: []distgo.DistID{"A", "B"},
						DistInfos: map[distgo.DistID]distgo.DistOutputInfo{
							"A": {
								PackagingExtension: "tgz",
							},
							"B": {
								PackagingExtension: "tgz",
							},
						},
					},
				},
				Project: distgo.ProjectInfo{
					Version: "1.0.0",
				},
			},
			"",
			"",
		},
		{
			"fail if there are multiple dists with different packaging extensions",
			distgo.ProductTaskOutputInfo{
				Product: distgo.ProductOutputInfo{
					ID:   "ProdID",
					Name: "foo",
					DistOutputInfos: &distgo.DistOutputInfos{
						DistIDs: []distgo.DistID{"A", "B"},
						DistInfos: map[distgo.DistID]distgo.DistOutputInfo{
							"A": {
								PackagingExtension: "tgz",
							},
							"B": {
								PackagingExtension: "json",
							},
						},
					},
				},
				Project: distgo.ProjectInfo{
					Version: "1.0.0",
				},
			},
			"",
			"product ProdID has dists with different packaging extensions: distID A with packaging: tgz vs. distID B with packaging: json",
		},
		{
			"succeed if there are multiple dists but only one has a packaging extensions",
			distgo.ProductTaskOutputInfo{
				Product: distgo.ProductOutputInfo{
					ID:   "ProdID",
					Name: "foo",
					DistOutputInfos: &distgo.DistOutputInfos{
						DistIDs: []distgo.DistID{"A", "B"},
						DistInfos: map[distgo.DistID]distgo.DistOutputInfo{
							"A": {
								PackagingExtension: "tgz",
							},
							"B": {
								PackagingExtension: "",
							},
						},
					},
				},
				Project: distgo.ProjectInfo{
					Version: "1.0.0",
				},
			},
			"",
			"",
		},
		{
			"succeed if there are multiple dists with different packaging extensions and packaging extension is specified",
			distgo.ProductTaskOutputInfo{
				Product: distgo.ProductOutputInfo{
					ID:   "ProdID",
					Name: "foo",
					DistOutputInfos: &distgo.DistOutputInfos{
						DistIDs: []distgo.DistID{"A", "B"},
						DistInfos: map[distgo.DistID]distgo.DistOutputInfo{
							"A": {
								PackagingExtension: "tgz",
							},
							"B": {
								PackagingExtension: "json",
							},
						},
					},
				},
				Project: distgo.ProjectInfo{
					Version: "1.0.0",
				},
			},
			"tgz",
			"",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pomName, pomContent, err := POM("com.palantir", tc.outputInfo, tc.packagingExtension)
			if tc.wantErrorStr == "" {
				require.NoError(t, err)
				assert.Equal(t, "foo-1.0.0.pom", pomName)
				assert.NotEmpty(t, pomContent)
			} else {
				assert.EqualError(t, err, tc.wantErrorStr)
				assert.Empty(t, pomName)
				assert.Empty(t, pomContent)
			}
		})
	}
}

func TestGetSinglePackagingExtensionForProduct(t *testing.T) {
	for _, tc := range []struct {
		name         string
		outputInfo   distgo.ProductTaskOutputInfo
		wantErrorStr string
	}{
		{
			"succeed if there is no dist output",
			distgo.ProductTaskOutputInfo{
				Product: distgo.ProductOutputInfo{
					ID:              "ProdID",
					DistOutputInfos: nil,
				},
			},
			"",
		},
		{
			"succeed if there is only a single dist",
			distgo.ProductTaskOutputInfo{
				Product: distgo.ProductOutputInfo{
					ID: "ProdID",
					DistOutputInfos: &distgo.DistOutputInfos{
						DistIDs: []distgo.DistID{"A"},
						DistInfos: map[distgo.DistID]distgo.DistOutputInfo{
							"A": {
								PackagingExtension: "tgz",
							},
						},
					},
				},
			},
			"",
		},
		{
			"succeed if there are multiple dists with the same packaging extension",
			distgo.ProductTaskOutputInfo{
				Product: distgo.ProductOutputInfo{
					ID: "ProdID",
					DistOutputInfos: &distgo.DistOutputInfos{
						DistIDs: []distgo.DistID{"A", "B"},
						DistInfos: map[distgo.DistID]distgo.DistOutputInfo{
							"A": {
								PackagingExtension: "tgz",
							},
							"B": {
								PackagingExtension: "tgz",
							},
						},
					},
				},
			},
			"",
		},
		{
			"fail if there are multiple dists with different packaging extensions",
			distgo.ProductTaskOutputInfo{
				Product: distgo.ProductOutputInfo{
					ID: "ProdID",
					DistOutputInfos: &distgo.DistOutputInfos{
						DistIDs: []distgo.DistID{"A", "B"},
						DistInfos: map[distgo.DistID]distgo.DistOutputInfo{
							"A": {
								PackagingExtension: "tgz",
							},
							"B": {
								PackagingExtension: "json",
							},
						},
					},
				},
			},
			"product ProdID has dists with different packaging extensions: distID A with packaging: tgz vs. distID B with packaging: json",
		},
		{
			"succeed if there are multiple dists but only one has a packaging extensions",
			distgo.ProductTaskOutputInfo{
				Product: distgo.ProductOutputInfo{
					ID: "ProdID",
					DistOutputInfos: &distgo.DistOutputInfos{
						DistIDs: []distgo.DistID{"A", "B"},
						DistInfos: map[distgo.DistID]distgo.DistOutputInfo{
							"A": {
								PackagingExtension: "tgz",
							},
							"B": {
								PackagingExtension: "",
							},
						},
					},
				},
			},
			"",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := getSinglePackagingExtensionForProduct(tc.outputInfo)
			if tc.wantErrorStr == "" {
				require.NoError(t, err)
			} else {
				assert.EqualError(t, err, tc.wantErrorStr)
			}
		})
	}
}
