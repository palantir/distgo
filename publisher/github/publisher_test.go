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
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/palantir/distgo/distgo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testArtifactContent = "fake tgz content"

// TestRunPublish_CreatesDraftReleaseThenPublishesAfterUpload verifies that RunPublish creates the release as a draft,
// uploads all the dist artifacts to it, and only publishes (un-drafts) the release once every upload has succeeded.
// GitHub's immutable-releases feature rejects asset uploads made to a release once it is published (non-draft).
func TestRunPublish_CreatesDraftReleaseThenPublishesAfterUpload(t *testing.T) {
	var (
		createReleaseDraft atomic.Pointer[bool]
		editReleaseDraft   atomic.Pointer[bool]
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
	productTaskOutputInfo := writeTestArtifact(t, projectDir, "foo", "foo-1.0.0-linux-amd64.tgz")

	publisher := new(githubPublisher)
	err := publisher.RunPublish(productTaskOutputInfo, []byte("{}\n"), testGitHubFlagValues(server.URL), false, io.Discard)
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

// TestRunPublish_ReusesExistingDraftRelease verifies that when a draft release already exists for the tag, RunPublish
// reuses that draft to upload the dist artifacts and finalize it instead of creating a new release.
func TestRunPublish_ReusesExistingDraftRelease(t *testing.T) {
	var (
		createReleaseCalled atomic.Bool
		uploadedAssetName   atomic.Pointer[string]
		editReleaseDraft    atomic.Pointer[bool]
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
	productTaskOutputInfo := writeTestArtifact(t, projectDir, "foo", "foo-1.0.0-linux-amd64.tgz")

	publisher := new(githubPublisher)
	err := publisher.RunPublish(productTaskOutputInfo, []byte("{}\n"), testGitHubFlagValues(server.URL), false, io.Discard)
	require.NoError(t, err)

	assert.False(t, createReleaseCalled.Load(), "existing draft release must be reused instead of creating a new release")

	gotUploadedAssetName := uploadedAssetName.Load()
	require.NotNil(t, gotUploadedAssetName, "dist artifact must be uploaded to the existing draft release")
	assert.Equal(t, "foo-1.0.0-linux-amd64.tgz", *gotUploadedAssetName)

	gotEditReleaseDraft := editReleaseDraft.Load()
	require.NotNil(t, gotEditReleaseDraft, "existing draft release must be published (un-drafted) after assets are uploaded")
	assert.False(t, *gotEditReleaseDraft)
}

// TestRunPublish_ReRunAfterAlreadyPublishedIsANoop verifies that re-running a publish for a tag that a
// previous run already fully published will be a noop since the tag is already published. Only looking for existing draft
// releases would cause the publisher to miss the finalized relese and create a second draft release which would
// ultimately fail to publish since you cannot have 2 releases with the same tag.
func TestRunPublish_ReRunAfterAlreadyPublishedIsANoop(t *testing.T) {
	var (
		createReleaseCalled atomic.Bool
		editReleaseCalled   atomic.Bool
		uploadCalled        atomic.Bool
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/testOwner/testRepo/releases", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `[{"id": 123, "draft": false, "tag_name": "1.0.0", "upload_url": "http://%s/upload/123/assets{?name,label}", "assets": [{"id": 1, "name": "foo-1.0.0-linux-amd64.tgz", "size": %d}]}]`, r.Host, len(testArtifactContent))
		case http.MethodPost:
			createReleaseCalled.Store(true)
			http.Error(w, "release creation must not be called when the tag is already published", http.StatusInternalServerError)
		default:
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/repos/testOwner/testRepo/releases/123", func(w http.ResponseWriter, r *http.Request) {
		editReleaseCalled.Store(true)
		http.Error(w, "release must not be re-published when it is already published", http.StatusInternalServerError)
	})
	mux.HandleFunc("/upload/123/assets", func(w http.ResponseWriter, r *http.Request) {
		uploadCalled.Store(true)
		http.Error(w, "asset must not be re-uploaded when it already matches the existing published release", http.StatusInternalServerError)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	projectDir := t.TempDir()
	fooOutputInfo := writeTestArtifact(t, projectDir, "foo", "foo-1.0.0-linux-amd64.tgz")

	publisher := new(githubPublisher)
	err := publisher.RunPublish(fooOutputInfo, []byte("{}\n"), testGitHubFlagValues(server.URL), false, io.Discard)
	require.NoError(t, err, "re-running a publish for an already-published tag must be a no-op, not an error")

	assert.False(t, createReleaseCalled.Load(), "must not create a duplicate release for a tag that is already published")
	assert.False(t, editReleaseCalled.Load(), "must not attempt to re-publish a release that is already published")
	assert.False(t, uploadCalled.Load(), "must not re-upload an asset that already matches the existing published release")
}

func TestRunPublishBatch_UploadsAllProductsBeforePublishingSharedRelease(t *testing.T) {
	var (
		operationsMu sync.Mutex
		operations   []string
		listCount    atomic.Int32
		isDraft      atomic.Bool
	)
	isDraft.Store(true)
	recordOperation := func(operation string) {
		operationsMu.Lock()
		defer operationsMu.Unlock()
		operations = append(operations, operation)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/testOwner/testRepo/releases", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		listCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `[{"id": 123, "draft": %t, "tag_name": "1.0.0", "upload_url": "http://%s/upload/123/assets{?name,label}"}]`, isDraft.Load(), r.Host)
	})
	mux.HandleFunc("/repos/testOwner/testRepo/releases/123", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPatch, r.Method)
		recordOperation("publish")
		isDraft.Store(false)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id": 123, "draft": false}`)
	})
	mux.HandleFunc("/upload/123/assets", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		recordOperation("upload:" + r.URL.Query().Get("name"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprint(w, `{"id": 1, "name": "asset"}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	projectDir := t.TempDir()
	fooOutputInfo := writeTestArtifact(t, projectDir, "foo", "foo-1.0.0-linux-amd64.tgz")
	barOutputInfo := writeTestArtifact(t, projectDir, "bar", "bar-1.0.0-linux-amd64.tgz")

	publisher := new(githubPublisher)
	err := publisher.RunPublishBatch([]distgo.BatchPublishInput{
		{ProductTaskOutputInfo: fooOutputInfo, ConfigYML: []byte("{}\n")},
		{ProductTaskOutputInfo: barOutputInfo, ConfigYML: []byte("{}\n")},
	}, testGitHubFlagValues(server.URL), false, io.Discard)
	require.NoError(t, err)

	assert.Equal(t, int32(1), listCount.Load(), "the shared release should be resolved once")
	assert.Equal(t, []string{
		"upload:foo-1.0.0-linux-amd64.tgz",
		"upload:bar-1.0.0-linux-amd64.tgz",
		"publish",
	}, operations)
}

// TestRunPublishBatch_UploadFailureIncludesProductID verifies that an upload failure is wrapped with the ID of the
// product whose upload failed, since RunPublishBatch's own top-level error otherwise only identifies the publisher
// type, not which of the (possibly many) products in the batch actually failed.
func TestRunPublishBatch_UploadFailureIncludesProductID(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/testOwner/testRepo/releases", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `[{"id": 123, "draft": true, "tag_name": "1.0.0", "upload_url": "http://%s/upload/123/assets{?name,label}"}]`, r.Host)
	})
	mux.HandleFunc("/upload/123/assets", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("name") == "bar-1.0.0-linux-amd64.tgz" {
			http.Error(w, "upload failed", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprint(w, `{"id": 1, "name": "asset"}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	projectDir := t.TempDir()
	fooOutputInfo := writeTestArtifact(t, projectDir, "foo", "foo-1.0.0-linux-amd64.tgz")
	barOutputInfo := writeTestArtifact(t, projectDir, "bar", "bar-1.0.0-linux-amd64.tgz")

	publisher := new(githubPublisher)
	err := publisher.RunPublishBatch([]distgo.BatchPublishInput{
		{ProductTaskOutputInfo: fooOutputInfo, ConfigYML: []byte("{}\n")},
		{ProductTaskOutputInfo: barOutputInfo, ConfigYML: []byte("{}\n")},
	}, testGitHubFlagValues(server.URL), false, io.Discard)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to publish product bar")
}

func TestRunPublishBatch_UploadFailureLeavesSharedReleaseAsDraft(t *testing.T) {
	var (
		editReleaseCalled atomic.Bool
		isDraft           atomic.Bool
	)
	isDraft.Store(true)

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/testOwner/testRepo/releases", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `[{"id": 123, "draft": true, "tag_name": "1.0.0", "upload_url": "http://%s/upload/123/assets{?name,label}"}]`, r.Host)
	})
	mux.HandleFunc("/repos/testOwner/testRepo/releases/123", func(w http.ResponseWriter, _ *http.Request) {
		editReleaseCalled.Store(true)
		isDraft.Store(false)
		http.Error(w, "release must remain a draft", http.StatusInternalServerError)
	})
	mux.HandleFunc("/upload/123/assets", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("name") == "bar-1.0.0-linux-amd64.tgz" {
			http.Error(w, "upload failed", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprint(w, `{"id": 1, "name": "asset"}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	projectDir := t.TempDir()
	fooOutputInfo := writeTestArtifact(t, projectDir, "foo", "foo-1.0.0-linux-amd64.tgz")
	barOutputInfo := writeTestArtifact(t, projectDir, "bar", "bar-1.0.0-linux-amd64.tgz")

	publisher := new(githubPublisher)
	err := publisher.RunPublishBatch([]distgo.BatchPublishInput{
		{ProductTaskOutputInfo: fooOutputInfo, ConfigYML: []byte("{}\n")},
		{ProductTaskOutputInfo: barOutputInfo, ConfigYML: []byte("{}\n")},
	}, testGitHubFlagValues(server.URL), false, io.Discard)
	require.Error(t, err)

	assert.False(t, editReleaseCalled.Load())
	assert.True(t, isDraft.Load())
}

// TestRunPublishBatch_RetrySkipsAlreadyUploadedAssets verifies that a retry of a partially failed publish will not
// re-upload already uploaded assets and should skip to the missing assets instead.
func TestRunPublishBatch_RetrySkipsAlreadyUploadedAssets(t *testing.T) {
	var (
		uploadedAssets   sync.Map
		barShouldFail    atomic.Bool
		fooUploadCount   atomic.Int32
		barUploadCount   atomic.Int32
		editReleaseCount atomic.Int32
	)
	barShouldFail.Store(true)

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/testOwner/testRepo/releases", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		var assetsJSON []string
		uploadedAssets.Range(func(key, _ any) bool {
			assetsJSON = append(assetsJSON, fmt.Sprintf(`{"id": 1, "name": %q, "size": %d}`, key.(string), len(testArtifactContent)))
			return true
		})
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `[{"id": 123, "draft": true, "tag_name": "1.0.0", "upload_url": "http://%s/upload/123/assets{?name,label}", "assets": [%s]}]`, r.Host, strings.Join(assetsJSON, ","))
	})
	mux.HandleFunc("/repos/testOwner/testRepo/releases/123", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPatch, r.Method)
		editReleaseCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id": 123, "draft": false}`)
	})
	mux.HandleFunc("/upload/123/assets", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		switch name {
		case "foo-1.0.0-linux-amd64.tgz":
			fooUploadCount.Add(1)
		case "bar-1.0.0-linux-amd64.tgz":
			barUploadCount.Add(1)
			if barShouldFail.Load() {
				http.Error(w, "upload failed", http.StatusInternalServerError)
				return
			}
		}
		uploadedAssets.Store(name, struct{}{})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprintf(w, `{"id": 1, "name": %q}`, name)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	projectDir := t.TempDir()
	fooOutputInfo := writeTestArtifact(t, projectDir, "foo", "foo-1.0.0-linux-amd64.tgz")
	barOutputInfo := writeTestArtifact(t, projectDir, "bar", "bar-1.0.0-linux-amd64.tgz")
	inputs := []distgo.BatchPublishInput{
		{ProductTaskOutputInfo: fooOutputInfo, ConfigYML: []byte("{}\n")},
		{ProductTaskOutputInfo: barOutputInfo, ConfigYML: []byte("{}\n")},
	}

	publisher := new(githubPublisher)
	err := publisher.RunPublishBatch(inputs, testGitHubFlagValues(server.URL), false, io.Discard)
	require.Error(t, err, "first attempt must fail while bar's upload is failing")
	assert.Equal(t, int32(1), fooUploadCount.Load(), "foo's asset must be uploaded once by the first attempt")
	assert.Equal(t, int32(0), editReleaseCount.Load(), "release must not be published while a product's upload is still failing")

	barShouldFail.Store(false)
	err = publisher.RunPublishBatch(inputs, testGitHubFlagValues(server.URL), false, io.Discard)
	require.NoError(t, err, "retry must succeed once bar's upload starts succeeding")

	assert.Equal(t, int32(1), fooUploadCount.Load(), "retry must not re-upload foo's asset, since it already succeeded on the first attempt")
	assert.Equal(t, int32(1), editReleaseCount.Load(), "release must be published exactly once, after the retry completes successfully")
}

// TestRunPublishBatch_RetryFailsWhenExistingAssetSizeDiffers ensures that a retry fails when an asset being uploaded
// differs in size from what is reported from the GitHub  release.
func TestRunPublishBatch_RetryFailsWhenExistingAssetSizeDiffers(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/testOwner/testRepo/releases", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `[{"id": 123, "draft": true, "tag_name": "1.0.0", "upload_url": "http://%s/upload/123/assets{?name,label}", "assets": [{"id": 1, "name": "foo-1.0.0-linux-amd64.tgz", "size": %d}]}]`, r.Host, len(testArtifactContent))
	})
	mux.HandleFunc("/repos/testOwner/testRepo/releases/123", func(w http.ResponseWriter, r *http.Request) {
		require.Fail(t, "release must not be published when a stale asset is detected")
	})
	mux.HandleFunc("/upload/123/assets", func(w http.ResponseWriter, r *http.Request) {
		require.Fail(t, "asset must not be re-uploaded when a stale asset is detected")
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	projectDir := t.TempDir()
	fooOutputInfo := writeTestArtifactWithContent(t, projectDir, "foo", "foo-1.0.0-linux-amd64.tgz", "different content for artifact")

	publisher := new(githubPublisher)
	err := publisher.RunPublish(fooOutputInfo, []byte("{}\n"), testGitHubFlagValues(server.URL), false, io.Discard)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match the local artifact")
}

//	TestRunPublishBatch_PublishesEachDistinctReleaseAfterUploadsFinish verifies that releases which do not share
//
// any products are published as soon as their own uploads finish, rather than waiting for every release in the batch to finish uploading first.
func TestRunPublishBatch_PublishesEachDistinctReleaseAfterUploadsFinish(t *testing.T) {
	var (
		operationsMu sync.Mutex
		operations   []string
	)
	recordOperation := func(operation string) {
		operationsMu.Lock()
		defer operationsMu.Unlock()
		operations = append(operations, operation)
	}

	mux := http.NewServeMux()
	for _, target := range []struct {
		repository string
		releaseID  int
	}{
		{repository: "fooRepo", releaseID: 123},
		{repository: "barRepo", releaseID: 456},
	} {
		releasesPath := fmt.Sprintf("/repos/testOwner/%s/releases", target.repository)
		uploadPath := fmt.Sprintf("/upload/%d/assets", target.releaseID)
		mux.HandleFunc(releasesPath, func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodGet, r.Method)
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `[{"id": %d, "draft": true, "tag_name": "1.0.0", "upload_url": "http://%s%s{?name,label}"}]`, target.releaseID, r.Host, uploadPath)
		})
		mux.HandleFunc(fmt.Sprintf("%s/%d", releasesPath, target.releaseID), func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodPatch, r.Method)
			recordOperation("publish:" + target.repository)
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"id": %d, "draft": false}`, target.releaseID)
		})
		mux.HandleFunc(uploadPath, func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodPost, r.Method)
			recordOperation("upload:" + r.URL.Query().Get("name"))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprint(w, `{"id": 1, "name": "asset"}`)
		})
	}
	server := httptest.NewServer(mux)
	defer server.Close()

	projectDir := t.TempDir()
	fooOutputInfo := writeTestArtifact(t, projectDir, "foo", "foo-1.0.0-linux-amd64.tgz")
	barOutputInfo := writeTestArtifact(t, projectDir, "bar", "bar-1.0.0-linux-amd64.tgz")
	configForRepository := func(repository string) []byte {
		return fmt.Appendf(nil, "api-url: %s\nuser: testUser\ntoken: testToken\nowner: testOwner\nrepository: %s\n", server.URL, repository)
	}

	publisher := new(githubPublisher)
	err := publisher.RunPublishBatch([]distgo.BatchPublishInput{
		{ProductTaskOutputInfo: fooOutputInfo, ConfigYML: configForRepository("fooRepo")},
		{ProductTaskOutputInfo: barOutputInfo, ConfigYML: configForRepository("barRepo")},
	}, nil, false, io.Discard)
	require.NoError(t, err)

	assert.Equal(t, []string{
		"upload:foo-1.0.0-linux-amd64.tgz",
		"publish:fooRepo",
		"upload:bar-1.0.0-linux-amd64.tgz",
		"publish:barRepo",
	}, operations, "each release must be published as soon as its own group finishes uploading, not after the whole batch uploads")
}

func writeTestArtifact(t *testing.T, projectDir string, productID distgo.ProductID, artifactName string) distgo.ProductTaskOutputInfo {
	return writeTestArtifactWithContent(t, projectDir, productID, artifactName, testArtifactContent)
}

func writeTestArtifactWithContent(t *testing.T, projectDir string, productID distgo.ProductID, artifactName string, content string) distgo.ProductTaskOutputInfo {
	artifactPath := filepath.Join(projectDir, "out", "dist", string(productID), "1.0.0", "os-arch-bin", artifactName)
	require.NoError(t, os.MkdirAll(filepath.Dir(artifactPath), 0755))
	require.NoError(t, os.WriteFile(artifactPath, []byte(content), 0644))
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

func testGitHubFlagValues(apiURL string) map[distgo.PublisherFlagName]any {
	return map[distgo.PublisherFlagName]any{
		githubPublisherAPIURLFlag.Name:     apiURL,
		githubPublisherUserFlag.Name:       "testUser",
		githubPublisherTokenFlag.Name:      "testToken",
		githubPublisherRepositoryFlag.Name: "testRepo",
		githubPublisherOwnerFlag.Name:      "testOwner",
	}
}
