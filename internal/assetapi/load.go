// Copyright 2016 Palantir Technologies, Inc.
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

package assetapi

import (
	"github.com/pkg/errors"
)

// LoadAssets loads the assets at the specified path and returns a map from AssetType to the paths for the assets of
// that type. Returns an error if any of the provided assets do not respond to the command that queries for their type
// or if the returned type is not a recognized asset type.
func LoadAssets(assets []string) (Assets, error) {
	loadedAssets := Assets{
		assets: make(map[AssetType][]Asset),
	}
	for _, currAsset := range assets {
		assetType, err := getAssetType(currAsset)
		if err != nil {
			return Assets{}, errors.Wrapf(err, "failed to get asset type for asset %s", currAsset)
		}
		taskInfos, err := GetTaskInfos(currAsset)
		// error only occurs if information is returned but not parsable, so propagate that
		if err != nil {
			return Assets{}, errors.Wrapf(err, "failed to get task infos for asset %s", currAsset)
		}
		loadedAssets.assets[assetType] = append(loadedAssets.assets[assetType], Asset{
			AssetPath: currAsset,
			AssetType: assetType,
			TaskInfos: taskInfos,
		})
	}
	return loadedAssets, nil
}
