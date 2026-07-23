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
	"github.com/palantir/distgo/publisher/internal/batchfixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssetSupportsBatchPublish(t *testing.T) {
	batchingPath := batchfixtures.Build(t, batchfixtures.Batching)
	nonBatchPath := batchfixtures.Build(t, batchfixtures.NonBatch)
	unsupportedPath := batchfixtures.Build(t, batchfixtures.Unsupported)

	assert.True(t, assetSupportsBatchPublish(batchingPath))
	assert.True(t, assetSupportsBatchPublish(nonBatchPath))
	assert.False(t, assetSupportsBatchPublish(unsupportedPath))
}

// TestBatchAssetPublisher_RunPublishBatch verifies that batchAssetPublisher correctly marshals inputs, invokes the
// run-publish-batch command on the underlying asset, and streams its output back.
func TestBatchAssetPublisher_RunPublishBatch(t *testing.T) {
	batchingPath := batchfixtures.Build(t, batchfixtures.Batching)

	p := &batchAssetPublisher{
		assetPublisher{
			assetPath: batchingPath,
		},
	}

	typeName, err := p.TypeName()
	require.NoError(t, err)
	assert.Equal(t, "batching", typeName)

	inputs := []distgo.BatchPublishInput{
		{ProductTaskOutputInfo: distgo.ProductTaskOutputInfo{Product: distgo.ProductOutputInfo{ID: "foo"}}},
		{ProductTaskOutputInfo: distgo.ProductTaskOutputInfo{Product: distgo.ProductOutputInfo{ID: "bar"}}},
	}
	var stdout bytes.Buffer
	err = p.RunPublishBatch(inputs, nil, false, &stdout)
	require.NoError(t, err)
	assert.Equal(t, "RunPublishBatch:bar,foo\n", stdout.String())
}
