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

package vulncheck_test

import (
	"bytes"
	"testing"

	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/distgo/vulncheck"
	"github.com/palantir/godel/v2/pkg/osarch"
	"github.com/stretchr/testify/assert"
)

func TestProductVulncheckOutputPaths(t *testing.T) {
	projectInfo := distgo.ProjectInfo{
		ProjectDir: "/project",
		Version:    "1.0.0",
	}
	t.Run("returns correct paths when build info exists", func(t *testing.T) {
		productOutputInfo := distgo.ProductOutputInfo{
			ID:   "myapp",
			Name: "myapp",
			BuildOutputInfo: &distgo.BuildOutputInfo{
				BuildOutputDir: "out/build",
				MainPkg:        "./cmd/myapp",
				OSArchs:        []osarch.OSArch{{OS: "linux", Arch: "amd64"}},
			},
		}
		assert.Equal(t, "/project/out/vulncheck/myapp/1.0.0", distgo.ProductVulncheckOutputDir(projectInfo, productOutputInfo))
		assert.Equal(t, "/project/out/vulncheck/myapp/1.0.0/vex.json", distgo.ProductVulncheckVEXPath(projectInfo, productOutputInfo))
	})
	t.Run("returns empty when no build info", func(t *testing.T) {
		productOutputInfo := distgo.ProductOutputInfo{
			ID:   "myapp",
			Name: "myapp",
		}
		assert.Equal(t, "", distgo.ProductVulncheckOutputDir(projectInfo, productOutputInfo))
		assert.Equal(t, "", distgo.ProductVulncheckVEXPath(projectInfo, productOutputInfo))
	})
}

func TestProductsSkipsNoBuild(t *testing.T) {
	projectInfo := distgo.ProjectInfo{
		ProjectDir: t.TempDir(),
		Version:    "1.0.0",
	}
	projectParam := distgo.ProjectParam{
		Products: map[distgo.ProductID]distgo.ProductParam{
			"no-build": {
				ID:   "no-build",
				Name: "no-build",
				// Build is nil — should be skipped
			},
		},
	}

	var buf bytes.Buffer
	err := vulncheck.Products(projectInfo, projectParam, nil, vulncheck.Options{DryRun: true}, &buf)
	assert.NoError(t, err)
	// no output expected since the product has no build config
	assert.Empty(t, buf.String())
}

func TestProductsDryRun(t *testing.T) {
	projectInfo := distgo.ProjectInfo{
		ProjectDir: t.TempDir(),
		Version:    "1.0.0",
	}
	projectParam := distgo.ProjectParam{
		Products: map[distgo.ProductID]distgo.ProductParam{
			"myapp": {
				ID:   "myapp",
				Name: "myapp",
				Build: &distgo.BuildParam{
					MainPkg:   "./cmd/myapp",
					OutputDir: "out/build",
					OSArchs:   []osarch.OSArch{{OS: "linux", Arch: "amd64"}},
				},
			},
		},
	}

	var buf bytes.Buffer
	err := vulncheck.Products(projectInfo, projectParam, nil, vulncheck.Options{DryRun: true}, &buf)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Running vulncheck for myapp")
	assert.Contains(t, buf.String(), "govulncheck -format openvex ./cmd/myapp")
}

func TestProductsDryRunFiltersByProductID(t *testing.T) {
	projectInfo := distgo.ProjectInfo{
		ProjectDir: t.TempDir(),
		Version:    "1.0.0",
	}
	projectParam := distgo.ProjectParam{
		Products: map[distgo.ProductID]distgo.ProductParam{
			"app-a": {
				ID:   "app-a",
				Name: "app-a",
				Build: &distgo.BuildParam{
					MainPkg:   "./cmd/app-a",
					OutputDir: "out/build",
					OSArchs:   []osarch.OSArch{{OS: "linux", Arch: "amd64"}},
				},
			},
			"app-b": {
				ID:   "app-b",
				Name: "app-b",
				Build: &distgo.BuildParam{
					MainPkg:   "./cmd/app-b",
					OutputDir: "out/build",
					OSArchs:   []osarch.OSArch{{OS: "linux", Arch: "amd64"}},
				},
			},
		},
	}

	var buf bytes.Buffer
	err := vulncheck.Products(projectInfo, projectParam, []distgo.ProductID{"app-a"}, vulncheck.Options{DryRun: true}, &buf)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "app-a")
	assert.NotContains(t, buf.String(), "app-b")
}
