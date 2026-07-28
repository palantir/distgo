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

package publishfixtures

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

const (
	// V2 registers both run-publish-v2 and the legacy run-publish command via [publisher.AssetRootCmd].
	V2 = "v2"
	// LegacyOnly hand-builds a cobra CLI that only supports run-publish, simulating an asset that does not support run-publish-v2.
	LegacyOnly = "legacyonly"
)

// Build compiles the named fixture (V2 or LegacyOnly) with "go build" into a temp directory and returns the
// path to the resulting binary.
func Build(t testing.TB, fixture string) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to determine the location of publishfixtures.go")
	}
	pkgDir := filepath.Join(filepath.Dir(thisFile), fixture)

	binPath := filepath.Join(t.TempDir(), fixture)
	cmd := exec.Command("go", "build", "-o", binPath, ".")
	cmd.Dir = pkgDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build fixture %q: %v\n%s", fixture, err, output)
	}
	return binPath
}
