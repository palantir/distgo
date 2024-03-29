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
	"strconv"
)

func CreateScriptContent(script, scriptIncludes string) string {
	if scriptIncludes == "" || script == "" {
		return script
	}
	return scriptIncludes + "\n" + script
}

// BuildScriptEnvVariables returns a map of environment variables for the script for the builder. The returned map
// contains the following environment variables:
//
//	PROJECT_DIR: the root directory of project
//	VERSION: the version of the project
//	PRODUCT: the name of the product
//
// The following environment variables are defined if the build configuration for the product is non-nil:
//
//	BUILD_DIR: the build output directory for the product ("{{OutputDir}}/{{ProductID}}/{{Version}}")
//	BUILD_NAME: the rendered NameTemplate for the build for this product
//	BUILD_OS_ARCH_COUNT: the number of OS/arch combinations for this product
//	BUILD_OS_ARCH_{#}: for 0 <= # < BUILD_OS_ARCHS_COUNT, contains the OS/arch for the build
func BuildScriptEnvVariables(outputInfo ProductTaskOutputInfo) map[string]string {
	m := map[string]string{
		"PROJECT_DIR": outputInfo.Project.ProjectDir,
		"VERSION":     outputInfo.Project.Version,
		"PRODUCT":     outputInfo.Product.Name,
	}

	// add build environment variables for current product
	addProductBuildEnvVariables(m, "", outputInfo.Project, outputInfo.Product)
	return m
}

// DistScriptEnvVariables returns a map of environment variables for the script for the dister with the specified
// DistID in the provided output configuration. The returned map contains the following environment variables:
//
//	PROJECT_DIR: the root directory of project
//	VERSION: the version of the project
//	PRODUCT: the name of the product
//	DEP_PRODUCT_ID_COUNT: the number of dependent products for the product
//	DEP_PRODUCT_ID_{#}: for 0 <= # < DEP_PRODUCT_IDS_COUNT, contains the dependent products for the product
//
// The following environment variables are defined if the build configuration for the product is non-nil:
//
//	BUILD_DIR: the build output directory for the product ("{{OutputDir}}/{{ProductID}}/{{Version}}")
//	BUILD_NAME: the rendered NameTemplate for the build for this product
//	BUILD_OS_ARCH_COUNT: the number of OS/arch combinations for this product
//	BUILD_OS_ARCH_{#}: for 0 <= # < BUILD_OS_ARCHS_COUNT, contains the OS/arch for the build
//
// The following environment variables are defined if the dist configuration for the product is non-nil:
//
//	DIST_ID: the DistID for the current distribution
//	DIST_DIR: the distribution output directory for the dist ("{{ProjectDir}}/{{OutputDir}}/{{ProductID}}/{{Version}}/{{DistID}}")
//	DIST_WORK_DIR: the distribution work directory for the dist ("{{ProjectDir}}/{{OutputDir}}/{{ProductID}}/{{Version}}/{{DistID}}/{{NameTemplateRendered}}")
//	DIST_NAME: the rendered NameTemplate for the distribution
//	DIST_ARTIFACT_COUNT: the number of artifacts generated by the current distribution
//	DIST_ARTIFACT_{#}: for 0 <= # < DIST_ARTIFACT_COUNT, the name of the dist artifact generated by the current distribution
//
// Each dependent product adds the following set of environment variables that start with "DEP_PRODUCT_ID_{#}_", where
// 0 <= # < DEP_PRODUCT_ID_COUNT:
//
// The following environment variables are defined if the build configuration for the product is non-nil:
//
//	DEP_PRODUCT_ID_{#}_BUILD_DIR: the build output directory for the product ("{{OutputDir}}/{{ProductID}}/{{Version}}")
//	DEP_PRODUCT_ID_{#}_BUILD_NAME: the rendered NameTemplate for the build for this product
//	DEP_PRODUCT_ID_{#}_BUILD_OS_ARCH_COUNT: the number of OS/arch combinations for this product
//	DEP_PRODUCT_ID_{#}_BUILD_OS_ARCH_{##}: for 0 <= ## < BUILD_OS_ARCH_COUNT, contains the OS/arch for the build
//
// The following environment variables are defined if the dist configuration for the product is non-nil:
//
//	DEP_PRODUCT_ID_{#}_DIST_ID_COUNT: the number of disters for this product
//	DEP_PRODUCT_ID_{#}_DIST_ID_{##}: for 0 <= ## < DIST_ID_COUNT, the DistID
//	DEP_PRODUCT_ID_{#}_DIST_ID_{##}_DIST_DIR: for 0 <= ## < DIST_ID_COUNT, the distribution output directory for the dist ("{{ProjectDir}}/{{OutputDir}}/{{ProductID}}/{{Version}}/{{DistID}}")
//	DEP_PRODUCT_ID_{#}_DIST_ID_{##}_DIST_WORK_DIR: for 0 <= ## < DIST_ID_COUNT, the distribution work directory for the dist ("{{ProjectDir}}/{{OutputDir}}/{{ProductID}}/{{Version}}/{{DistID}}/{{NameTemplateRendered}}")
//	DEP_PRODUCT_ID_{#}_DIST_ID_{##}_DIST_NAME: for 0 <= ## < DIST_ID_COUNT, the rendered NameTemplate for the distribution
//	DEP_PRODUCT_ID_{#}_DIST_ID_{##}_DIST_ARTIFACT_COUNT: for 0 <= ## < DIST_DISTER_IDS_COUNT, contains the number of artifacts generated by the dister
//	DEP_PRODUCT_ID_{#}_DIST_ID_{##}_DIST_ARTIFACT_{###}: for 0 <= ## < DIST_DISTER_IDS_COUNT and 0 <= ### < DIST_DISTER_IDS_{#}_DIST_ARTIFACTS_COUNT, contains the name of the specified dist artifact
func DistScriptEnvVariables(distID DistID, outputInfo ProductTaskOutputInfo) map[string]string {
	var sortedDepProductIDs []ProductID
	for productID := range outputInfo.Deps {
		sortedDepProductIDs = append(sortedDepProductIDs, productID)
	}
	sort.Sort(ByProductID(sortedDepProductIDs))

	m := map[string]string{
		"PROJECT_DIR":          outputInfo.Project.ProjectDir,
		"VERSION":              outputInfo.Project.Version,
		"PRODUCT":              string(outputInfo.Product.ID),
		"DEP_PRODUCT_ID_COUNT": strconv.Itoa(len(sortedDepProductIDs)),
	}
	for i, currDepProductID := range sortedDepProductIDs {
		m["DEP_PRODUCT_ID_"+strconv.Itoa(i)] = string(currDepProductID)
	}

	// add build environment variables for current product
	addProductBuildEnvVariables(m, "", outputInfo.Project, outputInfo.Product)

	// add dist environment variables manually for current (root) product
	m["DIST_ID"] = string(distID)
	m["DIST_DIR"] = ProductDistOutputDir(outputInfo.Project, outputInfo.Product, distID)
	m["DIST_WORK_DIR"] = ProductDistWorkDirs(outputInfo.Project, outputInfo.Product)[distID]
	m["DIST_NAME"] = outputInfo.Product.DistOutputInfos.DistInfos[distID].DistNameTemplateRendered
	distArtifacts := outputInfo.Product.DistOutputInfos.DistInfos[distID].DistArtifactNames
	m["DIST_ARTIFACT_COUNT"] = strconv.Itoa(len(distArtifacts))
	for i, distArtifact := range distArtifacts {
		m["DIST_ARTIFACT_"+strconv.Itoa(i)] = distArtifact
	}

	// add environment variables for dependent products
	for i, productID := range sortedDepProductIDs {
		prefix := fmt.Sprintf("DEP_PRODUCT_ID_%d_", i)
		addProductBuildEnvVariables(m, prefix, outputInfo.Project, outputInfo.Deps[productID])
		addProductDistEnvVariables(m, prefix, outputInfo.Project, outputInfo.Deps[productID])
	}
	return m
}

