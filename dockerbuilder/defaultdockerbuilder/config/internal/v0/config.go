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
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

type Config struct {
	BuildArgs []string `yaml:"build-args,omitempty"`

	// BuildArgsScript is the content of a script that is written to a file and run before this image is built to
	// provide supplemental "docker build" arguments for the image. The content of this value is written to a file and
	// executed. The script process inherits the environment variables of the Go process. Each line of output of the
	// script is provided to the "docker build" command as a separate argument. The arguments produced by the script are
	// appended to any arguments specified in BuildArgs.
	BuildArgsScript *string `yaml:"build-args-script,omitempty"`
}

func UpgradeConfig(cfgBytes []byte) ([]byte, error) {
	var cfg Config
	if err := yaml.UnmarshalStrict(cfgBytes, &cfg); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal default dockerbuilder v0 configuration")
	}
	return cfgBytes, nil
}
