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

package docker

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"text/template"
	"time"

	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/distgo/build"
	"github.com/palantir/distgo/distgo/dist"
	"github.com/palantir/godel/v2/pkg/osarch"
	"github.com/palantir/pkg/signals"
	"github.com/pkg/errors"
)

func BuildProducts(projectInfo distgo.ProjectInfo, projectParam distgo.ProjectParam, configModTime *time.Time, productDockerIDs []distgo.ProductDockerID, tagKeys []string, verbose, dryRun bool, stdout io.Writer) error {
	// determine products that match specified productDockerIDs
	productParams, err := distgo.ProductParamsForDockerProductArgs(projectParam.Products, productDockerIDs...)
	if err != nil {
		return err
	}
	productParams = distgo.ProductParamsForDockerTagKeys(productParams, tagKeys)

	// run build for products that require build artifact generation (not sufficient to just run dist because product
	// may declare build output but not dist output)
	var productParamsToBuild []distgo.ProductParam
	for _, currProduct := range productParams {
		requiresBuildParam, err := build.RequiresBuild(projectInfo, currProduct)
		if err != nil {
			return err
		}
		if requiresBuildParam == nil {
			continue
		}
		productParamsToBuild = append(productParamsToBuild, *requiresBuildParam)
	}
	if len(productParamsToBuild) != 0 {
		if err := build.Run(projectInfo, productParamsToBuild, build.Options{
			Parallel: true,
			DryRun:   dryRun,
		}, stdout); err != nil {
			return err
		}
	}

	// create a ProductBuildID and ProductDistID for all of the products for which a Docker action will be run
	var productDistIDs []distgo.ProductDistID
	for _, currProductParam := range productParams {
		productDistIDs = append(productDistIDs, distgo.ProductDistID(currProductParam.ID))
	}
	// run dist for products that require dist artifact generation
	if err := dist.Products(projectInfo, projectParam, configModTime, productDistIDs, dryRun, true, stdout); err != nil {
		return err
	}

	filteredDockerProductsMap := make(map[distgo.ProductID]distgo.ProductParam)
	// copy old values into new map
	for k, v := range projectParam.Products {
		filteredDockerProductsMap[k] = v
	}
	// copy computed params into map, which may filter dists for products
	for _, v := range productParams {
		filteredDockerProductsMap[v.ID] = v
	}
	// update products for projectParam
	projectParam.Products = filteredDockerProductsMap

	// sort Docker product tasks in topological order
	allProducts, _, _ := distgo.ClassifyProductParams(productParams)
	targetProducts, topoOrderedIDs, err := distgo.TopoSortProductParams(projectParam, allProducts)
	if err != nil {
		return err
	}
	for _, currID := range topoOrderedIDs {
		currProduct := targetProducts[currID]
		if err := RunBuild(projectInfo, currProduct, verbose, dryRun, stdout); err != nil {
			return err
		}
	}
	return nil
}

// RunBuild executes the Docker image build action for the specified product. The Docker outputs for all of the
// dependent products for the provided product must already exist, and the dist outputs for the current product and all
// of its dependent products must also exist in the proper locations.
func RunBuild(projectInfo distgo.ProjectInfo, productParam distgo.ProductParam, verbose, dryRun bool, stdout io.Writer) error {
	if productParam.Docker == nil {
		distgo.PrintlnOrDryRunPrintln(stdout, fmt.Sprintf("%s does not have Docker outputs; skipping build", productParam.ID), dryRun)
		return nil
	}

	var dockerIDs []distgo.DockerID
	for k := range productParam.Docker.DockerBuilderParams {
		dockerIDs = append(dockerIDs, k)
	}
	sort.Sort(distgo.ByDockerID(dockerIDs))

	productTaskOutputInfo, err := distgo.ToProductTaskOutputInfo(projectInfo, productParam)
	if err != nil {
		return err
	}

	allBuildArtifactPaths := productTaskOutputInfo.ProductDockerBuildArtifactPaths()
	allDistArtifactPaths := productTaskOutputInfo.ProductDockerDistArtifactPaths()

	for _, dockerID := range dockerIDs {
		if err := runSingleDockerBuild(
			projectInfo,
			productParam.ID,
			productParam.Name,
			dockerID,
			productParam.Docker.DockerBuilderParams[dockerID],
			productTaskOutputInfo,
			allBuildArtifactPaths[dockerID],
			allDistArtifactPaths[dockerID],
			verbose,
			dryRun,
			stdout,
		); err != nil {
			return err
		}
	}
	return nil
}

