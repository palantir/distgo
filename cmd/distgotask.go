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
	"maps"
	"slices"

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
`,
	}
)

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
			cmd, err := distgotaskproviderinternal.NewAssetProvidedTaskCommand(assetTaskInfo)
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

			// add command as a subcommand of the distgo-task command and mark name as used
			distgoTaskCmd.AddCommand(cmd)
			distgoTaskSubcommands[cmdName] = struct{}{}
		}
	}
	return nil
}
