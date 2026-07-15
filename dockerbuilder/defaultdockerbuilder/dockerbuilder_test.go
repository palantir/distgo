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

package defaultdockerbuilder

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/mholt/archiver/v3"
	"github.com/stretchr/testify/require"
)

// TestExtractToOCILayoutIsRerunnable verifies that extracting into an output directory that already contains the
// artifacts of a previous extraction succeeds. This guards against the failure seen when re-running "docker build" at
// the same version (e.g. a "-dirty" version, whose output directory name does not change between runs), where the tar
// extractor would otherwise refuse to overwrite the existing OCI layout files.
func TestExtractToOCILayoutIsRerunnable(t *testing.T) {
	// Build a minimal but valid OCI image layout and pack it into a tarball shaped like buildx's OCI output.
	srcLayoutDir := filepath.Join(t.TempDir(), "src")
	_, err := layout.Write(srcLayoutDir, mutate.AppendManifests(empty.Index, mutate.IndexAddendum{Add: empty.Image}))
	require.NoError(t, err)

	destDir := t.TempDir()
	tarball := filepath.Join(destDir, "image.tar")
	require.NoError(t, archiver.DefaultTar.Archive([]string{
		filepath.Join(srcLayoutDir, "oci-layout"),
		filepath.Join(srcLayoutDir, "index.json"),
		filepath.Join(srcLayoutDir, "blobs"),
	}, tarball))

	b := &DefaultDockerBuilder{}
	require.NoError(t, b.extractToOCILayout(destDir, tarball), "first extraction should succeed")
	require.NoError(t, b.extractToOCILayout(destDir, tarball), "re-extraction into a populated directory should succeed")

	// The source tarball must be preserved across re-extraction (it is the input, not stale output).
	_, err = os.Stat(tarball)
	require.NoError(t, err)
}
