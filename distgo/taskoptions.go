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

package distgo

import (
	"fmt"
	"sort"
	"strings"
)

// TaskOptions controls automatic prerequisite execution. The zero value preserves normal task behavior.
type TaskOptions struct {
	// NoDependentTasks requires prerequisite outputs to be current, including in dry-run mode.
	// Only the requested stage may execute, including for dependent products. Explicit commands
	// run by scripts or assets are outside this policy.
	NoDependentTasks bool
}

// RequiredTask describes prerequisite work that would be needed to make an output current.
type RequiredTask struct {
	Task    string
	Product ProductID
	Target  string
	Outputs []string
	Reason  string
}

// RequiredTasksError returns an error describing required prerequisite work, or nil if none is required.
// Entries and their output paths are sorted and duplicate entries are omitted without modifying the input.
func RequiredTasksError(operation string, required []RequiredTask) error {
	if len(required) == 0 {
		return nil
	}
	lines := make(map[string]struct{})
	for _, task := range required {
		outputs := append([]string(nil), task.Outputs...)
		sort.Strings(outputs)
		line := fmt.Sprintf("  %s required for %s (%s): %s; outputs: %s",
			task.Task, task.Product, task.Target, task.Reason, strings.Join(outputs, ", "))
		lines[line] = struct{}{}
	}
	ordered := make([]string, 0, len(lines))
	for line := range lines {
		ordered = append(ordered, line)
	}
	sort.Strings(ordered)
	return fmt.Errorf("cannot run %s with --no-dependent-tasks:\n%s\nrun the prerequisite stage and restore its outputs",
		operation, strings.Join(ordered, "\n"))
}
