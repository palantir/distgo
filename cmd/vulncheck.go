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
	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/distgo/vulncheck"
	"github.com/spf13/cobra"
)

var (
	vulncheckCmd = &cobra.Command{
		Use:   "vulncheck [flags] [product-ids]",
		Short: "Run vulnerability check and generate VEX documents for products",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectInfo, projectParam, err := distgoProjectParamFromFlags()
			if err != nil {
				return err
			}
			return vulncheck.Products(projectInfo, projectParam, distgo.ToProductIDs(args), vulncheck.Options{
				DryRun: vulncheckDryRunFlagVal,
			}, cmd.OutOrStdout())
		},
	}

	vulncheckPrintCmd = &cobra.Command{
		Use:   "print [flags] [product-ids]",
		Short: "Run vulnerability check and print results to stdout in text format",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectInfo, projectParam, err := distgoProjectParamFromFlags()
			if err != nil {
				return err
			}
			return vulncheck.PrintProducts(projectInfo, projectParam, distgo.ToProductIDs(args), vulncheck.Options{
				DryRun: vulncheckPrintDryRunFlagVal,
			}, cmd.OutOrStdout())
		},
	}

	vulncheckDryRunFlagVal      bool
	vulncheckPrintDryRunFlagVal bool
)

func init() {
	vulncheckCmd.Flags().BoolVar(&vulncheckDryRunFlagVal, "dry-run", false, "print the operations that would be performed")
	vulncheckPrintCmd.Flags().BoolVar(&vulncheckPrintDryRunFlagVal, "dry-run", false, "print the operations that would be performed")

	vulncheckCmd.AddCommand(vulncheckPrintCmd)
	rootCmd.AddCommand(vulncheckCmd)
}
