package cmd

import (
	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/distgotaskprovider"
	"github.com/palantir/distgo/internal/assetapi/distertaskproviderinternal"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// NewDistgoTaskDisterCommand returns a *cobra.Command that invokes the dister asset-provided task described by the
// provided taskInfo on the asset at the specified assetPath. The asset must be an asset that provides a dister
// asset-provided task.
func NewDistgoTaskDisterCommand(assetPath string, taskInfo distgotaskprovider.TaskInfo) *cobra.Command {
	return &cobra.Command{
		Use:   taskInfo.Name,
		Short: taskInfo.Description,
		RunE: func(cmd *cobra.Command, args []string) error {
			allConfigYAML, allProductTaskOutputInfos, err := getDisterTaskCommandArgs()
			if err != nil {
				return err
			}

			return distertaskproviderinternal.RunDisterTaskProviderAssetCommand(
				assetPath,
				taskInfo.Command,
				allConfigYAML,
				allProductTaskOutputInfos,
				args,
				cmd.OutOrStdout(),
				cmd.OutOrStderr(),
			)
		},
	}
}

func getDisterTaskCommandArgs() (map[distgo.ProductID]map[distgo.DistID][]byte, map[distgo.ProductID]distgo.ProductTaskOutputInfo, error) {
	projectInfo, projectParam, err := distgoProjectParamFromFlags()
	if err != nil {
		return nil, nil, err
	}

	// get all product params
	productParams, err := distgo.ProductParamsForDistProductArgs(projectParam.Products)
	if err != nil {
		return nil, nil, err
	}

	// map from ProductID -> DistID -> dister config
	allConfigYAML, err := getAllConfigYAML(productParams)
	if err != nil {
		return nil, nil, err
	}

	// map from ProductID -> ProductTaskOutputInfo
	allProductTaskOutputInfos, err := getAllProductTaskOutputInfos(projectInfo, productParams)
	if err != nil {
		return nil, nil, err
	}

	return allConfigYAML, allProductTaskOutputInfos, nil
}

func getAllProductTaskOutputInfos(projectInfo distgo.ProjectInfo, params []distgo.ProductParam) (map[distgo.ProductID]distgo.ProductTaskOutputInfo, error) {
	out := make(map[distgo.ProductID]distgo.ProductTaskOutputInfo)
	for _, param := range params {
		productTaskOutputInfo, err := distgo.ToProductTaskOutputInfo(projectInfo, param)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create product task output info for product %s", param.ID)
		}
		out[param.ID] = productTaskOutputInfo
	}
	return out, nil
}

func getAllConfigYAML(params []distgo.ProductParam) (map[distgo.ProductID]map[distgo.DistID][]byte, error) {
	out := make(map[distgo.ProductID]map[distgo.DistID][]byte)
	for _, param := range params {
		productDistConfigs, err := getDisterConfigs(param)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get dister configs for product %s", param.ID)
		}
		out[param.ID] = productDistConfigs
	}
	return out, nil
}

func getDisterConfigs(p distgo.ProductParam) (map[distgo.DistID][]byte, error) {
	if p.Dist == nil {
		return nil, nil
	}
	out := make(map[distgo.DistID][]byte)
	for distID, disterParam := range p.Dist.DistParams {
		disterWithConfig, ok := disterParam.Dister.(distgo.DisterWithConfig)
		if !ok {
			continue
		}
		configYML, err := disterWithConfig.ConfigYML()
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get config for dist %s", distID)
		}
		out[distID] = configYML
	}
	return out, nil
}
