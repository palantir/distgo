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

package dist

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/distgo/build"
	"github.com/palantir/pkg/matcher"
	"github.com/pkg/errors"
	"github.com/termie/go-shutil"
)

func Products(projectInfo distgo.ProjectInfo, projectParam distgo.ProjectParam, configModTime *time.Time, productDistIDs []distgo.ProductDistID, dryRun, parallel bool, stdout io.Writer) error {
	// pre-filter step: expand productDistIDs to include all dependent products
	var allDepProductDistIDs []distgo.ProductDistID
	for _, currDistID := range productDistIDs {
		productID, _ := currDistID.Parse()
		productParam := projectParam.Products[productID]
		for _, depProductID := range productParam.AllDependenciesSortedIDs() {
			allDepProductDistIDs = append(allDepProductDistIDs, distgo.ProductDistID(depProductID))
		}
	}
	productDistIDs = append(productDistIDs, allDepProductDistIDs...)

	productParams, err := distgo.ProductParamsForDistProductArgs(projectParam.Products, productDistIDs...)
	if err != nil {
		return err
	}

	filteredDistProductsMap := make(map[distgo.ProductID]distgo.ProductParam)
	// copy old values into new map
	for k, v := range projectParam.Products {
		filteredDistProductsMap[k] = v
	}
	// copy computed params into map, which may filter dists for products
	for _, v := range productParams {
		filteredDistProductsMap[v.ID] = v
	}
	// update products for projectParam
	projectParam.Products = filteredDistProductsMap

	allProducts, _, dependentProducts := distgo.ClassifyProductParams(productParams)
	var productParamsToBuild []distgo.ProductParam
	for _, currProductID := range sortedMapKeys(allProducts) {
		currProduct := projectParam.Products[currProductID]
		if _, ok := dependentProducts[currProductID]; !ok && currProduct.Dist == nil {
			// current product is not a dependency of any specified product and doesn't declare a dist output. In this
			// case, no need to build the build outputs because they will not be used.
			continue
		}
		requiresBuildParam, err := build.RequiresBuild(projectInfo, projectParam.Products[currProductID])
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
			Parallel: parallel,
			DryRun:   dryRun,
		}, stdout); err != nil {
			return err
		}
		// if any of the products needed to be re-built, require dist to be performed
		configModTime = nil
	}

	// sort dist product tasks in topological order
	targetProducts, topoOrderedIDs, err := distgo.TopoSortProductParams(projectParam, allProducts)
	if err != nil {
		return err
	}
	for _, currProductID := range topoOrderedIDs {
		requiresDistParam, err := RequiresDist(projectInfo, targetProducts[currProductID], configModTime)
		if err != nil {
			return err
		}
		if requiresDistParam == nil {
			continue
		}
		if err := Run(projectInfo, *requiresDistParam, dryRun, stdout); err != nil {
			return errors.Wrapf(err, "dist failed for %s", currProductID)
		}
	}
	return nil
}

