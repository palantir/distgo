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
	}, nil
}

func (p *githubPublisher) RunPublish(productTaskOutputInfo distgo.ProductTaskOutputInfo, cfgYML []byte, flagVals map[distgo.PublisherFlagName]interface{}, dryRun bool, stdout io.Writer) error {
	var cfg config.GitHub
	if err := yaml.Unmarshal(cfgYML, &cfg); err != nil {
		return errors.Wrapf(err, "failed to unmarshal configuration")
	}
	if err := publisher.SetRequiredStringConfigValues(flagVals,
		githubPublisherAPIURLFlag, &cfg.APIURL,
		githubPublisherUserFlag, &cfg.User,
		githubPublisherTokenFlag, &cfg.Token,
		githubPublisherRepositoryFlag, &cfg.Repository,
	); err != nil {
		return err
	}

	if err := publisher.SetConfigValue(flagVals, githubPublisherOwnerFlag, &cfg.Owner); err != nil {
		return err
	}
	if cfg.Owner == "" {
		cfg.Owner = cfg.User
	}

	if err := publisher.SetConfigValue(flagVals, githubAddVPrefixFlag, &cfg.AddVPrefix); err != nil {
		return err
	}

	client := github.NewClient(oauth2.NewClient(context.Background(), oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: cfg.Token},
	)))

	// if base URL does not end in "/", append it (trailing slash is required)
	if !strings.HasSuffix(cfg.APIURL, "/") {
		cfg.APIURL += "/"
	}
	// set base URL (should be of the form "https://api.github.com/")
	apiURL, err := url.Parse(cfg.APIURL)
	if err != nil {
		return errors.Wrapf(err, "failed to parse %s as URL for API calls", cfg.APIURL)
	}
	client.BaseURL = apiURL

	releaseVersion := productTaskOutputInfo.Project.Version
	if cfg.AddVPrefix {
		releaseVersion = "v" + releaseVersion
	}

	distgo.PrintOrDryRunPrint(stdout, fmt.Sprintf("Creating GitHub release %s for %s/%s...", releaseVersion, cfg.Owner, cfg.Repository), dryRun)

	var releaseRes *github.RepositoryRelease
	if !dryRun {
		releaseRes, _, err = client.Repositories.CreateRelease(context.Background(), cfg.Owner, cfg.Repository, &github.RepositoryRelease{
			TagName: github.String(releaseVersion),
		})
		if err != nil {
			// newline to complement "..." output
			// no need for dry run print because beginning of line has already been printed
			_, _ = fmt.Fprintln(stdout)

			if ghErr, ok := err.(*github.ErrorResponse); ok && len(ghErr.Errors) > 0 && ghErr.Errors[0].Code == "already_exists" {
				// release already exists: attempt to get it instead
				gotRelease, _, err := client.Repositories.GetReleaseByTag(context.Background(), cfg.Owner, cfg.Repository, releaseVersion)
				if err != nil {
					return errors.Errorf("Failed to get GitHub release %s for %s/%s", releaseVersion, cfg.Owner, cfg.Repository)
				}
				// if release is found, use it and upload to the release
				releaseRes = gotRelease
			} else {
				return errors.Wrapf(err, "failed to create GitHub release %s for %s/%s...", releaseVersion, cfg.Owner, cfg.Repository)
			}
		}
	}
	// no need for dry run print because beginning of line has already been printed
	_, _ = fmt.Fprintln(stdout, "done")

	for _, currDistID := range productTaskOutputInfo.Product.DistOutputInfos.DistIDs {
		for _, currArtifactPath := range productTaskOutputInfo.ProductDistArtifactPaths()[currDistID] {
			if _, err := p.uploadFileAtPath(client, releaseRes, currArtifactPath, dryRun, stdout); err != nil {
				return err
			}
		}
	}
	return nil
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

	uploadURI, err := uploadURIForProduct(release.GetUploadURL(), path.Base(filePath))
	if err != nil {
		return "", err
	}

	uploadRes, _, err := githubUploadReleaseAssetWithProgress(context.Background(), client, uploadURI, f, stdout)
	if err != nil {
		return "", errors.Wrapf(err, "failed to upload artifact %s", filePath)
	}
	return uploadRes.GetBrowserDownloadURL(), nil
}

// uploadURIForProduct returns an asset upload URI using the provided upload template from the release creation
// response. See https://developer.github.com/v3/repos/releases/#response for the specifics of the API.
func uploadURIForProduct(githubUploadURLTemplate, name string) (string, error) {
	const nameTemplate = "name"

	t, err := uritemplates.Parse(githubUploadURLTemplate)
	if err != nil {
		return "", errors.Wrapf(err, "failed to parse upload URI template %q", githubUploadURLTemplate)
	}
	uploadURI, err := t.Expand(map[string]interface{}{
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
