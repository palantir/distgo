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

	"github.com/palantir/distgo/publisher/publisherfactory"
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
          script: |
            #!/bin/sh
            touch dist-hook
`), 0644))
	gittest.CommitAllFiles(t, dir, "Fixture")
	publishers, err := publisherfactory.New(nil, nil)
	require.NoError(t, err)
	tasks := [][]string{{"dist"}, {"docker", "build"}}
	for _, publisherType := range publishers.Types() {
		tasks = append(tasks, []string{"publish", publisherType})
	}
	for _, task := range tasks {
		args := append(append([]string(nil), task...), "--help")
		output, err := exec.Command(cli, args...).CombinedOutput()
		require.NoError(t, err, string(output))
		require.Contains(t, string(output), "--no-dependent-tasks")
		require.Contains(t, string(output), "--go-cache-root")
	}
	for _, task := range append([][]string{{"dist"}}, tasks[2:]...) {
		args := append([]string{"--project-dir", dir, "--config", config}, task...)
		args = append(args, "--no-dependent-tasks", "--dry-run")
		output, err := exec.Command(cli, args...).CombinedOutput()
		require.Error(t, err, string(output))
		require.Contains(t, string(output), "--no-dependent-tasks")
		require.Contains(t, string(output), "build required for foo")
		require.NoFileExists(t, filepath.Join(dir, "build-hook"))
		require.NoFileExists(t, filepath.Join(dir, "dist-hook"))
	}

	// Explicit false preserves normal dry-run behavior, including its existing build-script execution.
	command := exec.Command(cli, "--project-dir", dir, "--config", config, "dist", "--no-dependent-tasks=false", "--dry-run")
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	require.FileExists(t, filepath.Join(dir, "build-hook"))

	command = exec.Command(cli, "--project-dir", dir, "--config", config, "dist")
	output, err = command.CombinedOutput()
	require.NoError(t, err, string(output))
	require.FileExists(t, filepath.Join(dir, "dist-hook"))
	require.NoError(t, os.Remove(filepath.Join(dir, "build-hook")))
	require.NoError(t, os.Remove(filepath.Join(dir, "dist-hook")))

	command = exec.Command(cli, "--project-dir", dir, "--config", config, "dist", "--force", "--no-dependent-tasks")
	output, err = command.CombinedOutput()
	require.NoError(t, err, string(output))
	require.NoFileExists(t, filepath.Join(dir, "build-hook"))
	require.FileExists(t, filepath.Join(dir, "dist-hook"))
}
