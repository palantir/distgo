package cmd

import (
	"fmt"
	"maps"
	"slices"

	"github.com/palantir/distgo/internal/assetapi"
	"github.com/spf13/cobra"
)

var (
	distgoTaskCmd = &cobra.Command{
		Use:   "distgo-task [task-name]",
		Short: "Runs the distgo asset-provided task with the specified task name",
		Long:  `Runs a distgo asset-provided task with the specified name. The "verify" task is a special task
that runs the verification operation for all asset-provided tasks that register as verify tasks.
Tasks may also be invoked using their fully qualified name, which is [asset-type] [asset-name] [task-name].
If there are any conflicts between task names, they are not registered at the top level of distgo-task and must be
invoked using their fully qualified name.`,
	}
)

func init() {
	rootCmd.AddCommand(distgoTaskCmd)
}

// addAssetProvidedTaskCommands adds all asset-provided tasks from the provided assetsWithTaskInfos to the command tree.
// The registered commands are "distgo-task [asset-type] [asset-name] [task-name]" and "distgo-task [task-name]".
func addAssetProvidedTaskCommands(assetsWithTaskInfos []assetapi.Asset) error {
	// map from AssetType to subcommand for that assetType
	assetTypeSubCmds := make(map[assetapi.AssetType]*cobra.Command)

	// names of the top-level subcommands of the distgo-task command
	topLevelCommands := map[string]struct{}{
		"verify": {},
		string(assetapi.Dister): {},
		string(assetapi.Publisher): {},
		string(assetapi.DockerBuilder): {},
	}

	for _, asset := range assetsWithTaskInfos {
		// if taskInfos is nil or does not define any asset-provided tasks, nothing to do
		taskInfos := asset.TaskInfos
		if taskInfos == nil || len(taskInfos.TaskInfos) == 0 {
			continue
		}

		// currently, only support asset-provided tasks for Dister assets
		if asset.AssetType != assetapi.Dister {
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
			cmd := NewDistgoTaskDisterCommand(asset.AssetPath, taskInfo)

			// add fully qualified command to asset
			assetSubCmd.AddCommand(cmd)

			if taskInfo.VerifyOptionsVar != nil {
				// register command as a "verify" task
			}

			// if task is not marked to be registered as top-level command, continue
			if !taskInfo.RegisterAsTopLevelDistgoTaskCommand {
				continue
			}

			// if command name conflicts with an existing top-level command, continue
			cmdName := taskInfo.NameVar
			if _, ok := topLevelCommands[cmdName]; ok {
				continue
			}

			// add command as top-level command and mark name as used
			topLevelCommands[cmdName] = struct{}{}
			distgoTaskCmd.AddCommand(cmd)
		}
	}
	return nil
}
