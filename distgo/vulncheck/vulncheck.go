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

package vulncheck

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/palantir/distgo/distgo"
	"github.com/pkg/errors"
	"golang.org/x/vuln/scan"
)

type Options struct {
	DryRun bool
}

func Products(projectInfo distgo.ProjectInfo, projectParam distgo.ProjectParam, productIDs []distgo.ProductID, opts Options, stdout io.Writer) error {
	productParams, err := distgo.ProductParamsForProductArgs(projectParam.Products, productIDs...)
	if err != nil {
		return err
	}

	for _, currProductParam := range productParams {
		scanPkg := scanPkgForProduct(currProductParam)
		if scanPkg == "" {
			continue
		}
		currProductTaskOutputInfo, err := distgo.ToProductTaskOutputInfo(projectInfo, currProductParam)
		if err != nil {
			return errors.Wrapf(err, "failed to compute output information for %s", currProductParam.ID)
		}
		scanDir := scanDirForProduct(projectInfo, currProductParam)
		scanEnv := scanEnvForProduct(currProductParam)
		if err := executeVulncheck(currProductTaskOutputInfo, scanPkg, scanDir, scanEnv, opts, stdout); err != nil {
			return errors.Wrapf(err, "vulncheck failed for %s", currProductParam.ID)
		}
	}
	return nil
}

func executeVulncheck(outputInfo distgo.ProductTaskOutputInfo, mainPkg string, scanDir string, env []string, opts Options, stdout io.Writer) error {
	productName := outputInfo.Product.ID
	vexPath := outputInfo.ProductVulncheckVEXPath()

	vexDisplayPath := vexPath
	if wd, err := os.Getwd(); err == nil {
		if relPath, err := filepath.Rel(wd, vexPath); err == nil {
			vexDisplayPath = relPath
		}
	}

	distgo.PrintlnOrDryRunPrintln(stdout, fmt.Sprintf("Running vulncheck for %s, output: %s", productName, vexDisplayPath), opts.DryRun)

	if opts.DryRun {
		distgo.DryRunPrintln(stdout, fmt.Sprintf("Run: govulncheck -format openvex %s (in %s)", mainPkg, scanDir))
		return nil
	}

	outputDir := outputInfo.ProductVulncheckOutputDir()
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return errors.Wrapf(err, "failed to create vulncheck output directory %s", outputDir)
	}

	// scan.Command resolves package patterns relative to the process working
	// directory, so chdir to the scan directory (which contains the go.mod).
	origDir, err := os.Getwd()
	if err != nil {
		return errors.Wrapf(err, "failed to get working directory")
	}
	if err := os.Chdir(scanDir); err != nil {
		return errors.Wrapf(err, "failed to change to scan directory %s", scanDir)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	start := time.Now()

	vexOutput, err := runGovulncheck(mainPkg, env)
	if err != nil {
		return err
	}

	if err := os.WriteFile(vexPath, vexOutput, 0644); err != nil {
		return errors.Wrapf(err, "failed to write VEX file %s", vexPath)
	}

	elapsed := time.Since(start)
	distgo.PrintlnOrDryRunPrintln(stdout, fmt.Sprintf("Finished vulncheck for %s (%.3fs)", productName, elapsed.Seconds()), false)
	return nil
}

// scanPkgForProduct returns the package pattern to scan for the given product.
// Uses Vulncheck.Pkg if configured, otherwise falls back to Build.MainPkg.
// Returns "" if neither is available. The returned value is normalized to
// ensure relative filesystem paths have a "./" prefix, which Go tooling
// requires to distinguish them from import paths.
func scanPkgForProduct(p distgo.ProductParam) string {
	var pkg string
	if p.Vulncheck != nil && p.Vulncheck.Pkg != "" {
		pkg = p.Vulncheck.Pkg
	} else if p.Build != nil {
		pkg = p.Build.MainPkg
	}
	return ensureLocalPkgPrefix(pkg)
}

// scanDirForProduct returns the directory in which to run govulncheck. If
// Vulncheck.Dir is configured, it is resolved relative to the project root.
// Otherwise the project root itself is used.
func scanDirForProduct(projectInfo distgo.ProjectInfo, p distgo.ProductParam) string {
	if p.Vulncheck != nil && p.Vulncheck.Dir != "" {
		return filepath.Join(projectInfo.ProjectDir, p.Vulncheck.Dir)
	}
	return projectInfo.ProjectDir
}

// scanEnvForProduct returns environment variables to set when running
// govulncheck. It derives GOOS and GOARCH from the first entry in the
// product's build os-archs configuration (so that packages with
// platform-specific build constraints load correctly), then appends any
// explicit env overrides from the vulncheck config. Explicit values take
// precedence because scan.Cmd uses the last value for each key.
func scanEnvForProduct(p distgo.ProductParam) []string {
	var env []string
	if p.Build != nil && len(p.Build.OSArchs) > 0 {
		first := p.Build.OSArchs[0]
		env = append(env, "GOOS="+first.OS, "GOARCH="+first.Arch)
	}
	if p.Vulncheck != nil && len(p.Vulncheck.Env) > 0 {
		env = append(env, p.Vulncheck.Env...)
	}
	return env
}

// ensureLocalPkgPrefix adds a "./" prefix to package patterns that look like
// relative filesystem paths but are missing it. Go tooling treats bare paths
// (e.g. "out/build/sourcecode/operator") as import paths rather than local
// directories, which causes resolution failures.
func ensureLocalPkgPrefix(pkg string) string {
	if pkg == "" {
		return ""
	}
	// Already has a local prefix or is a wildcard pattern with one.
	if strings.HasPrefix(pkg, "./") || strings.HasPrefix(pkg, "../") {
		return pkg
	}
	// Looks like a Go import path (contains a dot in the first path element,
	// e.g. "github.com/...") — leave it alone.
	firstSlash := strings.Index(pkg, "/")
	firstElem := pkg
	if firstSlash != -1 {
		firstElem = pkg[:firstSlash]
	}
	if strings.Contains(firstElem, ".") {
		return pkg
	}
	return "./" + pkg
}

func runGovulncheck(mainPkg string, env []string) ([]byte, error) {
	ctx := context.Background()
	cmd := scan.Command(ctx, "-format", "openvex", mainPkg)

	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return nil, errors.Wrapf(err, "failed to start govulncheck")
	}

	err := cmd.Wait()
	if err != nil {
		// exit code 3 means vulnerabilities were found, which is not an error
		if exitCoder, ok := err.(interface{ ExitCode() int }); ok && exitCoder.ExitCode() == 3 {
			return stdoutBuf.Bytes(), nil
		}
		return nil, errors.Wrapf(err, "govulncheck failed: %s", stderrBuf.String())
	}

	return stdoutBuf.Bytes(), nil
}
