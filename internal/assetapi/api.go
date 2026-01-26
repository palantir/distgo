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

package assetapi

import (
	"encoding/json"
	"os/exec"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// AssetType is the name of the type of asset.
type AssetType string

// Asset types supported by distgo.
const (
	Dister        AssetType = "dister"
	Publisher     AssetType = "publisher"
	DockerBuilder AssetType = "docker-builder"
)

// Asset represents a distgo asset.
type Asset struct {
	// AssetPath is the path to the asset.
	AssetPath string

	// AssetType is the type of asset.
	AssetType AssetType
}

// Assets represents a collection of distgo assets.
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

const AssetTypeCommand = "asset-type"

func NewAssetTypeCmd(assetType AssetType) *cobra.Command {
	return &cobra.Command{
		Use:   AssetTypeCommand,
		Short: "Prints the JSON representation of the asset type",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOutput, err := json.Marshal(assetType)
			if err != nil {
				return errors.Wrapf(err, "failed to marshal JSON")
			}
			cmd.Print(string(jsonOutput))
			return nil
		},
	}
}

func getAssetType(assetPath string) (AssetType, error) {
	cmd := exec.Command(assetPath, AssetTypeCommand)
	outputBytes, err := cmd.CombinedOutput()
	if err != nil {
		return "", errors.Wrapf(err, "failed to run command %v, output: %s", cmd.Args, string(outputBytes))
	}

	var assetType AssetType
	if err := json.Unmarshal(outputBytes, &assetType); err != nil {
		return "", errors.Wrapf(err, "failed to unmarshal JSON")
	}

	switch assetType {
	case Dister, Publisher, DockerBuilder:
		return assetType, nil
	default:
		return "", errors.Errorf("unrecognized asset type: %s", assetType)
	}
}
