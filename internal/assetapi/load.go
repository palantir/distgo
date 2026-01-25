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
	"maps"
	"slices"

	"github.com/pkg/errors"
)

type Assets struct {
	assets map[AssetType][]Asset
}

// GetAssetPathsForType returns the paths to the asset of the provided type.
func (a *Assets) GetAssetPathsForType(assetType AssetType) []string {
	var out []string
	for _, asset := range a.assets[assetType] {
		out = append(out, asset.AssetPath)
	}
	return out
}

// AssetsWithTaskInfos returns all the Asset structs that have a non-nil TaskInfos field. The assets in the returned
// slice are ordered by the natural ordering of AssetType and, within a type, occur in the same order as they occur
// in the value slice of the assets map.
func (a *Assets) AssetsWithTaskInfos() []Asset {
	var out []Asset
	for _, assetType := range slices.Sorted(maps.Keys(a.assets)) {
		for _, asset := range a.assets[assetType] {
			if asset.TaskInfos == nil {
				continue
			}
			out = append(out, asset)
		}
	}
	return out
}

type Asset struct {
	// path to the asset
	AssetPath string

	// type of asset
	AssetType AssetType

	// information for asset-provided tasks. nil if the asset does not have any asset-provided tasks.
	TaskInfos *TaskInfos
}

// LoadAssets loads the assets at the specified path and returns an Assets struct that represents the loaded assets.
// Returns an error if any of the provided assets do not respond to the command that queries for their type or if the
// returned type is not a recognized asset type.
func LoadAssets(assetPaths []string) (Assets, error) {
	loadedAssets := Assets{
		assets: make(map[AssetType][]Asset),
	}
	for _, currAsset := range assetPaths {
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
