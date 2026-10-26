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

package integration_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/palantir/godel/pkg/products/v2"
	"github.com/palantir/pkg/gittest"
	"github.com/stretchr/testify/require"
)

func TestNoDependentTasksCLI(t *testing.T) {
	cli, err := products.Bin("dist-plugin")
	require.NoError(t, err)
	dir := t.TempDir()
	gittest.InitGitDir(t, dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/fixture\n\ngo 1.21\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0644))
	config := filepath.Join(dir, "dist.yml")
	require.NoError(t, os.WriteFile(config, []byte(`products:
  foo:
    build:
      main-pkg: .
      script: |
        #!/bin/sh
        touch build-hook
    dist:
      disters:
        bin:
          type: bin
`), 0644))
	gittest.CommitAllFiles(t, dir, "Fixture")
	for _, task := range [][]string{{"dist"}, {"docker", "build"}} {
		args := append(append([]string(nil), task...), "--help")
		output, err := exec.Command(cli, args...).CombinedOutput()
		require.NoError(t, err, string(output))
		require.Contains(t, string(output), "--no-dependent-tasks")
	}
	command := exec.Command(cli, "--project-dir", dir, "--config", config, "dist", "--no-dependent-tasks", "--dry-run")
	output, err := command.CombinedOutput()
	require.Error(t, err)
	require.Contains(t, string(output), "--no-dependent-tasks")
	require.Contains(t, string(output), "build required for foo")
	_, err = os.Stat(filepath.Join(dir, "build-hook"))
	require.True(t, os.IsNotExist(err))

	// Explicit false preserves normal dry-run behavior, including its existing build-script execution.
	command = exec.Command(cli, "--project-dir", dir, "--config", config, "dist", "--no-dependent-tasks=false", "--dry-run")
	output, err = command.CombinedOutput()
	require.NoError(t, err, string(output))
	_, err = os.Stat(filepath.Join(dir, "build-hook"))
	require.NoError(t, err)
}
