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

package distgo

import (
	"fmt"
	"sort"
	"strings"

	"github.com/palantir/godel/v2/pkg/osarch"
	"github.com/pkg/errors"
)

// ProductParamsForProductArgs returns the ProductParams from the provided inputProducts for the specified ProductIDs.
//
// Returns an error if any of the ProductID values cannot be resolved to a configuration in the provided inputProducts.
func ProductParamsForProductArgs(inputProducts map[ProductID]ProductParam, productIDs ...ProductID) ([]ProductParam, error) {
	// error if project does not contain any productIDs
	if len(inputProducts) == 0 {
		return nil, errors.Errorf("project does not contain any productIDs")
	}
	// if no productIDs were specified, return project's productIDs unmodified
	if len(productIDs) == 0 {
		return toSortedProductParams(inputProducts), nil
	}

	productIDSet := make(map[ProductID]struct{})
	for _, currProductID := range productIDs {
		productIDSet[currProductID] = struct{}{}
	}
	validIDs := make(map[ProductID]struct{})
	for productID := range inputProducts {
		validIDs[productID] = struct{}{}
	}
	var validIDsSorted []ProductID
	for productID := range validIDs {
		validIDsSorted = append(validIDsSorted, productID)
	}
	sort.Sort(ByProductID(validIDsSorted))

	var invalidIDs []ProductID
	for _, productID := range productIDs {
		if _, ok := validIDs[productID]; ok {
			continue
		}
		invalidIDs = append(invalidIDs, productID)
	}
	sort.Sort(ByProductID(invalidIDs))
	if len(invalidIDs) > 0 {
		return nil, errors.Errorf("product(s) %v not valid -- valid values are %v", invalidIDs, validIDsSorted)
	}

	filteredProducts := make(map[ProductID]ProductParam)
	for _, productID := range productIDs {
		filteredProducts[productID] = inputProducts[productID]
	}
	return toSortedProductParams(filteredProducts), nil
}

// ProductBuildID identifies a product or a specific build for a product. A ProductBuildID is one of the following:
//   - {{ProductID}} (e.g. "foo"), which specifies that all OS/Archs for the product should be built
//   - {{ProductID}}.{{OSArch}} (e.g. "foo.darwin-amd64"), which specifies that the specified OS/Arch for the specified
//     product should be built
type ProductBuildID string

func NewProductBuildID(productID ProductID, osArch osarch.OSArch) ProductBuildID {
	if osArch == (osarch.OSArch{}) {
		return ProductBuildID(productID)
	}
	return ProductBuildID(fmt.Sprintf("%s.%s", productID, osArch.String()))
}

func (id ProductBuildID) Parse() (ProductID, osarch.OSArch, error) {
	currProductID := ProductID(id)
	var osArch osarch.OSArch
	if dotIdx := strings.Index(string(id), "."); dotIdx != -1 {
		currProductID = ProductID(id[:dotIdx])
		osArchVal, err := osarch.New(string(id[dotIdx+1:]))
		if err != nil {
			return "", osarch.OSArch{}, errors.Wrapf(err, "failed to parse os-arch for %s", id)
		}
		osArch = osArchVal
	}
	return currProductID, osArch, nil
}

func ToProductBuildIDs(in []string) []ProductBuildID {
	var ids []ProductBuildID
	for _, id := range in {
		ids = append(ids, ProductBuildID(id))
	}
	return ids
}

