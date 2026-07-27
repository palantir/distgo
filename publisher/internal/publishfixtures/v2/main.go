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

package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/publisher"
	"github.com/palantir/pkg/cobracli"
)

const typeName = "v2"

type v2Publisher struct{}

func (p *v2Publisher) TypeName() (string, error) {
	return typeName, nil
}

func (p *v2Publisher) Flags() ([]distgo.PublisherFlag, error) {
	return nil, nil
}

func (p *v2Publisher) RunPublish(inputs []distgo.ProductPublishInfo, flagVals map[distgo.PublisherFlagName]any, dryRun bool, stdout io.Writer) error {
	ids := make([]string, 0, len(inputs))
	for _, input := range inputs {
		ids = append(ids, string(input.ProductTaskOutputInfo.Product.ID))
	}
	_, _ = fmt.Fprintf(stdout, "RunPublish:%s\n", strings.Join(ids, ","))
	return nil
}

func creator() publisher.Creator {
	return publisher.NewCreator(typeName, func() distgo.Publisher {
		return &v2Publisher{}
	})
}

func upgradeConfig(cfgBytes []byte) ([]byte, error) {
	return cfgBytes, nil
}

func main() {
	rootCmd := publisher.AssetRootCmd(creator(), upgradeConfig, "test fixture: run-publish-v2 publisher")
	os.Exit(cobracli.ExecuteWithDefaultParams(rootCmd))
}
