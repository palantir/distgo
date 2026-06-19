// Copyright 2026 Palantir Technologies, Inc.
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

package productmavencoord_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/nmiyake/pkg/dirs"
	"github.com/palantir/distgo/distgo"
	distgoconfig "github.com/palantir/distgo/distgo/config"
	"github.com/palantir/distgo/distgo/productmavencoord"
	"github.com/palantir/distgo/distgo/testfuncs"
	"github.com/palantir/pkg/gittest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun(t *testing.T) {
	rootDir, cleanup, err := dirs.TempDir("", "")
	require.NoError(t, err)
	defer cleanup()

	for i, tc := range []struct {
		name                string
		projectCfg          distgoconfig.ProjectConfig
		specifiedProductIDs []distgo.ProductID
		setupProjectDir     func(projectDir string)
		want                string
		wantError           string
	}{
		{
			name: "prints maven coordinates for all products when no IDs specified",
			projectCfg: distgoconfig.ProjectConfig{
				Products: distgoconfig.ToProductsMap(map[distgo.ProductID]distgoconfig.ProductConfig{
					"foo": {
						Publish: distgoconfig.ToPublishConfig(&distgoconfig.PublishConfig{
							GroupID: new("com.example"),
						}),
					},
					"bar": {
						Publish: distgoconfig.ToPublishConfig(&distgoconfig.PublishConfig{
							GroupID: new("com.example"),
						}),
					},
				}),
			},
			specifiedProductIDs: nil,
			setupProjectDir: func(projectDir string) {
				gittest.CreateGitTag(t, projectDir, "1.0.0")
			},
			want: `com.example:bar:1.0.0
com.example:foo:1.0.0
`,
		},
		{
			name: "prints maven coordinates for specified products only",
			projectCfg: distgoconfig.ProjectConfig{
				Products: distgoconfig.ToProductsMap(map[distgo.ProductID]distgoconfig.ProductConfig{
					"foo": {
						Publish: distgoconfig.ToPublishConfig(&distgoconfig.PublishConfig{
							GroupID: new("com.example.foo"),
						}),
					},
					"bar": {
						Publish: distgoconfig.ToPublishConfig(&distgoconfig.PublishConfig{
							GroupID: new("com.example.bar"),
						}),
					},
					"baz": {
						Publish: distgoconfig.ToPublishConfig(&distgoconfig.PublishConfig{
							GroupID: new("com.example.baz"),
						}),
					},
				}),
			},
			specifiedProductIDs: []distgo.ProductID{"foo", "baz"},
			setupProjectDir: func(projectDir string) {
				gittest.CreateGitTag(t, projectDir, "2.1.0")
			},
			want: `com.example.foo:foo:2.1.0
com.example.baz:baz:2.1.0
`,
		},
		{
			name: "filters out non-existent product IDs",
			projectCfg: distgoconfig.ProjectConfig{
				Products: distgoconfig.ToProductsMap(map[distgo.ProductID]distgoconfig.ProductConfig{
					"foo": {
						Publish: distgoconfig.ToPublishConfig(&distgoconfig.PublishConfig{
							GroupID: new("com.example"),
						}),
					},
				}),
			},
			specifiedProductIDs: []distgo.ProductID{"foo", "nonexistent", "alsonotreal"},
			setupProjectDir: func(projectDir string) {
				gittest.CreateGitTag(t, projectDir, "1.5.0")
			},
			want: `com.example:foo:1.5.0
`,
		},
		{
			name: "prints nothing when only non-existent product IDs specified",
			projectCfg: distgoconfig.ProjectConfig{
				Products: distgoconfig.ToProductsMap(map[distgo.ProductID]distgoconfig.ProductConfig{
					"foo": {
						Publish: distgoconfig.ToPublishConfig(&distgoconfig.PublishConfig{
							GroupID: new("com.example"),
						}),
					},
				}),
			},
			specifiedProductIDs: []distgo.ProductID{"nonexistent"},
			setupProjectDir: func(projectDir string) {
				gittest.CreateGitTag(t, projectDir, "1.0.0")
			},
			want: "",
		},
		{
			name: "uses product name when specified instead of ID",
			projectCfg: distgoconfig.ProjectConfig{
				Products: distgoconfig.ToProductsMap(map[distgo.ProductID]distgoconfig.ProductConfig{
					"foo-id": {
						Name: new("foo-name"),
						Publish: distgoconfig.ToPublishConfig(&distgoconfig.PublishConfig{
							GroupID: new("com.example"),
						}),
					},
				}),
			},
			specifiedProductIDs: nil,
			setupProjectDir: func(projectDir string) {
				gittest.CreateGitTag(t, projectDir, "1.0.0")
			},
			want: `com.example:foo-name:1.0.0
`,
		},
		{
			name: "returns error when product has no group ID",
			projectCfg: distgoconfig.ProjectConfig{
				Products: distgoconfig.ToProductsMap(map[distgo.ProductID]distgoconfig.ProductConfig{
					"foo": {},
				}),
			},
			specifiedProductIDs: nil,
			setupProjectDir: func(projectDir string) {
				gittest.CreateGitTag(t, projectDir, "1.0.0")
			},
			wantError: "group-id",
		},
		{
			name: "prints multiple products with different group IDs in sorted order",
			projectCfg: distgoconfig.ProjectConfig{
				Products: distgoconfig.ToProductsMap(map[distgo.ProductID]distgoconfig.ProductConfig{
					"zebra": {
						Publish: distgoconfig.ToPublishConfig(&distgoconfig.PublishConfig{
							GroupID: new("com.zebra"),
						}),
					},
					"alpha": {
						Publish: distgoconfig.ToPublishConfig(&distgoconfig.PublishConfig{
							GroupID: new("com.alpha"),
						}),
					},
					"middle": {
						Publish: distgoconfig.ToPublishConfig(&distgoconfig.PublishConfig{
							GroupID: new("com.middle"),
						}),
					},
				}),
			},
			specifiedProductIDs: nil,
			setupProjectDir: func(projectDir string) {
				gittest.CreateGitTag(t, projectDir, "3.0.0")
			},
			want: `com.alpha:alpha:3.0.0
com.middle:middle:3.0.0
com.zebra:zebra:3.0.0
`,
		},
		{
			name: "handles empty products map",
			projectCfg: distgoconfig.ProjectConfig{
				Products: distgoconfig.ToProductsMap(map[distgo.ProductID]distgoconfig.ProductConfig{}),
			},
			specifiedProductIDs: nil,
			setupProjectDir: func(projectDir string) {
				gittest.CreateGitTag(t, projectDir, "1.0.0")
			},
			want: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			projectDir, err := os.MkdirTemp(rootDir, "")
			require.NoError(t, err, "Case %d: %s", i, tc.name)

			gittest.InitGitDir(t, projectDir)
			tc.setupProjectDir(projectDir)

			projectParam := testfuncs.NewProjectParam(t, tc.projectCfg, projectDir, "")
			projectInfo, err := projectParam.ProjectInfo(projectDir)
			require.NoError(t, err, "Case %d: %s", i, tc.name)

			buf := &bytes.Buffer{}
			err = productmavencoord.Run(projectInfo, projectParam, tc.specifiedProductIDs, buf)

			if tc.wantError != "" {
				require.Error(t, err, "Case %d: %s", i, tc.name)
				assert.Contains(t, err.Error(), tc.wantError, "Case %d: %s", i, tc.name)
			} else {
				require.NoError(t, err, "Case %d: %s", i, tc.name)
				assert.Equal(t, tc.want, buf.String(), "Case %d: %s", i, tc.name)
			}
		})
	}
}
