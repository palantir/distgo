package distertaskproviderinternal

import (
	"io"
	"os"
	"os/exec"

	"github.com/palantir/distgo/distgo"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

const (
	AllConfigYMLFlagName             = "all-config-yml"
	AllProductTaskOutputInfoFlagName = "all-product-task-output-info"
)

// TaskRunner is an interface that runs the task provided by a dister task provider.
// This is a copy of the dister/distertaskprovider.TaskRunner interface that is defined in this package to avoid
// package import cycles.
type TaskRunner interface {
	RunTask(
		allConfigYML map[distgo.ProductID]map[distgo.DistID][]byte,
		allProductTaskOutputInfos map[distgo.ProductID]distgo.ProductTaskOutputInfo,
		args []string,
		stdout, stderr io.Writer,
	) error
}

// NewTaskProviderCommand returns a new *cobra.Command with the provided name and description that runs the provided
// TaskRunner. The command is configured with the AllConfigYMLFlagName and AllProductTaskOutputInfoFlagName flags and
// handles the translation from the flag values to the typed values passed to the RunTask function.
func NewTaskProviderCommand(name, short string, runner TaskRunner) *cobra.Command {
	var (
		allConfigYMLFlagVal             string
		allProductTaskOutputInfoFlagVal string
	)

	cmd := &cobra.Command{
		Use:   name,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			allConfigYML, err := readValueFromYAMLFile[map[distgo.ProductID]map[distgo.DistID][]byte](allConfigYMLFlagVal)
			if err != nil {
				return err
			}
			allProductTaskOutputInfos, err := readValueFromYAMLFile[map[distgo.ProductID]distgo.ProductTaskOutputInfo](allProductTaskOutputInfoFlagVal)
			if err != nil {
				return err
			}
			return runner.RunTask(allConfigYML, allProductTaskOutputInfos, args, cmd.OutOrStdout(), cmd.OutOrStderr())
		},
	}

	cmd.Flags().StringVar(&allConfigYMLFlagVal, AllConfigYMLFlagName, "", "file containing YAML representation of all config YAML for dister")
	cmd.Flags().StringVar(&allProductTaskOutputInfoFlagVal, AllProductTaskOutputInfoFlagName, "", "file containing YAML representation of all ProductTaskOutputInfo for dister")

	return cmd
}

// RunDisterTaskProviderAssetCommand runs a dister TaskProvider task provided by an asset.
// Invokes the asset at the provided assetPath with cmdArgs, the flags AllConfigYMLFlagName and
// AllProductTaskOutputInfoFlagName with their respective values (which are files that have the content of the provided
// allConfigYML and allProductTaskOutputInfos arguments marshalled as YAML), and providedArgs, and uses the provided
// stdout and stderr writers as stdout and stderr.
func RunDisterTaskProviderAssetCommand(
	assetPath string,
	cmdArgs []string,
	allConfigYML map[distgo.ProductID]map[distgo.DistID][]byte,
	allProductTaskOutputInfos map[distgo.ProductID]distgo.ProductTaskOutputInfo,
	providedArgs []string,
	stdout,
	stderr io.Writer,
) error {
	var allArgs []string

	// append cmdArgs
	allArgs = append(allArgs, cmdArgs...)

	// append flags and flag values
	allConfigYMLFile, err := writeValueToYAMLFile(allConfigYML)
	if err != nil {
		return errors.Wrapf(err, "failed to write allConfigYML %v to file", allConfigYML)
	}
	allArgs = append(allArgs, "--"+AllConfigYMLFlagName, allConfigYMLFile)

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
