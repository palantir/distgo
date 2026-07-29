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
	"crypto/sha256"
	"fmt"
	"io"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/google/go-github/v89/github"
	"github.com/jtacoma/uritemplates"
	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/publisher"
	"github.com/palantir/distgo/publisher/github/config"
	"github.com/pkg/errors"
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

func (p *githubPublisher) RunPublish(inputs []distgo.ProductPublishInfo, flagVals map[distgo.PublisherFlagName]any, dryRun bool, stdout io.Writer) error {
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
	var releases []githubReleaseProducts
	releaseKeyToIndex := make(map[githubReleaseKey]int)
	for _, input := range inputs {
		productTaskOutputInfo := input.ProductTaskOutputInfo
		publisher.FilterProductTaskOutputInfoArtifactNames(&productTaskOutputInfo, filterRegexp, excludeRegexp)

		cfg, key, err := resolveGitHubReleaseConfig(input.PublisherConfigYML, flagVals, productTaskOutputInfo.Project.Version)
		if err != nil {
			return errors.Wrapf(err, "failed to resolve GitHub config for product %s", productTaskOutputInfo.Product.ID)
		}

		releaseIndex, ok := releaseKeyToIndex[key]
		if !ok {
			// if release target does not exist for the key, create it
			target, err := newGitHubReleaseTarget(cfg, key)
			if err != nil {
				return errors.Wrapf(err, "failed to resolve GitHub release target for product %s", productTaskOutputInfo.Product.ID)
			}
			releaseIndex = len(releases)
			releaseKeyToIndex[key] = releaseIndex
			releases = append(releases, githubReleaseProducts{target: target})
		} else if existingToken := releases[releaseIndex].target.cfg.Token; existingToken != cfg.Token {
			// githubReleaseKey does not include the token, so products that otherwise resolve to the same release
			// but specify different tokens would silently use whichever token belonged to the first product seen.
			// There's no well-defined choice of token to use for the release as a whole in that case, so fail loudly.
			return errors.Errorf("product %s resolves to the same GitHub release (%s/%s, tag %s) as an earlier product in this batch, but specifies a different token", productTaskOutputInfo.Product.ID, key.owner, key.repository, key.releaseVersion)
		}
		releases[releaseIndex].products = append(releases[releaseIndex].products, productTaskOutputInfo)
	}

	// With grouping done, run the core prepare/upload/publish workflow once per release. For each release, create or
	// reuse the release, upload every product's assets to the release, then publish it.
	for _, releaseProducts := range releases {
		release, err := prepareGitHubRelease(releaseProducts.target, dryRun, stdout)
		if err != nil {
			return err
		}

		for _, productTaskOutputInfo := range releaseProducts.products {
			for _, currDistID := range productTaskOutputInfo.Product.DistOutputInfos.DistIDs {
				for _, currArtifactPath := range productTaskOutputInfo.ProductDistArtifactPaths()[currDistID] {
					if _, err := p.uploadFileAtPath(releaseProducts.target.client, release, currArtifactPath, dryRun, stdout); err != nil {
						return errors.Wrapf(err, "failed to publish product %s", productTaskOutputInfo.Product.ID)
					}
				}
			}
		}

		// Nothing left to do if the release is already live. Dry-run's release is always nil, so this never skips there.
		if !dryRun && !release.GetDraft() {
			continue
		}
		if err := publishGitHubRelease(releaseProducts.target, release, dryRun, stdout); err != nil {
			return err
		}
	}
	return nil
}

// githubReleaseProducts stores the products that should be published for a particular GitHub release.
type githubReleaseProducts struct {
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
	client, err := github.NewClient(
		github.WithAuthToken(cfg.Token),
		github.WithURLs(&cfg.APIURL, &cfg.APIURL),
	)
	if err != nil {
		return githubReleaseTarget{}, errors.Wrapf(err, "failed to create GitHub client for %s", cfg.APIURL)
	}

	return githubReleaseTarget{
		key:    key,
		client: client,
		cfg:    cfg,
	}, nil
}

// prepareGitHubRelease creates or reuses the GitHub release for the given target, returning it as a draft so that
// GitHub's immutable-releases feature does not reject the asset uploads that follow.
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
			releaseRes, _, err = target.client.Repositories.CreateRelease(context.Background(), target.cfg.Owner, target.cfg.Repository, github.CreateReleaseRequest{
				TagName: target.key.releaseVersion,
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

// publishGitHubRelease un-drafts the given release now that all of its batch's assets have been uploaded.
func publishGitHubRelease(target githubReleaseTarget, release *github.RepositoryRelease, dryRun bool, stdout io.Writer) error {
	distgo.PrintOrDryRunPrint(stdout, fmt.Sprintf("Publishing GitHub release %s for %s/%s...", target.key.releaseVersion, target.cfg.Owner, target.cfg.Repository), dryRun)
	if !dryRun {
		if _, _, err := target.client.Repositories.UpdateRelease(context.Background(), target.cfg.Owner, target.cfg.Repository, release.GetID(), github.UpdateReleaseRequest{
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
		matches, err := existingAssetMatchesLocalFile(existingAsset, f)
		if err != nil {
			return "", errors.Wrapf(err, "failed to compare existing GitHub asset %s to local artifact %s", assetName, filePath)
		}
		if !matches {
			return "", errors.Errorf("GitHub release already has an asset named %s that does not match the local artifact %s", assetName, filePath)
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
			return release.Assets[i]
		}
	}
	return nil
}

// existingAssetMatchesLocalFile reports whether existingAsset's content matches the local file, read from f. GitHub
// populates the digest of uploaded release assets, so prefer comparing SHA-256 digests when one is present; fall
// back to comparing size only for assets that predate digest support. f must be positioned at the start, and its
// position is restored to the start before returning so it can still be used for the upload afterward.
func existingAssetMatchesLocalFile(existingAsset *github.ReleaseAsset, f *os.File) (bool, error) {
	defer func() {
		_, _ = f.Seek(0, io.SeekStart)
	}()

	if digest := existingAsset.GetDigest(); digest != "" {
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return false, err
		}
		return fmt.Sprintf("sha256:%x", h.Sum(nil)) == digest, nil
	}

	stat, err := f.Stat()
	if err != nil {
		return false, err
	}
	return int64(existingAsset.GetSize()) == stat.Size(), nil
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
	req, err := client.NewUploadRequest(ctx, uploadURI, reader, stat.Size(), mediaType)
	if err != nil {
		return nil, nil, err
	}

	asset := new(github.ReleaseAsset)
	resp, err := client.Do(req, asset)
	if err != nil {
		return nil, resp, err
	}
	return asset, resp, nil
}
