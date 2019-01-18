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

package distgo

import (
	"sort"

	"github.com/palantir/godel/pkg/osarch"
	"github.com/pkg/errors"
)

type DockerID string

type ByDockerID []DockerID

func (a ByDockerID) Len() int           { return len(a) }
func (a ByDockerID) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByDockerID) Less(i, j int) bool { return a[i] < a[j] }

type DockerTagID string

type ByDockerTagID []DockerTagID

func (a ByDockerTagID) Len() int           { return len(a) }
func (a ByDockerTagID) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByDockerTagID) Less(i, j int) bool { return a[i] < a[j] }

type DockerParam struct {
	// Repository is the Docker repository. This value is made available to TagTemplates as {{Repository}}.
	Repository string

	// DockerBuilderParams contains the Docker params for this distribution.
	DockerBuilderParams map[DockerID]DockerBuilderParam
}

type DockerOutputInfos struct {
	DockerIDs                []DockerID                           `json:"dockerIds"`
	Repository               string                               `json:"repository"`
	DockerBuilderOutputInfos map[DockerID]DockerBuilderOutputInfo `json:"dockerBuilderOutputInfos"`
}

type OSArchID string

type ByOSArchID []OSArchID

func (a ByOSArchID) Len() int           { return len(a) }
func (a ByOSArchID) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByOSArchID) Less(i, j int) bool { return a[i] < a[j] }

type DockerBuilderOutputInfo struct {
	ContextDir            string                              `json:"contextDir"`
	DockerfilePath        string                              `json:"dockerfilePath"`
	InputProductsDir      string                              `json:"inputProductsDir"`
	RenderedTags          []string                            `json:"renderedDockerTags"`
	RenderedTagsMap       map[DockerTagID]string              `json:"renderedDockerTagsMap"`
	InputBuilds           map[ProductID]map[OSArchID]struct{} `json:"inputBuilds"`
	InputDists            map[ProductID]map[DistID]struct{}   `json:"inputDists"`
	InputDistsOutputPaths map[ProductID]map[DistID][]string   `json:"inputDistsOutputPaths"`
}

func (doi *DockerBuilderOutputInfo) InputBuildProductIDs() []ProductID {
	var productIDs []ProductID
	for k := range doi.InputBuilds {
		productIDs = append(productIDs, k)
	}
	sort.Sort(ByProductID(productIDs))
	return productIDs
}

func (doi *DockerBuilderOutputInfo) InputBuildOSArchs(productID ProductID) []OSArchID {
	var osArchIDs []OSArchID
	for k := range doi.InputBuilds[productID] {
		osArchIDs = append(osArchIDs, k)
	}
	sort.Sort(ByOSArchID(osArchIDs))
	return osArchIDs
}

func (doi *DockerBuilderOutputInfo) InputDistProductIDs() []ProductID {
	var productIDs []ProductID
	for k := range doi.InputDists {
		productIDs = append(productIDs, k)
	}
	sort.Sort(ByProductID(productIDs))
	return productIDs
}

func (doi *DockerBuilderOutputInfo) InputDistDistIDs(productID ProductID) []DistID {
	var distIDs []DistID
	for k := range doi.InputDists[productID] {
		distIDs = append(distIDs, k)
	}
	sort.Sort(ByDistID(distIDs))
	return distIDs
}

func (p *DockerParam) ToDockerOutputInfos(productID ProductID, version string) (DockerOutputInfos, error) {
	var dockerIDs []DockerID
	var dockerOutputInfos map[DockerID]DockerBuilderOutputInfo
	if len(p.DockerBuilderParams) > 0 {
		dockerOutputInfos = make(map[DockerID]DockerBuilderOutputInfo)
		for dockerID, dockerBuilderParam := range p.DockerBuilderParams {
			dockerIDs = append(dockerIDs, dockerID)
			currDockerOutputInfo, err := dockerBuilderParam.ToDockerBuilderOutputInfo(productID, version, p.Repository)
			if err != nil {
				return DockerOutputInfos{}, err
			}
			dockerOutputInfos[dockerID] = currDockerOutputInfo
		}
	}
	sort.Sort(ByDockerID(dockerIDs))
	return DockerOutputInfos{
		DockerIDs:                dockerIDs,
		Repository:               p.Repository,
		DockerBuilderOutputInfos: dockerOutputInfos,
	}, nil
}