// ProductParamsForBuildProductArgs returns the ProductParams from the provided inputProducts for the specified
// ProductBuildIDs. The ProductParam values in the returned slice will reflect the items specified by the build IDs. If
// the osArchs parameter is non-empty, then the returned results will only include ProductParam values that match the
// provided osArchs. For example, if the project defines a product "foo" with OS-Archs "darwin-amd64" and "linux-amd64"
// and the productBuildID is "foo.darwin-amd64", the returned ProductParam will only contain "darwin-amd64" in the build
// configuration. Returns an error if any of the productBuildID values cannot be resolved to a configuration in the
// provided inputProducts.
func ProductParamsForBuildProductArgs(inputProducts map[ProductID]ProductParam, osArchs []osarch.OSArch, productBuildIDs ...ProductBuildID) ([]ProductParam, error) {
	// error if project does not contain any productBuildIDs
	if len(inputProducts) == 0 {
		return nil, errors.Errorf("project does not contain any products")
	}
	// if no productBuildIDs were specified, return project's productBuildIDs unmodified
	if len(productBuildIDs) == 0 {
		return filterProductParamsToOSArch(toSortedProductParams(inputProducts), osArchs), nil
	}

	productIDToOSArchs := make(map[ProductID][]osarch.OSArch)
	for _, currProductBuildID := range productBuildIDs {
		currProductID, osArch, err := currProductBuildID.Parse()
		if err != nil {
			return nil, err
		}
		productIDToOSArchs[currProductID] = append(productIDToOSArchs[currProductID], osArch)
	}
	validIDs := make(map[string]struct{})
	for productID, productParam := range inputProducts {
		validIDs[string(productID)] = struct{}{}
		if productParam.Build == nil {
			continue
		}
		for _, osArch := range productParam.Build.OSArchs {
			validIDs[fmt.Sprintf("%s.%s", productID, osArch)] = struct{}{}
		}
	}
	validIDsSorted := stringSetToSortedSlice(validIDs)

	var invalidIDs []string
	for productID, osArchs := range productIDToOSArchs {
		for _, currOSArch := range osArchs {
			currCombinedID := string(productID)
			if currOSArch != (osarch.OSArch{}) {
				currCombinedID = fmt.Sprintf("%s.%s", productID, currOSArch.String())
			}
			if _, ok := validIDs[currCombinedID]; ok {
				continue
			}
			invalidIDs = append(invalidIDs, currCombinedID)
		}
	}
	sort.Strings(invalidIDs)
	if len(invalidIDs) > 0 {
		return nil, errors.Errorf("build product(s) %v not valid -- valid values are %v", invalidIDs, validIDsSorted)
	}

	// all IDs are valid. For any ID that has an empty OS/Arch as a value, expand to all OS/Archs.
	for productID, osArchs := range productIDToOSArchs {
		allVals := false
		for _, currOSArchs := range osArchs {
			if currOSArchs == (osarch.OSArch{}) {
				allVals = true
				break
			}
		}
		if !allVals || inputProducts[productID].Build == nil {
			continue
		}

		var allOSArchs []osarch.OSArch
		for _, osArch := range inputProducts[productID].Build.OSArchs {
			allOSArchs = append(allOSArchs, osArch)
		}
		sort.Sort(byOSArch(allOSArchs))
		productIDToOSArchs[productID] = allOSArchs
	}

	filteredProducts := make(map[ProductID]ProductParam)
	for productID, osArchs := range productIDToOSArchs {
		currProductParam := inputProducts[productID]
		if currProductParam.Build == nil {
			continue
		}

		// modify copy so that original value remains the same
		buildCopy := *currProductParam.Build
		buildCopy.OSArchs = osArchs
		currProductParam.Build = &buildCopy

		filteredProducts[productID] = currProductParam
	}
	return filterProductParamsToOSArch(toSortedProductParams(filteredProducts), osArchs), nil
}

// If osArchs is non-empty, returns a new ProductParam slice that contains only ProductParam values in the input where
// at least one of the OSArchs in Build.OSArchs of the ProductParam is in the provided osArchs param. The Build.OSArchs
// of the ProductParam values in the returned slice will also only contain the OSArchs that match the filter input. If
// osArchs is empty, then the input is returned unmodified.
func filterProductParamsToOSArch(in []ProductParam, osArchs []osarch.OSArch) []ProductParam {
	// if filter set is empty, no need to filter
	if len(osArchs) == 0 {
		return in
	}

	osArchsMap := make(map[osarch.OSArch]struct{})
	for _, osArch := range osArchs {
		osArchsMap[osArch] = struct{}{}
	}

	var out []ProductParam
	for _, currParam := range in {
		filtered := filterOSArch(currParam.Build.OSArchs, osArchsMap)
		if len(filtered) == 0 {
			continue
		}
		currParam.Build.OSArchs = filtered
		out = append(out, currParam)
	}
	return out
}

