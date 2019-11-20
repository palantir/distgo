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

package imports

import (
	"fmt"
	"os"
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/tools/go/packages"
)

// GoFiles is a map from package paths to the names of the buildable .go source files (.go files excluding Cgo and test
// files) in the package.
type GoFiles map[string][]string

// NewerThan returns true if the modification time of any of the GoFiles is newer than that of the provided file.
func (g GoFiles) NewerThan(fi os.FileInfo) (bool, error) {
	for _, files := range g {
		for _, goFile := range files {
			currPath := goFile
			currFi, err := os.Stat(currPath)
			if err != nil {
				return false, errors.Wrapf(err, "Failed to stat file %v", currPath)
			}
			if currFi.ModTime().After(fi.ModTime()) {
				return true, nil
			}
		}
	}
	return false, nil
}

// AllFiles returns a map that contains all of the non-standard library Go files that are imported by (and thus are
// required to build) the package at the specified file path (including the package itself) using the specified GOOS and
// GOARCH. If GOOS or GOARCH is empty, the default value for the current environment is used. The keys in the returned
// map are the package or module names and the values are a slice of the paths of the .go source files in the package
// (excluding Cgo and test files).
func AllFiles(pkgDir, goos, goarch string) (GoFiles, error) {
	// package or module name to all non-test Go files in the package
	pkgFiles := make(map[string][]string)

	env := os.Environ()
	if goos != "" {
		env = append(env, fmt.Sprintf("GOOS=%s", goos))
	}
	if goarch != "" {
		env = append(env, fmt.Sprintf("GOARCH=%s", goarch))
	}
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports,
		Dir:  pkgDir,
		Env:  env,
	}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		return nil, err
	}
	pkgsToProcess := pkgs

	for len(pkgsToProcess) > 0 {
		currPkg := pkgsToProcess[0]
		pkgsToProcess = pkgsToProcess[1:]
		if _, ok := pkgFiles[currPkg.PkgPath]; ok {
			continue
		}

		// add all files for the current package to output
		pkgFiles[currPkg.PkgPath] = currPkg.GoFiles

		// convert all non-built-in imports into packages and add to packages to process
		for importPath, importPkg := range currPkg.Imports {
			if !strings.Contains(importPath, ".") {
				// if import is a standard package, skip
				continue
			}
			pkgsToProcess = append(pkgsToProcess, importPkg)
		}
	}
	return pkgFiles, nil
}
