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

package maven

import (
	"github.com/palantir/distgo/distgo"
)

var (
	NoPOMFlag = distgo.PublisherFlag{
		Name:        "no-pom",
		Description: "if true, does not generate and publish a POM",
		Type:        distgo.BoolFlag,
	}
	PackagingFlag = distgo.PublisherFlag{
		Name:        "packaging",
		Description: "sets the packaging property for the POM (overrides value provided by the dister)",
		Type:        distgo.StringFlag,
	}
)
