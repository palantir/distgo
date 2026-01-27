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

package cmdinternal

import (
	"os"

	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/distgo/config"
	godelconfig "github.com/palantir/godel/v2/framework/godel/config"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

// GlobalFlagValsAndFactories is a struct that stores the global flag values and factories set by commands.
type GlobalFlagValsAndFactories struct {
	DebugFlagVal            bool
	ProjectDirFlagVal       string
	DistgoConfigFileFlagVal string
	GodelConfigFileFlagVal  string
	AssetsFlagVal           []string

	CLIProjectVersionerFactory distgo.ProjectVersionerFactory
	CLIDisterFactory           distgo.DisterFactory
	CLIDefaultDisterCfg        config.DisterConfig
	CLIDockerBuilderFactory    distgo.DockerBuilderFactory
	CLIPublisherFactory        distgo.PublisherFactory
}

func DistgoProjectParamFromFlagVals(flagValsAndFactories GlobalFlagValsAndFactories) (distgo.ProjectInfo, distgo.ProjectParam, error) {
	return distgoProjectParamFromVals(
		flagValsAndFactories.ProjectDirFlagVal,
		flagValsAndFactories.DistgoConfigFileFlagVal,
		flagValsAndFactories.GodelConfigFileFlagVal,
		flagValsAndFactories.CLIProjectVersionerFactory,
		flagValsAndFactories.CLIDisterFactory,
		flagValsAndFactories.CLIDefaultDisterCfg,
		flagValsAndFactories.CLIDockerBuilderFactory,
		flagValsAndFactories.CLIPublisherFactory,
	)
}

func distgoProjectParamFromVals(
	projectDir,
	distgoConfigFile,
	godelConfigFile string,
	projectVersionerFactory distgo.ProjectVersionerFactory,
	disterFactory distgo.DisterFactory,
	defaultDisterCfg config.DisterConfig,
	dockerBuilderFactory distgo.DockerBuilderFactory,
	publisherFactory distgo.PublisherFactory,
) (distgo.ProjectInfo, distgo.ProjectParam, error) {

	var distgoCfg config.ProjectConfig
	if distgoConfigFile != "" {
		cfg, err := loadConfigFromFile(
			distgoConfigFile,
			projectVersionerFactory,
			disterFactory,
			dockerBuilderFactory,
			publisherFactory,
		)
		if err != nil {
			return distgo.ProjectInfo{}, distgo.ProjectParam{}, err
		}
		distgoCfg = cfg
	}
	if godelConfigFile != "" {
		excludes, err := godelconfig.ReadGodelConfigExcludesFromFile(godelConfigFile)
		if err != nil {
			return distgo.ProjectInfo{}, distgo.ProjectParam{}, err
		}
		distgoCfg.Exclude.Add(excludes)
	}
	projectParam, err := distgoCfg.ToParam(projectDir, projectVersionerFactory, disterFactory, defaultDisterCfg, dockerBuilderFactory, publisherFactory)
	if err != nil {
		return distgo.ProjectInfo{}, distgo.ProjectParam{}, err
	}
	projectInfo, err := projectParam.ProjectInfo(projectDir)
	if err != nil {
		return distgo.ProjectInfo{}, distgo.ProjectParam{}, err
	}
	return projectInfo, projectParam, nil
}

func loadConfigFromFile(
	cfgFile string,
	projectVersionerFactory distgo.ProjectVersionerFactory,
	disterFactory distgo.DisterFactory,
	dockerBuilderFactory distgo.DockerBuilderFactory,
	publisherFactory distgo.PublisherFactory,
) (config.ProjectConfig, error) {

	cfgBytes, err := os.ReadFile(cfgFile)
	if os.IsNotExist(err) {
		return config.ProjectConfig{}, nil
	}
	if err != nil {
		return config.ProjectConfig{}, errors.Wrapf(err, "failed to read configuration file")
	}
	upgradedCfgBytes, err := config.UpgradeConfig(
		cfgBytes,
		projectVersionerFactory,
		disterFactory,
		dockerBuilderFactory,
		publisherFactory,
	)
	if err != nil {
		return config.ProjectConfig{}, errors.Wrapf(err, "failed to upgrade configuration")
	}

	var cfg config.ProjectConfig
	if err := yaml.Unmarshal(upgradedCfgBytes, &cfg); err != nil {
		return config.ProjectConfig{}, errors.Wrapf(err, "failed to unmarshal configuration")
	}
	return cfg, nil
}
