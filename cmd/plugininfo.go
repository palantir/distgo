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

package cmd

import (
	"fmt"
	"os"

	"github.com/palantir/godel/v2/framework/pluginapi/v2/pluginapi"
	"github.com/spf13/cobra"
)

var (
	Version    = "unspecified"
	PluginInfo = pluginapi.MustNewPluginInfo(
		"com.palantir.distgo",
		"dist-plugin",
		Version,
		pluginapi.PluginInfoUsesConfigFile(),
		pluginapi.PluginInfoGlobalFlagOptions(
			pluginapi.GlobalFlagOptionsParamDebugFlag("--"+pluginapi.DebugFlagName),
			pluginapi.GlobalFlagOptionsParamProjectDirFlag("--"+pluginapi.ProjectDirFlagName),
			pluginapi.GlobalFlagOptionsParamGodelConfigFlag("--"+pluginapi.GodelConfigFlagName),
			pluginapi.GlobalFlagOptionsParamConfigFlag("--"+pluginapi.ConfigFlagName),
		),
		newTaskInfoFromCmd(artifactsCmd),
		newTaskInfoFromCmd(buildCmd),
		newTaskInfoFromCmd(cleanCmd),
		newTaskInfoFromCmd(distCmd),
		newTaskInfoFromCmd(dockerCmd),
		newTaskInfoFromCmd(productsCmd),
		newTaskInfoFromCmd(projectVersionCmd),
		newTaskInfoFromCmd(publishCmd),
		newTaskInfoFromCmd(runCmd),
		pluginapi.PluginInfoTaskInfo(
			"verify-dist-config",
			"Verify dist config(s)",
			pluginapi.TaskInfoCommand("verify-dist-config"),
			pluginapi.TaskInfoVerifyOptions(),
		),
		pluginapi.PluginInfoUpgradeConfigTaskInfo(
			pluginapi.UpgradeConfigTaskInfoCommand("upgrade-config"),
			pluginapi.LegacyConfigFile("dist.yml"),
		),
	)
)

func newTaskInfoFromCmd(cmd *cobra.Command) pluginapi.PluginInfoParam {
	return pluginapi.PluginInfoTaskInfo(
		cmd.Name(),
		cmd.Short,
		pluginapi.TaskInfoCommand(cmd.Name()),
	)
}

var (
	verifyDistConfigCmd = &cobra.Command{
		Use:   "verify-dist-config",
		Short: "Verify dist config(s)",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, e := fmt.Fprintf(os.Stderr, "os.Args: %v\n", os.Args)
			return e
		},
	}
)

func init() {
	rootCmd.AddCommand(verifyDistConfigCmd)
}
