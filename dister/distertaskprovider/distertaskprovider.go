package distertaskprovider

import (
	"github.com/palantir/distgo/dister/distertaskprovider/distertaskproviderapi"
	"github.com/palantir/distgo/distgotaskprovider"
	"github.com/palantir/distgo/internal/assetapi"
	"github.com/palantir/distgo/internal/assetapi/distertaskproviderinternal"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// AddDisterTaskCommands adds all commands related to dister-provided tasks for the specified dister type specified by
// tasks to the provided rootCmd. Also registers the task-info command that returns the task infos for the tasks.
func AddDisterTaskCommands(rootCmd *cobra.Command, disterName string, tasks []distertaskproviderapi.DisterTask) error {
	taskInfosMap := make(map[string]distgotaskprovider.TaskInfo)
	for _, task := range tasks {
		taskInfosMap[task.TaskInfo.Name] = task.TaskInfo
	}

	// add task-infos command
	taskInfosCmd := assetapi.NewTaskInfosCommand(assetapi.TaskInfos{
		AssetName: disterName,
		TaskInfos: taskInfosMap,
	})
	rootCmd.AddCommand(taskInfosCmd)

	// add task commands
	for _, task := range tasks {
		// in theory, command slice can be empty (meaning that top-level asset is invoked for task) or contain multiple
		// elements (meaning that a subcommand is invoked for the asset task), but for automatic registration, start by
		// enforcing requirement that command must be a slice with a single element.
		if len(task.TaskInfo.Command) != 1 {
			return errors.Errorf("function only supports registering tasks with a single command value, but was %v", task.TaskInfo.Command)
		}

		taskCommand := distertaskproviderinternal.NewTaskProviderCommand(disterName, task.TaskInfo.Command[0], task.TaskInfo.Description, task.TaskRunner)
		rootCmd.AddCommand(taskCommand)
	}

	return nil
}
