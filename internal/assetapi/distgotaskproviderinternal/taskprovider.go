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
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// AssetProvidedTaskDispatcher returns an "untyped" AssetProvidedTask that delegates to supported typed implementations
// of AssetProvidedTask based on the AssetType of the asset-provided task.
func AssetProvidedTaskDispatcher() AssetProvidedTask[any] {
	return &assetProvidedTaskFactory{}
}

type assetProvidedTaskFactory struct {
	// this struct will store an interface implementation for each supported AssetType
	// and its function implementations will delegate based on the type, as well as performing
	// type conversions. This is effectively a union type. Necessary because Go generics don't
	// support erasure/storing instantiated types within a single untyped type.
}

var _ AssetProvidedTask[any] = (*assetProvidedTaskFactory)(nil)

func (a *assetProvidedTaskFactory) NewAssetProvidedTaskCommand(assetTaskInfo assetapi.AssetTaskInfo) (*cobra.Command, error) {
	switch assetTaskInfo.AssetType {
	default:
		return nil, errors.Errorf("asset type %q is not supported for asset-provided tasks", assetTaskInfo.AssetType)
	}
}

func (a *assetProvidedTaskFactory) CreateVerifyTaskInput(assetType assetapi.AssetType, projectInfo distgo.ProjectInfo, projectParam distgo.ProjectParam) (any, error) {
	switch assetType {
	default:
		return nil, errors.Errorf("asset type %q is not supported for asset-provided tasks", assetType)
	}
}

func (a *assetProvidedTaskFactory) RunVerifyTask(verifyTaskInfo assetapi.AssetTaskInfo, input any, applyMode bool, stdout, stderr io.Writer) error {
	switch verifyTaskInfo.AssetType {
	default:
		return errors.Errorf("asset type %q is not supported for verify tasks", verifyTaskInfo.AssetType)
	}
}

type AssetProvidedTask[T any] interface {
	// NewAssetProvidedTaskCommand returns the *cobra.Command for the provided assetTaskInfo. If an asset type supports
	// providing asset-provided tasks, this function should return a valid command if an assetTaskInfo for the asset
	// type is provided.
	NewAssetProvidedTaskCommand(assetTaskInfo assetapi.AssetTaskInfo) (*cobra.Command, error)

	// CreateVerifyTaskInput returns the input value that should be used for calls to the RunVerifyTask function
	// constructed from the provided projectInfo and projectParam.
	CreateVerifyTaskInput(assetType assetapi.AssetType, projectInfo distgo.ProjectInfo, projectParam distgo.ProjectParam) (T, error)

	// RunVerifyTask runs the provided verify using the input returned by CreateInput for the type of the provided task.
	RunVerifyTask(verifyTaskInfo assetapi.AssetTaskInfo, input T, applyMode bool, stdout, stderr io.Writer) error
}

func NewAssetProvidedTaskCommand(assetTaskInfo assetapi.AssetTaskInfo) (*cobra.Command, error) {
	return nil, errors.Errorf("creating asset-provided task for AssetType %s is not supported", assetTaskInfo.AssetType)
}
