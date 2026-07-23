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

	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/publisher/internal/batchfixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAssetPublisherCreators_BatchCapability verifies that AssetPublisherCreators returns a [distgo.Publisher] that
// satisfies [distgo.BatchPublisher] for an asset that supports run-publish-batch and one that does not for
// an unsupported asset that does not support the command.
func TestAssetPublisherCreators_BatchCapability(t *testing.T) {
	batchingPath := batchfixtures.Build(t, batchfixtures.Batching)
	nonBatchPath := batchfixtures.Build(t, batchfixtures.NonBatch)
	unsupportedPath := batchfixtures.Build(t, batchfixtures.Unsupported)

	creators, _, err := AssetPublisherCreators(batchingPath, nonBatchPath, unsupportedPath)
	require.NoError(t, err)
	require.Len(t, creators, 3)

	byTypeName := make(map[string]distgo.Publisher)
	for _, creator := range creators {
		byTypeName[creator.TypeName()] = creator.Publisher()
	}

	_, ok := byTypeName["batching"].(distgo.BatchPublisher)
	assert.True(t, ok)

	_, ok = byTypeName["nonbatch"].(distgo.BatchPublisher)
	assert.True(t, ok)

	_, ok = byTypeName["unsupported"].(distgo.BatchPublisher)
	assert.False(t, ok)
}