// if filter is non-empty, returns a new osArch slice that contains only the values in the input that are also keys in
// filter. If filter is empty, the input is returned unmodified.
func filterOSArch(in []osarch.OSArch, filter map[osarch.OSArch]struct{}) []osarch.OSArch {
	if len(filter) == 0 {
		return in
	}
	var out []osarch.OSArch
	for _, v := range in {
		if _, ok := filter[v]; !ok {
			continue
		}
		out = append(out, v)
	}
	return out
}

// ProductDistID identifies a product or a specific dist for a product. A ProductDistID is one of the following:
//   - {{ProductID}} (e.g. "foo"), which specifies that all dists for the product should be built
//   - {{ProductID}}.{{DistID}} (e.g. "foo.os-arch-bin"), which specifies that the specified DistID for the specified
//     product should be built
type ProductDistID string

func (id ProductDistID) Parse() (ProductID, DistID) {
	currProductID := ProductID(id)
	var distID DistID
	if dotIdx := strings.Index(string(id), "."); dotIdx != -1 {
		currProductID = ProductID(id[:dotIdx])
		distID = DistID(id[dotIdx+1:])
	}
	return currProductID, distID
}

func NewProductDistID(productID ProductID, distID DistID) ProductDistID {
	if distID == "" {
		return ProductDistID(productID)
	}
	return ProductDistID(fmt.Sprintf("%s.%s", productID, distID))
}

func ToProductDistIDs(in []string) []ProductDistID {
	var ids []ProductDistID
	for _, id := range in {
		ids = append(ids, ProductDistID(id))
	}
	return ids
}

// ProductParamsForDistProductArgs returns the ProductParams from the provided inputProducts for the specified
// productDistIDs. The ProductParam values in the returned slice will reflect the items specified by the dister IDs. For
// example, if the project defines a product "foo" with DistParams "os-arch-bin" and "manual" and the productDistID is
// "foo.os-arch-bin", the returned ProductParam will only contain "os-arch-bin" in the dist configuration. Returns an
// error if any of the productDistID values cannot be resolved to a configuration in the provided inputProducts.
func ProductParamsForDistProductArgs(inputProducts map[ProductID]ProductParam, productDistIDs ...ProductDistID) ([]ProductParam, error) {
	// error if project does not contain any productDistIDs
	if len(inputProducts) == 0 {
		return nil, errors.Errorf("project does not contain any products")
	}
	// if no productDistIDs were specified, return project's productDistIDs unmodified
	if len(productDistIDs) == 0 {
		return toSortedProductParams(inputProducts), nil
	}

	productIDToDistIDs := make(map[ProductID][]DistID)
	for _, currProductDistID := range productDistIDs {
		currProductID, distID := currProductDistID.Parse()
		productIDToDistIDs[currProductID] = append(productIDToDistIDs[currProductID], distID)
	}
	validIDs := make(map[string]struct{})
	for productID, productParam := range inputProducts {
		validIDs[string(productID)] = struct{}{}
		if productParam.Dist == nil {
			continue
		}
		for distID := range (*productParam.Dist).DistParams {
			validIDs[fmt.Sprintf("%s.%s", productID, distID)] = struct{}{}
		}
	}
	validIDsSorted := stringSetToSortedSlice(validIDs)

	var invalidIDs []string
	for productID, distIDs := range productIDToDistIDs {
		for _, currDistID := range distIDs {
			currCombinedID := string(productID)
			if currDistID != "" {
				currCombinedID = fmt.Sprintf("%s.%s", productID, currDistID)
			}
			if _, ok := validIDs[currCombinedID]; ok {
				continue
			}
			invalidIDs = append(invalidIDs, currCombinedID)
		}
	}
	sort.Strings(invalidIDs)
	if len(invalidIDs) > 0 {
		return nil, errors.Errorf("dist product(s) %v not valid -- valid values are %v", invalidIDs, validIDsSorted)
	}

	// all IDs are valid. For any ID that has "" as a value, expand to all dists.
	for productID, distIDs := range productIDToDistIDs {
		allVals := false
		for _, currDistID := range distIDs {
			if currDistID == "" {
				allVals = true
				break
			}
		}
		if !allVals || inputProducts[productID].Dist == nil {
			continue
		}
		var allDistIDs []DistID
		for k := range (*inputProducts[productID].Dist).DistParams {
			allDistIDs = append(allDistIDs, k)
		}
		sort.Sort(ByDistID(allDistIDs))
		productIDToDistIDs[productID] = allDistIDs
	}

	filteredProducts := make(map[ProductID]ProductParam)
	for productID, distIDs := range productIDToDistIDs {
		currProductParam := inputProducts[productID]
		if currProductParam.Dist == nil {
			continue
		}
		var newDisters map[DistID]DisterParam
		if len(distIDs) > 0 {
			newDisters = make(map[DistID]DisterParam)
			for _, currDistID := range distIDs {
				newDisters[currDistID] = (*currProductParam.Dist).DistParams[currDistID]
			}
		}

		// modify copy so that original value remains the same
		distCopy := *currProductParam.Dist
		distCopy.DistParams = newDisters
		currProductParam.Dist = &distCopy

		filteredProducts[productID] = currProductParam
	}
	return toSortedProductParams(filteredProducts), nil
}

