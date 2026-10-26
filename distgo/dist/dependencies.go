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
// It applies the normal product and configuration filters without executing task actions or scripts.
func RequiredTasks(projectInfo distgo.ProjectInfo, projectParam distgo.ProjectParam, configModTime *time.Time, productDistIDs []distgo.ProductDistID) ([]distgo.RequiredTask, error) {
	projectParam, products, err := selectProducts(projectParam, productDistIDs)
	if err != nil {
		return nil, err
	}
	required, err := build.RequiredTasks(projectInfo, buildPrerequisites(projectParam, products))
	if err != nil {
		return nil, err
	}
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
			if reason := disterRequiresDistReason(distID, info, configModTime); reason != "" {
				required = append(required, distgo.RequiredTask{
					Task: "dist", Product: id, Target: string(distID),
					Outputs: outputs[distID], Reason: reason,
				})
			}
		}
	}
	return required, nil
}
