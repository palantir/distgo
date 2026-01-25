package cmd

import (
	"fmt"

	"github.com/palantir/distgo/internal/assetapi"
	"github.com/palantir/distgo/internal/assetapi/distertaskproviderinternal"
	"github.com/spf13/cobra"
)

var (
	distgoTaskVerifyCmd = &cobra.Command{
		Use:   "verify [flags]",
		Short: "Runs the distgo asset-provided tasks that registered as verify tasks",
		RunE: func(cmd *cobra.Command, _ []string) error {
			// if there are no verify tasks, return immediately
			if len(verifyTaskInfos) == 0 {
				return nil
			}

			allConfigYAML, allProductTaskOutputInfos, err := getDisterTaskCommandArgs()
			if err != nil {
				return err
			}

			errorOccurred := false
			for _, verifyTaskInfo := range verifyTaskInfos {
				var applyArgs []string
				if distgoTaskVerifyApplyFlagVal {
					applyArgs = verifyTaskInfo.TaskInfo.VerifyOptionsVar.ApplyTrueArgsVar
				} else {
					applyArgs = verifyTaskInfo.TaskInfo.VerifyOptionsVar.ApplyFalseArgsVar
				}

				if err := distertaskproviderinternal.RunDisterTaskProviderAssetCommand(
					verifyTaskInfo.AssetPath,
					verifyTaskInfo.TaskInfo.CommandVar,
					allConfigYAML,
					allProductTaskOutputInfos,
					applyArgs,
					cmd.OutOrStdout(),
					cmd.OutOrStderr(),
				); err != nil {
					// if error occurred, record and print.
					// Continue because all verification tasks should run.
					errorOccurred = true
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), err.Error())
				}
			}
			if errorOccurred {
				return fmt.Errorf("")
			}
			return nil
		},
	}
)

var (
	distgoTaskVerifyApplyFlagVal bool

	// stores all information registered by assets for verify tasks.
	// Global because the distgoTaskVerifyCmd must be able to reference the variable and that command must be
	// initialized before assets are loaded, but value of the variable is populated by asset loading.
	verifyTaskInfos []assetapi.VerifyTaskInfo
)

func init() {
	distgoTaskVerifyCmd.Flags().BoolVar(&distgoTaskVerifyApplyFlagVal, "apply", false, "apply changes when possible")

	distgoTaskCmd.AddCommand(distgoTaskVerifyCmd)
}
