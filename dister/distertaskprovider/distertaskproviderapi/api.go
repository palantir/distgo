package distertaskproviderapi

import (
	"io"

	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/distgotaskprovider"
	"github.com/spf13/cobra"
)

type DisterConfigYAML struct {
	DisterName string `yaml:"dister-name"`
	ConfigYAML []byte `yaml:"config-yaml"`
}

type DisterTask struct {
	TaskRunner TaskRunner
	TaskInfo   distgotaskprovider.TaskInfo
}

// TaskRunner is an interface that runs the task provided by a dister task provider.
type TaskRunner interface {
	// ConfigureCommand configures the provided *cobra.Command for the task runner. If a *cobra.Command is constructed
	// for the runner, this function is called on the command after it is constructed. The typical use case for this
	// is to register flags on the command that can be used/referenced in RunTask.
	ConfigureCommand(cmd *cobra.Command)

	// RunTask runs the task associated with the runner. It is provided with the configuration YML for all the disters
	// of the dister type and the ProductTaskOutputInfos associated with the disters of the dister type. The args
	// parameter contains all the arguments provided to the command that invoked the task. Task output can be written
	// to the provided stdout and stderr writers.
	RunTask(
		disterConfigYAML map[distgo.ProductID]map[distgo.DistID]DisterConfigYAML,
		allProductTaskOutputInfos map[distgo.ProductID]distgo.ProductTaskOutputInfo,
		args []string,
		stdout, stderr io.Writer,
	) error
}
