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
	"testing"

	"github.com/palantir/distgo/publisher/internal/publishfixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAssetPublisherCreators_V2Capability verifies that AssetPublisherCreators constructs the run-publish-v2 wrapper
// for an asset that supports the command and falls back to the legacy wrapper for one that does not.
func TestAssetPublisherCreators_V2Capability(t *testing.T) {
	v2Path := publishfixtures.Build(t, publishfixtures.V2)
	legacyOnlyPath := publishfixtures.Build(t, publishfixtures.LegacyOnly)

	creators, _, err := AssetPublisherCreators(v2Path, legacyOnlyPath)
	require.NoError(t, err)
	require.Len(t, creators, 2)

	byTypeName := make(map[string]any)
	for _, creator := range creators {
		byTypeName[creator.TypeName()] = creator.Publisher()
	}

	_, ok := byTypeName["v2"].(*assetPublisher)
	assert.True(t, ok)

	_, ok = byTypeName["legacyonly"].(*legacyAssetPublisher)
	assert.True(t, ok)
}
