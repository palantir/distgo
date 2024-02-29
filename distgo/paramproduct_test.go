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
	for _, tc := range []struct {
		name     string
		in       distgo.ProductTaskOutputInfo
		expected distgo.ProductTaskOutputInfo
	}{
		{
			name: "product name is set - noop",
			in: distgo.ProductTaskOutputInfo{
				Product: distgo.ProductOutputInfo{
					ID:   "foo",
					Name: "bar",
				},
			},
			expected: distgo.ProductTaskOutputInfo{
				Product: distgo.ProductOutputInfo{
					ID:   "foo",
					Name: "bar",
				},
			},
		},
		{
			name: "product name not set - set to product ID",
			in: distgo.ProductTaskOutputInfo{
				Product: distgo.ProductOutputInfo{
					ID: "foo",
				},
			},
			expected: distgo.ProductTaskOutputInfo{
				Product: distgo.ProductOutputInfo{
					ID:   "foo",
					Name: "foo",
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			jsonBytes, err := json.Marshal(tc.in)
			require.NoError(t, err)

			var ptoi distgo.ProductTaskOutputInfo
			err = json.Unmarshal(jsonBytes, &ptoi)

			assert.NoError(t, err)
			assert.Equal(t, tc.expected, ptoi)
		})
	}
}
