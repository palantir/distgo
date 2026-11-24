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

package cmd

import (
	"io"
	"testing"

	"github.com/palantir/distgo/distgo"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestNoDependentTasksFlag(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want bool
	}{
		{nil, false},
		{[]string{"--no-dependent-tasks"}, true},
		{[]string{"--no-dependent-tasks=false"}, false},
	} {
		command := &cobra.Command{Use: "test"}
		var value bool
		addNoDependentTasksFlag(command, &value)
		require.NoError(t, command.ParseFlags(tc.args))
		require.Equal(t, tc.want, value)
	}
}

type flagPublisher struct{ reservedFlag string }

func (p flagPublisher) TypeName() (string, error) { return "flag-test", nil }
func (p flagPublisher) Flags() ([]distgo.PublisherFlag, error) {
	if p.reservedFlag != "" {
		return []distgo.PublisherFlag{{Name: distgo.PublisherFlagName(p.reservedFlag), Type: distgo.BoolFlag}}, nil
	}
	return nil, nil
}
func (p flagPublisher) RunPublish([]distgo.ProductPublishInfo, map[distgo.PublisherFlagName]any, bool, io.Writer) error {
	return nil
}

func TestPublishSubcommandsRegisterIndependentFlags(t *testing.T) {
	for _, name := range []string{"flag-test-one", "flag-test-two"} {
		addPublishSubcommands([]string{name}, []distgo.Publisher{flagPublisher{}})
		command, _, err := publishCmd.Find([]string{name})
		require.NoError(t, err)
		require.Equal(t, name, command.Name())
		t.Cleanup(func() { publishCmd.RemoveCommand(command) })
		require.NotNil(t, command.Flags().Lookup(noDependentTasksFlagName))
		require.NotNil(t, command.Flags().Lookup(goCacheRootFlagName))
	}
	first, _, err := publishCmd.Find([]string{"flag-test-one"})
	require.NoError(t, err)
	second, _, err := publishCmd.Find([]string{"flag-test-two"})
	require.NoError(t, err)
	require.NoError(t, first.ParseFlags([]string{"--no-dependent-tasks", "--go-cache-root", t.TempDir()}))
	enabled, err := second.Flags().GetBool(noDependentTasksFlagName)
	require.NoError(t, err)
	require.False(t, enabled)
	cacheRoot, err := second.Flags().GetString(goCacheRootFlagName)
	require.NoError(t, err)
	require.Empty(t, cacheRoot)
}

func TestPublisherCannotShadowTaskFlags(t *testing.T) {
	for _, name := range []string{noDependentTasksFlagName, goCacheRootFlagName} {
		require.Panics(t, func() {
			addPublishSubcommands([]string{"flag-test-conflict"}, []distgo.Publisher{flagPublisher{reservedFlag: name}})
		})
	}
}