// Run executes the Dist action for the specified product. Produces both the dist output directory and the dist
// artifacts for all of the disters for the product. The outputs for the dependent products for the provided product
// must already exist in the proper locations.
func Run(projectInfo distgo.ProjectInfo, productParam distgo.ProductParam, dryRun bool, stdout io.Writer) error {
	if productParam.Dist == nil {
		distgo.PrintlnOrDryRunPrintln(stdout, fmt.Sprintf("%s does not define a dist configuration; skipping dist", productParam.ID), dryRun)
		return nil
	}

	productOutputInfo, err := productParam.ToProductOutputInfo(projectInfo.Version)
	if err != nil {
		return err
	}

	productTaskOutputInfo, err := distgo.ToProductTaskOutputInfo(projectInfo, productParam)
	if err != nil {
		return err
	}
	distWorkDirs := distgo.ProductDistWorkDirs(projectInfo, productOutputInfo)

	for _, currDistID := range productTaskOutputInfo.Product.DistOutputInfos.DistIDs {
		// create empty output directory
		distWorkDir := distWorkDirs[currDistID]
		if !dryRun {
			// remove output directory if it already exists
			if err := os.RemoveAll(distWorkDir); err != nil {
				return errors.Wrapf(err, "failed to remove dist output directory %s", distWorkDir)
			}
			// create output directory
			if err := os.MkdirAll(distWorkDir, 0755); err != nil {
				return errors.Wrapf(err, "failed to create dist output directory %s", distWorkDir)
			}
		}

		distgo.PrintlnOrDryRunPrintln(stdout, fmt.Sprintf("Creating distribution for %s at %v", productParam.ID, strings.Join(outputArtifactDisplayPaths(distgo.ProductDistArtifactPaths(projectInfo, productOutputInfo)[currDistID]), ", ")), dryRun)
		if !dryRun {
			currDistParam := productParam.Dist.DistParams[currDistID]

			// copy input dir contents
			if currDistParam.InputDir.Path != "" {
				if err := copyInputDir(path.Join(projectInfo.ProjectDir, currDistParam.InputDir.Path), currDistParam.InputDir.Exclude, distWorkDir); err != nil {
					return errors.Wrapf(err, "failed to copy input directory")
				}
			}

			// run dist task
			runDistOutput, err := currDistParam.Dister.RunDist(currDistID, productTaskOutputInfo)
			if err != nil {
				return err
			}
			// execute dist script
			if err := distgo.WriteAndExecuteScript(projectInfo, currDistParam.Script, distgo.DistScriptEnvVariables(currDistID, productTaskOutputInfo), stdout); err != nil {
				return errors.Wrapf(err, "failed to execute dist script")
			}
			// generate dist artifacts
			if err := currDistParam.Dister.GenerateDistArtifacts(currDistID, productTaskOutputInfo, runDistOutput); err != nil {
				return err
			}
		}
		distgo.PrintlnOrDryRunPrintln(stdout, fmt.Sprintf("Finished creating %s distribution for %s", currDistID, productParam.ID), dryRun)
	}
	return nil
}

func copyInputDir(inputDir string, exclude matcher.Matcher, dstDir string) error {
	inputDirFiles, err := ioutil.ReadDir(inputDir)
	if err != nil {
		return errors.Wrapf(err, "failed to list files in input directory %s", inputDir)
	}

	if _, err := os.Stat(dstDir); err != nil {
		return errors.Wrapf(err, "failed to stat destination directory %s", dstDir)
	}

	for _, topLevelInputFile := range inputDirFiles {
		topLevelFileName := topLevelInputFile.Name()
		srcPath := path.Join(inputDir, topLevelFileName)
		dstPath := path.Join(dstDir, topLevelFileName)

		// copy file
		if !topLevelInputFile.IsDir() {
			if exclude != nil && exclude.Match(topLevelFileName) {
				continue
			}
			if _, err := shutil.Copy(srcPath, dstPath, false); err != nil {
				return errors.Wrapf(err, "failed to copy file %s", topLevelFileName)
			}
			continue
		}

		// copy directory recursively
		if err := shutil.CopyTree(srcPath, dstPath, &shutil.CopyTreeOptions{
			CopyFunction: shutil.Copy,
			Ignore: func(dir string, files []os.FileInfo) []string {
				if exclude == nil {
					return nil
				}
				var ignoredNames []string
				for _, f := range files {
					// exclude matcher matches against relative paths: make the current path relative to the input directory
					relPathFromRoot, err := filepath.Rel(inputDir, path.Join(dir, f.Name()))
					if err != nil {
						relPathFromRoot = path.Join(dir, f.Name())
					}
					if exclude.Match(relPathFromRoot) {
						ignoredNames = append(ignoredNames, f.Name())
					}
				}
				return ignoredNames
			},
		}); err != nil {
			return errors.Wrapf(err, "failed to copy directory %s", topLevelFileName)
		}
	}
	return nil
}

func outputArtifactDisplayPaths(in []string) []string {
	if in == nil {
		return nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return in
	}
	out := make([]string, len(in))
	for i, outputArtifactPath := range in {
		outputArtifactDisplayPath := outputArtifactPath
		if relPath, err := filepath.Rel(wd, outputArtifactPath); err == nil {
			outputArtifactDisplayPath = relPath
		}
		out[i] = outputArtifactDisplayPath
	}
	return out
}

func sortedMapKeys(m map[distgo.ProductID]struct{}) []distgo.ProductID {
	var out []distgo.ProductID
	for k := range m {
		out = append(out, k)
	}
	sort.Sort(distgo.ByProductID(out))
	return out
}
