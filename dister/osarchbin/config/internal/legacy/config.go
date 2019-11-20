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

package legacy

import (
	v0 "github.com/palantir/distgo/dister/osarchbin/config/internal/v0"
	"github.com/palantir/godel/v2/pkg/osarch"
	"github.com/palantir/godel/v2/pkg/versionedconfig"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

type Config struct {
	versionedconfig.ConfigWithLegacy `yaml:",inline,omitempty"`
	OSArchs                          []osarch.OSArch `yaml:"os-archs,omitempty"`
}

func UpgradeConfig(cfgBytes []byte) ([]byte, error) {
	var legacyCfg Config
	if err := yaml.UnmarshalStrict(cfgBytes, &legacyCfg); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal osarchbin dister legacy configuration")
	}
	upgradedCfg := v0.Config{
		OSArchs: legacyCfg.OSArchs,
	}
	upgradedCfgBytes, err := yaml.Marshal(upgradedCfg)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal osarchbin dister v0 configuration")
	}
	return upgradedCfgBytes, nil
}
