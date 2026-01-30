// Copyright 2016 Palantir Technologies, Inc.
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

package git_test

import (
	"os"
	"path"
	"regexp"
	"testing"

	"github.com/palantir/distgo/pkg/git"
	"github.com/palantir/pkg/gittest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectVersion(t *testing.T) {
	tmpDir := t.TempDir()

	for i, currCase := range []struct {
		gitOperations func(gitDir string)
		want          string
	}{
		{
			gitOperations: func(gitDir string) {},
			want:          "^unspecified$",
		},
		{
			gitOperations: func(gitDir string) {
				gittest.CreateBranch(t, gitDir, "featureBranch")
				gittest.CommitRandomFile(t, gitDir, "commit on feature branch")
				gittest.CreateGitTag(t, gitDir, "featureBranchTag1.0")
				gittest.RunGitCommand(t, gitDir, "checkout", "master")
			},
			want: "^unspecified$",
		},
		{
			gitOperations: func(gitDir string) {
				gittest.CommitRandomFile(t, gitDir, "Second commit")
				err := os.WriteFile(path.Join(gitDir, "foo"), []byte("foo"), 0644)
				require.NoError(t, err)
			},
			want: "^unspecified$",
		},
		{
			gitOperations: func(gitDir string) {
				gittest.CreateGitTag(t, gitDir, "0.0.1")
			},
			want: "^" + regexp.QuoteMeta("0.0.1") + "$",
		},
		{
			gitOperations: func(gitDir string) {
				gittest.CreateGitTag(t, gitDir, "1.0.1")
				gittest.CreateGitTag(t, gitDir, "0.0.1")
			},
			// if same commit has multiple tags, matches based on order returned by git
			want: "^" + regexp.QuoteMeta("0.0.1") + "$",
		},
		{
			gitOperations: func(gitDir string) {
				gittest.CreateGitTag(t, gitDir, "v0.0.1")
			},
			want: "^" + regexp.QuoteMeta("0.0.1") + "$",
		},
		{
			gitOperations: func(gitDir string) {
				gittest.CreateGitTag(t, gitDir, "version-string")
			},
			want: "^" + regexp.QuoteMeta("version-string") + "$",
		},
		{
			gitOperations: func(gitDir string) {
				gittest.CreateGitTag(t, gitDir, "0.0.1")
				err := os.WriteFile(path.Join(gitDir, "foo"), []byte("foo"), 0644)
				require.NoError(t, err)
			},
			want: "^" + regexp.QuoteMeta("0.0.1.dirty") + "$",
		},
		{
			gitOperations: func(gitDir string) {
				gittest.CreateGitTag(t, gitDir, "0.0.1")
				gittest.CommitRandomFile(t, gitDir, "Second commit")
			},
			want: "^" + regexp.QuoteMeta("0.0.1-1-g") + "[a-f0-9]{7}$",
		},
		{
			gitOperations: func(gitDir string) {
				gittest.CreateGitTag(t, gitDir, "0.0.1")
				gittest.CommitRandomFile(t, gitDir, "Second commit")
				err := os.WriteFile(path.Join(gitDir, "foo"), []byte("foo"), 0644)
				require.NoError(t, err)
			},
			want: "^" + regexp.QuoteMeta("0.0.1-1-g") + "[a-f0-9]{7}" + regexp.QuoteMeta(".dirty") + "$",
		},
		{
			gitOperations: func(gitDir string) {
				gittest.CreateGitTag(t, gitDir, "v1.0.0")
			},
			want: "^" + regexp.QuoteMeta("1.0.0") + "$",
		},
		{
			gitOperations: func(gitDir string) {
				gittest.CreateGitTag(t, gitDir, "0.0.1")

				gittest.CreateBranch(t, gitDir, "hotfix-branch")
				gittest.CommitRandomFile(t, gitDir, "hotfix commit")
				gittest.CreateGitTag(t, gitDir, "0.0.1-hotfix")

				gittest.RunGitCommand(t, gitDir, "checkout", "master")
				gittest.Merge(t, gitDir, "hotfix-branch")
			},
			want: "^" + regexp.QuoteMeta("0.0.1-1-g") + "[a-f0-9]{7}$",
		},
		{
			gitOperations: func(gitDir string) {
				gittest.CreateGitTag(t, gitDir, "0.0.1")

				gittest.CreateBranch(t, gitDir, "hotfix-branch")
				gittest.CommitRandomFile(t, gitDir, "hotfix commit")
				gittest.CreateGitTag(t, gitDir, "0.0.1-hotfix")

				gittest.RunGitCommand(t, gitDir, "checkout", "master")
				gittest.Merge(t, gitDir, "hotfix-branch")

				gittest.CreateGitTag(t, gitDir, "0.0.2")
			},
			want: "^" + regexp.QuoteMeta("0.0.2") + "$",
		},
	} {
		currTmp, err := os.MkdirTemp(tmpDir, "")
		require.NoError(t, err)

		gittest.InitGitDir(t, currTmp)
		currCase.gitOperations(currTmp)

		got, err := git.ProjectVersion(currTmp)
		require.NoError(t, err)

		assert.Regexp(t, currCase.want, got, "Case %d", i)
	}
}

func TestProjectVersionWithPrefix(t *testing.T) {
	tmpDir := t.TempDir()

	for i, currCase := range []struct {
		gitOperations func(gitDir string)
		tagPrefix     string
		want          string
	}{
		{
			gitOperations: func(gitDir string) {},
			want:          "^unspecified$",
		},
		{
			gitOperations: func(gitDir string) {
				gittest.CreateGitTag(t, gitDir, "@org/product-1@0.0.1")
				gittest.CreateGitTag(t, gitDir, "@org/product-2@0.0.2")
			},
			tagPrefix: "@org/product-2@",
			want:      "^" + regexp.QuoteMeta("@org/product-2@0.0.2") + "$",
		},
		{
			gitOperations: func(gitDir string) {
				gittest.CreateGitTag(t, gitDir, "@org/product-1@0.0.1")
				gittest.CommitRandomFile(t, gitDir, "first modification")

				gittest.CommitRandomFile(t, gitDir, "second modification")
				gittest.CreateGitTag(t, gitDir, "@org/product-2@0.0.2")

				gittest.CommitRandomFile(t, gitDir, "third modification")
			},
			tagPrefix: "@org/product-1@",
			want:      "^" + regexp.QuoteMeta("@org/product-1@0.0.1-3-g") + "[a-f0-9]{7}$",
		},
		{
			gitOperations: func(gitDir string) {
				gittest.CreateGitTag(t, gitDir, "@org/product-1@0.0.1")
				gittest.CommitRandomFile(t, gitDir, "first modification")

				gittest.CommitRandomFile(t, gitDir, "second modification")
				gittest.CreateGitTag(t, gitDir, "@org/product-2@0.0.2")

				gittest.CommitRandomFile(t, gitDir, "third modification")
			},
			tagPrefix: "@org/product-2@",
			want:      "^" + regexp.QuoteMeta("@org/product-2@0.0.2-1-g") + "[a-f0-9]{7}$",
		},
	} {
		currTmp, err := os.MkdirTemp(tmpDir, "")
		require.NoError(t, err)

		gittest.InitGitDir(t, currTmp)
		currCase.gitOperations(currTmp)

		got, err := git.ProjectVersionWithPrefix(currTmp, currCase.tagPrefix)
		require.NoError(t, err)

		assert.Regexp(t, currCase.want, got, "Case %d", i)
	}
}
