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

package cmd

import (
	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/distgo/productmavencoord"
	"github.com/spf13/cobra"
)

var productMavenCoordCmd = &cobra.Command{
	Use:   "product-maven-coord [product-ids]",
	Short: "Print the maven coordinate(s) of the specified product(s) in this project",
	RunE: func(cmd *cobra.Command, args []string) error {
		projectInfo, projectParam, err := distgoProjectParamFromFlags()
		if err != nil {
			return err
		}
		return productmavencoord.Run(projectInfo, projectParam, distgo.ToProductIDs(args), cmd.OutOrStdout())
	},
}

func init() {
	rootCmd.AddCommand(productMavenCoordCmd)
}