func runSingleDockerBuild(
	projectInfo distgo.ProjectInfo,
	productID distgo.ProductID,
	productName string,
	dockerID distgo.DockerID,
	dockerBuilderParam distgo.DockerBuilderParam,
	productTaskOutputInfo distgo.ProductTaskOutputInfo,
	buildArtifactPaths map[distgo.ProductID]map[osarch.OSArch]string,
	distArtifactPaths map[distgo.ProductID]map[distgo.DistID][]string,
	verbose, dryRun bool,
	stdout io.Writer) (rErr error) {

	if !dryRun {
		// link build artifacts into context directory
		for productID, valMap := range buildArtifactPaths {
			currOutputInfo := productTaskOutputInfo.AllProductOutputInfosMap()[productID]
			buildArtifactSrcPaths := distgo.ProductBuildArtifactPaths(projectInfo, currOutputInfo)
			for osArch, buildArtifactDstPath := range valMap {
				if err := os.MkdirAll(path.Dir(buildArtifactDstPath), 0755); err != nil {
					return errors.Wrapf(err, "failed to create directories")
				}
				if err := createNewHardLink(buildArtifactSrcPaths[osArch], buildArtifactDstPath); err != nil {
					return errors.Wrapf(err, "failed to link build artifact into context directory")
				}
			}
		}

		// link dist artifacts into context directory
		for productID, valMap := range distArtifactPaths {
			currOutputInfo := productTaskOutputInfo.AllProductOutputInfosMap()[productID]
			dstArtifactSrcPaths := distgo.ProductDistArtifactPaths(projectInfo, currOutputInfo)
			for distID, distArtifactDstPaths := range valMap {
				for i, currDstArtifactPath := range distArtifactDstPaths {
					if err := os.MkdirAll(path.Dir(currDstArtifactPath), 0755); err != nil {
						return errors.Wrapf(err, "failed to create directories")
					}
					if err := createNewHardLink(dstArtifactSrcPaths[distID][i], currDstArtifactPath); err != nil {
						return errors.Wrapf(err, "failed to link dist artifact into context directory")
					}
				}
			}
		}

		// write and execute Docker script
		if err := distgo.WriteAndExecuteScript(projectInfo, dockerBuilderParam.Script, distgo.DockerScriptEnvVariables(dockerID, productTaskOutputInfo), stdout); err != nil {
			return errors.Wrapf(err, "failed to execute Docker script")
		}

		pathToContextDir := path.Join(projectInfo.ProjectDir, dockerBuilderParam.ContextDir)
		dockerfilePath := path.Join(pathToContextDir, dockerBuilderParam.DockerfilePath)
		originalDockerfileBytes, err := ioutil.ReadFile(dockerfilePath)
		if err != nil {
			return errors.Wrapf(err, "failed to read Dockerfile %s", dockerBuilderParam.DockerfilePath)
		}

		renderedDockerfile := string(originalDockerfileBytes)
		if !dockerBuilderParam.DisableTemplateRendering {
			if renderedDockerfile, err = distgo.RenderTemplate(string(originalDockerfileBytes), nil,
				distgo.ProductTemplateFunction(productName),
				distgo.VersionTemplateFunction(projectInfo.Version),
				distgo.RepositoryTemplateFunction(productTaskOutputInfo.Product.DockerOutputInfos.Repository),
				distgo.RepositoryLiteralTemplateFunction(productTaskOutputInfo.Product.DockerOutputInfos.Repository),
				inputBuildArtifactTemplateFunction(dockerID, pathToContextDir, buildArtifactPaths),
				inputDistArtifactsTemplateFunction(dockerID, pathToContextDir, distArtifactPaths),
				tagTemplateFunction(productTaskOutputInfo),
				tagsTemplateFunction(productTaskOutputInfo),
			); err != nil {
				return err
			}
		}
		if renderedDockerfile != string(originalDockerfileBytes) {
			// Dockerfile contained templates and rendering them changes file: overwrite file with rendered version
			// and restore afterwards
			if err := ioutil.WriteFile(dockerfilePath, []byte(renderedDockerfile), 0644); err != nil {
				return errors.Wrapf(err, "failed to write rendered Dockerfile")
			}

			cleanupCtx, cancel := signals.ContextWithShutdown(context.Background())
			cleanupDone := make(chan struct{})
			defer func() {
				cancel()
				<-cleanupDone
			}()
			go func() {
				select {
				case <-cleanupCtx.Done():
					if err := ioutil.WriteFile(dockerfilePath, originalDockerfileBytes, 0644); err != nil && rErr == nil {
						rErr = errors.Wrapf(err, "failed to restore original Dockerfile content")
					}
				}
				cleanupDone <- struct{}{}
			}()
		}
	}

	distgo.PrintlnOrDryRunPrintln(stdout, fmt.Sprintf("Running Docker build for configuration %s of product %s...", dockerID, productID), dryRun)
	// run the Docker build task
	return dockerBuilderParam.DockerBuilder.RunDockerBuild(dockerID, productTaskOutputInfo, verbose, dryRun, stdout)
}

