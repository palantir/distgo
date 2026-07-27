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

package publisher

import (
	"encoding/json"
	"io"
	"os/exec"
	"strconv"

	"github.com/palantir/distgo/distgo"
	"github.com/pkg/errors"
)

type assetPublisher struct {
	assetPath string
}

func (p *assetPublisher) TypeName() (string, error) {
	nameCmd := exec.Command(p.assetPath, nameCmdName)
	outputBytes, err := nameCmd.CombinedOutput()
	if err != nil {
		return "", errors.Wrapf(err, "command %v failed with output %s", nameCmd.Args, string(outputBytes))
	}
	var typeName string
	if err := json.Unmarshal(outputBytes, &typeName); err != nil {
		return "", errors.Wrapf(err, "failed to unmarshal JSON")
	}
	return typeName, nil
}

func (p *assetPublisher) Flags() ([]distgo.PublisherFlag, error) {
	flagsCmd := exec.Command(p.assetPath, flagsCmdName)
	outputBytes, err := flagsCmd.CombinedOutput()
	if err != nil {
		return nil, errors.Wrapf(err, "command %v failed with output %s", flagsCmd.Args, string(outputBytes))
	}
	var flags []distgo.PublisherFlag
	if err := json.Unmarshal(outputBytes, &flags); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal JSON")
	}
	return flags, nil
}

func (p *assetPublisher) RunPublish(inputs []distgo.ProductPublishInfo, flagVals map[distgo.PublisherFlagName]any, dryRun bool, stdout io.Writer) error {
	inputsJSON, err := json.Marshal(inputs)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal JSON for inputs")
	}
	flagValsJSON, err := json.Marshal(flagVals)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal JSON for flagVals")
	}

	args := []string{
		runPublishV2CmdName,
		"--" + runPublishV2CmdInputsFlagName, string(inputsJSON),
		"--" + runPublishCmdFlagValsFlagName, string(flagValsJSON),
		"--" + runPublishCmdDryRunFlagName + "=" + strconv.FormatBool(dryRun),
	}

	runPublishCmd := exec.Command(p.assetPath, args...)
	runPublishCmd.Stdout = stdout
	runPublishCmd.Stderr = stdout

	if err := runPublishCmd.Run(); err != nil {
		return errors.Wrapf(err, "command %v failed", runPublishCmd.Args[0])
	}
	return nil
}

// assetSupportsV2Publish reports whether the asset at assetPath registers the run-publish-v2 command.
func assetSupportsV2Publish(assetPath string) bool {
	return exec.Command(assetPath, runPublishV2CmdName, "--help").Run() == nil
}

// legacyAssetPublisher wraps an assetPublisher to support the legacy per-product publishing.
// This is used for assets that do not yet support the run-publish-v2 command.
type legacyAssetPublisher struct {
	assetPublisher
}

func (p *legacyAssetPublisher) RunPublish(inputs []distgo.ProductPublishInfo, flagVals map[distgo.PublisherFlagName]any, dryRun bool, stdout io.Writer) error {
	flagValsJSON, err := json.Marshal(flagVals)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal JSON for flagVals")
	}
	for _, input := range inputs {
		productTaskOutputInfoJSON, err := json.Marshal(input.ProductTaskOutputInfo)
		if err != nil {
			return errors.Wrapf(err, "failed to marshal JSON for productTaskOutputInfo")
		}
		cfgYMLString := string(input.PublisherConfigYML)
		if cfgYMLString == "" {
			cfgYMLString = "{}"
		}

		args := []string{runPublishCmdName}
		args = append(args, "--"+runPublishCmdProductTaskOutputInfoFlagName, string(productTaskOutputInfoJSON))
		args = append(args, "--"+runPublishCmdConfigYMLFlagName, cfgYMLString)
		args = append(args, "--"+runPublishCmdFlagValsFlagName, string(flagValsJSON))
		args = append(args, "--"+runPublishCmdDryRunFlagName+"="+strconv.FormatBool(dryRun))

		runPublishCmd := exec.Command(p.assetPath, args...)
		runPublishCmd.Stdout = stdout
		runPublishCmd.Stderr = stdout

		if err := runPublishCmd.Run(); err != nil {
			return errors.Wrapf(err, "failed to publish %s", input.ProductTaskOutputInfo.Product.ID)
		}
	}
	return nil
}
