// Copyright 2024 Palantir Technologies, Inc.
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

package distgo_test

import (
	"encoding/json"
	"testing"

	"github.com/palantir/distgo/distgo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnmarshalProductTaskOutputInfo(t *testing.T) {
	jsonBytesWithProductName, err := json.Marshal(distgo.ProductTaskOutputInfo{
		Product: distgo.ProductOutputInfo{
			ID:   "foo",
			Name: "bar",
		},
	})
	require.NoError(t, err)
	var ptoi distgo.ProductTaskOutputInfo
	err = json.Unmarshal(jsonBytesWithProductName, &ptoi)
	assert.NoError(t, err)
	assert.Equal(t, distgo.ProductTaskOutputInfo{
		Product: distgo.ProductOutputInfo{
			ID:   "foo",
			Name: "bar",
		},
	}, ptoi)

	jsonBytesWithoutProductName, err := json.Marshal(distgo.ProductTaskOutputInfo{
		Product: distgo.ProductOutputInfo{
			ID: "foo",
		},
	})
	require.NoError(t, err)
	err = json.Unmarshal(jsonBytesWithoutProductName, &ptoi)
	assert.NoError(t, err)
	assert.Equal(t, distgo.ProductTaskOutputInfo{
		Product: distgo.ProductOutputInfo{
			ID:   "foo",
			Name: "foo",
		},
	}, ptoi)
}
