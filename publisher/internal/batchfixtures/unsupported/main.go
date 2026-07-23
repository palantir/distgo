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

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/internal/assetapi"
	"github.com/palantir/godel/v2/framework/pluginapi"
	"github.com/palantir/pkg/cobracli"
	"github.com/spf13/cobra"
)

const typeName = "unsupported"

func main() {
	rootCmd := &cobra.Command{
		Use: typeName,
	}
	rootCmd.AddCommand(&cobra.Command{
		Use: "name",
		RunE: func(cmd *cobra.Command, args []string) error {
			outputJSON, err := json.Marshal(typeName)
			if err != nil {
				return err
			}
			cmd.Print(string(outputJSON))
			return nil
		},
	})
	rootCmd.AddCommand(assetapi.NewAssetTypeCmd(assetapi.Publisher))
	rootCmd.AddCommand(&cobra.Command{
		Use: "flags",
		RunE: func(cmd *cobra.Command, args []string) error {
			outputJSON, err := json.Marshal([]distgo.PublisherFlag{})
			if err != nil {
				return err
			}
			cmd.Print(string(outputJSON))
			return nil
		},
	})

	var productTaskOutputInfoFlagVal string
	runPublishCmd := &cobra.Command{
		Use: "run-publish",
		RunE: func(cmd *cobra.Command, args []string) error {
			var productTaskOutputInfo distgo.ProductTaskOutputInfo
			if err := json.Unmarshal([]byte(productTaskOutputInfoFlagVal), &productTaskOutputInfo); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "RunPublish:%s\n", productTaskOutputInfo.Product.ID)
			return nil
		},
	}
	runPublishCmd.Flags().StringVar(&productTaskOutputInfoFlagVal, "product-task-output-info", "", "")
	runPublishCmd.Flags().String("config-yml", "", "")
	runPublishCmd.Flags().String("flag-vals", "", "")
	runPublishCmd.Flags().Bool("dry-run", false, "")
	rootCmd.AddCommand(runPublishCmd)
	rootCmd.AddCommand(pluginapi.CobraUpgradeConfigCmd(func(cfgBytes []byte) ([]byte, error) {
		return cfgBytes, nil
	}))

	os.Exit(cobracli.ExecuteWithDefaultParams(rootCmd))
}
