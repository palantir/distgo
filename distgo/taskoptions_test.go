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

package distgo_test

import (
	"testing"

	"github.com/palantir/distgo/distgo"
	"github.com/stretchr/testify/require"
)

func TestRequiredTasksErrorIsStable(t *testing.T) {
	a := distgo.RequiredTask{Task: "dist", Product: "z", Target: "tgz", Outputs: []string{"z", "a"}, Reason: "stale"}
	b := distgo.RequiredTask{Task: "build", Product: "a", Target: "linux-amd64", Outputs: []string{"binary"}, Reason: "missing"}
	first := distgo.RequiredTasksError("publish", []distgo.RequiredTask{a, b, a})
	second := distgo.RequiredTasksError("publish", []distgo.RequiredTask{b, a})
	require.EqualError(t, first, second.Error())
	require.Equal(t, []string{"z", "a"}, a.Outputs)
	require.Nil(t, distgo.RequiredTasksError("publish", nil))
}
