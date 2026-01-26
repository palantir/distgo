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

package cmd

import (
	"os"
	"time"

	"github.com/palantir/distgo/dister"
	"github.com/palantir/distgo/dister/disterfactory"
	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/distgo/config"
	"github.com/palantir/distgo/dockerbuilder"
	"github.com/palantir/distgo/dockerbuilder/dockerbuilderfactory"
	"github.com/palantir/distgo/internal/assetapi"
	"github.com/palantir/distgo/projectversioner/projectversionerfactory"
	"github.com/palantir/distgo/publisher"
	"github.com/palantir/distgo/publisher/publisherfactory"
	godelconfig "github.com/palantir/godel/v2/framework/godel/config"
	"github.com/palantir/godel/v2/framework/pluginapi"
	"github.com/palantir/pkg/cobracli"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

var (
	debugFlagVal            bool
	projectDirFlagVal       string
	distgoConfigFileFlagVal string
	godelConfigFileFlagVal  string
	assetsFlagVal           []string

	// stores the loaded assets. Assigned once at program startup.
	loadedAssets assetapi.Assets

	cliProjectVersionerFactory distgo.ProjectVersionerFactory
	cliDisterFactory           distgo.DisterFactory
	cliDefaultDisterCfg        config.DisterConfig
	cliDockerBuilderFactory    distgo.DockerBuilderFactory
	cliPublisherFactory        distgo.PublisherFactory
)

var rootCmd = &cobra.Command{
	Use: "distgo",
}

func Execute() int {
	return cobracli.ExecuteWithDebugVarAndDefaultParams(rootCmd, &debugFlagVal)
}

func restoreRootFlagsFn() func() {
	origProjectDirFlagVal := projectDirFlagVal
	origDistgoConfigFileFlagVal := distgoConfigFileFlagVal
	origGodelConfigFileFlagVal := godelConfigFileFlagVal
	origAssetsFlagVal := assetsFlagVal
	return func() {
		projectDirFlagVal = origProjectDirFlagVal
		distgoConfigFileFlagVal = origDistgoConfigFileFlagVal
		godelConfigFileFlagVal = origGodelConfigFileFlagVal
		assetsFlagVal = origAssetsFlagVal
	}
}

// LoadAssets loads the distgo assets from the global program arguments and stores the returned assets in the
// loadedAssets package-level variable.
func LoadAssets(args []string) error {
	// create the restoreFn to defer. Don't want to inline as part of defer
	// itself because it's the function returned by restoreRootFlagsFn that
	// should be deferred (and the logic to create it needs to run before defer).
	restoreFn := restoreRootFlagsFn()
	// restore the root flags to undo any parsing done by rootCmd.ParseFlags
	defer restoreFn()

	// parse the flags to retrieve the value of the "--assets" flag. Ignore any errors that occur in flag parsing so
	// that, if provided flags are invalid, the regular logic handles the error printing.
	_ = rootCmd.ParseFlags(args)
	allAssets, err := assetapi.LoadAssets(assetsFlagVal)
	if err != nil {
		return errors.Wrapf(err, "failed to load distgo assets")
	}
	loadedAssets = allAssets
	return nil
}

// AddAssetCommands adds commands provided by assets. It is guaranteed that LoadAssets has been called before this
// function, and thus loadedAssets is set/initialized.
func AddAssetCommands() error {
	// add publish subcommands from Publisher assets
	if err := addPublishSubcommandsFromAssets(loadedAssets.GetAssetPathsForType(assetapi.Publisher)); err != nil {
		return errors.Wrapf(err, "failed to add publish subcommands from distgo assets")
	}
	// add asset-provided task commands
	if err := addAssetProvidedTaskCommands(loadedAssets.AssetsWithTaskInfos()); err != nil {
		return errors.Wrapf(err, "failed to add commands from asset-provided tasks")
	}
	return nil
}

func init() {
	pluginapi.AddDebugPFlagPtr(rootCmd.PersistentFlags(), &debugFlagVal)
	pluginapi.AddProjectDirPFlagPtr(rootCmd.PersistentFlags(), &projectDirFlagVal)
	pluginapi.AddConfigPFlagPtr(rootCmd.PersistentFlags(), &distgoConfigFileFlagVal)
	pluginapi.AddGodelConfigPFlagPtr(rootCmd.PersistentFlags(), &godelConfigFileFlagVal)
	pluginapi.AddAssetsPFlagPtr(rootCmd.PersistentFlags(), &assetsFlagVal)

	// Performs global initialization that can return errors.
	// The logic in the function is run after the CLI command tree has been set up, so it cannot add or modify state
	// that impacts the CLI command tree. Logic for adding commands from assets should be put in the AddAssetCommands
	// function.
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		// initialize ProjectVersionerFactory
		var err error
		// sets value of package-level variable
		// arguments to New will become non-nil if/when support for projectversioner assets are added
		if cliProjectVersionerFactory, err = projectversionerfactory.New(nil, nil); err != nil {
			return err
		}

		allAssets := loadedAssets

		// initialize disters from Dister assets
		assetDisters, upgraderDisters, err := dister.AssetDisterCreators(allAssets.GetAssetPathsForType(assetapi.Dister)...)
		if err != nil {
			return err
		}
		// sets value of package-level variable
		if cliDisterFactory, err = disterfactory.New(assetDisters, upgraderDisters); err != nil {
			return err
		}
		// sets value of package-level variable
		if cliDefaultDisterCfg, err = disterfactory.DefaultConfig(); err != nil {
			return err
		}

		// initialize docker builders from DockerBuilder assets
		assetDockerBuilders, upgraderDockerBuilders, err := dockerbuilder.AssetDockerBuilderCreators(allAssets.GetAssetPathsForType(assetapi.DockerBuilder)...)
		if err != nil {
			return err
		}
		// sets value of package-level variable
		if cliDockerBuilderFactory, err = dockerbuilderfactory.New(assetDockerBuilders, upgraderDockerBuilders); err != nil {
			return err
		}

		// sets value of package-level variable
		verifyTaskInfos = assetapi.GetTaskProviderVerifyTasksFromAssets(allAssets)

		return nil
	}
}

