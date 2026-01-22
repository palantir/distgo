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

package main

import (
	"fmt"
	"os"

	"github.com/palantir/distgo/cmd"
	"github.com/palantir/godel/v2/framework/pluginapi/v2/pluginapi"
)

func main() {
	if ok := pluginapi.InfoCmd(os.Args, os.Stdout, cmd.PluginInfo); ok {
		return
	}

	// load assets
	printErrorAndExitIfNonNil(cmd.LoadAssets(os.Args[1:]))

	// add commands provided by assets
	printErrorAndExitIfNonNil(cmd.AddAssetCommands())

	os.Exit(cmd.Execute())
}

// If the provided error is non-nil, prints it to stderr and exits with exit code 1.
// Is a noop if err is nil.
func printErrorAndExitIfNonNil(err error) {
	// no error: do nothing
	if err == nil {
		return
	}
	// print error to os.Stderr and exit
	_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}
