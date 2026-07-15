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

package github

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/palantir/distgo/distgo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunPublish_CreatesDraftReleaseThenPublishesAfterUpload verifies that RunPublish creates the release as a draft,
// uploads all of the dist artifacts to it, and only publishes (un-drafts) the release once every upload has succeeded.
// GitHub's immutable-releases feature rejects asset uploads made to a release once it is published (non-draft).
func TestRunPublish_CreatesDraftReleaseThenPublishesAfterUpload(t *testing.T) {
	var (
		createReleaseDraft atomic.Pointer[bool]
		editReleaseDraft   atomic.Pointer[bool]
		uploadedAssetName  atomic.Pointer[string]
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/testOwner/testRepo/releases", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		var body struct {
			Draft *bool `json:"draft"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		createReleaseDraft.Store(body.Draft)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id": 123, "draft": true, "upload_url": "http://%s/upload/123/assets{?name,label}"}`, r.Host)
	})
	mux.HandleFunc("/repos/testOwner/testRepo/releases/123", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPatch, r.Method)
		var body struct {
			Draft *bool `json:"draft"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		editReleaseDraft.Store(body.Draft)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id": 123, "draft": false}`)
	})
	mux.HandleFunc("/upload/123/assets", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		name := r.URL.Query().Get("name")
		uploadedAssetName.Store(&name)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprint(w, `{"id": 1, "name": "asset"}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	projectDir := t.TempDir()
	artifactPath := filepath.Join(projectDir, "out", "dist", "foo", "1.0.0", "os-arch-bin", "foo-1.0.0-linux-amd64.tgz")
	require.NoError(t, os.MkdirAll(filepath.Dir(artifactPath), 0755))
	require.NoError(t, os.WriteFile(artifactPath, []byte("fake tgz content"), 0644))

	productTaskOutputInfo := distgo.ProductTaskOutputInfo{
		Project: distgo.ProjectInfo{
			ProjectDir: projectDir,
			Version:    "1.0.0",
		},
		Product: distgo.ProductOutputInfo{
			ID: "foo",
			DistOutputInfos: &distgo.DistOutputInfos{
				DistOutputDir: "out/dist",
				DistIDs:       []distgo.DistID{"os-arch-bin"},
				DistInfos: map[distgo.DistID]distgo.DistOutputInfo{
					"os-arch-bin": {
						DistNameTemplateRendered: "foo-1.0.0",
						DistArtifactNames:        []string{"foo-1.0.0-linux-amd64.tgz"},
						PackagingExtension:       "tgz",
					},
				},
			},
		},
	}

	flagVals := map[distgo.PublisherFlagName]any{
		githubPublisherAPIURLFlag.Name:     server.URL,
		githubPublisherUserFlag.Name:       "testUser",
		githubPublisherTokenFlag.Name:      "testToken",
		githubPublisherRepositoryFlag.Name: "testRepo",
		githubPublisherOwnerFlag.Name:      "testOwner",
	}

	publisher := new(githubPublisher)
	err := publisher.RunPublish(productTaskOutputInfo, []byte("{}\n"), flagVals, false, io.Discard)
	require.NoError(t, err)

	gotCreateReleaseDraft := createReleaseDraft.Load()
	require.NotNil(t, gotCreateReleaseDraft, "release creation request must specify a draft value")
	assert.True(t, *gotCreateReleaseDraft, "release must be created as a draft so that GitHub's immutable-releases feature does not reject the subsequent asset uploads")

	gotUploadedAssetName := uploadedAssetName.Load()
	require.NotNil(t, gotUploadedAssetName, "dist artifact must be uploaded")
	assert.Equal(t, "foo-1.0.0-linux-amd64.tgz", *gotUploadedAssetName)

	gotEditReleaseDraft := editReleaseDraft.Load()
	require.NotNil(t, gotEditReleaseDraft, "release must be published (un-drafted) after assets are uploaded")
	assert.False(t, *gotEditReleaseDraft)
}
