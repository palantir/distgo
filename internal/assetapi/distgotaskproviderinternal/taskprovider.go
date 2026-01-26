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
	"github.com/palantir/distgo/internal/assetapi"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// NewAssetProvidedTaskCommand returns the *cobra.Command for the provided assetTaskInfo. If an asset type supports
// providing asset-provided tasks, this function should return a valid command if an assetTaskInfo for the asset type is
// provided.
func NewAssetProvidedTaskCommand(assetTaskInfo assetapi.AssetTaskInfo) (*cobra.Command, error) {
	return nil, errors.Errorf("creating asset-provided task for AssetType %s is not supported", assetTaskInfo.AssetType)
}
