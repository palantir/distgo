package cmd

import (
	"fmt"
	"io"
	"maps"
	"slices"

	"github.com/palantir/distgo/internal/assetapi"
	"github.com/palantir/distgo/internal/assetapi/distertaskproviderinternal"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	distgoTaskCmd = &cobra.Command{
		Use:   "distgo-task [--verify (--apply)?] | [task-name] [task-flags] [task-args] | [asset-type] [asset-name] [task-name] [task-flags] [task-args]",
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
			return runVerifyTask(cmd.OutOrStdout(), cmd.OutOrStderr(), distgoTaskVerifyApplyFlagVal)
		},
	}
)

var (
	distgoTaskVerifyFlagVal      bool
	distgoTaskVerifyApplyFlagVal bool

	// stores all information registered by assets for verify tasks.
	// Global because the distgoTaskVerifyCmd must be able to reference the variable and that command must be
	// initialized before assets are loaded, but value of the variable is populated by asset loading.
	verifyTaskInfos []assetapi.VerifyTaskInfo
)

func init() {
	distgoTaskCmd.Flags().BoolVar(&distgoTaskVerifyFlagVal, "verify", false, "run the verify operation for tasks")
	distgoTaskCmd.Flags().BoolVar(&distgoTaskVerifyApplyFlagVal, "apply", false, "apply verify changes when possible")

	rootCmd.AddCommand(distgoTaskCmd)
}

// addAssetProvidedTaskCommands adds all asset-provided tasks from the provided assetsWithTaskInfos to the command tree.
// The registered commands are "distgo-task [asset-type] [asset-name] [task-name]" and "distgo-task [task-name]".
func addAssetProvidedTaskCommands(assetsWithTaskInfos []assetapi.Asset) error {
	// map from AssetType to subcommand for that assetType
	assetTypeSubCmds := make(map[assetapi.AssetType]*cobra.Command)

	// names of the top-level subcommands of the distgo-task command
	topLevelCommands := map[string]struct{}{
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

			// if task is not marked to be registered as top-level command, continue
			if !taskInfo.RegisterAsTopLevelDistgoTaskCommand {
				continue
			}

			// if command name conflicts with an existing top-level command, continue
			cmdName := taskInfo.Name
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

func runVerifyTask(stdout, stderr io.Writer, applyMode bool) error {
	// if there are no verify tasks, return immediately
	if len(verifyTaskInfos) == 0 {
		return nil
	}

	// information used by dister verifiers. If verifier support for other asset types is added later, logic should
	// be added here to compute values used by those types. At that point, it may also make sense to add logic to make
	// it such that only values that will used will be computed (or come up with some other design for invocation).
	allConfigYAML, allProductTaskOutputInfos, err := getDisterTaskCommandArgs()
	if err != nil {
		return err
	}

	errorOccurred := false
	for _, verifyTaskInfo := range verifyTaskInfos {
		var applyArgs []string
		if applyMode {
			applyArgs = verifyTaskInfo.TaskInfo.VerifyOptions.ApplyTrueArgs
		} else {
			applyArgs = verifyTaskInfo.TaskInfo.VerifyOptions.ApplyFalseArgs
		}

		var err error

		switch verifyTaskInfo.AssetType {
		case assetapi.Dister:
			err = distertaskproviderinternal.RunDisterTaskProviderAssetCommand(
				verifyTaskInfo.AssetPath,
				verifyTaskInfo.TaskInfo.Command,
				allConfigYAML,
				allProductTaskOutputInfos,
				applyArgs,
				stdout,
				stderr,
			)
		default:
			return errors.Errorf("asset type %q is not supported for verify tasks", verifyTaskInfo.AssetType)
		}

		if err != nil {
			// if error occurred, record and print.
			// Continue because all verification tasks should run.
			errorOccurred = true
			_, _ = fmt.Fprintln(stderr, err.Error())
		}
	}
	if errorOccurred {
		return fmt.Errorf("")
	}
	return nil
}