// addPublishSubcommandsFromAssets adds the publish commands provided by assets.
func addPublishSubcommandsFromAssets(publisherAssets []string) error {
	assetPublishers, upgraderPublishers, err := publisher.AssetPublisherCreators(publisherAssets...)
	if err != nil {
		return err
	}

	cliPublisherFactory, err = publisherfactory.New(assetPublishers, upgraderPublishers)
	if err != nil {
		return err
	}

	publisherTypeNames := cliPublisherFactory.Types()
	var publishers []distgo.Publisher
	for _, typeName := range publisherTypeNames {
		currPublisher, err := cliPublisherFactory.NewPublisher(typeName)
		if err != nil {
			return errors.Wrapf(err, "failed to create publisher %q", typeName)
		}
		publishers = append(publishers, currPublisher)
	}

	// add publish commands from assets
	addPublishSubcommands(publisherTypeNames, publishers)
	return nil
}

func distgoProjectParamFromFlags() (distgo.ProjectInfo, distgo.ProjectParam, error) {
	return distgoProjectParamFromVals(projectDirFlagVal, distgoConfigFileFlagVal, godelConfigFileFlagVal, cliProjectVersionerFactory, cliDisterFactory, cliDefaultDisterCfg, cliDockerBuilderFactory, cliPublisherFactory)
}

func distgoConfigModTime() *time.Time {
	if distgoConfigFileFlagVal == "" {
		return nil
	}
	fi, err := os.Stat(distgoConfigFileFlagVal)
	if err != nil {
		return nil
	}
	modTime := fi.ModTime()
	return &modTime
}

func distgoProjectParamFromVals(projectDir, distgoConfigFile, godelConfigFile string, projectVersionerFactory distgo.ProjectVersionerFactory, disterFactory distgo.DisterFactory, defaultDisterCfg config.DisterConfig, dockerBuilderFactory distgo.DockerBuilderFactory, publisherFactory distgo.PublisherFactory) (distgo.ProjectInfo, distgo.ProjectParam, error) {
	var distgoCfg config.ProjectConfig
	if distgoConfigFile != "" {
		cfg, err := loadConfigFromFile(distgoConfigFile)
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
	projectInfo, err := projectParam.ProjectInfo(projectDirFlagVal)
	if err != nil {
		return distgo.ProjectInfo{}, distgo.ProjectParam{}, err
	}
	return projectInfo, projectParam, nil
}

func loadConfigFromFile(cfgFile string) (config.ProjectConfig, error) {
	cfgBytes, err := os.ReadFile(cfgFile)
	if os.IsNotExist(err) {
		return config.ProjectConfig{}, nil
	}
	if err != nil {
		return config.ProjectConfig{}, errors.Wrapf(err, "failed to read configuration file")
	}
	upgradedCfgBytes, err := config.UpgradeConfig(cfgBytes, cliProjectVersionerFactory, cliDisterFactory, cliDockerBuilderFactory, cliPublisherFactory)
	if err != nil {
		return config.ProjectConfig{}, errors.Wrapf(err, "failed to upgrade configuration")
	}

	var cfg config.ProjectConfig
	if err := yaml.Unmarshal(upgradedCfgBytes, &cfg); err != nil {
		return config.ProjectConfig{}, errors.Wrapf(err, "failed to unmarshal configuration")
	}
	return cfg, nil
}
