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

package docker

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildDSSEEnvelope(t *testing.T) {
	vexDoc := `{"@context":"https://openvex.dev/ns/v0.2.0","@id":"test","author":"govulncheck","statements":[]}`
	subjectDigest := v1.Hash{Algorithm: "sha256", Hex: "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"}

	envBytes, err := buildDSSEEnvelope("registry.example.com/myapp:1.0.0", subjectDigest, []byte(vexDoc))
	require.NoError(t, err)

	// Verify DSSE envelope structure.
	var env dsseEnvelope
	err = json.Unmarshal(envBytes, &env)
	require.NoError(t, err)

	assert.Equal(t, inTotoPayloadType, env.PayloadType)
	assert.Empty(t, env.Signatures, "envelope should be unsigned")

	// Decode and verify the in-toto statement.
	stmtBytes, err := base64.StdEncoding.DecodeString(env.Payload)
	require.NoError(t, err)

	var stmt inTotoStatement
	err = json.Unmarshal(stmtBytes, &stmt)
	require.NoError(t, err)

	assert.Equal(t, inTotoStatementType, stmt.Type)
	assert.Equal(t, openVEXPredicateType, stmt.PredicateType)
	require.Len(t, stmt.Subject, 1)
	assert.Equal(t, "registry.example.com/myapp:1.0.0", stmt.Subject[0].Name)
	assert.Equal(t, subjectDigest.Hex, stmt.Subject[0].Digest[subjectDigest.Algorithm])

	// Verify the predicate is the original VEX document.
	var predicateMap map[string]interface{}
	err = json.Unmarshal(stmt.Predicate, &predicateMap)
	require.NoError(t, err)
	assert.Equal(t, "https://openvex.dev/ns/v0.2.0", predicateMap["@context"])
}

func TestAttachVEXAttestationDryRun(t *testing.T) {
	// Create a temporary VEX file.
	tmpDir := t.TempDir()
	vexPath := filepath.Join(tmpDir, "vex.json")
	err := os.WriteFile(vexPath, []byte(`{"@context":"https://openvex.dev/ns/v0.2.0"}`), 0644)
	require.NoError(t, err)

	ref, err := name.ParseReference("registry.example.com/myapp:1.0.0")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = attachVEXAttestation(
		ref,
		v1.Hash{Algorithm: "sha256", Hex: "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"},
		vexPath,
		true,  // dryRun
		false, // insecure
		&buf,
	)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Attaching VEX attestation")
	assert.Contains(t, buf.String(), "sha256-abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890.att")
}

func TestStaticLayer(t *testing.T) {
	content := []byte("test content for static layer")
	mt := types.MediaType("application/vnd.dsse.envelope.v1+json")
	layer := newStaticLayer(content, mt)

	// Verify media type.
	gotMT, err := layer.MediaType()
	require.NoError(t, err)
	assert.Equal(t, mt, gotMT)

	// Verify size.
	size, err := layer.Size()
	require.NoError(t, err)
	assert.Equal(t, int64(len(content)), size)

	// Verify compressed content.
	rc, err := layer.Compressed()
	require.NoError(t, err)
	defer rc.Close()
	data, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, content, data)

	// Verify uncompressed content (same as compressed for raw content).
	rc2, err := layer.Uncompressed()
	require.NoError(t, err)
	defer rc2.Close()
	data2, err := io.ReadAll(rc2)
	require.NoError(t, err)
	assert.Equal(t, content, data2)

	// Verify digest == diffID (no compression).
	digest, err := layer.Digest()
	require.NoError(t, err)
	diffID, err := layer.DiffID()
	require.NoError(t, err)
	assert.Equal(t, digest, diffID)
	assert.Equal(t, "sha256", digest.Algorithm)
	assert.NotEmpty(t, digest.Hex)
}
