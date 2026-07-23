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

	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/publisher"
	"github.com/palantir/pkg/cobracli"
)

const typeName = "nonbatch"

type nonBatchPublisher struct{}

func (p *nonBatchPublisher) TypeName() (string, error) {
	return typeName, nil
}

func (p *nonBatchPublisher) Flags() ([]distgo.PublisherFlag, error) {
	return nil, nil
}

func (p *nonBatchPublisher) RunPublish(productTaskOutputInfo distgo.ProductTaskOutputInfo, cfgYML []byte, flagVals map[distgo.PublisherFlagName]any, dryRun bool, stdout io.Writer) error {
	_, _ = fmt.Fprintf(stdout, "RunPublish:%s\n", productTaskOutputInfo.Product.ID)
	return nil
}

func creator() publisher.Creator {
	return publisher.NewCreator(typeName, func() distgo.Publisher {
		return &nonBatchPublisher{}
	})
}

func upgradeConfig(cfgBytes []byte) ([]byte, error) {
	return cfgBytes, nil
}

func main() {
	rootCmd := publisher.AssetRootCmd(creator(), upgradeConfig, "test fixture: nonbatch publisher, rebuilt")
	os.Exit(cobracli.ExecuteWithDefaultParams(rootCmd))
}
