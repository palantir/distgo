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

package integration_test

import (
	"fmt"
	"testing"

	"github.com/nmiyake/pkg/gofiles"
	"github.com/palantir/distgo/publisher/internal/batchfixtures"
	"github.com/palantir/distgo/publisher/publishertester"
	"github.com/palantir/godel/v2/framework/pluginapitester"
	"github.com/palantir/godel/v2/pkg/products"
	"github.com/stretchr/testify/require"
)

const godelYML = `exclude:
  names:
    - "\\..+"
    - "vendor"
  paths:
    - "godel"
`

func distPluginYML(publishTypeName string) string {
	return fmt.Sprintf(`
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
        %s:
          config: {}
  bar:
    build:
      main-pkg: ./foo
    dist:
      disters:
        type: os-arch-bin
    publish:
      group-id: com.test.group
      info:
        %s:
          config: {}
`, publishTypeName, publishTypeName)
}

var specs = []gofiles.GoFileSpec{
	{
		RelPath: "go.mod",
		Src:     `module foo`,
	},
	{
		RelPath: "foo/foo.go",
		Src:     `package main; func main() {}`,
	},
}

// TestBatchFixtureBatching verifies that a two-product publish against an asset that implements
// distgo.BatchPublisher produces exactly one combined RunPublishBatch line.
func TestBatchFixtureBatching(t *testing.T) {
	pluginPath, err := products.Bin("dist-plugin")
	require.NoError(t, err)
	assetPath := batchfixtures.Build(t, batchfixtures.Batching)

	publishertester.RunAssetPublishTest(t,
		pluginapitester.NewPluginProvider(pluginPath),
		pluginapitester.NewAssetProvider(assetPath),
		"batching",
		[]publishertester.TestCase{
			{
				Name:        "coordinates both products in a single RunPublishBatch call",
				Specs:       specs,
				ConfigFiles: map[string]string{"godel/config/godel.yml": godelYML, "godel/config/dist-plugin.yml": distPluginYML("batching")},
				WantOutput: func(projectDir string) string {
					return "RunPublishBatch:bar,foo\n"
				},
			},
		},
	)
}

// TestBatchFixtureNonBatch verifies that a two-product publish against an asset that does not implement batching, but
// supports the command, invokes the run-publish-batch but falls back to the RunPublish loop.
func TestBatchFixtureNonBatch(t *testing.T) {
	pluginPath, err := products.Bin("dist-plugin")
	require.NoError(t, err)
	assetPath := batchfixtures.Build(t, batchfixtures.NonBatch)

	publishertester.RunAssetPublishTest(t,
		pluginapitester.NewPluginProvider(pluginPath),
		pluginapitester.NewAssetProvider(assetPath),
		"nonbatch",
		[]publishertester.TestCase{
			{
				Name:        "publishes both products correctly via the default per-input loop",
				Specs:       specs,
				ConfigFiles: map[string]string{"godel/config/godel.yml": godelYML, "godel/config/dist-plugin.yml": distPluginYML("nonbatch")},
				WantOutput: func(projectDir string) string {
					return "RunPublish:bar\nRunPublish:foo\n"
				},
			},
		},
	)
}

// TestBatchFixtureUnsupported verifies that a two-product publish against an asset that does not support batch
// publishing at all, runs through the existing run-publish loop instead of the run-publish-batch fallback.
func TestBatchFixtureUnsupported(t *testing.T) {
	pluginPath, err := products.Bin("dist-plugin")
	require.NoError(t, err)
	assetPath := batchfixtures.Build(t, batchfixtures.Unsupported)

	publishertester.RunAssetPublishTest(t,
		pluginapitester.NewPluginProvider(pluginPath),
		pluginapitester.NewAssetProvider(assetPath),
		"unsupported",
		[]publishertester.TestCase{
			{
				Name:        "publishes both products correctly via the host's existing per-product loop",
				Specs:       specs,
				ConfigFiles: map[string]string{"godel/config/godel.yml": godelYML, "godel/config/dist-plugin.yml": distPluginYML("unsupported")},
				WantOutput: func(projectDir string) string {
					return "RunPublish:bar\nRunPublish:foo\n"
				},
			},
		},
	)
}
