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
	"net/url"
	"os/exec"
	"strings"

	giturls "github.com/whilp/git-urls"
)

type gitParams struct {
	gitURL string
	webURL string
}

func (g gitParams) GitURL() string {
	return g.gitURL
}

func (g gitParams) WebURL() string {
	return g.webURL
}

func getRepoOrigin() gitParams {
	out, err := exec.Command("git", "remote", "get-url", "origin").CombinedOutput()
	if err != nil {
		// TODO: Log warning?
		return gitParams{}
	}
	remote := strings.TrimSpace(string(out))
	if len(remote) == 0 {
		return gitParams{}
	}
	return parseRepoOrigin(remote)
}

func parseRepoOrigin(remote string) gitParams {
	u, err := giturls.Parse(remote)
	if err != nil {
		return gitParams{
			gitURL: remote,
		}
	}
	path := strings.TrimSuffix(strings.TrimSuffix(u.Path, "/"), ".git")
	webURL := (&url.URL{Scheme: "https", Host: u.Host, Path: path}).String()
	return gitParams{
		gitURL: remote,
		webURL: webURL,
	}
}
