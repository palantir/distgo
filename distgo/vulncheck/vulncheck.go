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
	"encoding/json"
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
		pkgs := scanPkgsForProduct(currProductParam)
		if len(pkgs) == 0 {
			continue
		}
		currProductTaskOutputInfo, err := distgo.ToProductTaskOutputInfo(projectInfo, currProductParam)
		if err != nil {
			return errors.Wrapf(err, "failed to compute output information for %s", currProductParam.ID)
		}
		scanDir := scanDirForProduct(projectInfo, currProductParam)
		scanEnv := scanEnvForProduct(currProductParam)
		if err := executeVulncheck(currProductTaskOutputInfo, pkgs, scanDir, scanEnv, opts, stdout); err != nil {
			return errors.Wrapf(err, "vulncheck failed for %s", currProductParam.ID)
		}
	}
	return nil
}

func executeVulncheck(outputInfo distgo.ProductTaskOutputInfo, pkgs []string, scanDir string, env []string, opts Options, stdout io.Writer) error {
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
		distgo.DryRunPrintln(stdout, fmt.Sprintf("Run: govulncheck -format openvex %s (in %s)", strings.Join(pkgs, " "), scanDir))
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

	vexOutput, err := runGovulncheck(pkgs, env)
	if err != nil {
		return err
	}

	// govulncheck sets the product @id to "Unknown Product" which prevents
	// Trivy from matching VEX statements against scan results. Trivy's
	// go-vex matching requires the product @id to be a PURL that matches
	// the scanned component. Promote subcomponents to products so that the
	// module PURLs (e.g. pkg:golang/stdlib@v1.24.0) are directly matchable.
	vexOutput, err = rewriteVEXProducts(vexOutput)
	if err != nil {
		return errors.Wrap(err, "failed to rewrite VEX products for Trivy compatibility")
	}

	if err := os.WriteFile(vexPath, vexOutput, 0644); err != nil {
		return errors.Wrapf(err, "failed to write VEX file %s", vexPath)
	}

	elapsed := time.Since(start)
	distgo.PrintlnOrDryRunPrintln(stdout, fmt.Sprintf("Finished vulncheck for %s (%.3fs)", productName, elapsed.Seconds()), false)
	return nil
}

// scanPkgsForProduct returns the package patterns to scan for the given product.
// Uses Vulncheck.Pkgs if configured, otherwise falls back to Build.MainPkg.
// Returns nil if neither is available. Each returned value is normalized to
// ensure relative filesystem paths have a "./" prefix, which Go tooling
// requires to distinguish them from import paths.
func scanPkgsForProduct(p distgo.ProductParam) []string {
	if p.Vulncheck != nil && len(p.Vulncheck.Pkgs) > 0 {
		pkgs := make([]string, len(p.Vulncheck.Pkgs))
		for i, pkg := range p.Vulncheck.Pkgs {
			pkgs[i] = ensureLocalPkgPrefix(pkg)
		}
		return pkgs
	}
	if p.Build != nil && p.Build.MainPkg != "" {
		return []string{p.Build.MainPkg}
	}
	return nil
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
//
// To override the Go stdlib version used for vulnerability analysis (which
// defaults to the host toolchain version), set GOVERSION in the env list.
// This is useful when the scan directory contains source compiled with a
// different Go version than the host, e.g.:
//
//	env:
//	  - GOVERSION=go1.24.0
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

// PrintProducts runs govulncheck in text mode for the specified products and
// writes the human-readable output to stdout.
func PrintProducts(projectInfo distgo.ProjectInfo, projectParam distgo.ProjectParam, productIDs []distgo.ProductID, opts Options, stdout io.Writer) error {
	productParams, err := distgo.ProductParamsForProductArgs(projectParam.Products, productIDs...)
	if err != nil {
		return err
	}

	for _, currProductParam := range productParams {
		pkgs := scanPkgsForProduct(currProductParam)
		if len(pkgs) == 0 {
			continue
		}
		scanDir := scanDirForProduct(projectInfo, currProductParam)
		scanEnv := scanEnvForProduct(currProductParam)
		if err := executePrintVulncheck(currProductParam.ID, pkgs, scanDir, scanEnv, opts, stdout); err != nil {
			return errors.Wrapf(err, "vulncheck failed for %s", currProductParam.ID)
		}
	}
	return nil
}

func executePrintVulncheck(productID distgo.ProductID, pkgs []string, scanDir string, env []string, opts Options, stdout io.Writer) error {
	distgo.PrintlnOrDryRunPrintln(stdout, fmt.Sprintf("Running vulncheck for %s", productID), opts.DryRun)

	if opts.DryRun {
		distgo.DryRunPrintln(stdout, fmt.Sprintf("Run: govulncheck %s (in %s)", strings.Join(pkgs, " "), scanDir))
		return nil
	}

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

	return runGovulncheckText(pkgs, env, stdout)
}

func runGovulncheckText(pkgs []string, env []string, stdout io.Writer) error {
	ctx := context.Background()
	args := append([]string{}, pkgs...)
	cmd := scan.Command(ctx, args...)

	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}

	cmd.Stdout = stdout
	cmd.Stderr = stdout

	if err := cmd.Start(); err != nil {
		return errors.Wrapf(err, "failed to start govulncheck")
	}

	err := cmd.Wait()
	if err != nil {
		if exitCoder, ok := err.(interface{ ExitCode() int }); ok && exitCoder.ExitCode() == 3 {
			return nil
		}
		return errors.Wrapf(err, "govulncheck failed")
	}
	return nil
}

