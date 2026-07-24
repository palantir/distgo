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

package publish

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/distgo/dist"
	"github.com/pkg/errors"
)

func Products(projectInfo distgo.ProjectInfo, projectParam distgo.ProjectParam, configModTime *time.Time, productDistIDs []distgo.ProductDistID, publisher distgo.Publisher, flagVals map[distgo.PublisherFlagName]any, dryRun bool, stdout io.Writer) error {
	// run dist for products (will only run dist for productDistIDs that require dist artifact generation)
	if err := dist.Products(projectInfo, projectParam, configModTime, productDistIDs, dryRun, true, stdout); err != nil {
		return err
	}

	productParams, err := distgo.ProductParamsForDistProductArgs(projectParam.Products, productDistIDs...)
	if err != nil {
		return err
	}

	publisherType, err := publisher.TypeName()
	if err != nil {
		return errors.Wrapf(err, "failed to determine type of publisher")
	}

	var inputs []distgo.BatchPublishInput
	for _, currProduct := range productParams {
		input, err := preparePublishInput(projectInfo, currProduct, publisherType, dryRun, stdout)
		if err != nil {
			return err
		}
		if input == nil {
			continue
		}
		inputs = append(inputs, *input)
	}
	if len(inputs) == 0 {
		return nil
	}

	if batchPublisher, ok := publisher.(distgo.BatchPublisher); ok {
		if err := batchPublisher.RunPublishBatch(inputs, flagVals, dryRun, stdout); err != nil {
			return errors.Wrapf(err, "failed to publish products using %s publisher", publisherType)
		}
		return nil
	}

	for _, input := range inputs {
		if err := publisher.RunPublish(input.ProductTaskOutputInfo, input.ConfigYML, flagVals, dryRun, stdout); err != nil {
			return errors.Wrapf(err, "failed to publish %s using %s publisher", input.ProductTaskOutputInfo.Product.ID, publisherType)
		}
	}
	return nil
}

// Run executes the publish action for the specified product. Produces both the dist output directory and the dist
// artifacts for the product. The outputs for the dependent products for the provided product must already exist in the
// proper locations.
func Run(projectInfo distgo.ProjectInfo, productParam distgo.ProductParam, publisher distgo.Publisher, flagVals map[distgo.PublisherFlagName]any, dryRun bool, stdout io.Writer) error {
	publisherType, err := publisher.TypeName()
	if err != nil {
		return errors.Wrapf(err, "failed to determine type of publisher")
	}
	input, err := preparePublishInput(projectInfo, productParam, publisherType, dryRun, stdout)
	if err != nil {
		return err
	}
	if input == nil {
		return nil
	}
	if err := publisher.RunPublish(input.ProductTaskOutputInfo, input.ConfigYML, flagVals, dryRun, stdout); err != nil {
		return errors.Wrapf(err, "failed to publish %s using %s publisher", productParam.ID, publisherType)
	}

	return nil
}

func preparePublishInput(projectInfo distgo.ProjectInfo, productParam distgo.ProductParam, publisherType string, dryRun bool, stdout io.Writer) (*distgo.BatchPublishInput, error) {
	if productParam.Dist == nil {
		distgo.PrintlnOrDryRunPrintln(stdout, fmt.Sprintf("%s does not have dist outputs; skipping publish", productParam.ID), dryRun)
		return nil, nil
	}

	// verify that dist artifacts to publish exists (dist is skipped in dry-run mode, so the
	// artifacts are never actually written to disk and there is nothing to verify)
	if !dryRun {
		productOutputInfo, err := productParam.ToProductOutputInfo(projectInfo.Version)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to compute output info")
		}
		for _, currDistID := range productOutputInfo.DistOutputInfos.DistIDs {
			for _, currArtifactPath := range distgo.ProductDistArtifactPaths(projectInfo, productOutputInfo)[currDistID] {
				if _, err := os.Stat(currArtifactPath); os.IsNotExist(err) {
					return nil, errors.Errorf("distribution artifact for product %s with dist %s does not exist at %s", productParam.ID, currDistID, currArtifactPath)
				}
			}
		}
	}

	productTaskOutputInfo, err := distgo.ToProductTaskOutputInfo(projectInfo, productParam)
	if err != nil {
		return nil, err
	}
	var publishCfgBytes []byte
	if productParam.Publish != nil {
		publishCfgBytes = productParam.Publish.PublishInfo[distgo.PublisherTypeID(publisherType)].ConfigBytes
	}
	return &distgo.BatchPublishInput{
		ProductTaskOutputInfo: productTaskOutputInfo,
		ConfigYML:             publishCfgBytes,
	}, nil
}
