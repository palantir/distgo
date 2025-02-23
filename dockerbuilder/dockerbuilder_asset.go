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

package dockerbuilder

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"

	"github.com/palantir/distgo/distgo"
	"github.com/pkg/errors"
)

type assetDockerBuilder struct {
	assetPath string
	cfgYML    string
}

func (d *assetDockerBuilder) TypeName() (string, error) {
	nameCmd := exec.Command(d.assetPath, nameCmdName)
	outputBytes, err := runCommand(nameCmd)
	if err != nil {
		return "", err
	}
	var typeName string
	if err := json.Unmarshal(outputBytes, &typeName); err != nil {
		return "", errors.Wrapf(err, "failed to unmarshal JSON")
	}
	return typeName, nil
}

func (d *assetDockerBuilder) VerifyConfig() error {
	verifyConfigCmd := exec.Command(d.assetPath, verifyConfigCmdName,
		"--"+commonCmdConfigYMLFlagName, d.cfgYML,
	)
	if _, err := runCommand(verifyConfigCmd); err != nil {
		return err
	}
	return nil
}

func (d *assetDockerBuilder) RunDockerBuild(dockerID distgo.DockerID, productTaskOutputInfo distgo.ProductTaskOutputInfo, verbose, dryRun bool, stdout io.Writer) error {
	productTaskOutputInfoJSON, err := json.Marshal(productTaskOutputInfo)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal JSON")
	}
	runDockerBuildCmd := exec.Command(d.assetPath, runDockerBuildCmdName,
		"--"+commonCmdConfigYMLFlagName, d.cfgYML,
		"--"+runDockerBuildCmdDockerIDFlagName, string(dockerID),
		"--"+runDockerBuildCmdProductTaskOutputInfoFlagName, string(productTaskOutputInfoJSON),
		"--"+runDockerBuildCmdVerboseFlagName+"="+strconv.FormatBool(verbose),
		"--"+runDockerBuildCmdDryRunFlagName+"="+strconv.FormatBool(dryRun),
	)
	runDockerBuildCmd.Stdout = stdout

	stderrBuf := &bytes.Buffer{}
	runDockerBuildCmd.Stderr = stderrBuf
	if err := runDockerBuildCmd.Run(); err != nil {
		errOutput := stderrBuf.String()
		if errOutput == "" {
			errOutput = fmt.Sprintf("failed to run Docker builder %s", d.assetPath)
		}
		return errors.Wrap(err, strings.TrimSpace(strings.TrimPrefix(errOutput, "Error: ")))
	}
	return nil
}

func runCommand(cmd *exec.Cmd) ([]byte, error) {
	outputBytes, err := cmd.CombinedOutput()
	if err != nil {
		return outputBytes, errors.New(strings.TrimSpace(strings.TrimPrefix(string(outputBytes), "Error: ")))
	}
	return outputBytes, nil
}
