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

	// save and restore working directory since scan.Command resolves package
	// patterns relative to the process working directory
	origDir, err := os.Getwd()
	if err != nil {
		return errors.Wrapf(err, "failed to get working directory")
	}
	if err := os.Chdir(projectInfo.ProjectDir); err != nil {
		return errors.Wrapf(err, "failed to change to project directory %s", projectInfo.ProjectDir)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	for _, currProductParam := range productParams {
		if currProductParam.Build == nil {
			continue
		}
		currProductTaskOutputInfo, err := distgo.ToProductTaskOutputInfo(projectInfo, currProductParam)
		if err != nil {
			return errors.Wrapf(err, "failed to compute output information for %s", currProductParam.ID)
		}
		if err := executeVulncheck(currProductTaskOutputInfo, currProductParam.Build.MainPkg, opts, stdout); err != nil {
			return errors.Wrapf(err, "vulncheck failed for %s", currProductParam.ID)
		}
	}
	return nil
}

func executeVulncheck(outputInfo distgo.ProductTaskOutputInfo, mainPkg string, opts Options, stdout io.Writer) error {
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
		distgo.DryRunPrintln(stdout, fmt.Sprintf("Run: govulncheck -format openvex %s", mainPkg))
		return nil
	}

	outputDir := outputInfo.ProductVulncheckOutputDir()
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return errors.Wrapf(err, "failed to create vulncheck output directory %s", outputDir)
	}

	start := time.Now()

	vexOutput, err := runGovulncheck(mainPkg)
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

func runGovulncheck(mainPkg string) ([]byte, error) {
	ctx := context.Background()
	cmd := scan.Command(ctx, "-format", "openvex", mainPkg)

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
