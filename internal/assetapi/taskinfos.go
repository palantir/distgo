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
	"fmt"
	"maps"
	"os/exec"
	"slices"

	"github.com/palantir/distgo/distgotaskprovider"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

const TaskInfosCommand = "task-infos"

type TaskInfos struct {
	// AssetName is the name of the asset. Must be globally unique for an asset of a given type.
	// Should be human-readable and short (and use kebab-case if it has multiple components), as this value is used as
	// part of the fully qualified asset-provided task command.
	AssetName string `json:"asset-name"`

	// TaskInfos specifies the tasks provided by the asset.
	TaskInfos map[string]distgotaskprovider.TaskInfo `json:"task-infos"`
}

// NewTaskInfosCommand returns a command that marshals the provided taskInfos as JSON and prints it to stdout.
func NewTaskInfosCommand(taskInfos TaskInfos) *cobra.Command {
	return &cobra.Command{
		Use:   TaskInfosCommand,
		Short: "Prints the JSON representation of the asset-provided tasks information",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOutput, err := json.Marshal(taskInfos)
			if err != nil {
				return errors.Wrapf(err, "failed to marshal JSON")
			}
			_, _ = fmt.Fprint(cmd.OutOrStdout(), string(jsonOutput))
			return nil
		},
	}
}

// GetTaskInfos returns the TaskInfos returned by unmarshalling the JSON returned by invoking the TaskInfosCommand on
// the asset at the specified path. Returns nil if the asset does not provide any tasks (including returning an error
// when invoking the command on the asset at the provided path). Returns an error only in the case where invoking the
// TaskInfosCommand on the asset return with an exit code of 0 but produces output that cannot be unmarshalled as a
// *TaskInfos from JSON.
func GetTaskInfos(assetPath string) (*TaskInfos, error) {
	cmd := exec.Command(assetPath, TaskInfosCommand)
	outputBytes, err := cmd.Output()
	if err != nil {
		// assets providing task information is optional, so if there is an error invoking the command,
		// assume that the asset is not a task provider (but do not propagate error further)
		return nil, nil
	}

	var taskInfos *TaskInfos
	if err := json.Unmarshal(outputBytes, &taskInfos); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal output %q as JSON into type %T", string(outputBytes), taskInfos)
	}
	return taskInfos, nil
}

// GetTaskProviderVerifyTasksFromAssets returns a slice that contains the VerifyTaskInfo for all the provided assets.
func GetTaskProviderVerifyTasksFromAssets(assets Assets) []AssetTaskInfo {
	var verifyTaskInfos []AssetTaskInfo
	for _, assetType := range slices.Sorted(maps.Keys(assets.assets)) {
		for _, currAsset := range assets.assets[assetType] {
			if currAsset.TaskInfos == nil {
				continue
			}
			for _, currTaskInfo := range currAsset.TaskInfos.TaskInfos {
				if currTaskInfo.VerifyOptions == nil {
					continue
				}
				// task with non-nil verify option
				verifyTaskInfos = append(verifyTaskInfos, AssetTaskInfo{
					AssetPath: currAsset.AssetPath,
					AssetType: currAsset.AssetType,
					AssetName: currAsset.TaskInfos.AssetName,
					TaskInfo:  currTaskInfo,
				})
			}
		}
	}
	return verifyTaskInfos
}
