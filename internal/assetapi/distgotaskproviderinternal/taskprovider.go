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
	"io"

	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/internal/assetapi"
	"github.com/palantir/distgo/internal/cmdinternal"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// AssetProvidedTask is an interface that acts as a creator/dispatcher for asset-provided tasks.
// Generally, a single implementation will correspond to a single asset type (for example, dister, publisher, etc.).
// The NewAssetProvidedTaskCommand and RunVerifyTask corresponds to a particular asset of the type (identified by the
// provided information), while CreateVerifyTaskInput is run once per asset type, with its output provided to all
// individual runs of RunVerifyTask. For this reason, the type/struct returned by CreateVerifyTaskInput should contain
// the information for all possible individual asset tasks for the type, and the RunVerifyTask should contain logic that
// narrows down the input information to the specific asset or task if needed.
type AssetProvidedTask[T any] interface {
	// NewAssetProvidedTaskCommand returns the *cobra.Command for the provided assetTaskInfo. The
	// globalFlagValsAndFactories is a pointer to the cmdinternal.GlobalFlagValsAndFactories used by the cmd package,
	// and its fields are set by the global flags by the time the returned command is invoked. If an asset type supports
	// providing asset-provided tasks, this function should return a valid command if an assetTaskInfo for the asset
	// type is provided.
	NewAssetProvidedTaskCommand(assetTaskInfo assetapi.AssetTaskInfo, globalFlagValsAndFactories *cmdinternal.GlobalFlagValsAndFactories) (*cobra.Command, error)

	// CreateVerifyTaskInput returns the input value that should be used for calls to the RunVerifyTask function
	// constructed from the provided projectInfo and projectParam. Verify task input should be common across all
	// instances of a given asset type.
	CreateVerifyTaskInput(assetType assetapi.AssetType, projectInfo distgo.ProjectInfo, projectParam distgo.ProjectParam) (T, error)

	// RunVerifyTask runs the provided verify using the input returned by CreateInput for the type of the provided task.
	RunVerifyTask(verifyTaskInfo assetapi.AssetTaskInfo, input T, applyMode bool, stdout, stderr io.Writer) error
}

// AssetProvidedTaskDispatcher returns an "untyped" AssetProvidedTask that delegates to supported typed implementations
// of AssetProvidedTask based on the AssetType of the asset-provided task.
func AssetProvidedTaskDispatcher() AssetProvidedTask[any] {
	return &assetProvidedTaskDispatcher{
		disterAssetProvidedTask: &disterAssetProvidedTask{},
	}
}

// assetProvidedTaskDispatcher is an implementation of AssetProvidedTask[any] that dispatches to a stored
// AssetProvidedTask based on asset type.
type assetProvidedTaskDispatcher struct {
	// AssetProvidedTask for disters
	disterAssetProvidedTask AssetProvidedTask[DisterVerifyTaskInput]
}

var _ AssetProvidedTask[any] = (*assetProvidedTaskDispatcher)(nil)

func (a *assetProvidedTaskDispatcher) NewAssetProvidedTaskCommand(assetTaskInfo assetapi.AssetTaskInfo, globalFlagValsAndFactories *cmdinternal.GlobalFlagValsAndFactories) (*cobra.Command, error) {
	switch assetTaskInfo.AssetType {
	case assetapi.Dister:
		return a.disterAssetProvidedTask.NewAssetProvidedTaskCommand(assetTaskInfo, globalFlagValsAndFactories)
	default:
		return nil, errors.Errorf("asset type %q is not supported for asset-provided tasks", assetTaskInfo.AssetType)
	}
}

func (a *assetProvidedTaskDispatcher) CreateVerifyTaskInput(assetType assetapi.AssetType, projectInfo distgo.ProjectInfo, projectParam distgo.ProjectParam) (any, error) {
	switch assetType {
	case assetapi.Dister:
		return a.disterAssetProvidedTask.CreateVerifyTaskInput(assetType, projectInfo, projectParam)
	default:
		return nil, errors.Errorf("asset type %q is not supported for asset-provided tasks", assetType)
	}
}

func (a *assetProvidedTaskDispatcher) RunVerifyTask(verifyTaskInfo assetapi.AssetTaskInfo, input any, applyMode bool, stdout, stderr io.Writer) error {
	switch verifyTaskInfo.AssetType {
	case assetapi.Dister:
		typedInput, ok := input.(DisterVerifyTaskInput)
		if !ok {
			return errors.Errorf("invalid input type %T, expected DisterVerifyTaskInput", input)
		}
		return a.disterAssetProvidedTask.RunVerifyTask(verifyTaskInfo, typedInput, applyMode, stdout, stderr)
	default:
		return errors.Errorf("asset type %q is not supported for verify tasks", verifyTaskInfo.AssetType)
	}
}
