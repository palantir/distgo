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

package publisher

import (
	"bytes"
	"testing"

	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/publisher/internal/publishfixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssetSupportsV2Publish(t *testing.T) {
	v2Path := publishfixtures.Build(t, publishfixtures.V2)
	legacyOnlyPath := publishfixtures.Build(t, publishfixtures.LegacyOnly)

	assert.True(t, assetSupportsV2Publish(v2Path))
	assert.False(t, assetSupportsV2Publish(legacyOnlyPath))
}

// TestAssetPublisher_RunPublish verifies that assetPublisher marshals the full batch of inputs once, passes it to
// the run-publish-v2 command as a flag, and streams the asset's output back.
func TestAssetPublisher_RunPublish(t *testing.T) {
	v2Path := publishfixtures.Build(t, publishfixtures.V2)

	p := &assetPublisher{
		assetPath: v2Path,
	}

	typeName, err := p.TypeName()
	require.NoError(t, err)
	assert.Equal(t, "v2", typeName)

	inputs := []distgo.ProductPublishInfo{
		{ProductTaskOutputInfo: distgo.ProductTaskOutputInfo{Product: distgo.ProductOutputInfo{ID: "foo"}}},
		{ProductTaskOutputInfo: distgo.ProductTaskOutputInfo{Product: distgo.ProductOutputInfo{ID: "bar"}}},
	}
	var stdout bytes.Buffer
	err = p.RunPublish(inputs, nil, false, &stdout)
	require.NoError(t, err)
	assert.Equal(t, "RunPublish:foo,bar\n", stdout.String())
}

// TestLegacyAssetPublisher_RunPublish verifies that legacyAssetPublisher falls back to invoking the legacy
// run-publish command once per input.
func TestLegacyAssetPublisher_RunPublish(t *testing.T) {
	v2Path := publishfixtures.Build(t, publishfixtures.V2)

	p := &legacyAssetPublisher{
		assetPublisher{
			assetPath: v2Path,
		},
	}

	inputs := []distgo.ProductPublishInfo{
		{ProductTaskOutputInfo: distgo.ProductTaskOutputInfo{Product: distgo.ProductOutputInfo{ID: "foo"}}},
		{ProductTaskOutputInfo: distgo.ProductTaskOutputInfo{Product: distgo.ProductOutputInfo{ID: "bar"}}},
	}
	var stdout bytes.Buffer
	err := p.RunPublish(inputs, nil, false, &stdout)
	require.NoError(t, err)
	assert.Equal(t, "RunPublish:foo\nRunPublish:bar\n", stdout.String())
}