func runGovulncheck(pkgs []string, env []string) ([]byte, error) {
	ctx := context.Background()
	args := []string{"-format", "openvex"}
	args = append(args, pkgs...)
	cmd := scan.Command(ctx, args...)

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

// vexDocument and related types mirror the OpenVEX JSON structure produced by
// govulncheck, used only for post-processing the product identifiers.
type vexDocument struct {
	Context    string         `json:"@context,omitempty"`
	ID         string         `json:"@id,omitempty"`
	Author     string         `json:"author,omitempty"`
	Timestamp  string         `json:"timestamp,omitempty"`
	Version    int            `json:"version,omitempty"`
	Tooling    string         `json:"tooling,omitempty"`
	Statements []vexStatement `json:"statements,omitempty"`
}

type vexStatement struct {
	Vulnerability   json.RawMessage `json:"vulnerability,omitempty"`
	Products        []vexProduct    `json:"products,omitempty"`
	Status          string          `json:"status,omitempty"`
	Justification   string          `json:"justification,omitempty"`
	ImpactStatement string          `json:"impact_statement,omitempty"`
}

type vexProduct struct {
	ID            string         `json:"@id,omitempty"`
	Subcomponents []vexComponent `json:"subcomponents,omitempty"`
}

type vexComponent struct {
	ID string `json:"@id,omitempty"`
}

// rewriteVEXProducts transforms govulncheck's VEX output to be compatible
// with Trivy's OpenVEX matching logic. govulncheck sets the product @id to
// the literal string "Unknown Product" with module PURLs as subcomponents.
// Trivy's go-vex library requires the product @id to match the scanned
// component's PURL — it checks product matching before subcomponent matching,
// so the "Unknown Product" string causes all statements to be silently
// ignored.
//
// This function promotes each subcomponent to a top-level product so that
// Trivy can match the module PURL (e.g. "pkg:golang/stdlib@v1.24.0")
// directly against its scan results during dependency tree traversal.
func rewriteVEXProducts(data []byte) ([]byte, error) {
	var doc vexDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return data, err
	}

	changed := false
	for i := range doc.Statements {
		stmt := &doc.Statements[i]
		var newProducts []vexProduct
		for _, p := range stmt.Products {
			if p.ID == "Unknown Product" && len(p.Subcomponents) > 0 {
				for _, sc := range p.Subcomponents {
					newProducts = append(newProducts, vexProduct{ID: sc.ID})
				}
				changed = true
			} else {
				newProducts = append(newProducts, p)
			}
		}
		stmt.Products = newProducts
	}

	if !changed {
		return data, nil
	}

	return json.MarshalIndent(doc, "", "  ")
}
