package distertaskprovider

import (
	"io"

	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/internal/assetapi/distertaskproviderinternal"
	"github.com/spf13/cobra"
)

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

func NewTaskProviderCommand(name, short string, runner TaskRunner) *cobra.Command {
	var (
		allConfigYMLFlagVal             string
		allProductTaskOutputInfoFlagVal string
	)

	cmd := &cobra.Command{
		Use:   name,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			allConfigYML, err := distertaskproviderinternal.ReadValueFromYAMLFile[map[distgo.ProductID]map[distgo.DistID][]byte](allConfigYMLFlagVal)
			if err != nil {
				return err
			}
			allProductTaskOutputInfos, err := distertaskproviderinternal.ReadValueFromYAMLFile[map[distgo.ProductID]distgo.ProductTaskOutputInfo](allProductTaskOutputInfoFlagVal)
			if err != nil {
				return err
			}
			return runner.RunTask(allConfigYML, allProductTaskOutputInfos, args, cmd.OutOrStdout(), cmd.OutOrStderr())
		},
	}

	cmd.Flags().StringVar(&allConfigYMLFlagVal, distertaskproviderinternal.AllConfigYMLFlagName, "", "file containing YAML representation of all config YAML for dister")
	cmd.Flags().StringVar(&allProductTaskOutputInfoFlagVal, distertaskproviderinternal.AllProductTaskOutputInfoFlagName, "", "file containing YAML representation of all ProductTaskOutputInfo for dister")

	return cmd
}
