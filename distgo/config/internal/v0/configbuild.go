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

import (
	"github.com/palantir/godel/v2/pkg/osarch"
)

type BuildConfig struct {
	// NameTemplate is the template used for the executable output. The following template parameters can be used in the
	// template:
	//   * {{Product}}: the name of the product
	//   * {{Version}}: the version of the project
	//
	// If a value is not specified, "{{Product}}" is used as the default value.
	NameTemplate *string `yaml:"name-template,omitempty"`

	// OutputDir specifies the default build output directory for products executables built by the "build" task. The
	// executables generated by "build" are written to "{{OutputDir}}/{{ID}}/{{Version}}/{{OSArch}}/{{NameTemplate}}".
	//
	// If not specified, "out/build" is used as the default value.
	OutputDir *string `yaml:"output-dir,omitempty"`

	// MainPkg is the location of the main package for the product relative to the project root directory. For example,
	// "./distgo/main".
	MainPkg *string `yaml:"main-pkg,omitempty"`

	// BuildArgsScript is the content of a script that is written to a file and run before this product is built
	// to provide supplemental build arguments for the product. The content of this value is written to a file and
	// executed. The script process uses the project directory as its working directory and inherits the environment
	// variables of the Go process. Each line of the output of the script is provided to the "build" command as a
	// separate argument. For example, the following script would add the arguments "-ldflags" "-X" "main.year=$YEAR" to
	// the build command:
	//
	//   #!/usr/bin/env bash
	//   YEAR=$(date +%Y)
	//   echo "-ldflags"
	//   echo "-X"
	//   echo "main.year=$YEAR"
	BuildArgsScript *string `yaml:"build-args-script,omitempty"`

	// VersionVar is the path to a variable that is set with the version information for the build. For example,
	// "github.com/palantir/godel/v2/cmd/godel.Version". If specified, it is provided to the "build" command as an
	// ldflag.
	VersionVar *string `yaml:"version-var,omitempty"`

	// Environment specifies values for the environment variables that should be set for the build. For example,
	// the following sets CGO to false:
	//
	//   environment:
	//     CGO_ENABLED: "0"
	Environment *map[string]string `yaml:"environment,omitempty"`

	// OSEnvironment specifies values for the environment variables that should be set for the build that are specific
	// to an OS. The key is the OS portion of the "{OS}-{Arch}" target and the value is a map with the same structure as
	// the "Environment" map. Values in this map are set after the "Environment" map but before the "OSArchsEnvironment"
	// map. For example, a value of map[string]map[string]string{"linux":{"CGO_ENABLED": "0"} would set "CGO_ENABLED"
	// when building binaries for the "linux" OS.
	OSEnvironment *map[string]map[string]string `yaml:"os-environment,omitempty"`

	// OSArchsEnvironment specifies values for the environment variables that should be set for the build that are
	// specific to an OS/Architecture. The key is the OS/Arch formatted in the form "{OS}-{Arch}" and the value is a
	// map with the same structure as the "Environment" map. Values in this map are set after the "Environment" and
	// "OSEnvironment" maps. For example, a value of map[string]map[string]string{"darwin-arm64":{"CC": "/usr/osxcross/bin/o64-clang"}
	// would set "CC=/usr/osxcross/bin/o64-clang" when building binaries for the "darwin-arm64" OS/Arch.
	OSArchsEnvironment *map[string]map[string]string `yaml:"os-archs-environment,omitempty"`

	// Script is the content of a script that is written to a file and run before the build processes start. The script
	// process inherits the environment variables of the Go process and also has project-related environment variables.
	// Refer to the documentation for the distgo.BuildScriptEnvVariables function for the extra environment variables.
	Script *string `yaml:"script,omitempty"`

	// OSArchs specifies the GOOS and GOARCH pairs for which the product is built. If blank, defaults to the GOOS
	// and GOARCH of the host system at runtime.
	OSArchs *[]osarch.OSArch `yaml:"os-archs,omitempty"`
}