// DockerScriptEnvVariables returns a map of environment variables for the script for the Docker builder with the
// specified DockerID in the provided output configuration. The returned map contains the following environment
// variables:
//
//	PROJECT_DIR: the root directory of project
//	VERSION: the version of the project
//	PRODUCT: the name of the product
//	DOCKER_ID: the DockerID for the current distribution
//
// The following environment variables are defined if the Docker configuration for the product is non-nil:
//
//	CONTEXT_DIR: the path to the context directory
func DockerScriptEnvVariables(dockerID DockerID, outputInfo ProductTaskOutputInfo) map[string]string {
	m := map[string]string{
		"PROJECT_DIR": outputInfo.Project.ProjectDir,
		"VERSION":     outputInfo.Project.Version,
		"PRODUCT":     string(outputInfo.Product.ID),
		"DOCKER_ID":   string(dockerID),
	}

	if outputInfo.Product.DockerOutputInfos != nil {
		currInfo := outputInfo.Product.DockerOutputInfos.DockerBuilderOutputInfos[dockerID]
		m["CONTEXT_DIR"] = currInfo.ContextDir
	}
	return m
}

func addProductBuildEnvVariables(varMap map[string]string, prefix string, projectInfo ProjectInfo, productInfo ProductOutputInfo) {
	if productInfo.BuildOutputInfo == nil {
		return
	}
	varMap[prefix+"BUILD_DIR"] = ProductBuildOutputDir(projectInfo, productInfo)
	varMap[prefix+"BUILD_NAME"] = productInfo.BuildOutputInfo.BuildNameTemplateRendered
	varMap[prefix+"BUILD_OS_ARCH_COUNT"] = strconv.Itoa(len(productInfo.BuildOutputInfo.OSArchs))
	for i, osArch := range productInfo.BuildOutputInfo.OSArchs {
		varMap[prefix+"BUILD_OS_ARCH_"+strconv.Itoa(i)] = osArch.String()
	}
}

func addProductDistEnvVariables(varMap map[string]string, prefix string, projectInfo ProjectInfo, productInfo ProductOutputInfo) {
	if productInfo.DistOutputInfos == nil {
		return
	}
	varMap[prefix+"DIST_ID_COUNT"] = strconv.Itoa(len(productInfo.DistOutputInfos.DistIDs))
	for i, distID := range productInfo.DistOutputInfos.DistIDs {
		currDistIDPrefix := prefix + "DIST_ID_" + strconv.Itoa(i)
		varMap[currDistIDPrefix] = string(distID)

		varMap[currDistIDPrefix+"_DIST_DIR"] = ProductDistOutputDir(projectInfo, productInfo, distID)
		varMap[currDistIDPrefix+"_DIST_WORK_DIR"] = ProductDistWorkDirs(projectInfo, productInfo)[distID]
		varMap[currDistIDPrefix+"_DIST_NAME"] = productInfo.DistOutputInfos.DistInfos[distID].DistNameTemplateRendered

		distArtifacts := productInfo.DistOutputInfos.DistInfos[distID].DistArtifactNames
		varMap[currDistIDPrefix+"_DIST_ARTIFACT_COUNT"] = strconv.Itoa(len(distArtifacts))
		for j, distArtifact := range distArtifacts {
			varMap[currDistIDPrefix+"_DIST_ARTIFACT_"+strconv.Itoa(j)] = distArtifact
		}
	}
}
