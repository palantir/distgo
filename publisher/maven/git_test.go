// Copyright 2022 Palantir Technologies, Inc.
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

	"github.com/stretchr/testify/assert"
)

func TestParseRepoOrigin(t *testing.T) {
	for _, tc := range []struct {
		Remote string
		URL    string
	}{
		{
			Remote: "github.com:palantir/distgo.git",
			URL:    "https://github.com/palantir/distgo",
		},
		{
			Remote: "git@github.com:palantir/distgo.git",
			URL:    "https://github.com/palantir/distgo",
		},
		{
			Remote: "ssh://git@github.com/palantir/distgo.git",
			URL:    "https://github.com/palantir/distgo",
		},
		{
			Remote: "ssh://git@github.com:8443/palantir/distgo.git",
			URL:    "https://github.com:8443/palantir/distgo",
		},
		{
			Remote: "https://github.com/palantir/distgo.git",
			URL:    "https://github.com/palantir/distgo",
		},
		{
			Remote: "https://github.com/palantir/distgo.git/",
			URL:    "https://github.com/palantir/distgo",
		},
		{
			Remote: "https://github.com/palantir/distgo",
			URL:    "https://github.com/palantir/distgo",
		},
	} {
		t.Run(tc.Remote, func(t *testing.T) {
			out := parseRepoOrigin(tc.Remote)
			assert.Equal(t, tc.Remote, out.gitURL)
			assert.Equal(t, tc.URL, out.webURL)
		})
	}
}
