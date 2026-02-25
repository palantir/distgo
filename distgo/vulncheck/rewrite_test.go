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

package vulncheck

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRewriteVEXProducts(t *testing.T) {
	t.Run("promotes subcomponents to products", func(t *testing.T) {
		input := `{
  "@context": "https://openvex.dev/ns/v0.2.0",
  "@id": "govulncheck/vex:abc123",
  "author": "Unknown Author",
  "timestamp": "2026-02-25T00:00:00Z",
  "version": 1,
  "tooling": "https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck",
  "statements": [
    {
      "vulnerability": {"@id": "https://pkg.go.dev/vuln/GO-2025-1234", "name": "GO-2025-1234"},
      "products": [
        {
          "@id": "Unknown Product",
          "subcomponents": [
            {"@id": "pkg:golang/stdlib@v1.24.0"}
          ]
        }
      ],
      "status": "not_affected",
      "justification": "vulnerable_code_not_in_execute_path",
      "impact_statement": "Govulncheck determined that the vulnerable code isn't called"
    }
  ]
}`
		output, err := rewriteVEXProducts([]byte(input))
		require.NoError(t, err)

		var doc vexDocument
		require.NoError(t, json.Unmarshal(output, &doc))

		require.Len(t, doc.Statements, 1)
		require.Len(t, doc.Statements[0].Products, 1)
		assert.Equal(t, "pkg:golang/stdlib@v1.24.0", doc.Statements[0].Products[0].ID)
		assert.Empty(t, doc.Statements[0].Products[0].Subcomponents)
	})

	t.Run("promotes multiple subcomponents", func(t *testing.T) {
		input := `{
  "statements": [
    {
      "vulnerability": {"name": "GO-2025-1234"},
      "products": [
        {
          "@id": "Unknown Product",
          "subcomponents": [
            {"@id": "pkg:golang/stdlib@v1.24.0"},
            {"@id": "pkg:golang/golang.org/x/net@v0.30.0"}
          ]
        }
      ],
      "status": "affected"
    }
  ]
}`
		output, err := rewriteVEXProducts([]byte(input))
		require.NoError(t, err)

		var doc vexDocument
		require.NoError(t, json.Unmarshal(output, &doc))

		require.Len(t, doc.Statements[0].Products, 2)
		assert.Equal(t, "pkg:golang/stdlib@v1.24.0", doc.Statements[0].Products[0].ID)
		assert.Equal(t, "pkg:golang/golang.org/x/net@v0.30.0", doc.Statements[0].Products[1].ID)
	})

	t.Run("preserves non-Unknown products", func(t *testing.T) {
		input := `{
  "statements": [
    {
      "vulnerability": {"name": "GO-2025-1234"},
      "products": [
        {"@id": "pkg:oci/myapp?repository_url=ghcr.io/org/myapp"}
      ],
      "status": "not_affected"
    }
  ]
}`
		output, err := rewriteVEXProducts([]byte(input))
		require.NoError(t, err)

		// Should return original data unchanged.
		var doc vexDocument
		require.NoError(t, json.Unmarshal(output, &doc))
		assert.Equal(t, "pkg:oci/myapp?repository_url=ghcr.io/org/myapp", doc.Statements[0].Products[0].ID)
	})

	t.Run("handles no statements", func(t *testing.T) {
		input := `{"@context": "https://openvex.dev/ns/v0.2.0", "version": 1}`
		output, err := rewriteVEXProducts([]byte(input))
		require.NoError(t, err)
		// No change expected.
		assert.JSONEq(t, input, string(output))
	})

	t.Run("preserves metadata fields", func(t *testing.T) {
		input := `{
  "@context": "https://openvex.dev/ns/v0.2.0",
  "@id": "govulncheck/vex:abc",
  "author": "Test Author",
  "timestamp": "2026-02-25T12:00:00Z",
  "version": 1,
  "tooling": "govulncheck",
  "statements": [
    {
      "vulnerability": {"@id": "https://pkg.go.dev/vuln/GO-2025-5678", "name": "GO-2025-5678", "aliases": ["CVE-2025-99999"]},
      "products": [{"@id": "Unknown Product", "subcomponents": [{"@id": "pkg:golang/stdlib@v1.24.0"}]}],
      "status": "not_affected",
      "justification": "vulnerable_code_not_in_execute_path",
      "impact_statement": "Govulncheck determined that the vulnerable code isn't called"
    }
  ]
}`
		output, err := rewriteVEXProducts([]byte(input))
		require.NoError(t, err)

		var doc vexDocument
		require.NoError(t, json.Unmarshal(output, &doc))

		assert.Equal(t, "https://openvex.dev/ns/v0.2.0", doc.Context)
		assert.Equal(t, "govulncheck/vex:abc", doc.ID)
		assert.Equal(t, "Test Author", doc.Author)
		assert.Equal(t, "govulncheck", doc.Tooling)
		assert.Equal(t, 1, doc.Version)
		assert.Equal(t, "not_affected", doc.Statements[0].Status)
		assert.Equal(t, "vulnerable_code_not_in_execute_path", doc.Statements[0].Justification)
	})
}
