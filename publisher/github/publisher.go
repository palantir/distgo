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

package github

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/google/go-github/v28/github"
	"github.com/jtacoma/uritemplates"
	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/publisher"
	"github.com/palantir/distgo/publisher/github/config"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"gopkg.in/cheggaaa/pb.v1"
	"gopkg.in/yaml.v2"
)

const TypeName = "github"

type githubPublisher struct{}

func PublisherCreator() publisher.Creator {
	return publisher.NewCreator(TypeName, func() distgo.Publisher {
		return &githubPublisher{}
	})
}

func (p *githubPublisher) TypeName() (string, error) {
	return TypeName, nil
}

var (
	githubPublisherAPIURLFlag = distgo.PublisherFlag{
		Name:        "api-url",
		Description: "GitHub API URL",
		Type:        distgo.StringFlag,
	}
	githubPublisherUserFlag = distgo.PublisherFlag{
		Name:        "user",
		Description: "GitHub user",
		Type:        distgo.StringFlag,
	}
	githubPublisherTokenFlag = distgo.PublisherFlag{
		Name:        "token",
		Description: "GitHub token",
		Type:        distgo.StringFlag,
	}
	githubPublisherRepositoryFlag = distgo.PublisherFlag{
		Name:        "repository",
		Description: "repository that is the destination for the publish",
		Type:        distgo.StringFlag,
	}
	githubPublisherOwnerFlag = distgo.PublisherFlag{
		Name:        "owner",
		Description: "GitHub owner of the destination repository for the publish (if unspecified, user will be used)",
		Type:        distgo.StringFlag,
	}
	githubAddVPrefixFlag = distgo.PublisherFlag{
		Name:        "add-v-prefix",
		Description: "If true, adds 'v' as a prefix to the version (for example, \"v1.2.3\")",
		Type:        distgo.BoolFlag,
	}
)

func (p *githubPublisher) Flags() ([]distgo.PublisherFlag, error) {
	return []distgo.PublisherFlag{
		githubPublisherAPIURLFlag,
		githubPublisherUserFlag,
		githubPublisherTokenFlag,
		githubPublisherRepositoryFlag,
		githubPublisherOwnerFlag,
		githubAddVPrefixFlag,
		publisher.ArtifactNamesFilterFlag,
		publisher.ArtifactNamesExcludeFlag,
	}, nil
}

func (p *githubPublisher) RunPublish(productTaskOutputInfo distgo.ProductTaskOutputInfo, cfgYML []byte, flagVals map[distgo.PublisherFlagName]any, dryRun bool, stdout io.Writer) error {
	return p.RunPublishBatch([]distgo.BatchPublishInput{{
		ProductTaskOutputInfo: productTaskOutputInfo,
		ConfigYML:             cfgYML,
	}}, flagVals, dryRun, stdout)
}

func (p *githubPublisher) RunPublishBatch(inputs []distgo.BatchPublishInput, flagVals map[distgo.PublisherFlagName]any, dryRun bool, stdout io.Writer) error {
	filterRegexp, err := publisher.GetArtifactNamesFilterFlagValue(flagVals)
	if err != nil {
		return err
	}
	excludeRegexp, err := publisher.GetArtifactNamesExcludeFlagValue(flagVals)
	if err != nil {
		return err
	}

	// Group inputs by release key so that products sharing a release upload together and publish once, since a
	// shared release must not be published until all of its products have uploaded.
	var batches []githubReleaseBatch
	batchIndexByKey := make(map[githubReleaseKey]int)
	for _, input := range inputs {
		productTaskOutputInfo := input.ProductTaskOutputInfo
		publisher.FilterProductTaskOutputInfoArtifactNames(&productTaskOutputInfo, filterRegexp, excludeRegexp)

		cfg, key, err := resolveGitHubReleaseConfig(input.ConfigYML, flagVals, productTaskOutputInfo.Project.Version)
		if err != nil {
			return errors.Wrapf(err, "failed to resolve GitHub config for product %s", productTaskOutputInfo.Product.ID)
		}

		batchIndex, ok := batchIndexByKey[key]
		if !ok {
			// Only the first product seen for a given release key needs a client. Every other product in the
			// batch reuses this target instead of re-resolving.
			target, err := newGitHubReleaseTarget(cfg, key)
			if err != nil {
				return errors.Wrapf(err, "failed to resolve GitHub release target for product %s", productTaskOutputInfo.Product.ID)
			}
			batchIndex = len(batches)
			batchIndexByKey[key] = batchIndex
			batches = append(batches, githubReleaseBatch{target: target})
		}
		batches[batchIndex].products = append(batches[batchIndex].products, productTaskOutputInfo)
	}

	// With grouping done, run the core prepare/upload/publish workflow once per batch. For each batch, create or
	// reuse the release, upload every product's assets in the batch, then publish it for the whole batch at once.
	for _, batch := range batches {
		release, err := prepareGitHubRelease(batch.target, dryRun, stdout)
		if err != nil {
			return err
		}

		for _, productTaskOutputInfo := range batch.products {
			for _, currDistID := range productTaskOutputInfo.Product.DistOutputInfos.DistIDs {
				for _, currArtifactPath := range productTaskOutputInfo.ProductDistArtifactPaths()[currDistID] {
					if _, err := p.uploadFileAtPath(batch.target.client, release, currArtifactPath, dryRun, stdout); err != nil {
						return errors.Wrapf(err, "failed to publish product %s", productTaskOutputInfo.Product.ID)
					}
				}
			}
		}

		// Nothing left to do if the release is already live. Dry-run's release is always nil, so this never skips there.
		if !dryRun && !release.GetDraft() {
			continue
		}
		if err := publishGitHubRelease(batch.target, release, dryRun, stdout); err != nil {
			return err
		}
	}
	return nil
}

