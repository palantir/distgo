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

package publisher

import (
	"encoding/json"

	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/internal/assetapi"
	"github.com/palantir/godel/v2/framework/pluginapi"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

func AssetRootCmd(creator Creator, upgradeConfigFn pluginapi.UpgradeConfigFn, short string) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   creator.TypeName(),
		Short: short,
	}

	publisher := creator.Publisher()
	rootCmd.AddCommand(newNameCmd(publisher))
	rootCmd.AddCommand(assetapi.NewAssetTypeCmd(assetapi.Publisher))
	rootCmd.AddCommand(newFlagsCmd(publisher))
	rootCmd.AddCommand(newRunPublishCmd(publisher))
	rootCmd.AddCommand(newRunPublishV2Cmd(publisher))
	rootCmd.AddCommand(pluginapi.CobraUpgradeConfigCmd(upgradeConfigFn))

	return rootCmd
}

const nameCmdName = "name"

func newNameCmd(publisher distgo.Publisher) *cobra.Command {
	return &cobra.Command{
		Use:   nameCmdName,
		Short: "Print the name of the publisher",
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := publisher.TypeName()
			if err != nil {
				return err
			}
			outputJSON, err := json.Marshal(name)
			if err != nil {
				return errors.Wrapf(err, "failed to marshal output as JSON")
			}
			cmd.Print(string(outputJSON))
			return nil
		},
	}
}

const flagsCmdName = "flags"

func newFlagsCmd(publisher distgo.Publisher) *cobra.Command {
	flagsCmd := &cobra.Command{
		Use:   flagsCmdName,
		Short: "Prints the specifications for the flags supported by this publish operation",
		RunE: func(cmd *cobra.Command, args []string) error {
			flags, err := publisher.Flags()
			if err != nil {
				return err
			}
			outputJSON, err := json.Marshal(flags)
			if err != nil {
				return errors.Wrapf(err, "failed to marshal output as JSON")
			}
			cmd.Print(string(outputJSON))
			return nil
		},
	}
	return flagsCmd
}

const (
	runPublishCmdName                          = "run-publish"
	runPublishCmdProductTaskOutputInfoFlagName = "product-task-output-info"
	runPublishCmdConfigYMLFlagName             = "config-yml"
	runPublishCmdFlagValsFlagName              = "flag-vals"
	runPublishCmdDryRunFlagName                = "dry-run"
)

// newRunPublishCmd returns the legacy run-publish command that publishes a single product.
func newRunPublishCmd(publisher distgo.Publisher) *cobra.Command {
	var (
		productTaskOutputInfoFlagVal string
		configYMLFlagVal             string
		flagValsFlagVal              string
		dryRunFlagVal                bool
	)
	runDistCmd := &cobra.Command{
		Use:   runPublishCmdName,
		Short: "Runs the publish action for a single ProductTaskOutputInfo",
		RunE: func(cmd *cobra.Command, args []string) error {
			var productTaskOutputInfo distgo.ProductTaskOutputInfo
			if err := json.Unmarshal([]byte(productTaskOutputInfoFlagVal), &productTaskOutputInfo); err != nil {
				return errors.Wrapf(err, "failed to unmarshal JSON %s", productTaskOutputInfoFlagVal)
			}
			var flagVals map[distgo.PublisherFlagName]any
			if err := json.Unmarshal([]byte(flagValsFlagVal), &flagVals); err != nil {
				return errors.Wrapf(err, "failed to unmarshal JSON %s", flagValsFlagVal)
			}
			inputs := []distgo.ProductPublishInfo{{
				ProductTaskOutputInfo: productTaskOutputInfo,
				PublisherConfigYML:    []byte(configYMLFlagVal),
			}}
			return publisher.RunPublish(inputs, flagVals, dryRunFlagVal, cmd.OutOrStdout())
		},
	}
	runDistCmd.Flags().StringVar(&productTaskOutputInfoFlagVal, runPublishCmdProductTaskOutputInfoFlagName, "", "JSON representation of distgo.ProductTaskOutputInfo")
	runDistCmd.Flags().StringVar(&configYMLFlagVal, runPublishCmdConfigYMLFlagName, "", "the configuration YML for this publish operation")
	runDistCmd.Flags().StringVar(&flagValsFlagVal, runPublishCmdFlagValsFlagName, "", "JSON representation of map[distgo.PublisherFlag]any")
	runDistCmd.Flags().BoolVar(&dryRunFlagVal, runPublishCmdDryRunFlagName, false, "true if the operation should be run as a dry run")
	mustMarkFlagsRequired(runDistCmd,
		runPublishCmdProductTaskOutputInfoFlagName,
		runPublishCmdConfigYMLFlagName,
		runPublishCmdFlagValsFlagName,
		runPublishCmdDryRunFlagName,
	)
	return runDistCmd
}

const (
	runPublishV2CmdName           = "run-publish-v2"
	runPublishV2CmdInputsFlagName = "inputs"
)

// newRunPublishV2Cmd returns the run-publish-v2 command which publishes the provided []distgo.ProductPublishInfo.
func newRunPublishV2Cmd(publisher distgo.Publisher) *cobra.Command {
	var (
		inputsFlagVal   string
		flagValsFlagVal string
		dryRunFlagVal   bool
	)
	runPublishV2Cmd := &cobra.Command{
		Use:   runPublishV2CmdName,
		Short: "Runs the publish action for the provided products",
		RunE: func(cmd *cobra.Command, args []string) error {
			var inputs []distgo.ProductPublishInfo
			if err := json.Unmarshal([]byte(inputsFlagVal), &inputs); err != nil {
				return errors.Wrapf(err, "failed to unmarshal JSON %s", inputsFlagVal)
			}
			var flagVals map[distgo.PublisherFlagName]any
			if err := json.Unmarshal([]byte(flagValsFlagVal), &flagVals); err != nil {
				return errors.Wrapf(err, "failed to unmarshal JSON %s", flagValsFlagVal)
			}
			return publisher.RunPublish(inputs, flagVals, dryRunFlagVal, cmd.OutOrStdout())
		},
	}
	runPublishV2Cmd.Flags().StringVar(&inputsFlagVal, runPublishV2CmdInputsFlagName, "", "JSON representation of []distgo.ProductPublishInfo")
	runPublishV2Cmd.Flags().StringVar(&flagValsFlagVal, runPublishCmdFlagValsFlagName, "", "JSON representation of map[distgo.PublisherFlag]any")
	runPublishV2Cmd.Flags().BoolVar(&dryRunFlagVal, runPublishCmdDryRunFlagName, false, "true if the operation should be run as a dry run")
	mustMarkFlagsRequired(
		runPublishV2Cmd,
		runPublishV2CmdInputsFlagName,
		runPublishCmdFlagValsFlagName,
		runPublishCmdDryRunFlagName,
	)
	return runPublishV2Cmd
}

func mustMarkFlagsRequired(cmd *cobra.Command, flagNames ...string) {
	for _, currFlagName := range flagNames {
		if err := cmd.MarkFlagRequired(currFlagName); err != nil {
			panic(err)
		}
	}
}
