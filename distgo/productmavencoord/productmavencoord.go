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

package productmavencoord

import (
	"fmt"
	"io"

	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/publisher"
)

func Run(projectInfo distgo.ProjectInfo, projectParam distgo.ProjectParam, specifiedProductIDs []distgo.ProductID, stdout io.Writer) error {
	productParams, err := distgo.ProductParamsForProductArgs(projectParam.Products, specifiedProductIDs...)
	if err != nil {
		return err
	}
	for _, productParam := range productParams {
		productID := productParam.ID
		productTaskOutputInfo, err := distgo.ToProductTaskOutputInfo(projectInfo, projectParam.Products[productID])
		if err != nil {
			return err
		}

		groupID, err := publisher.GetRequiredGroupID(nil, productTaskOutputInfo)
		if err != nil {
			return err
		}

		_, _ = fmt.Fprintf(stdout, "%s:%s:%s\n", groupID, productTaskOutputInfo.Product.Name, productTaskOutputInfo.Project.Version)
	}
	return nil
}
