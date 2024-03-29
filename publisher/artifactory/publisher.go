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

package artifactory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/publisher"
	"github.com/palantir/distgo/publisher/artifactory/config"
	"github.com/palantir/distgo/publisher/maven"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

const TypeName = "artifactory" // publishes output artifacts to Artifactory

type Publisher interface {
	distgo.Publisher
	ArtifactoryRunPublish(productTaskOutputInfo distgo.ProductTaskOutputInfo, cfgYML []byte, flagVals map[distgo.PublisherFlagName]interface{}, dryRun bool, stdout io.Writer) ([]string, error)
}

func PublisherCreator() publisher.Creator {
	return publisher.NewCreator(TypeName, func() distgo.Publisher {
		return NewArtifactoryPublisher()
	})
}

func NewArtifactoryPublisher() Publisher {
	return &artifactoryPublisher{}
}

type artifactoryPublisher struct{}

func (p *artifactoryPublisher) TypeName() (string, error) {
	return TypeName, nil
}

var (
	PublisherRepositoryFlag = distgo.PublisherFlag{
		Name:        "repository",
		Description: "repository that is the destination for the publish",
		Type:        distgo.StringFlag,
	}
)

func (p *artifactoryPublisher) Flags() ([]distgo.PublisherFlag, error) {
	return append(publisher.BasicConnectionInfoFlags(),
		PublisherRepositoryFlag,
		publisher.GroupIDFlag,
		publisher.ArtifactNamesFilterFlag,
		publisher.ArtifactNamesExcludeFlag,
		maven.NoPOMFlag,
	), nil
}

func (p *artifactoryPublisher) RunPublish(productTaskOutputInfo distgo.ProductTaskOutputInfo, cfgYML []byte, flagVals map[distgo.PublisherFlagName]interface{}, dryRun bool, stdout io.Writer) error {
	_, err := p.ArtifactoryRunPublish(productTaskOutputInfo, cfgYML, flagVals, dryRun, stdout)
	return err
}

func (p *artifactoryPublisher) ArtifactoryRunPublish(productTaskOutputInfo distgo.ProductTaskOutputInfo, cfgYML []byte, flagVals map[distgo.PublisherFlagName]interface{}, dryRun bool, stdout io.Writer) ([]string, error) {
	var cfg config.Artifactory
	if err := yaml.Unmarshal(cfgYML, &cfg); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal configuration")
	}
	groupID, err := publisher.GetRequiredGroupID(flagVals, productTaskOutputInfo)
	if err != nil {
		return nil, err
	}
	if err := cfg.BasicConnectionInfo.SetValuesFromFlags(flagVals); err != nil {
		return nil, err
	}
	if err := publisher.SetRequiredStringConfigValue(flagVals, PublisherRepositoryFlag, &cfg.Repository); err != nil {
		return nil, err
	}
	if err := publisher.SetConfigValue(flagVals, maven.NoPOMFlag, &cfg.NoPOM); err != nil {
		return nil, err
	}

	filterRegexp, err := publisher.GetArtifactNamesFilterFlagValue(flagVals)
	if err != nil {
		return nil, err
	}
	excludeRegexp, err := publisher.GetArtifactNamesExcludeFlagValue(flagVals)
	if err != nil {
		return nil, err
	}
	publisher.FilterProductTaskOutputInfoArtifactNames(&productTaskOutputInfo, filterRegexp, excludeRegexp)

	artifactoryURL := strings.Join([]string{cfg.URL, "artifactory"}, "/")
	productPath := publisher.MavenProductPath(productTaskOutputInfo, groupID)
	artifactExists := func(dstFileName string, checksums publisher.Checksums, username, password string) bool {
		rawCheckArtifactURL := strings.Join([]string{artifactoryURL, "api", "storage", cfg.Repository, productPath, dstFileName}, "/")
		checkArtifactURL, err := url.Parse(rawCheckArtifactURL)
		if err != nil {
			return false
		}

		header := http.Header{}
		req := http.Request{
			Method: http.MethodGet,
			URL:    checkArtifactURL,
			Header: header,
		}
		req.SetBasicAuth(username, password)

		if resp, err := http.DefaultClient.Do(&req); err == nil {
			defer func() {
				// nothing to be done if close fails
				_ = resp.Body.Close()
			}()

			if bytes, err := ioutil.ReadAll(resp.Body); err == nil {
				var jsonMap map[string]*json.RawMessage
				if err := json.Unmarshal(bytes, &jsonMap); err == nil {
					if checksumJSON, ok := jsonMap["Checksums"]; ok && checksumJSON != nil {
						var dstChecksums publisher.Checksums
						if err := json.Unmarshal(*checksumJSON, &dstChecksums); err == nil {
							return checksums.Match(dstChecksums)
						}
					}
				}
			}
			return false
		}
		return false
	}

	deploymentURL, err := p.getDeploymentURL(cfg)
	if err != nil {
		return nil, err
	}
	baseURL := strings.Join([]string{deploymentURL, productPath}, "/")
	artifactPaths, uploadedURLs, err := cfg.BasicConnectionInfo.UploadDistArtifacts(productTaskOutputInfo, baseURL, artifactExists, dryRun, stdout)
	if err != nil {
		return nil, err
	}
	var artifactNames []string
	for _, currArtifactPath := range artifactPaths {
		artifactNames = append(artifactNames, path.Base(currArtifactPath))
	}

	// if no artifacts were uploaded (for example, because all artifacts were filtered out based on regular
	// expressions), nothing more to do (don't upload POM).
	if len(uploadedURLs) == 0 {
		return uploadedURLs, nil
	}

	if !cfg.NoPOM {
		pomName, pomContent, err := maven.POM(groupID, productTaskOutputInfo)
		if err != nil {
			return nil, err
		}
		artifactNames = append(artifactNames, pomName)
		// do not include POM in uploadedURLs
		if _, err := cfg.UploadFile(publisher.NewFileInfoFromBytes([]byte(pomContent)), baseURL, pomName, artifactExists, dryRun, stdout); err != nil {
			return nil, err
		}
	}

	if !dryRun {
		// compute SHA-256 Checksums for artifacts
		if err := p.computeArtifactChecksums(cfg, artifactoryURL, productPath, artifactNames); err != nil {
			// if triggering checksum computation fails, print message but don't throw error
			_, _ = fmt.Fprintln(stdout, "Uploading artifacts succeeded, but failed to trigger computation of SHA-256 checksums:", err)
		}
	}
	return uploadedURLs, nil
}

