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

package build

import (
	"os"
	"sync"

	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/distgo/build/imports"
	"github.com/palantir/godel/v2/pkg/osarch"
	"github.com/pkg/errors"
)

// RequiresBuild returns a pointer to a distgo.ProductParam that contains only the OS/arch parameters for the outputs
// that require building. A product is considered to require building if its output executable does not exist or if the
// output executable's modification date is older than any of the files (Go source, embedded, or other non-Go source)
// required to build the product. Returns nil if all of the outputs exist and are up-to-date.
func RequiresBuild(projectInfo distgo.ProjectInfo, productParam distgo.ProductParam) (*distgo.ProductParam, error) {
	if productParam.Build == nil {
		return nil, nil
	}

	// create a copy of the build parameter so that it is safe for modification
	buildCopy := *productParam.Build
	productParam.Build = &buildCopy

	productTaskOutputInfo, err := distgo.ToProductTaskOutputInfo(projectInfo, productParam)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to compute output information for %s", productParam.ID)
	}

	pathsMap := productTaskOutputInfo.ProductBuildArtifactPaths()
	buildArgs := productBuildArgs(productTaskOutputInfo, *productParam.Build)
	var requiresBuildOSArchs []osarch.OSArch
	for _, currOSArch := range productParam.Build.OSArchs {
		if checkBuildOutputCurrent(productTaskOutputInfo, *productParam.Build, buildArgs, currOSArch, pathsMap[currOSArch], false) == nil {
			// if the build artifact for the product already exists and none of the input files for the
			// product are newer than the build artifact, consider spec up-to-date
			continue
		}
		requiresBuildOSArchs = append(requiresBuildOSArchs, currOSArch)
	}

	if len(requiresBuildOSArchs) == 0 {
		return nil, nil
	}
	productParam.Build.OSArchs = requiresBuildOSArchs
	return &productParam, nil
}

// RequiredTasks reports missing, stale, or unverifiable build outputs without executing build actions.
func RequiredTasks(projectInfo distgo.ProjectInfo, products []distgo.ProductParam) ([]distgo.RequiredTask, error) {
	var required []distgo.RequiredTask
	for _, product := range products {
		if product.Build == nil {
			continue
		}
		info, err := distgo.ToProductTaskOutputInfo(projectInfo, product)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to compute output information for %s", product.ID)
		}
		paths := info.ProductBuildArtifactPaths()
		buildArgs := productBuildArgs(info, *product.Build)
		for _, target := range product.Build.OSArchs {
			output := paths[target]
			if err := checkBuildOutputCurrent(info, *product.Build, buildArgs, target, output, true); err != nil {
				required = append(required, distgo.RequiredTask{
					Task: "build", Product: product.ID, Target: target.String(),
					Outputs: []string{output}, Reason: err.Error(),
				})
			}
		}
	}
	return required, nil
}

// checkBuildOutputCurrent returns nil if the output is current, or an error describing why it must be rebuilt.
// Packages are loaded as go build loads them: from the project directory with the product's environment and build
// arguments. Inspection failures require rebuilding, preserving RequiresBuild's existing conservative behavior. Checked
// mode, used by --no-dependent-tasks, also rejects package load errors and directory outputs.
func checkBuildOutputCurrent(info distgo.ProductTaskOutputInfo, buildParam distgo.BuildParam, buildArgs func() ([]string, error), target osarch.OSArch, output string, checked bool) error {
	fi, err := os.Stat(output)
	if err != nil {
		return errors.Wrap(err, "cannot inspect output")
	}
	if checked && fi.IsDir() {
		// A directory cannot serve as an executable, even if its timestamp appears current.
		return errors.New("output is a directory")
	}
	args, err := buildArgs()
	if err != nil {
		return errors.Wrap(err, "cannot compute build arguments")
	}
	files, err := imports.AllFilesWithOptions(imports.LoadOptions{
		Dir:        info.Project.ProjectDir,
		Pattern:    buildParam.MainPkg,
		GOOS:       target.OS,
		GOARCH:     target.Arch,
		Env:        productBuildEnv(buildParam, target),
		BuildFlags: args,
		Strict:     checked,
	})
	if err != nil {
		return errors.Wrap(err, "cannot inspect build inputs")
	}
	newer, err := files.NewerThan(fi)
	if err != nil {
		return errors.Wrap(err, "cannot compare input timestamps")
	}
	if newer {
		return errors.New("output is older than its build inputs")
	}
	return nil
}

// productBuildArgs returns a function that computes the product's build arguments at most once. The build args script
// does not depend on the target, so it need not run for each OS/arch.
func productBuildArgs(info distgo.ProductTaskOutputInfo, buildParam distgo.BuildParam) func() ([]string, error) {
	return sync.OnceValues(func() ([]string, error) {
		return buildParam.BuildArgs(info)
	})
}