func inputBuildArtifactTemplateFunction(dockerID distgo.DockerID, pathToContextDir string, buildArtifactPaths map[distgo.ProductID]map[osarch.OSArch]string) distgo.TemplateFunction {
	return func(fnMap template.FuncMap) {
		fnMap["InputBuildArtifact"] = func(productID, osArchStr string) (string, error) {
			osArchMap, ok := buildArtifactPaths[distgo.ProductID(productID)]
			if !ok {
				return "", errors.Errorf("product %s is not a build input for Docker task %s", productID, dockerID)
			}
			osArch, err := osarch.New(osArchStr)
			if err != nil {
				return "", errors.Wrapf(err, "input %s is not a valid OS/Arch", osArchStr)
			}
			dst, ok := osArchMap[osArch]
			if !ok {
				return "", errors.Errorf("OS/Arch %s for product %s is not defined as a build input for Docker task %s", osArchStr, productID, dockerID)
			}
			pathFromContextDir, err := filepath.Rel(pathToContextDir, dst)
			if err != nil {
				return "", errors.Wrapf(err, "failed to determine path")
			}
			return pathFromContextDir, nil
		}
	}
}

func inputDistArtifactsTemplateFunction(dockerID distgo.DockerID, pathToContextDir string, distArtifactPaths map[distgo.ProductID]map[distgo.DistID][]string) distgo.TemplateFunction {
	return func(fnMap template.FuncMap) {
		fnMap["InputDistArtifacts"] = func(productID, distID string) ([]string, error) {
			distIDsMap, ok := distArtifactPaths[distgo.ProductID(productID)]
			if !ok {
				return nil, errors.Errorf("product %s is not a dist input for Docker task %s", productID, dockerID)
			}
			dstArtifactPaths, ok := distIDsMap[distgo.DistID(distID)]
			if !ok {
				return nil, errors.Errorf("dist %s is not defined as a dist input for Docker task %s", distID, dockerID)
			}

			var outPaths []string
			for _, artifactPath := range dstArtifactPaths {
				pathFromContextDir, err := filepath.Rel(pathToContextDir, artifactPath)
				if err != nil {
					return nil, errors.Wrapf(err, "failed to determine path")
				}
				outPaths = append(outPaths, pathFromContextDir)
			}
			return outPaths, nil
		}
	}
}

func tagTemplateFunction(productTaskOutputInfo distgo.ProductTaskOutputInfo) distgo.TemplateFunction {
	primaryProductID := productTaskOutputInfo.Product.ID
	allOutputInfos := productTaskOutputInfo.AllProductOutputInfosMap()
	return func(fnMap template.FuncMap) {
		fnMap["Tag"] = func(productID, dockerID, tagKey string) (string, error) {
			dockerBuilderOutput, err := getDockerBuilderOutputInfo(primaryProductID, allOutputInfos, productID, dockerID)
			if err != nil {
				return "", err
			}
			return dockerBuilderOutput.RenderedTagsMap[distgo.DockerTagID(tagKey)], nil
		}
	}
}

func tagsTemplateFunction(productTaskOutputInfo distgo.ProductTaskOutputInfo) distgo.TemplateFunction {
	primaryProductID := productTaskOutputInfo.Product.ID
	allOutputInfos := productTaskOutputInfo.AllProductOutputInfosMap()
	return func(fnMap template.FuncMap) {
		fnMap["Tags"] = func(productID, dockerID string) ([]string, error) {
			dockerBuilderOutput, err := getDockerBuilderOutputInfo(primaryProductID, allOutputInfos, productID, dockerID)
			if err != nil {
				return nil, err
			}
			return dockerBuilderOutput.RenderedTags, nil
		}
	}
}

func getDockerBuilderOutputInfo(primaryProductID distgo.ProductID, allOutputInfos map[distgo.ProductID]distgo.ProductOutputInfo, productID, dockerID string) (distgo.DockerBuilderOutputInfo, error) {
	productOutputInfo, ok := allOutputInfos[distgo.ProductID(productID)]
	if !ok {
		return distgo.DockerBuilderOutputInfo{}, errors.Errorf("product %s is not the product or a dependent product of %s", productID, primaryProductID)
	}
	if productOutputInfo.DockerOutputInfos == nil {
		return distgo.DockerBuilderOutputInfo{}, errors.Errorf("product %s does not declare Docker outputs", productID)
	}
	dockerBuilderOutput, ok := productOutputInfo.DockerOutputInfos.DockerBuilderOutputInfos[distgo.DockerID(dockerID)]
	if !ok {
		return distgo.DockerBuilderOutputInfo{}, errors.Errorf("product %s does not contain an entry for DockerID %s", productID, dockerID)
	}
	return dockerBuilderOutput, nil
}

func createNewHardLink(src, dst string) error {
	if _, err := os.Stat(dst); err == nil {
		// ensure the target does not exists before creating a new one
		if err := os.Remove(dst); err != nil {
			return errors.Wrapf(err, "failed to remove existing file")
		}
	}
	if err := os.Link(src, dst); err != nil {
		return errors.Wrapf(err, "failed to create hard link %s from %s", dst, src)
	}
	return nil
}
