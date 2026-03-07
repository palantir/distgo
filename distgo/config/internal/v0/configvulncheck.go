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

package v0

type VulncheckConfig struct {
	// Pkgs is the list of package patterns to scan with govulncheck.
	// Multiple patterns are scanned in a single invocation, producing one VEX document
	// covering all binaries. If not specified, the product's build main-pkg is used.
	Pkgs []string `yaml:"pkgs,omitempty"`
	// Dir is the working directory in which to run govulncheck, relative to the project root.
	// Use this when the Go module is in a subdirectory (e.g. "out/build/sourcecode").
	// If not specified, the project root is used.
	Dir *string `yaml:"dir,omitempty"`
	// Env is a list of environment variables to set when running govulncheck, in "KEY=VALUE" format.
	// For example, ["GOOS=linux", "GOARCH=amd64"] to scan Linux packages on a macOS host.
	//
	// Set GOVERSION to override the Go stdlib version used for vulnerability analysis.
	// By default, govulncheck uses the host toolchain version. When the scanned source
	// is compiled with a different Go version (e.g. inside a Docker builder image),
	// set GOVERSION to match that version for accurate stdlib vulnerability reporting:
	//   env: ["GOVERSION=go1.24.0"]
	Env []string `yaml:"env,omitempty"`
}
