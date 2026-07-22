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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/palantir/distgo/distgo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunPublish_CreatesDraftRelease verifies that RunPublish creates the release as a draft, uploads the dist
// artifacts, and leaves it as a draft.
func TestRunPublish_CreatesDraftRelease(t *testing.T) {
	var (
		createReleaseDraft atomic.Pointer[bool]
		editReleaseCalled  atomic.Bool
		uploadedAssetName  atomic.Pointer[string]
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/testOwner/testRepo/releases", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			// no existing release for the tag, so RunPublish must create one
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `[]`)
		case http.MethodPost:
			var body struct {
				Draft *bool `json:"draft"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			createReleaseDraft.Store(body.Draft)
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"id": 123, "draft": true, "upload_url": "http://%s/upload/123/assets{?name,label}"}`, r.Host)
		default:
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/repos/testOwner/testRepo/releases/123", func(w http.ResponseWriter, r *http.Request) {
		editReleaseCalled.Store(true)
		http.Error(w, "RunPublish must not publish (un-draft) the release itself", http.StatusInternalServerError)
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

	assert.False(t, editReleaseCalled.Load(), "RunPublish must leave the release as a draft; FinalizePublish is responsible for publishing it")
}

// TestRunPublish_ReusesExistingDraftRelease verifies that when a draft release already exists for the tag,
// RunPublish reuses that draft to upload the dist artifacts, and leaves it as a draft.
func TestRunPublish_ReusesExistingDraftRelease(t *testing.T) {
	var (
		createReleaseCalled atomic.Bool
		uploadedAssetName   atomic.Pointer[string]
		editReleaseCalled   atomic.Bool
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/testOwner/testRepo/releases", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `[{"id": 123, "draft": true, "tag_name": "1.0.0", "upload_url": "http://%s/upload/123/assets{?name,label}"}]`, r.Host)
		case http.MethodPost:
			createReleaseCalled.Store(true)
			http.Error(w, "release creation must not be called when a draft already exists for the tag", http.StatusInternalServerError)
		default:
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/repos/testOwner/testRepo/releases/123", func(w http.ResponseWriter, r *http.Request) {
		editReleaseCalled.Store(true)
		http.Error(w, "RunPublish must not publish (un-draft) the release itself", http.StatusInternalServerError)
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

	assert.False(t, createReleaseCalled.Load(), "existing draft release must be reused instead of creating a new release")

	gotUploadedAssetName := uploadedAssetName.Load()
	require.NotNil(t, gotUploadedAssetName, "dist artifact must be uploaded to the existing draft release")
	assert.Equal(t, "foo-1.0.0-linux-amd64.tgz", *gotUploadedAssetName)

	assert.False(t, editReleaseCalled.Load(), "RunPublish must leave the reused release as a draft; FinalizePublish is responsible for publishing it")
}

// TestFinalizePublish_PublishesDraftRelease verifies that FinalizePublish finds and publishes (un-drafts) an
// existing draft release for the product's tag.
func TestFinalizePublish_PublishesDraftRelease(t *testing.T) {
	var editReleaseDraft atomic.Pointer[bool]

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/testOwner/testRepo/releases", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `[{"id": 123, "draft": true, "tag_name": "1.0.0"}]`)
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
	server := httptest.NewServer(mux)
	defer server.Close()

	productTaskOutputInfo := distgo.ProductTaskOutputInfo{
		Project: distgo.ProjectInfo{
			Version: "1.0.0",
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
	err := publisher.FinalizePublish(productTaskOutputInfo, []byte("{}\n"), flagVals, false, io.Discard)
	require.NoError(t, err)

	gotEditReleaseDraft := editReleaseDraft.Load()
	require.NotNil(t, gotEditReleaseDraft, "the draft release must be published (un-drafted)")
	assert.False(t, *gotEditReleaseDraft)
}

// TestFinalizePublish_DryRun verifies that FinalizePublish prints the publish message and makes no GitHub API calls
// when dryRun is true.
func TestFinalizePublish_DryRun(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "FinalizePublish must not call the GitHub API in dry-run mode", http.StatusInternalServerError)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	productTaskOutputInfo := distgo.ProductTaskOutputInfo{
		Project: distgo.ProjectInfo{
			Version: "1.0.0",
		},
	}
	flagVals := map[distgo.PublisherFlagName]any{
		githubPublisherAPIURLFlag.Name:     server.URL,
		githubPublisherUserFlag.Name:       "testUser",
		githubPublisherTokenFlag.Name:      "testToken",
		githubPublisherRepositoryFlag.Name: "testRepo",
		githubPublisherOwnerFlag.Name:      "testOwner",
	}

	var stdout bytes.Buffer
	publisher := new(githubPublisher)
	err := publisher.FinalizePublish(productTaskOutputInfo, []byte("{}\n"), flagVals, true, &stdout)
	require.NoError(t, err)

	assert.Equal(t, "[DRY RUN] Publishing GitHub release 1.0.0 for testOwner/testRepo...done\n", stdout.String())
}

// TestFinalizePublish_NoopIfAlreadyPublished verifies that FinalizePublish is a no-op when there's no draft release for the tag.
func TestFinalizePublish_NoopIfAlreadyPublished(t *testing.T) {
	var editReleaseCalled atomic.Bool

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/testOwner/testRepo/releases", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		// release for the tag exists but is no longer a draft
		_, _ = fmt.Fprint(w, `[{"id": 123, "draft": false, "tag_name": "1.0.0"}]`)
	})
	mux.HandleFunc("/repos/testOwner/testRepo/releases/123", func(w http.ResponseWriter, r *http.Request) {
		editReleaseCalled.Store(true)
		http.Error(w, "FinalizePublish must not attempt to edit an already-published release", http.StatusInternalServerError)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	productTaskOutputInfo := distgo.ProductTaskOutputInfo{
		Project: distgo.ProjectInfo{
			Version: "1.0.0",
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
	err := publisher.FinalizePublish(productTaskOutputInfo, []byte("{}\n"), flagVals, false, io.Discard)
	require.NoError(t, err)

	assert.False(t, editReleaseCalled.Load(), "FinalizePublish must not call EditRelease when the release is already published")
}

// TestRunPublish_MultipleProductsShareDraftThenFinalize verifies the behavior when two products
// publish to the same tag, RunPublish runs for each, then FinalizePublish runs once per product.
// Only one release should be created, both assets should upload to it, and it should be
// un-drafted exactly once even though FinalizePublish runs twice.
func TestRunPublish_MultipleProductsShareDraftThenFinalize(t *testing.T) {
	var (
		createReleaseCount atomic.Int32
		editReleaseCount   atomic.Int32
		uploadedAssetNames sync.Map
	)

	// simulates the release's draft state across requests
	isDraft := atomic.Bool{}
	isDraft.Store(true)

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/testOwner/testRepo/releases", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			if isDraft.Load() {
				_, _ = fmt.Fprintf(w, `[{"id": 123, "draft": true, "tag_name": "1.0.0", "upload_url": "http://%s/upload/123/assets{?name,label}"}]`, r.Host)
			} else {
				_, _ = fmt.Fprint(w, `[]`)
			}
		case http.MethodPost:
			createReleaseCount.Add(1)
			http.Error(w, "release creation must not be called when a draft already exists for the tag", http.StatusInternalServerError)
		default:
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/repos/testOwner/testRepo/releases/123", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPatch, r.Method)
		editReleaseCount.Add(1)
		isDraft.Store(false)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id": 123, "draft": false}`)
	})
	mux.HandleFunc("/upload/123/assets", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		uploadedAssetNames.Store(r.URL.Query().Get("name"), struct{}{})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprint(w, `{"id": 1, "name": "asset"}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	projectDir := t.TempDir()
	writeArtifact := func(productID, name string) {
		artifactPath := filepath.Join(projectDir, "out", "dist", productID, "1.0.0", "os-arch-bin", name)
		require.NoError(t, os.MkdirAll(filepath.Dir(artifactPath), 0755))
		require.NoError(t, os.WriteFile(artifactPath, []byte("fake content"), 0644))
	}
	writeArtifact("foo", "foo-1.0.0-linux-amd64.tgz")
	writeArtifact("bar", "bar-1.0.0-linux-amd64.tgz")

	newProductTaskOutputInfo := func(productID distgo.ProductID, artifactName string) distgo.ProductTaskOutputInfo {
		return distgo.ProductTaskOutputInfo{
			Project: distgo.ProjectInfo{
				ProjectDir: projectDir,
				Version:    "1.0.0",
			},
			Product: distgo.ProductOutputInfo{
				ID: productID,
				DistOutputInfos: &distgo.DistOutputInfos{
					DistOutputDir: "out/dist",
					DistIDs:       []distgo.DistID{"os-arch-bin"},
					DistInfos: map[distgo.DistID]distgo.DistOutputInfo{
						"os-arch-bin": {
							DistNameTemplateRendered: string(productID) + "-1.0.0",
							DistArtifactNames:        []string{artifactName},
							PackagingExtension:       "tgz",
						},
					},
				},
			},
		}
	}

	flagVals := map[distgo.PublisherFlagName]any{
		githubPublisherAPIURLFlag.Name:     server.URL,
		githubPublisherUserFlag.Name:       "testUser",
		githubPublisherTokenFlag.Name:      "testToken",
		githubPublisherRepositoryFlag.Name: "testRepo",
		githubPublisherOwnerFlag.Name:      "testOwner",
	}

	publisher := new(githubPublisher)
	fooOutputInfo := newProductTaskOutputInfo("foo", "foo-1.0.0-linux-amd64.tgz")
	barOutputInfo := newProductTaskOutputInfo("bar", "bar-1.0.0-linux-amd64.tgz")

	require.NoError(t, publisher.RunPublish(fooOutputInfo, []byte("{}\n"), flagVals, false, io.Discard))
	require.NoError(t, publisher.RunPublish(barOutputInfo, []byte("{}\n"), flagVals, false, io.Discard))

	assert.Equal(t, int32(0), createReleaseCount.Load(), "only one release must ever be created")
	_, fooUploaded := uploadedAssetNames.Load("foo-1.0.0-linux-amd64.tgz")
	assert.True(t, fooUploaded, "foo's asset must be uploaded")
	_, barUploaded := uploadedAssetNames.Load("bar-1.0.0-linux-amd64.tgz")
	assert.True(t, barUploaded, "bar's asset must be uploaded")

	// Products calls FinalizePublish once per product and the release must still be un-drafted only once.
	require.NoError(t, publisher.FinalizePublish(fooOutputInfo, []byte("{}\n"), flagVals, false, io.Discard))
	require.NoError(t, publisher.FinalizePublish(barOutputInfo, []byte("{}\n"), flagVals, false, io.Discard))

	assert.Equal(t, int32(1), editReleaseCount.Load(), "the shared release must be published (un-drafted) exactly once, even though FinalizePublish was called once per product")
}
