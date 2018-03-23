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

package publishertest

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"testing"
	"time"

	"github.com/nmiyake/pkg/dirs"
	"github.com/palantir/godel/pkg/osarch"
	"github.com/palantir/pkg/gittest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/palantir/distgo/dister/disterfactory"
	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/distgo/config"
	"github.com/palantir/distgo/distgo/dist"
	"github.com/palantir/distgo/distgo/publish"
	"github.com/palantir/distgo/dockerbuilder/dockerbuilderfactory"
	"github.com/palantir/distgo/publisher/publisherfactory"
)

type TestCase struct {
	Name            string
	ProjectCfg      config.ProjectConfig
	WantOutput      func(projectDir string) string
	WantErrorRegexp string
}

func Run(t *testing.T, publisherImpl distgo.Publisher, dryRun bool, testCases ...TestCase) {
	tmp, cleanup, err := dirs.TempDir("", "")
	defer cleanup()
	require.NoError(t, err)

	for i, tc := range testCases {
		projectDir, err := ioutil.TempDir(tmp, "")
		require.NoError(t, err, "Case %d: %s", i, tc.Name)

		gittest.InitGitDir(t, projectDir)
		err = os.MkdirAll(path.Join(projectDir, "foo"), 0755)
		require.NoError(t, err, "Case %d: %s", i, tc.Name)
		err = ioutil.WriteFile(path.Join(projectDir, "foo", "main.go"), []byte("package main; func main(){}"), 0644)
		require.NoError(t, err, "Case %d: %s", i, tc.Name)
		err = ioutil.WriteFile(path.Join(projectDir, ".gitignore"), []byte("/out\n"), 0644)
		gittest.CommitAllFiles(t, projectDir, "Initial commit")
		gittest.CreateGitTag(t, projectDir, "1.0.0")

		disterFactory, err := disterfactory.New(nil, nil)
		require.NoError(t, err, "Case %d: %s", i, tc.Name)
		defaultDistCfg, err := disterfactory.DefaultConfig()
		require.NoError(t, err, "Case %d: %s", i, tc.Name)
		dockerBuilderFactory, err := dockerbuilderfactory.New(nil, nil)
		require.NoError(t, err, "Case %d: %s", i, tc.Name)
		publisherFactory, err := publisherfactory.New(nil, nil)
		require.NoError(t, err, "Case %d: %s", i, tc.Name)

		projectParam, err := tc.ProjectCfg.ToParam(projectDir, disterFactory, defaultDistCfg, dockerBuilderFactory, publisherFactory)
		require.NoError(t, err, "Case %d: %s", i, tc.Name)

		projectInfo, err := projectParam.ProjectInfo(projectDir)
		require.NoError(t, err, "Case %d: %s", i, tc.Name)

		// run "dist" to ensure that dist outputs exist
		preDistTime := time.Now().Truncate(time.Second).Add(-1 * time.Second)
		output := &bytes.Buffer{}
		err = dist.Products(projectInfo, projectParam, nil, nil, false, output)
		require.NoError(t, err, "Case %d: %s\nOutput: %s", i, tc.Name, output.String())

		output = &bytes.Buffer{}
		err = publish.Products(projectInfo, projectParam, &preDistTime, nil, publisherImpl, nil, dryRun, output)
		if tc.WantErrorRegexp == "" {
			require.NoError(t, err, "Case %d: %s", i, tc.Name)
			assert.Equal(t, tc.WantOutput(projectDir), output.String(), "Case %d: %s", i, tc.Name)
		} else {
			require.Error(t, err, fmt.Sprintf("Case %d: %s", i, tc.Name))
			assert.Regexp(t, regexp.MustCompile(tc.WantErrorRegexp), err.Error(), "Case %d: %s", i, tc.Name)
		}
	}
}

func StringPtr(in string) *string {
	return &in
}

func MustMapSlicePtr(in interface{}) *yaml.MapSlice {
	out, err := distgo.ToMapSlice(in)
	if err != nil {
		panic(err)
	}
	return &out
}

func MustOSArch(in string) osarch.OSArch {
	osArch, err := osarch.New(in)
	if err != nil {
		panic(err)
	}
	return osArch
}
