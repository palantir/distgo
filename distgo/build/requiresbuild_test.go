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

package build_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/distgo/build"
	"github.com/palantir/godel/v2/pkg/osarch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequiredTasksUsesProductBuildSettings(t *testing.T) {
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOTOOLCHAIN", "local")
	for _, tc := range []struct {
		name       string
		buildParam distgo.BuildParam
		wantReason string
	}{
		{
			name:       "missing build tag reports unloadable inputs",
			wantReason: "cannot inspect build inputs",
		},
		{
			name:       "build tag from environment",
			buildParam: distgo.BuildParam{Environment: map[string]string{"GOFLAGS": "-tags=custom"}},
		},
		{
			name:       "build tag from build args script",
			buildParam: distgo.BuildParam{BuildArgsScript: "#!/bin/sh\necho -tags\necho custom\n"},
		},
		{
			name:       "relative build flag resolved from project directory",
			buildParam: distgo.BuildParam{BuildArgsScript: "#!/bin/sh\necho -tags=custom\necho -modfile=alternate.mod\n"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			projectDir := t.TempDir()
			base := time.Unix(1_000_000_000, 0)
			writeFile(t, filepath.Join(projectDir, "go.mod"), "module example.com/fixture\n\ngo 1.21\n", base)
			writeFile(t, filepath.Join(projectDir, "alternate.mod"), "module example.com/fixture\n\ngo 1.21\n", base)
			writeFile(t, filepath.Join(projectDir, "foo", "main.go"), "//go:build custom\n\npackage main\n\nfunc main() {}\n", base)
			buildParam := tc.buildParam
			buildParam.NameTemplate, buildParam.OutputDir, buildParam.MainPkg = "{{Product}}", "out/build", "./foo"
			buildParam.OSArchs = []osarch.OSArch{osarch.Current()}
			product := distgo.ProductParam{ID: "foo", Name: "foo", Build: &buildParam}
			projectInfo := distgo.ProjectInfo{ProjectDir: projectDir, Version: "1.0.0"}
			info, err := distgo.ToProductTaskOutputInfo(projectInfo, product)
			require.NoError(t, err)
			writeFile(t, info.ProductBuildArtifactPaths()[osarch.Current()], "binary", base.Add(time.Minute))
			required, err := build.RequiredTasks(projectInfo, []distgo.ProductParam{product})
			require.NoError(t, err)
			if tc.wantReason == "" {
				assert.Empty(t, required)
				return
			}
			require.Len(t, required, 1)
			assert.Contains(t, required[0].Reason, tc.wantReason)
		})
	}
}

func TestRequiresBuildUsesProductBuildSettings(t *testing.T) {
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOTOOLCHAIN", "local")
	projectDir := t.TempDir()
	base := time.Unix(1_000_000_000, 0)
	writeFile(t, filepath.Join(projectDir, "go.mod"), "module example.com/fixture\n\ngo 1.21\n", base)
	writeFile(t, filepath.Join(projectDir, "foo", "main.go"), "package main\n\nfunc main() {}\n", base)
	// only included in the build with the custom tag, and newer than the binary
	writeFile(t, filepath.Join(projectDir, "foo", "tagged.go"), "//go:build custom\n\npackage main\n", base.Add(2*time.Minute))
	product := distgo.ProductParam{ID: "foo", Name: "foo", Build: &distgo.BuildParam{
		NameTemplate: "{{Product}}", OutputDir: "out/build", MainPkg: "./foo",
		OSArchs:     []osarch.OSArch{osarch.Current()},
		Environment: map[string]string{"GOFLAGS": "-tags=custom"},
	}}
	projectInfo := distgo.ProjectInfo{ProjectDir: projectDir, Version: "1.0.0"}
	info, err := distgo.ToProductTaskOutputInfo(projectInfo, product)
	require.NoError(t, err)
	writeFile(t, info.ProductBuildArtifactPaths()[osarch.Current()], "binary", base.Add(time.Minute))
	got, err := build.RequiresBuild(projectInfo, product)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, []osarch.OSArch{osarch.Current()}, got.Build.OSArchs)
}

func TestRequiresBuildRunsBuildArgsScriptOncePerProduct(t *testing.T) {
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOTOOLCHAIN", "local")
	projectDir := t.TempDir()
	base := time.Unix(1_000_000_000, 0)
	writeFile(t, filepath.Join(projectDir, "go.mod"), "module example.com/fixture\n\ngo 1.21\n", base)
	writeFile(t, filepath.Join(projectDir, "foo", "main.go"), "package main\n\nfunc main() {}\n", base)
	other, err := osarch.New("linux-arm64")
	require.NoError(t, err)
	if other == osarch.Current() {
		other, err = osarch.New("linux-amd64")
		require.NoError(t, err)
	}
	product := distgo.ProductParam{ID: "foo", Name: "foo", Build: &distgo.BuildParam{
		NameTemplate: "{{Product}}", OutputDir: "out/build", MainPkg: "./foo",
		OSArchs:         []osarch.OSArch{osarch.Current(), other},
		BuildArgsScript: "#!/bin/sh\necho run >> build-args-calls\n",
	}}
	projectInfo := distgo.ProjectInfo{ProjectDir: projectDir, Version: "1.0.0"}
	info, err := distgo.ToProductTaskOutputInfo(projectInfo, product)
	require.NoError(t, err)
	for _, output := range info.ProductBuildArtifactPaths() {
		writeFile(t, output, "binary", base.Add(time.Minute))
	}
	got, err := build.RequiresBuild(projectInfo, product)
	require.NoError(t, err)
	assert.Nil(t, got)
	calls, err := os.ReadFile(filepath.Join(projectDir, "build-args-calls"))
	require.NoError(t, err)
	assert.Equal(t, "run\n", string(calls))
}

func writeFile(t *testing.T, name, content string, modTime time.Time) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(name), 0755))
	require.NoError(t, os.WriteFile(name, []byte(content), 0644))
	require.NoError(t, os.Chtimes(name, modTime, modTime))
}
