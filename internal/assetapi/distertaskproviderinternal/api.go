// Copyright 2026 Palantir Technologies, Inc.
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

package distertaskproviderinternal

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/palantir/distgo/dister/distertaskprovider/distertaskproviderapi"
	"github.com/palantir/distgo/distgo"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

const (
	DisterConfigYMLFlagName          = "dister-config-yml"
	AllProductTaskOutputInfoFlagName = "all-product-task-output-info"
)

// ProductsDisterConfig stores the YAML configuration for disters. It is represented as a map from ProductID to DistID
// to DisterConfigYAML.
type ProductsDisterConfig map[distgo.ProductID]map[distgo.DistID]distertaskproviderapi.DisterConfigYAML

// NewTaskProviderCommand returns a new *cobra.Command with the provided name and description that runs the provided
// TaskRunner for the dister with the provided name. The command is configured with the DisterConfigYMLFlagName and
// AllProductTaskOutputInfoFlagName flags and handles the translation from the flag values to the typed values passed to
// the RunTask function.
func NewTaskProviderCommand(name, short string, runner distertaskproviderapi.TaskRunner) *cobra.Command {
	var (
		disterConfigYMLFlagVal          string
		allProductTaskOutputInfoFlagVal string
	)

	cmd := &cobra.Command{
		Use:   name,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			disterConfigYAML, err := readValueFromYAMLFile[ProductsDisterConfig](disterConfigYMLFlagVal)
			if err != nil {
				return err
			}

			allProductTaskOutputInfos, err := readValueFromYAMLFile[map[distgo.ProductID]distgo.ProductTaskOutputInfo](allProductTaskOutputInfoFlagVal)
			if err != nil {
				return err
			}
			if err := runner.RunTask(disterConfigYAML, allProductTaskOutputInfos, args, cmd.OutOrStdout(), cmd.OutOrStderr()); err != nil {
				// if there was an error in RunTask, return an error so that command exits with non-zero error code, but
				// make the error message empty so that the CLI framework doesn't add its own error output.
				return fmt.Errorf("")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&disterConfigYMLFlagVal, DisterConfigYMLFlagName, "", "YAML file that contains the ProductsDisterConfig that contains all the config YAML for the dister")
	cmd.Flags().StringVar(&allProductTaskOutputInfoFlagVal, AllProductTaskOutputInfoFlagName, "", "YAML file containing all ProductTaskOutputInfo")

	// run command configuration logic provided by the runner
	runner.ConfigureCommand(cmd)

	return cmd
}

// FilterDisterConfigYAML returns a new map that contains only the entries in the input map where the config DisterName
// field is equal to the provided disterName. The returned map only includes ProductID keys for values that have at
// least 1 dister entry. The returned map has its own new copies of the inner maps. The config values are shallow copies
// (the byte slice for config is not cloned).
func FilterDisterConfigYAML(allConfigYAML ProductsDisterConfig, disterName string) ProductsDisterConfig {
	// filter config YAML to just the config for the dister
	disterConfigYAML := make(ProductsDisterConfig)
	for productID, distIDToConfig := range allConfigYAML {
		newDistIDToConfig := make(map[distgo.DistID]distertaskproviderapi.DisterConfigYAML)
		for distID, config := range distIDToConfig {
			// skip entries for disters that do not match provided name
			if config.DisterName != disterName {
				continue
			}
			newDistIDToConfig[distID] = config
		}
		// do not add entry for productID if all entries were filtered out
		if len(newDistIDToConfig) == 0 {
			continue
		}
		disterConfigYAML[productID] = newDistIDToConfig
	}
	return disterConfigYAML
}

// RunDisterTaskProviderAssetCommand runs a dister TaskProvider task provided by an asset.
// Invokes the asset at the provided assetPath with cmdArgs, the flags DisterConfigYMLFlagName and
// AllProductTaskOutputInfoFlagName with their respective values (which are files that have the content of the provided
// allConfigYML and allProductTaskOutputInfos arguments marshalled as YAML), and providedArgs, and uses the provided
// stdout and stderr writers as stdout and stderr.
func RunDisterTaskProviderAssetCommand(
	assetPath string,
	cmdArgs []string,
	disterConfigYAML ProductsDisterConfig,
	allProductTaskOutputInfos map[distgo.ProductID]distgo.ProductTaskOutputInfo,
	providedArgs []string,
	stdout,
	stderr io.Writer,
) error {
	var allArgs []string

	// append cmdArgs
	allArgs = append(allArgs, cmdArgs...)

	// append flags and flag values
	configYAMLFile, err := writeValueToYAMLFile(disterConfigYAML)
	if err != nil {
		return errors.Wrapf(err, "failed to write disterConfigYAML %v to file", disterConfigYAML)
	}
	allArgs = append(allArgs, "--"+DisterConfigYMLFlagName, configYAMLFile)

	allProductTaskOutputInfosFile, err := writeValueToYAMLFile(allProductTaskOutputInfos)
	if err != nil {
		return errors.Wrapf(err, "failed to write allProductTaskOutputInfos %v to file", allProductTaskOutputInfos)
	}
	allArgs = append(allArgs, "--"+AllProductTaskOutputInfoFlagName, allProductTaskOutputInfosFile)

	// append providedArgs
	allArgs = append(allArgs, providedArgs...)

	cmd := exec.Command(assetPath, allArgs...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			// command ran, but exited with non-0 exit code: propagate error, but don't wrap or include message, as it
			// is expected that the command itself will output information related to the error
			return fmt.Errorf("")
		}
		// if command failed for reason other than non-0 exit code, the behavior is unexpected, so wrap error and
		// include message to help with debugging
		return errors.Wrapf(err, "failed to run command %v", cmd.Args)
	}
	return nil
}

// readValueFromYAMLFile returns a value T obtained by unmarshalling the YAML bytes in the file at the specified path
// into the variable.
func readValueFromYAMLFile[T any](yamlFilePath string) (T, error) {
	var output T
	// although it would be more efficient to unmarshal directly from input file, read file bytes and then unmarshal
	// the read bytes to the target value to make failures/errors clearer and because the objects for which this is used
	// should all be of reasonable size.
	yamlBytes, err := os.ReadFile(yamlFilePath)
	if err != nil {
		return output, errors.Wrapf(err, "failed to read YAML file %q", yamlFilePath)
	}
	if err := yaml.Unmarshal(yamlBytes, &output); err != nil {
		return output, errors.Wrapf(err, "failed to unmarshal YAML bytes %q", string(yamlBytes))
	}
	return output, nil
}

// writeValueToYAMLFile marshals the provided value as YAML and writes it to a temporary file (created by os.CreateTemp)
// and returns the path to the file.
func writeValueToYAMLFile[T any](value T) (string, error) {
	// although it would be more efficient to marshal directly to output file, marshal to bytes and then write bytes to
	// file to make failures/errors clearer and because the objects for which this is used should all be of reasonable
	// size.
	yamlBytes, err := yaml.Marshal(value)
	if err != nil {
		return "", errors.Wrapf(err, "failed to marshal %v as YAML", value)
	}

	f, err := os.CreateTemp("", "dister-task-provider-*.yml")
	if err != nil {
		return "", errors.Wrapf(err, "failed to create temp file for YAML file")
	}
	defer func() {
		_ = f.Close()
	}()

	if _, err := f.Write(yamlBytes); err != nil {
		return "", errors.Wrapf(err, "failed to write YAML bytes %q to file %q", string(yamlBytes), f.Name())
	}

	return f.Name(), nil
}
