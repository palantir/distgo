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
	var publishedProducts []publishedProduct
	for _, currProduct := range productParams {
		publishedProduct, err := publishProduct(projectInfo, currProduct, publisher, flagVals, dryRun, stdout)
		if err != nil {
			return err
		}
		if publishedProduct != nil {
			publishedProducts = append(publishedProducts, *publishedProduct)
		}
	}

	// Finalize once every product has uploaded, since some publishers share one release across products.
	return finalizePublishedProducts(publisher, publishedProducts, flagVals, dryRun, stdout)
}

type publishedProduct struct {
	productTaskOutputInfo distgo.ProductTaskOutputInfo
	cfgYML                []byte
}

// finalizePublishedProducts calls FinalizePublish once per published product, if publisher implements distgo.FinalizingPublisher.
// It finalizes per product rather than once for all products since products can have different publish config, e.g. different owner/repo.
func finalizePublishedProducts(publisher distgo.Publisher, published []publishedProduct, flagVals map[distgo.PublisherFlagName]any, dryRun bool, stdout io.Writer) error {
	finalizer, ok := publisher.(distgo.FinalizingPublisher)
	if !ok {
		return nil
	}
	publisherType, err := publisher.TypeName()
	if err != nil {
		return errors.Wrapf(err, "failed to determine type of publisher")
	}
	for _, currPublished := range published {
		if err := finalizer.FinalizePublish(currPublished.productTaskOutputInfo, currPublished.cfgYML, flagVals, dryRun, stdout); err != nil {
			return errors.Wrapf(err, "failed to finalize publish for %s using %s publisher", currPublished.productTaskOutputInfo.Product.ID, publisherType)
		}
	}
	return nil
}

// Run executes the publish action for the specified product, then finalizes it immediately if the publisher supports it.
// Produces both the dist output directory and the dist artifacts for the product. The outputs for the dependent products for the provided product must already exist in
// the proper locations.
func Run(projectInfo distgo.ProjectInfo, productParam distgo.ProductParam, publisher distgo.Publisher, flagVals map[distgo.PublisherFlagName]any, dryRun bool, stdout io.Writer) error {
	published, err := publishProduct(projectInfo, productParam, publisher, flagVals, dryRun, stdout)
	if err != nil {
		return err
	}
	if published == nil {
		return nil
	}
	return finalizePublishedProducts(publisher, []publishedProduct{*published}, flagVals, dryRun, stdout)
}

// publishProduct runs RunPublish for a single product. It returns a publishedProduct containing the product's output info and publish config if RunPublish actually ran,
// or nil if the product was skipped if it had no dist outputs.
func publishProduct(projectInfo distgo.ProjectInfo, productParam distgo.ProductParam, publisher distgo.Publisher, flagVals map[distgo.PublisherFlagName]any, dryRun bool, stdout io.Writer) (*publishedProduct, error) {
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

	// run publish
	productTaskOutputInfo, err := distgo.ToProductTaskOutputInfo(projectInfo, productParam)
	if err != nil {
		return nil, err
	}
	publisherType, err := publisher.TypeName()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to determine type of publisher")
	}
	var publishCfgBytes []byte
	if productParam.Publish != nil {
		publishCfgBytes = productParam.Publish.PublishInfo[distgo.PublisherTypeID(publisherType)].ConfigBytes
	}
	if err := publisher.RunPublish(productTaskOutputInfo, publishCfgBytes, flagVals, dryRun, stdout); err != nil {
		return nil, errors.Wrapf(err, "failed to publish %s using %s publisher", productParam.ID, publisherType)
	}

	return &publishedProduct{
		productTaskOutputInfo: productTaskOutputInfo,
		cfgYML:                publishCfgBytes,
	}, nil
}
