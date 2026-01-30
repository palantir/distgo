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

package distertaskproviderapi

import (
	"io"

	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/distgotaskprovider"
	"github.com/spf13/cobra"
)

// DisterConfigYAML represents the configuration YAML for a dister.
type DisterConfigYAML struct {
	DisterName string `yaml:"dister-name"`
	ConfigYAML []byte `yaml:"config-yaml"`
}

// DisterTask packages a TaskInfo and TaskRunner for a dister asset-provided tasks.
type DisterTask struct {
	TaskInfo   distgotaskprovider.TaskInfo
	TaskRunner TaskRunner
}

// TaskRunner is an interface that runs the task provided by a dister task provider.
type TaskRunner interface {
	// ConfigureCommand configures the provided *cobra.Command for the task runner. If a *cobra.Command is constructed
	// for the runner, this function is called on the command after it is constructed. The typical use case for this
	// is to register flags on the command that can be used/referenced in RunTask.
	ConfigureCommand(cmd *cobra.Command)

	// RunTask runs the task associated with the runner. It is provided with the configuration YAML for all the disters
	// of the dister type and the ProductTaskOutputInfos associated with the disters of the dister type. The args
	// parameter contains all the arguments provided to the command that invoked the task. Task output can be written
	// to the provided stdout and stderr writers.
	RunTask(
		disterConfigYAML map[distgo.ProductID]map[distgo.DistID]DisterConfigYAML,
		allProductTaskOutputInfos map[distgo.ProductID]distgo.ProductTaskOutputInfo,
		args []string,
		stdout, stderr io.Writer,
	) error
}