// githubReleaseBatch groups the products that resolve to the same GitHub release so they can be uploaded together
// and published once.
type githubReleaseBatch struct {
	target   githubReleaseTarget
	products []distgo.ProductTaskOutputInfo
}

// githubReleaseKey identifies a distinct GitHub release so that products resolving to the same one are grouped and
// published together instead of once per product.
type githubReleaseKey struct {
	apiURL         string
	owner          string
	repository     string
	releaseVersion string
}

// githubReleaseTarget bundles the client and resolved config needed for a single GitHub release.
type githubReleaseTarget struct {
	key    githubReleaseKey
	client *github.Client
	cfg    config.GitHub
}

// resolveGitHubReleaseConfig resolves a product's publish configuration and the release key used to group it with
// other products publishing to the same release.
func resolveGitHubReleaseConfig(cfgYML []byte, flagVals map[distgo.PublisherFlagName]any, projectVersion string) (config.GitHub, githubReleaseKey, error) {
	var cfg config.GitHub
	if err := yaml.Unmarshal(cfgYML, &cfg); err != nil {
		return config.GitHub{}, githubReleaseKey{}, errors.Wrapf(err, "failed to unmarshal configuration")
	}
	if err := publisher.SetRequiredStringConfigValues(flagVals,
		githubPublisherAPIURLFlag, &cfg.APIURL,
		githubPublisherUserFlag, &cfg.User,
		githubPublisherTokenFlag, &cfg.Token,
		githubPublisherRepositoryFlag, &cfg.Repository,
	); err != nil {
		return config.GitHub{}, githubReleaseKey{}, err
	}

	if err := publisher.SetConfigValue(flagVals, githubPublisherOwnerFlag, &cfg.Owner); err != nil {
		return config.GitHub{}, githubReleaseKey{}, err
	}
	if cfg.Owner == "" {
		cfg.Owner = cfg.User
	}

	if err := publisher.SetConfigValue(flagVals, githubAddVPrefixFlag, &cfg.AddVPrefix); err != nil {
		return config.GitHub{}, githubReleaseKey{}, err
	}

	// if base URL does not end in "/", append it (trailing slash is required)
	if !strings.HasSuffix(cfg.APIURL, "/") {
		cfg.APIURL += "/"
	}

	releaseVersion := projectVersion
	if cfg.AddVPrefix {
		releaseVersion = "v" + releaseVersion
	}
	return cfg, githubReleaseKey{
		apiURL:         cfg.APIURL,
		owner:          cfg.Owner,
		repository:     cfg.Repository,
		releaseVersion: releaseVersion,
	}, nil
}

