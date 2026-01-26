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

package distgotaskprovider

// TaskInfo is the information needed to create a distgo task.
// Based on the godel TaskInfo definition at https://github.com/palantir/godel/blob/8537d0ea9067d3bdd36d5db06069b71fde92188b/framework/pluginapi/v2/pluginapi/taskinfo.go#L44.
type TaskInfo struct {
	// Name is the name of the task. This is the task/command name registered by the distgo TaskProvider API.
	Name string `json:"name"`

	// Description is the description of the task. It is used as the "Short" description of the task/command.
	Description string `json:"description"`

	// Command specifies the arguments used to invoke the task/command on the asset. In many instances, this may be the
	// same value as Name, but it may be different if the task name for the purposes of the TaskProvider API is
	// different from the command used to invoke the task on the asset.
	Command []string `json:"command"`

	// RegisterAsTopLevelDistgoTaskCommand indicates whether this task should be registered as a top-level command under
	// the "distgo-task" task. The command is always registered as a fully qualified command regardless of this value.
	// Even if this value is true, the command may not be registered as a top-level command if its name conflicts with
	// any default values or other with top-level commands registered by other assets.
	RegisterAsTopLevelDistgoTaskCommand bool `json:"registerAsTopLevelDistgoTaskCommand"`
}
