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

package git

import (
	"testing"

	"github.com/palantir/pkg/gittest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		{Remote: "/tmp/local.git", URL: "https:///tmp/local"},
		{Remote: "../local.git/", URL: "https://../local"},
		{Remote: "file:///tmp/local.git", URL: "https:///tmp/local"},
	} {
		t.Run(tc.Remote, func(t *testing.T) {
			gitURL, webURL := parseRepoOrigin(tc.Remote)
			assert.Equal(t, tc.Remote, gitURL)
			assert.Equal(t, tc.URL, webURL)
		})
	}
}

func TestRepoOrigin(t *testing.T) {
	t.Run("not a repository", func(t *testing.T) {
		gitURL, webURL := RepoOrigin(t.TempDir())
		assert.Empty(t, gitURL)
		assert.Empty(t, webURL)
	})
	t.Run("repository origin", func(t *testing.T) {
		dir := t.TempDir()
		gittest.RunGitCommand(t, dir, "init")
		gitURL, webURL := RepoOrigin(dir)
		assert.Empty(t, gitURL)
		assert.Empty(t, webURL)

		gittest.RunGitCommand(t, dir, "remote", "add", "origin", "git@github.com:example/project.git")
		gitURL, webURL = RepoOrigin(dir)
		require.Equal(t, "git@github.com:example/project.git", gitURL)
		assert.Equal(t, "https://github.com/example/project", webURL)
	})
}