// newGitHubReleaseTarget builds the GitHub client for the given configuration and bundles it with the release key.
func newGitHubReleaseTarget(cfg config.GitHub, key githubReleaseKey) (githubReleaseTarget, error) {
	client := github.NewClient(oauth2.NewClient(context.Background(), oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: cfg.Token},
	)))

	// set base URL (should be of the form "https://api.github.com/")
	apiURL, err := url.Parse(cfg.APIURL)
	if err != nil {
		return githubReleaseTarget{}, errors.Wrapf(err, "failed to parse %s as URL for API calls", cfg.APIURL)
	}
	client.BaseURL = apiURL

	return githubReleaseTarget{
		key:    key,
		client: client,
		cfg:    cfg,
	}, nil
}

func prepareGitHubRelease(target githubReleaseTarget, dryRun bool, stdout io.Writer) (*github.RepositoryRelease, error) {
	var releaseRes *github.RepositoryRelease
	if !dryRun {
		var err error
		releaseRes, err = findExistingRelease(target.client, target.cfg.Owner, target.cfg.Repository, target.key.releaseVersion)
		if err != nil {
			return nil, err
		}
	}

	if releaseRes != nil && releaseRes.GetDraft() {
		distgo.PrintOrDryRunPrint(stdout, fmt.Sprintf("Using existing draft GitHub release %s for %s/%s...", target.key.releaseVersion, target.cfg.Owner, target.cfg.Repository), dryRun)
	} else if releaseRes != nil {
		distgo.PrintOrDryRunPrint(stdout, fmt.Sprintf("GitHub release %s for %s/%s is already published...", target.key.releaseVersion, target.cfg.Owner, target.cfg.Repository), dryRun)
	} else {
		distgo.PrintOrDryRunPrint(stdout, fmt.Sprintf("Creating GitHub release %s for %s/%s...", target.key.releaseVersion, target.cfg.Owner, target.cfg.Repository), dryRun)
		if !dryRun {
			// create the release as a draft since GitHub's immutable-releases feature rejects asset uploads to a
			// non-draft release, so uploads must happen before the release is published.
			var err error
			releaseRes, _, err = target.client.Repositories.CreateRelease(context.Background(), target.cfg.Owner, target.cfg.Repository, &github.RepositoryRelease{
				TagName: new(target.key.releaseVersion),
				Draft:   new(true),
			})
			if err != nil {
				// newline to complement "..." output
				_, _ = fmt.Fprintln(stdout)

				if ghErr, ok := err.(*github.ErrorResponse); ok && len(ghErr.Errors) > 0 && ghErr.Errors[0].Code == "already_exists" {
					// release already exists: attempt to get it instead
					gotRelease, _, err := target.client.Repositories.GetReleaseByTag(context.Background(), target.cfg.Owner, target.cfg.Repository, target.key.releaseVersion)
					if err != nil {
						return nil, errors.Errorf("Failed to get GitHub release %s for %s/%s", target.key.releaseVersion, target.cfg.Owner, target.cfg.Repository)
					}
					// if release is found, use it and upload to the release
					releaseRes = gotRelease
				} else {
					return nil, errors.Wrapf(err, "failed to create GitHub release %s for %s/%s...", target.key.releaseVersion, target.cfg.Owner, target.cfg.Repository)
				}
			}
		}
	}
	// no need for dry run print because beginning of line has already been printed
	_, _ = fmt.Fprintln(stdout, "done")
	return releaseRes, nil
}

func publishGitHubRelease(target githubReleaseTarget, release *github.RepositoryRelease, dryRun bool, stdout io.Writer) error {
	distgo.PrintOrDryRunPrint(stdout, fmt.Sprintf("Publishing GitHub release %s for %s/%s...", target.key.releaseVersion, target.cfg.Owner, target.cfg.Repository), dryRun)
	if !dryRun {
		if _, _, err := target.client.Repositories.EditRelease(context.Background(), target.cfg.Owner, target.cfg.Repository, release.GetID(), &github.RepositoryRelease{
			Draft: new(false),
		}); err != nil {
			_, _ = fmt.Fprintln(stdout)
			return errors.Wrapf(err, "failed to publish GitHub release %s for %s/%s after uploading assets", target.key.releaseVersion, target.cfg.Owner, target.cfg.Repository)
		}
	}
	_, _ = fmt.Fprintln(stdout, "done")
	return nil
}