// computeArtifactChecksums uses the "api/checksum/sha256" endpoint to compute the checksums for the provided artifacts.
func (p *artifactoryPublisher) computeArtifactChecksums(cfg config.Artifactory, artifactoryURL, productPath string, artifactNames []string) error {
	for _, currArtifactName := range artifactNames {
		currArtifactURL := strings.Join([]string{productPath, currArtifactName}, "/")
		if err := p.artifactorySetSHA256Checksum(cfg, artifactoryURL, currArtifactURL); err != nil {
			return errors.Wrapf(err, "")
		}
	}
	return nil
}

func (p *artifactoryPublisher) artifactorySetSHA256Checksum(cfg config.Artifactory, baseURLString, filePath string) (rErr error) {
	apiURLString := baseURLString + "/api/checksum/sha256"
	uploadURL, err := url.Parse(apiURLString)
	if err != nil {
		return errors.Wrapf(err, "failed to parse %s as URL", apiURLString)
	}

	jsonContent := fmt.Sprintf(`{"repoKey":"%s","path":"%s"}`, cfg.Repository, filePath)
	reader := strings.NewReader(jsonContent)

	header := http.Header{}
	header.Set("Content-Type", "application/json")
	req := http.Request{
		Method:        http.MethodPost,
		URL:           uploadURL,
		Header:        header,
		Body:          ioutil.NopCloser(reader),
		ContentLength: int64(len([]byte(jsonContent))),
	}
	req.SetBasicAuth(cfg.Username, cfg.Password)

	resp, err := http.DefaultClient.Do(&req)
	if err != nil {
		return errors.Wrapf(err, "failed to trigger computation of SHA-256 checksum for %s", filePath)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil && rErr == nil {
			rErr = errors.Wrapf(err, "failed to close response body for URL %s", apiURLString)
		}
	}()

	if resp.StatusCode >= http.StatusBadRequest {
		return errors.Errorf("triggering computation of SHA-256 checksum for %s resulted in response: %s", filePath, resp.Status)
	}
	return nil
}

func (p *artifactoryPublisher) getDeploymentURL(cfg config.Artifactory) (string, error) {
	url := strings.Join([]string{cfg.URL, "artifactory", cfg.Repository}, "/")
	encodedProps, err := encodeProperties(cfg.Properties)
	if err != nil {
		return "", err
	}

	return strings.Join(append([]string{url}, encodedProps...), ";"), nil
}

// encodeProperties takes in a map[string]string of properties, renders each value as a Go template, and returns
// the sorted slice of strings of the form `key=renderedVal`. The Go template can include the `env` function,
// which fetches an environment variable. All key and renderedVal fields will be properly URL encoded.
func encodeProperties(properties map[string]string) ([]string, error) {
	if len(properties) == 0 {
		return nil, nil
	}

	tmpl := template.New("properties").Funcs(template.FuncMap{
		"env": os.Getenv,
	})
	var encoded []string
	for k, v := range properties {
		parsed, err := tmpl.Clone()
		if err != nil {
			return nil, errors.Wrapf(err, "failed to clone template")
		}
		parsed, err = parsed.Parse(v)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse template")
		}
		output := &bytes.Buffer{}
		if err := parsed.Execute(output, nil); err != nil {
			return nil, errors.Wrapf(err, "failed to execute template")
		}
		if outStr := output.String(); len(outStr) > 0 {
			encoded = append(encoded, fmt.Sprintf("%s=%s", url.PathEscape(k), url.PathEscape(outStr)))
		}
	}
	sort.Strings(encoded)
	return encoded, nil
}