// ProductDockerID identifies a product, a specific Docker builder for a product, or a specific Docker tag for a
// specific Docker builder for a product. A ProductDockerID is one of the following:
//   - {{ProductID}} (e.g. "foo"), which identifies all Docker images and its tags for the product
//   - {{ProductID}}.{{DockerID}} (e.g. "foo.prod-docker"), which specifies all of the tags for the specified DockerID
//     for the specified product
//   - {{ProductID}}.{{DockerID}}.{{DockerTagID}} (e.g. "foo.prod-docker.release"), which specifies a specific tag for
//     the specified DockerID for the specified product
type ProductDockerID string

func (id ProductDockerID) Parse() (ProductID, DockerID, DockerTagID) {
	productID := ProductID(id)
	var dockerID DockerID
	var tagID DockerTagID
	if dotIdx := strings.Index(string(id), "."); dotIdx != -1 {
		productID = ProductID(id[:dotIdx])

		rest := string(id[dotIdx+1:])
		dockerID = DockerID(rest)
		if secondDotIdx := strings.Index(rest, "."); secondDotIdx != -1 {
			dockerID = DockerID(rest[:secondDotIdx])
			tagID = DockerTagID(rest[secondDotIdx+1:])
		}
	}
	return productID, dockerID, tagID
}

func NewProductDockerID(productID ProductID, dockerID DockerID, dockerTagID DockerTagID) ProductDockerID {
	idStr := string(productID)
	if dockerID != "" {
		idStr += "." + string(dockerID)
		if dockerTagID != "" {
			idStr += "." + string(dockerTagID)
		}
	}
	return ProductDockerID(idStr)
}

func ToProductDockerIDs(in []string) []ProductDockerID {
	var ids []ProductDockerID
	for _, id := range in {
		ids = append(ids, ProductDockerID(id))
	}
	return ids
}

