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

package config

import (
	"github.com/palantir/distgo/distgo"
	v0 "github.com/palantir/distgo/distgo/config/internal/v0"
)

type VulncheckConfig v0.VulncheckConfig

func ToVulncheckConfig(in *VulncheckConfig) *v0.VulncheckConfig {
	return (*v0.VulncheckConfig)(in)
}

func (cfg *VulncheckConfig) ToParam(defaultCfg VulncheckConfig) distgo.VulncheckParam {
	env := defaultCfg.Env
	if cfg != nil && len(cfg.Env) > 0 {
		env = cfg.Env
	}

	pkgs := defaultCfg.Pkgs
	if cfg != nil && len(cfg.Pkgs) > 0 {
		pkgs = cfg.Pkgs
	}

	return distgo.VulncheckParam{
		Pkgs: pkgs,
		Dir:  getConfigStringValue(cfg.Dir, defaultCfg.Dir, ""),
		Env:  env,
	}
}
