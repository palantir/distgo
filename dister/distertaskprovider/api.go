package distertaskprovider

import (
	"io"

	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/distgotaskprovider"
	"github.com/palantir/distgo/internal/assetapi"
	"github.com/palantir/distgo/internal/assetapi/distertaskproviderinternal"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

type DisterTask struct {
	TaskRunner TaskRunner
	TaskInfo   distgotaskprovider.TaskInfo
}

// TaskRunner is an interface that runs the task provided by a dister task provider.
type TaskRunner interface {
	// RunTask runs the task associated with the runner. It is provided with the configuration YML for all the disters
	// of the dister type and the ProductTaskOutputInfos associated with the disters of the dister type. The args
	// parameter contains all the arguments provided to the command that invoked the task. Task output can be written
	// to the provided stdout and stderr writers.
	RunTask(
		allConfigYML map[distgo.ProductID]map[distgo.DistID][]byte,
		allProductTaskOutputInfos map[distgo.ProductID]distgo.ProductTaskOutputInfo,
		args []string,
		stdout, stderr io.Writer,
	) error
}

// AddDisterTaskCommands adds all commands related to dister-provided tasks for the specified dister type specified by
// tasks to the provided rootCmd. Also registers the task-info command that returns the task infos for the tasks.
func AddDisterTaskCommands(rootCmd *cobra.Command, disterName string, tasks []DisterTask) error {
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

		taskCommand := distertaskproviderinternal.NewTaskProviderCommand(task.TaskInfo.Command[0], task.TaskInfo.Description, task.TaskRunner)
		rootCmd.AddCommand(taskCommand)
	}

	return nil
}

// type assertion exists to enforce that the exported interface is compatible with the internal package interface
// (which only exists to prevent package import cycles).
var _ TaskRunner = (distertaskproviderinternal.TaskRunner)(nil)