// ProductParamsForDockerProductArgs returns the ProductParams from the provided inputProducts for the specified
// productDockerIDs. The ProductParam values in the returned slice will reflect the items specified by the DockerIDs.
// For example, if the project defines a product "foo" with DockerBuilderParams "docker-prod" and "docker-dev" and both
// of them have tags named "snapshot" and "release" and the productDockerID is "foo.docker-prod.release", the returned
// ProductParam will only contain "docker-prod" with the tag "release" in the Docker configuration. Returns an error if
// any of the productDockerID values cannot be resolved to a configuration in the provided project.
func ProductParamsForDockerProductArgs(inputProducts map[ProductID]ProductParam, productDockerIDs ...ProductDockerID) ([]ProductParam, error) {
	if len(inputProducts) == 0 {
		return nil, errors.Errorf("project does not contain any products")
	}
	// if no productDockerIDs were specified, return project's productDockerIDs unmodified
	if len(productDockerIDs) == 0 {
		return toSortedProductParams(inputProducts), nil
	}

	validIDs := make(map[string]struct{})
	for productID, productParam := range inputProducts {
		validIDs[string(productID)] = struct{}{}
		if productParam.Docker == nil {
			continue
		}
		for dockerID, param := range (*productParam.Docker).DockerBuilderParams {
			validIDs[fmt.Sprintf("%s.%s", productID, dockerID)] = struct{}{}
			for _, dockerTagID := range param.TagTemplates.OrderedKeys {
				validIDs[fmt.Sprintf("%s.%s.%s", productID, dockerID, dockerTagID)] = struct{}{}
			}
		}
	}
	validIDsSorted := stringSetToSortedSlice(validIDs)

	var invalidIDs []string
	for _, providedProductDockerID := range productDockerIDs {
		if _, ok := validIDs[string(providedProductDockerID)]; ok {
			continue
		}
		invalidIDs = append(invalidIDs, string(providedProductDockerID))
	}
	sort.Strings(invalidIDs)
	if len(invalidIDs) > 0 {
		return nil, errors.Errorf("Docker product(s) %v not valid -- valid values are %v", invalidIDs, validIDsSorted)
	}

	productIDToDockerIDToTags := make(map[ProductID]map[DockerID]map[DockerTagID]struct{})
	for _, currProductDockerID := range productDockerIDs {
		currProductID, dockerID, dockerTagID := currProductDockerID.Parse()

		// get DockerID map (even if DockerID is empty, this will add entry for ProductID with an empty map as the value)
		currDockerIDMap := productIDToDockerIDToTags[currProductID]
		if currDockerIDMap == nil {
			currDockerIDMap = make(map[DockerID]map[DockerTagID]struct{})
			productIDToDockerIDToTags[currProductID] = currDockerIDMap
		}
		if dockerID == "" {
			continue
		}

		// get DockerTagID map (even if DockerTagID is empty, this will add entry for DockerID with an empty map as the value)
		currDockerTagMap := currDockerIDMap[dockerID]
		if currDockerTagMap == nil {
			currDockerTagMap = make(map[DockerTagID]struct{})
			currDockerIDMap[dockerID] = currDockerTagMap
		}
		if dockerTagID == "" {
			continue
		}

		currDockerTagMap[dockerTagID] = struct{}{}
	}

	// all IDs are valid: expand any IDs that do not specify an exact Docker tag
	for productID, dockerIDsMap := range productIDToDockerIDToTags {
		// if no Docker configuration exists for products, no expansion to be done
		if inputProducts[productID].Docker == nil {
			continue
		}

		// only product is specified: add all Docker tags for it
		if len(dockerIDsMap) == 0 {
			newDockerIDsMap := make(map[DockerID]map[DockerTagID]struct{})
			for currDockerID, currDockerParam := range inputProducts[productID].Docker.DockerBuilderParams {
				tagsMap := make(map[DockerTagID]struct{})
				for _, currTagID := range currDockerParam.TagTemplates.OrderedKeys {
					tagsMap[currTagID] = struct{}{}
				}
				newDockerIDsMap[currDockerID] = tagsMap
			}
			productIDToDockerIDToTags[productID] = newDockerIDsMap
			continue
		}

		for currDockerID, tagIDsMap := range dockerIDsMap {
			if len(tagIDsMap) != 0 {
				// tags are specified: nothing to do
				continue
			}

			// no tags specified for current image: expand to all tags
			newTagIDsMap := make(map[DockerTagID]struct{})
			for _, currTagID := range inputProducts[productID].Docker.DockerBuilderParams[currDockerID].TagTemplates.OrderedKeys {
				newTagIDsMap[currTagID] = struct{}{}
			}
			dockerIDsMap[currDockerID] = newTagIDsMap
		}
	}

	filteredProducts := make(map[ProductID]ProductParam)
	for productID, dockerIDsMap := range productIDToDockerIDToTags {
		currProductParam := inputProducts[productID]
		if currProductParam.Docker == nil {
			continue
		}
		var newDockerBuilders map[DockerID]DockerBuilderParam
		if len(dockerIDsMap) > 0 {
			newDockerBuilders = make(map[DockerID]DockerBuilderParam)
			for currDockerID, dockerTagsMap := range dockerIDsMap {
				currBuilderParam := (*currProductParam.Docker).DockerBuilderParams[currDockerID]

				tagTemplatesMap := TagTemplatesMap{
					Templates: make(map[DockerTagID]string),
				}
				for _, currTagKey := range currBuilderParam.TagTemplates.OrderedKeys {
					if _, ok := dockerTagsMap[currTagKey]; !ok {
						continue
					}
					// if current tag is in the dockerTagsMap (the filter), add it
					tagTemplatesMap.Templates[currTagKey] = currBuilderParam.TagTemplates.Templates[currTagKey]
					tagTemplatesMap.OrderedKeys = append(tagTemplatesMap.OrderedKeys, currTagKey)
				}
				currBuilderParam.TagTemplates = tagTemplatesMap
				newDockerBuilders[currDockerID] = currBuilderParam
			}
		}

		// modify copy so that original value remains the same
		dockerCopy := *currProductParam.Docker
		dockerCopy.DockerBuilderParams = newDockerBuilders
		currProductParam.Docker = &dockerCopy

		filteredProducts[productID] = currProductParam
	}
	return toSortedProductParams(filteredProducts), nil
}

