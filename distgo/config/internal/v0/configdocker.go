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

package v0

import (
	"strconv"

	"gopkg.in/yaml.v2"

	"github.com/palantir/distgo/distgo"
)

type DockerConfig struct {
	// Repository is the repository that is made available to the tag and Dockerfile templates.
	Repository *string `yaml:"repository,omitempty"`

	// DockerBuilderParams contains the Docker params for this distribution.
	DockerBuildersConfig *DockerBuildersConfig `yaml:"docker-builders,omitempty"`
}

type DockerBuildersConfig map[distgo.DockerID]DockerBuilderConfig

type DockerBuilderConfig struct {
	// Type is the type of the DockerBuilder. This field must be non-nil and non-empty and resolve to a valid DockerBuilder.
	Type *string `yaml:"type,omitempty"`
	// Script is the content of a script that is written to a file and run before the build task. This script is run
	// before the Dockerfile is read and rendered. The content of this value is written to a file and executed with the
	// project directory as the working directory. The script process inherits the environment variables of the Go
	// process and also has Docker-related environment variables. Refer to the documentation for the
	// distgo.DockerScriptEnvVariables function for the extra environment variables.
	Script *string `yaml:"script,omitempty"`
	// Config is the YAML configuration content for the DockerBuilder.
	Config *yaml.MapSlice `yaml:"config,omitempty"`
	// DockerfilePath is the path to the Dockerfile that is used to build the Docker image. The path is interpreted
	// relative to ContextDir. The content of the Dockerfile supports using Go templates. The following template
	// parameters can be used in the template:
	//   * {{Product}}: the name of the product
	//   * {{Version}}: the version of the project
	//   * {{Repository}}: the Docker repository for the operation
	//   * {{InputBuildArtifact(productID, osArch string) (string, error)}}: the path to the build artifact for the specified input product
	//   * {{InputDistArtifacts(productID, distID string) ([]string, error)}}: the paths to the dist artifacts for the specified input product
	//   * {{Tags(productID, dockerID string) ([]string, error)}}: the rendered tags for the specified Docker image sorted in ascending order
	DockerfilePath *string `yaml:"dockerfile-path,omitempty"`
	// DisableTemplateRendering disables rendering the Go templates in the Dockerfile when set to true. This should only
	// be set to true if the Dockerfile does not use the Docker task templating and contains other Go templating -- in
	// this case, disabling rendering removes the need for the extra level of indirection usually necessary to render Go
	// templates using Go templates.
	DisableTemplateRendering *bool `yaml:"disable-template-rendering,omitempty"`
	// ContextDir is the Docker context directory for building the Docker image.
	ContextDir *string `yaml:"context-dir,omitempty"`
	// InputProductsDir is the directory in the context dir in which input products are written.
	InputProductsDir *string `yaml:"input-products-dir,omitempty"`
	// InputBuilds specifies the products whose build outputs should be made available to the Docker build task. The
	// specified products will be hard-linked into the context directory. The referenced products must be this product
	// or one of its declared dependencies.
	InputBuilds *[]distgo.ProductBuildID `yaml:"input-builds,omitempty"`
	// InputDists specifies the products whose dist outputs should be made available to the Docker build task. The
	// specified dists will be hard-linked into the context directory. The referenced products must be this product
	// or one of its declared dependencies.
	InputDists *[]distgo.ProductDistID `yaml:"input-dists,omitempty"`
	// InputDistsOutputPaths is an optional parameter that allows the paths of the input dists to be a specific
	// hard-coded location. The default behavior of InputDists places the dist outputs in a subdirectory of
	// InputProductsDir and relies on using the {{InputDistArtifacts}} template function to render their locations.
	// The InputDistsOutputPaths can be used to specify hard-coded paths for the dist outputs relative to the context
	// directory instead. Every key in InputDistsOutputPaths must identify a specific dist that is specified in
	// InputDists, even if the specification in InputDists is done at a product level. For example, if InputDists
	// specifies product "foo" and "foo" has "bar" and "baz" defined as dist types, then the only valid keys for
	// InputDistsOutputPaths are "foo.bar" and "foo.baz". The values are the locations that the distribution artifacts
	// should be placed, where each slice index must map to a dist artifact output index. If an output path is specified
	// for a distribution artifact, that path becomes the canonical one for that artifact for this Docker task -- the
	// artifact is placed only in that location, and that location is returned by the {{InputDistArtifacts}} template
	// function.
	InputDistsOutputPaths *map[distgo.ProductDistID][]string `yaml:"input-dist-output-paths,omitempty"`
	// TagTemplates specifies the templates that should be used to render the tag(s) for the Docker image. If multiple
	// values are specified, the image will be tagged with all of them.
	TagTemplates *TagTemplatesMap `yaml:"tag-templates,omitempty"`
}

type TagTemplatesMap map[distgo.DockerTagID]string

func (t *TagTemplatesMap) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var strVal string
	if err := unmarshal(&strVal); err == nil && strVal != "" {
		// value is specified as single string: unmarshal with default key
		*t = TagTemplatesMap{
			"default": strVal,
		}
		return nil
	}

	var strSliceVal []string
	if err := unmarshal(&strSliceVal); err == nil && len(strSliceVal) > 0 {
		// value is specified as string array: unmarshal with keys as string representation of index
		*t = make(TagTemplatesMap, len(strSliceVal))
		for i, val := range strSliceVal {
			(*t)[distgo.DockerTagID(strconv.Itoa(i))] = val
		}
		return nil
	}

	type TagTemplatesMapAlias TagTemplatesMap
	var val TagTemplatesMapAlias
	if err := unmarshal(&val); err != nil {
		return err
	}
	*t = TagTemplatesMap(val)
	return nil
}
