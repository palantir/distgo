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

package dist

import (
	"time"

	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/distgo/build"
)

// RequiredTasks reports all build and distribution work needed for the selected distribution dependency graph.
// It applies the normal product and configuration filters without executing task actions or build, dist, or Docker
// scripts. Build args scripts are run to determine the flags used to load build inputs.
func RequiredTasks(projectInfo distgo.ProjectInfo, projectParam distgo.ProjectParam, configModTime *time.Time, productDistIDs []distgo.ProductDistID) ([]distgo.RequiredTask, error) {
	projectParam, products, err := selectProducts(projectParam, productDistIDs)
	if err != nil {
		return nil, err
	}
	products = productsWithDistOutputs(products)
	required, err := build.RequiredTasks(projectInfo, buildPrerequisites(projectParam, products))
	if err != nil {
		return nil, err
	}
	distributions, err := requiredDistTasks(projectInfo, projectParam, products, configModTime)
	if err != nil {
		return nil, err
	}
	return append(required, distributions...), nil
}

// RequiredDistTasks is RequiredTasks without build prerequisites, for callers that check builds themselves.
func RequiredDistTasks(projectInfo distgo.ProjectInfo, projectParam distgo.ProjectParam, configModTime *time.Time, productDistIDs []distgo.ProductDistID) ([]distgo.RequiredTask, error) {
	projectParam, products, err := selectProducts(projectParam, productDistIDs)
	if err != nil {
		return nil, err
	}
	return requiredDistTasks(projectInfo, projectParam, productsWithDistOutputs(products), configModTime)
}

func requiredDistTasks(projectInfo distgo.ProjectInfo, projectParam distgo.ProjectParam, products []distgo.ProductParam, configModTime *time.Time) ([]distgo.RequiredTask, error) {
	var required []distgo.RequiredTask
	allProducts, _, _ := distgo.ClassifyProductParams(products)
	targets, ids, err := distgo.TopoSortProductParams(projectParam, allProducts)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		product := targets[id]
		if product.Dist == nil {
			continue
		}
		info, err := distgo.ToProductTaskOutputInfo(projectInfo, product)
		if err != nil {
			return nil, err
		}
		outputs := info.ProductDistWorkDirsAndArtifactPaths()
		for _, distID := range info.Product.DistOutputInfos.DistIDs {
			if reason := disterRequiresDistReason(distID, info, configModTime, true); reason != "" {
				required = append(required, distgo.RequiredTask{
					Task: "dist", Product: id, Target: string(distID),
					Outputs: outputs[distID], Reason: reason,
				})
			}
		}
	}
	return required, nil
}

func productsWithDistOutputs(products []distgo.ProductParam) []distgo.ProductParam {
	var active []distgo.ProductParam
	for _, product := range products {
		if product.Dist != nil && len(product.Dist.DistParams) != 0 {
			active = append(active, product)
		}
	}
	return active
}