// findExistingRelease returns the first release (draft or already published) whose tag matches the provided tag, or
// nil if no such release exists.
func findExistingRelease(client *github.Client, owner, repo, tag string) (*github.RepositoryRelease, error) {
	opt := &github.ListOptions{PerPage: 100}
	for {
		releases, resp, err := client.Repositories.ListReleases(context.Background(), owner, repo, opt)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list existing GitHub releases for %s/%s", owner, repo)
		}
		for _, release := range releases {
			if release.GetTagName() == tag {
				return release, nil
			}
		}
		if resp.NextPage == 0 {
			return nil, nil
		}
		opt.Page = resp.NextPage
	}
}

func (p *githubPublisher) uploadFileAtPath(client *github.Client, release *github.RepositoryRelease, filePath string, dryRun bool, stdout io.Writer) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to open artifact %s for upload", filePath)
	}
	defer func() {
		_ = f.Close()
	}()

	if dryRun {
		distgo.DryRunPrintln(stdout, fmt.Sprintf("Uploading %s to GitHub (destination URL cannot be computed in dry run)", f.Name()))
		return "", nil
	}

	assetName := path.Base(filePath)
	if existingAsset := findReleaseAsset(release, assetName); existingAsset != nil {
		stat, err := f.Stat()
		if err != nil {
			return "", errors.Wrapf(err, "failed to stat artifact %s", filePath)
		}
		if int64(existingAsset.GetSize()) != stat.Size() {
			return "", errors.Errorf("GitHub release already has an asset named %s (%d bytes) that does not match the local artifact %s (%d bytes)", assetName, existingAsset.GetSize(), filePath, stat.Size())
		}
		_, _ = fmt.Fprintf(stdout, "%s already uploaded to GitHub, skipping\n", f.Name())
		return existingAsset.GetBrowserDownloadURL(), nil
	}

	uploadURI, err := uploadURIForProduct(release.GetUploadURL(), assetName)
	if err != nil {
		return "", err
	}

	uploadRes, _, err := githubUploadReleaseAssetWithProgress(context.Background(), client, uploadURI, f, stdout)
	if err != nil {
		return "", errors.Wrapf(err, "failed to upload artifact %s", filePath)
	}
	return uploadRes.GetBrowserDownloadURL(), nil
}

// findReleaseAsset returns the asset in release.Assets with the given name, or nil if there is no match.
func findReleaseAsset(release *github.RepositoryRelease, name string) *github.ReleaseAsset {
	for i := range release.Assets {
		if release.Assets[i].GetName() == name {
			return &release.Assets[i]
		}
	}
	return nil
}

// uploadURIForProduct returns an asset upload URI using the provided upload template from the release creation
// response. See https://developer.github.com/v3/repos/releases/#response for the specifics of the API.
func uploadURIForProduct(githubUploadURLTemplate, name string) (string, error) {
	const nameTemplate = "name"

	t, err := uritemplates.Parse(githubUploadURLTemplate)
	if err != nil {
		return "", errors.Wrapf(err, "failed to parse upload URI template %q", githubUploadURLTemplate)
	}
	uploadURI, err := t.Expand(map[string]any{
		nameTemplate: name,
	})
	if err != nil {
		return "", errors.Wrapf(err, "failed to expand URI template %q with %q = %q", githubUploadURLTemplate, nameTemplate, name)
	}
	return uploadURI, nil
}

// Based on github.Repositories.UploadReleaseAsset. Adds support for progress reporting.
func githubUploadReleaseAssetWithProgress(ctx context.Context, client *github.Client, uploadURI string, file *os.File, stdout io.Writer) (*github.ReleaseAsset, *github.Response, error) {
	stat, err := file.Stat()
	if err != nil {
		return nil, nil, err
	}
	if stat.IsDir() {
		return nil, nil, errors.New("the asset to upload can't be a directory")
	}

	_, _ = fmt.Fprintf(stdout, "Uploading %s to %s\n", file.Name(), uploadURI)
	bar := pb.New(int(stat.Size())).SetUnits(pb.U_BYTES)
	bar.Output = stdout
	bar.SetMaxWidth(120)
	bar.Start()
	defer bar.Finish()
	reader := bar.NewProxyReader(file)

	mediaType := mime.TypeByExtension(filepath.Ext(file.Name()))
	req, err := client.NewUploadRequest(uploadURI, reader, stat.Size(), mediaType)
	if err != nil {
		return nil, nil, err
	}

	asset := new(github.ReleaseAsset)
	resp, err := client.Do(ctx, req, asset)
	if err != nil {
		return nil, resp, err
	}
	return asset, resp, nil
}