// ProductParamsForDockerTagKeys returns the ProductParams from the provided inputProducts that have Docker tags whose
// keys are contained in the provided keys. For example, if the provided keys are "release" and "snapshot", the returned
// ProductParams will only contain Docker configurations that have tags that match one or both of those keys (and the
// Docker configuration will be updated to contain only the tags that match those keys). Returns the provided input
// unmodified if tagKeys does not contain any elements.
func ProductParamsForDockerTagKeys(inputProducts []ProductParam, tagKeys []string) []ProductParam {
	// if no products or tag keys were specified, return unmodified products
	if len(inputProducts) == 0 || len(tagKeys) == 0 {
		return inputProducts
	}

	tagKeysMap := make(map[string]struct{})
	for _, k := range tagKeys {
		tagKeysMap[k] = struct{}{}
	}

	var filteredProducts []ProductParam
	for _, currProductParam := range inputProducts {
		if currProductParam.Docker == nil {
			continue
		}
		newDockerBuilders := make(map[DockerID]DockerBuilderParam)
		for dockerID, dockerBuilderParam := range currProductParam.Docker.DockerBuilderParams {
			filteredTagTemplates := TagTemplatesMap{
				Templates: make(map[DockerTagID]string),
			}
			for _, currTagKey := range dockerBuilderParam.TagTemplates.OrderedKeys {
				if _, ok := tagKeysMap[string(currTagKey)]; !ok {
					continue
				}
				filteredTagTemplates.Templates[currTagKey] = dockerBuilderParam.TagTemplates.Templates[currTagKey]
				filteredTagTemplates.OrderedKeys = append(filteredTagTemplates.OrderedKeys, currTagKey)
			}
			if len(filteredTagTemplates.Templates) == 0 {
				continue
			}
			dockerBuilderParam.TagTemplates = filteredTagTemplates
			newDockerBuilders[dockerID] = dockerBuilderParam
		}

		// modify copy so that original value remains the same
		dockerCopy := *currProductParam.Docker
		dockerCopy.DockerBuilderParams = newDockerBuilders
		currProductParam.Docker = &dockerCopy
		filteredProducts = append(filteredProducts, currProductParam)
	}
	return filteredProducts
}

func ClassifyProductParams(productParams []ProductParam) (allProducts map[ProductID]struct{}, specifiedProducts map[ProductID]struct{}, dependentProducts map[ProductID]struct{}) {
	allProducts = make(map[ProductID]struct{})
	specifiedProducts = make(map[ProductID]struct{})
	dependentProducts = make(map[ProductID]struct{}) // may contain keys in specifiedProducts if a specified product is a dependency
	for _, currProductParam := range productParams {
		specifiedProducts[currProductParam.ID] = struct{}{}
		allProducts[currProductParam.ID] = struct{}{}
		for k := range currProductParam.AllDependencies {
			dependentProducts[k] = struct{}{}
			allProducts[k] = struct{}{}
		}
	}
	return
}