type DockerBuilderParam struct {
	// DockerBuilder is the builder used to build the Docker image.
	DockerBuilder DockerBuilder

	// Script is the content of a script that is written to a file and run before the build task. This script is run
	// before the Dockerfile is read and rendered. The content of this value is written to a file and executed with the
	// project directory as the working directory. The script process inherits the environment variables of the Go
	// process and also has Docker-related environment variables. Refer to the documentation for the
	// distgo.DockerScriptEnvVariables function for the extra environment variables.
	Script string

	// DockerfilePath is the path to the Dockerfile that is used to build the Docker image. The path is interpreted
	// relative to ContextDir. The content of the Dockerfile supports using Go templates. The following template
	// parameters can be used in the template:
	//   * {{Product}}: the name of the product
	//   * {{Version}}: the version of the project
	//   * {{Repository}}: the Docker repository. If the repository is non-empty and does not end in a '/', appends '/'.
	//   * {{RepositoryLiteral}}: the Docker repository exactly as specified (does not append a trailing '/')
	//   * {{InputBuildArtifact(productID, osArch string) (string, error)}}: the path to the build artifact for the specified input product
	//   * {{InputDistArtifacts(productID, distID string) ([]string, error)}}: the paths to the dist artifacts for the specified input product
	//   * {{Tag(productID, dockerID, tagKey string) (string, error)}}: the rendered tag for the specified Docker image tag
	//   * {{Tags(productID, dockerID string) ([]string, error)}}: the rendered tags for the specified Docker image. Returned in the same order as defined in configuration.
	DockerfilePath string

	// DisableTemplateRendering disables rendering the Go templates in the Dockerfile when set to true. This should only
	// be set to true if the Dockerfile does not use the Docker task templating and contains other Go templating -- in
	// this case, disabling rendering removes the need for the extra level of indirection usually necessary to render Go
	// templates using Go templates.
	DisableTemplateRendering bool

	// ContextDir is the Docker context directory for building the Docker image.
	ContextDir string

	// Name of directory within ContextDir in which dependencies are linked.
	InputProductsDir string

	// InputBuilds stores the ProductBuildIDs for the input builds. The IDs must be unique and in expanded form.
	InputBuilds []ProductBuildID

	// InputDists stores the ProductDistIDs for the input dists. The IDs must be unique and in expanded form.
	InputDists []ProductDistID

	// InputDistsOutputPaths is a map from ProductDistID to overridden output paths for the dist artifacts for that
	// ProductDistID.
	InputDistsOutputPaths map[ProductDistID][]string

	// TagTemplates contains the templates for the tags that will be used to tag the image generated by this builder.
	// The tag should be the form that would be provided to the "docker tag" command -- for example,
	// "fedora/httpd:version1.0" or "myregistryhost:5000/fedora/httpd:version1.0".
	//
	// The tag templates are rendered using Go templates. The following template parameters can be used in the template:
	//   * {{Product}}: the name of the product
	//   * {{Version}}: the version of the project
	//   * {{Repository}}: the Docker repository. If the repository is non-empty and does not end in a '/', appends '/'.
	//   * {{RepositoryLiteral}}: the Docker repository exactly as specified (does not append a trailing '/')
	TagTemplates TagTemplatesMap
}

type TagTemplatesMap struct {
	Templates   map[DockerTagID]string
	OrderedKeys []DockerTagID
}

func (p *DockerBuilderParam) ToDockerBuilderOutputInfo(productID ProductID, version, repository string) (DockerBuilderOutputInfo, error) {
	renderedTagsMap := make(map[DockerTagID]string)
	var renderedTags []string
	for _, currTagTemplateKey := range p.TagTemplates.OrderedKeys {
		currRenderedTag, err := RenderTemplate(p.TagTemplates.Templates[currTagTemplateKey], nil,
			ProductTemplateFunction(productID),
			VersionTemplateFunction(version),
			RepositoryTemplateFunction(repository),
			RepositoryLiteralTemplateFunction(repository),
		)
		if err != nil {
			return DockerBuilderOutputInfo{}, err
		}
		renderedTags = append(renderedTags, currRenderedTag)
		renderedTagsMap[currTagTemplateKey] = currRenderedTag
	}
	var inputBuilds map[ProductID]map[OSArchID]struct{}
	if len(p.InputBuilds) > 0 {
		inputBuilds = make(map[ProductID]map[OSArchID]struct{})
		for _, productBuildID := range p.InputBuilds {
			productID, buildID, err := productBuildID.Parse()
			if err != nil {
				return DockerBuilderOutputInfo{}, err
			}
			if buildID == (osarch.OSArch{}) {
				return DockerBuilderOutputInfo{}, errors.Errorf("BuildID cannot be empty")
			}
			if _, ok := inputBuilds[productID]; !ok {
				inputBuilds[productID] = make(map[OSArchID]struct{})
			}
			inputBuilds[productID][OSArchID(buildID.String())] = struct{}{}
		}
	}
	var inputDists map[ProductID]map[DistID]struct{}
	if len(p.InputDists) > 0 {
		inputDists = make(map[ProductID]map[DistID]struct{})
		for _, productDistID := range p.InputDists {
			productID, distID := productDistID.Parse()
			if distID == "" {
				return DockerBuilderOutputInfo{}, errors.Errorf("DistID cannot be empty")
			}
			if _, ok := inputDists[productID]; !ok {
				inputDists[productID] = make(map[DistID]struct{})
			}
			inputDists[productID][distID] = struct{}{}
		}
	}
	var inputDistsOutputPaths map[ProductID]map[DistID][]string
	if len(p.InputDistsOutputPaths) > 0 {
		inputDistsOutputPaths = make(map[ProductID]map[DistID][]string)
		for productDistID, paths := range p.InputDistsOutputPaths {
			productID, distID := productDistID.Parse()
			if distID == "" {
				return DockerBuilderOutputInfo{}, errors.Errorf("DistID cannot be empty")
			}
			if _, ok := inputDistsOutputPaths[productID]; !ok {
				inputDistsOutputPaths[productID] = make(map[DistID][]string)
			}
			inputDistsOutputPaths[productID][distID] = paths
		}
	}
	return DockerBuilderOutputInfo{
		ContextDir:            p.ContextDir,
		DockerfilePath:        p.DockerfilePath,
		InputProductsDir:      p.InputProductsDir,
		RenderedTagsMap:       renderedTagsMap,
		RenderedTags:          renderedTags,
		InputBuilds:           inputBuilds,
		InputDists:            inputDists,
		InputDistsOutputPaths: inputDistsOutputPaths,
	}, nil
}
