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

package distgotaskproviderinternal

import (
	"fmt"
	"io"

	"github.com/palantir/distgo/dister/distertaskprovider/distertaskproviderapi"
	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/distgotaskprovider"
	"github.com/palantir/distgo/internal/assetapi"
	"github.com/palantir/distgo/internal/assetapi/distertaskproviderinternal"
	"github.com/palantir/distgo/internal/cmdinternal"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// DisterVerifyTaskInput is a struct that contains the information needed to run the "verify" task for dister
// asset-provided tasks.
type DisterVerifyTaskInput struct {
	// AllConfigYAML contains the configuration YAML for all products and disters. Configuration is filtered down to
	// just the target dister when provided to a particular dister.
	AllConfigYAML distertaskproviderinternal.ProductsDisterConfig

	// ProductTaskOutputInfo contains the ProductTaskOutputInfo for all products.
	ProductTaskOutputInfo map[distgo.ProductID]distgo.ProductTaskOutputInfo
}

// disterAssetProvidedTask is an implementation of AssetProvidedTask[DisterVerifyTaskInput] that enables dister assets
// to provide asset-provided tasks.
type disterAssetProvidedTask struct{}

var _ AssetProvidedTask[DisterVerifyTaskInput] = (*disterAssetProvidedTask)(nil)

func (d *disterAssetProvidedTask) NewAssetProvidedTaskCommand(assetTaskInfo assetapi.AssetTaskInfo, globalFlagValsAndFactories *cmdinternal.GlobalFlagValsAndFactories) (*cobra.Command, error) {
	return &cobra.Command{
		Use:   assetTaskInfo.TaskInfo.Name,
		Short: assetTaskInfo.TaskInfo.Description,
		RunE: func(cmd *cobra.Command, args []string) error {
			allConfigYAML, allProductTaskOutputInfos, err := getDisterTaskCommandArgsFromFlagVals(*globalFlagValsAndFactories)
			if err != nil {
				return err
			}
			disterConfigYAML := distertaskproviderinternal.FilterDisterConfigYAML(allConfigYAML, assetTaskInfo.AssetName)

			// prepend any global flag values
			args = append(getGlobalFlagArgs(assetTaskInfo.TaskInfo, *globalFlagValsAndFactories), args...)

			return distertaskproviderinternal.RunDisterTaskProviderAssetCommand(
				assetTaskInfo.AssetPath,
				assetTaskInfo.TaskInfo.Command,
				disterConfigYAML,
				allProductTaskOutputInfos,
				args,
				cmd.OutOrStdout(),
				cmd.OutOrStderr(),
			)
		},
	}, nil
}

// getGlobalFlagArgs returns a slice that contains the arguments that represent the flags for global flag values
// requested by the specified taskInfo.
func getGlobalFlagArgs(taskInfo distgotaskprovider.TaskInfo, globalFlagValsAndFactories cmdinternal.GlobalFlagValsAndFactories) []string {
	var flagArgs []string
	if debugFlagName := taskInfo.GlobalFlagOptions.DebugFlagName; debugFlagName != "" {
		flagArgs = append(flagArgs, fmt.Sprintf("--%s=%t", debugFlagName, globalFlagValsAndFactories.DebugFlagVal))
	}
	if projectDirFlagName := taskInfo.GlobalFlagOptions.ProjectDirFlagName; projectDirFlagName != "" && globalFlagValsAndFactories.ProjectDirFlagVal != "" {
		flagArgs = append(flagArgs, fmt.Sprintf("--%s=%s", projectDirFlagName, globalFlagValsAndFactories.ProjectDirFlagVal))
	}
	return flagArgs
}

func getDisterTaskCommandArgsFromFlagVals(globalFlagValsAndFactories cmdinternal.GlobalFlagValsAndFactories) (distertaskproviderinternal.ProductsDisterConfig, map[distgo.ProductID]distgo.ProductTaskOutputInfo, error) {
	projectInfo, projectParam, err := cmdinternal.DistgoProjectParamFromFlagVals(globalFlagValsAndFactories)
	if err != nil {
		return nil, nil, err
	}

	return getDisterTaskCommandArgsFromProjectInfoAndProjectParam(projectInfo, projectParam)
}

func getDisterTaskCommandArgsFromProjectInfoAndProjectParam(projectInfo distgo.ProjectInfo, projectParam distgo.ProjectParam) (distertaskproviderinternal.ProductsDisterConfig, map[distgo.ProductID]distgo.ProductTaskOutputInfo, error) {
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

func getAllConfigYAML(params []distgo.ProductParam) (distertaskproviderinternal.ProductsDisterConfig, error) {
	out := make(distertaskproviderinternal.ProductsDisterConfig)
	for _, param := range params {
		productDistConfigs, err := getDisterConfigs(param)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get dister configs for product %s", param.ID)
		}
		out[param.ID] = productDistConfigs
	}
	return out, nil
}

func getDisterConfigs(p distgo.ProductParam) (map[distgo.DistID]distertaskproviderapi.DisterConfigYAML, error) {
	if p.Dist == nil {
		return nil, nil
	}
	out := make(map[distgo.DistID]distertaskproviderapi.DisterConfigYAML)
	for distID, disterParam := range p.Dist.DistParams {
		disterWithConfig, ok := disterParam.Dister.(distgo.DisterWithConfig)
		if !ok {
			continue
		}
		disterTypeName, err := disterParam.Dister.TypeName()
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get dister type name for dist %s for product %s", distID, p.ID)
		}

		configYAML, err := disterWithConfig.ConfigYAML()
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get config for dist %s of dister type %s for product %s", distID, disterTypeName, p.ID)
		}
		out[distID] = distertaskproviderapi.DisterConfigYAML{
			DisterName: disterTypeName,
			ConfigYAML: configYAML,
		}
	}
	return out, nil
}

func (d *disterAssetProvidedTask) CreateVerifyTaskInput(_ assetapi.AssetType, projectInfo distgo.ProjectInfo, projectParam distgo.ProjectParam) (DisterVerifyTaskInput, error) {
	allConfigYAML, allProductTaskOutputInfos, err := getDisterTaskCommandArgsFromProjectInfoAndProjectParam(projectInfo, projectParam)
	if err != nil {
		return DisterVerifyTaskInput{}, err
	}
	return DisterVerifyTaskInput{
		AllConfigYAML:         allConfigYAML,
		ProductTaskOutputInfo: allProductTaskOutputInfos,
	}, nil
}

func (d *disterAssetProvidedTask) RunVerifyTask(verifyTaskInfo assetapi.AssetTaskInfo, input DisterVerifyTaskInput, applyMode bool, stdout, stderr io.Writer) error {
	disterConfigYAML := distertaskproviderinternal.FilterDisterConfigYAML(input.AllConfigYAML, verifyTaskInfo.AssetName)

	var applyArgs []string
	if applyMode {
		applyArgs = verifyTaskInfo.TaskInfo.VerifyOptions.ApplyTrueArgs
	} else {
		applyArgs = verifyTaskInfo.TaskInfo.VerifyOptions.ApplyFalseArgs
	}

	return distertaskproviderinternal.RunDisterTaskProviderAssetCommand(
		verifyTaskInfo.AssetPath,
		verifyTaskInfo.TaskInfo.Command,
		disterConfigYAML,
		input.ProductTaskOutputInfo,
		applyArgs,
		stdout,
		stderr,
	)
}