func TopoSortProductParams(projectParam ProjectParam, allProducts map[ProductID]struct{}) (map[ProductID]ProductParam, []ProductID, error) {
	targetProducts := make(map[ProductID]ProductParam)
	for currProductID := range allProducts {
		targetProducts[currProductID] = projectParam.Products[currProductID]
	}
	dependencyGraph, err := toDependencyGraph(targetProducts)
	if err != nil {
		return nil, nil, err
	}
	topoOrderedIDs, err := topologicalOrdering(dependencyGraph)
	if err != nil {
		return nil, nil, err
	}
	return targetProducts, topoOrderedIDs, nil
}

// toDependencyGraph returns a DAG representation of the provided product params where the nodes in the graph are
// products and the edges are the products that have the node as a first-level dependency. All of the products in the
// graph must be part of the productParams input.
func toDependencyGraph(productParams map[ProductID]ProductParam) (map[ProductID]map[ProductID]struct{}, error) {
	graph := make(map[ProductID]map[ProductID]struct{})
	for _, currProductParam := range productParams {
		if _, ok := graph[currProductParam.ID]; !ok {
			graph[currProductParam.ID] = make(map[ProductID]struct{})
		}
		for _, currDep := range currProductParam.FirstLevelDependencies {
			if _, ok := productParams[currDep]; !ok {
				return nil, errors.Errorf("product %s appears as a dependency of product %s but was not specified as a valid product", currDep, currProductParam.ID)
			}
			currDepMap, ok := graph[currDep]
			if !ok {
				currDepMap = make(map[ProductID]struct{})
				graph[currDep] = currDepMap
			}
			currDepMap[currProductParam.ID] = struct{}{}
		}
	}
	return graph, nil
}

// topologicalOrdering prints the topological ordering of the provided graph.
func topologicalOrdering(graph map[ProductID]map[ProductID]struct{}) ([]ProductID, error) {
	var order []ProductID
	// get all nodes in the graph and sort lexicographically for deterministic order
	var nodes []ProductID
	indeg := make(map[ProductID]int)
	for node := range graph {
		indeg[node] = 0
		nodes = append(nodes, node)
	}
	sort.Sort(ByProductID(nodes))
	// compute the incoming edges on each vertex
	for _, v := range nodes {
		for neighbor := range graph[v] {
			indeg[neighbor]++
		}
	}
	// q contains all vertices with in-degree zero
	var q []ProductID
	for _, v := range nodes {
		if indeg[v] == 0 {
			q = append(q, v)
		}
	}
	for len(q) > 0 {
		cur := q[0]
		q = q[1:]
		order = append(order, cur)
		var neighbors []ProductID
		// sort all the neighbors to ensure deterministic order
		for neighbor := range graph[cur] {
			neighbors = append(neighbors, neighbor)
		}
		sort.Sort(ByProductID(neighbors))
		for _, neighbor := range neighbors {
			indeg[neighbor]--
			if indeg[neighbor] == 0 {
				q = append(q, neighbor)
			}
		}
	}
	if len(order) != len(graph) {
		return nil, errors.Errorf("provided graph contains a cycle")
	}
	return order, nil
}

type byOSArch []osarch.OSArch

func (a byOSArch) Len() int           { return len(a) }
func (a byOSArch) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byOSArch) Less(i, j int) bool { return a[i].String() < a[j].String() }

func toSortedProductParams(products map[ProductID]ProductParam) []ProductParam {
	var productParams []ProductParam
	for _, currProduct := range products {
		productParams = append(productParams, currProduct)
	}
	sort.Slice(productParams, func(i, j int) bool {
		return productParams[i].ID < productParams[j].ID
	})
	return productParams
}

func stringSetToSortedSlice(in map[string]struct{}) []string {
	var out []string
	for k := range in {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
