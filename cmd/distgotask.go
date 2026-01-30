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

package cmd

import (
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/internal/assetapi"
	"github.com/palantir/distgo/internal/assetapi/distgotaskproviderinternal"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	distgoTaskCmd = &cobra.Command{
		Use:   "distgo-task",
		Short: "Runs the distgo asset-provided task with the specified task name",
		Long: `Runs a distgo asset-provided task with the specified name.

Tasks may also be invoked using their fully qualified name, which is [asset-type] [asset-name] [task-name]. If there are
any conflicts between task names, they are not registered at the top level of distgo-task and must be invoked using
their fully qualified name.

As a special case, running this command with no arguments but with the --verify flag runs the verification operation for
all asset-provided tasks that register as verify tasks. When running with the --verify flag, if the --apply flag is also
specified, the verification will attempt to apply any changes, while otherwise it will attempt to only verify state
without making any modifications.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// if not in verify mode, print help and exit
			if !distgoTaskVerifyFlagVal {
				return cmd.Help()
			}
			projectInfo, projectParam, err := distgoProjectParamFromFlags()
			if err != nil {
				return err
			}
			return runVerifyTask(projectInfo, projectParam, distgoTaskVerifyApplyFlagVal, cmd.OutOrStdout(), cmd.OutOrStderr())
		},
	}
)

var (
	distgoTaskVerifyFlagVal      bool
	distgoTaskVerifyApplyFlagVal bool

	// stores an instance of AssetProvidedTask[any] that dispatches to a typed AssetProvidedTask based on the asset
	// type.
	assetProvidedTaskDispatcher = distgotaskproviderinternal.AssetProvidedTaskDispatcher()

	// stores all information registered by assets for verify tasks.
	// Global because the distgoTaskVerifyCmd must be able to reference the variable and that command must be
	// initialized before assets are loaded, but value of the variable is populated by asset loading.
	// The TaskInfo.VerifyOptions field for these values must be non-nil.
	verifyTaskInfos []assetapi.AssetTaskInfo
)

func init() {
	distgoTaskCmd.Flags().BoolVar(&distgoTaskVerifyFlagVal, "verify", false, "run the verify operation for tasks")
	distgoTaskCmd.Flags().BoolVar(&distgoTaskVerifyApplyFlagVal, "apply", false, "apply verify changes when possible (only used if verify flag is set)")

	rootCmd.AddCommand(distgoTaskCmd)
}

// addAssetProvidedTaskCommands adds all asset-provided tasks from the provided assetsWithTaskInfos to the command tree
// by registering commands on distgoTaskCmd. This is a separate function because asset-provided commands can only be
// added after assets have been loaded. The registered commands are "distgo-task [asset-type] [asset-name] [task-name]"
// and "distgo-task [task-name]".
func addAssetProvidedTaskCommands(assetsWithTaskInfos []assetapi.Asset) error {
	// map from AssetType to subcommand for that assetType
	assetTypeSubCmds := make(map[assetapi.AssetType]*cobra.Command)

	// names of the top-level subcommands of the distgo-task command
	distgoTaskSubcommands := map[string]struct{}{
		string(assetapi.Dister):        {},
		string(assetapi.Publisher):     {},
		string(assetapi.DockerBuilder): {},
	}

	for _, asset := range assetsWithTaskInfos {
		// if taskInfos is nil or does not define any asset-provided tasks, nothing to do
		taskInfos := asset.TaskInfos
		if taskInfos == nil || len(taskInfos.TaskInfos) == 0 {
			continue
		}

		// get subcommand for asset type
		assetTypeSubCmd, ok := assetTypeSubCmds[asset.AssetType]
		if !ok {
			// initialize subcommand for asset type and add to map if not already present
			assetTypeSubCmd = &cobra.Command{
				Use: fmt.Sprintf("%s [asset-name]", asset.AssetType),
			}
			assetTypeSubCmds[asset.AssetType] = assetTypeSubCmd

			// register it as a subtask of the distgoTaskCmd
			distgoTaskCmd.AddCommand(assetTypeSubCmd)
		}

		// register asset name subcommand for all fully qualified commands
		assetSubCmd := &cobra.Command{
			Use: fmt.Sprintf("%s [task-name]", taskInfos.AssetName),
		}
		assetTypeSubCmd.AddCommand(assetSubCmd)

		for _, taskName := range slices.Sorted(maps.Keys(taskInfos.TaskInfos)) {
			taskInfo := taskInfos.TaskInfos[taskName]

			assetTaskInfo := assetapi.AssetTaskInfo{
				AssetPath: asset.AssetPath,
				AssetType: asset.AssetType,
				AssetName: taskInfos.AssetName,
				TaskInfo:  taskInfo,
			}

			// create the command for the asset-provided task
			cmd, err := assetProvidedTaskDispatcher.NewAssetProvidedTaskCommand(assetTaskInfo, &globalFlagValsAndFactories)
			if err != nil {
				return errors.Wrapf(err, "failed to create asset-provided task for asset %s of type %s at %s", taskInfos.AssetName, asset.AssetType, asset.AssetPath)
			}

			// add fully qualified command to asset subcommand
			assetSubCmd.AddCommand(cmd)

			// if task is not marked to be registered as a distgo-task subcommand, continue
			if !taskInfo.RegisterAsTopLevelDistgoTaskCommand {
				continue
			}

			// if command name conflicts with an existing distgo-task subcommand, continue
			cmdName := taskInfo.Name
			if _, ok := distgoTaskSubcommands[cmdName]; ok {
				continue
			}

			// create a new instance of the command to add as the subcommand: required because each instance of
			// *cobra.Command can only have a single parent Command.
			cmd, err = assetProvidedTaskDispatcher.NewAssetProvidedTaskCommand(assetTaskInfo, &globalFlagValsAndFactories)
			if err != nil {
				return errors.Wrapf(err, "failed to create asset-provided task for asset %s of type %s at %s", taskInfos.AssetName, asset.AssetType, asset.AssetPath)
			}

			// add command as a subcommand of the distgo-task command and mark name as used
			distgoTaskCmd.AddCommand(cmd)
			distgoTaskSubcommands[cmdName] = struct{}{}
		}
	}
	return nil
}

func runVerifyTask(
	projectInfo distgo.ProjectInfo,
	projectParam distgo.ProjectParam,
	applyMode bool,
	stdout,
	stderr io.Writer,
) error {
	// if there are no verify tasks, return immediately
	if len(verifyTaskInfos) == 0 {
		return nil
	}

	taskInputs := make(map[assetapi.AssetType]any)

	var errTaskStrings []string
	for _, verifyTaskInfo := range verifyTaskInfos {
		taskInput, ok := taskInputs[verifyTaskInfo.AssetType]
		if !ok {
			var err error
			// taskInput not yet created for this asset type: create it
			taskInput, err = assetProvidedTaskDispatcher.CreateVerifyTaskInput(verifyTaskInfo.AssetType, projectInfo, projectParam)
			// treat error at this level as blocking
			if err != nil {
				return errors.Wrapf(err, "failed to create verify task for asset %s of type %s", verifyTaskInfo.AssetName, verifyTaskInfo.AssetType)
			}
			taskInputs[verifyTaskInfo.AssetType] = taskInput
		}

		if err := assetProvidedTaskDispatcher.RunVerifyTask(verifyTaskInfo, globalFlagValsAndFactories, taskInput, applyMode, stdout, stderr); err != nil {
			// if error occurred, record and print.
			// Continue because all verification tasks should run.
			errTaskStrings = append(errTaskStrings, fmt.Sprintf("* %s.%s.%s", verifyTaskInfo.AssetType, verifyTaskInfo.AssetName, verifyTaskInfo.TaskInfo.Name))
			_, _ = fmt.Fprintln(stderr, err.Error())
		}
	}
	if len(errTaskStrings) > 0 {
		return fmt.Errorf("%d verify task(s) failed:\n%s", len(errTaskStrings), strings.Join(errTaskStrings, "\n"))
	}
	return nil
}
