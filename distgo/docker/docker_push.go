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

package docker

import (
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/palantir/distgo/distgo"
	"github.com/pkg/errors"
)

func PushProducts(projectInfo distgo.ProjectInfo, projectParam distgo.ProjectParam, productDockerIDs []distgo.ProductDockerID, tagKeys []string, dryRun bool, stdout io.Writer) error {
	// determine products that match specified productDockerIDs
	productParams, err := distgo.ProductParamsForDockerProductArgs(projectParam.Products, productDockerIDs...)
	if err != nil {
		return err
	}
	productParams = distgo.ProductParamsForDockerTagKeys(productParams, tagKeys)

	// run push only for specified products
	for _, currParam := range productParams {
		if err := RunPush(projectInfo, currParam, dryRun, stdout); err != nil {
			return err
		}
	}
	return nil
}

func RunPush(projectInfo distgo.ProjectInfo, productParam distgo.ProductParam, dryRun bool, stdout io.Writer) error {
	if productParam.Docker == nil {
		distgo.PrintlnOrDryRunPrintln(stdout, fmt.Sprintf("%s does not have Docker outputs; skipping build", productParam.ID), dryRun)
		return nil
	}

	var dockerIDs []distgo.DockerID
	for k := range productParam.Docker.DockerBuilderParams {
		dockerIDs = append(dockerIDs, k)
	}
	sort.Sort(distgo.ByDockerID(dockerIDs))

	productTaskOutputInfo, err := distgo.ToProductTaskOutputInfo(projectInfo, productParam)
	if err != nil {
		return err
	}

	for _, dockerID := range dockerIDs {
		if err := runSingleDockerPush(
			productParam.ID,
			dockerID,
			productTaskOutputInfo,
			dryRun,
			stdout,
		); err != nil {
			return err
		}
	}
	return nil
}

func runSingleDockerPush(
	productID distgo.ProductID,
	dockerID distgo.DockerID,
	productTaskOutputInfo distgo.ProductTaskOutputInfo,
	dryRun bool,
	stdout io.Writer) (rErr error) {

	// if an OCI artifact exists, push that. Otherwise, default to pushing the artifact in the docker daemon
	if _, err := layout.FromPath(productTaskOutputInfo.ProductDockerOCIDistOutputDir(dockerID)); err == nil {
		return runOCIPush(productID, dockerID, productTaskOutputInfo, dryRun, stdout)
	}
	return runDockerDaemonPush(productID, dockerID, productTaskOutputInfo, dryRun, stdout)
}

func runOCIPush(productID distgo.ProductID, dockerID distgo.DockerID, productTaskOutputInfo distgo.ProductTaskOutputInfo, dryRun bool, stdout io.Writer) error {
	index, err := layout.ImageIndexFromPath(productTaskOutputInfo.ProductDockerOCIDistOutputDir(dockerID))
	if err != nil {
		return errors.Wrapf(err, "failed to construct image index from OCI layout at path %s", productTaskOutputInfo.ProductDockerOCIDistOutputDir(dockerID))
	}

	for _, tag := range productTaskOutputInfo.Product.DockerOutputInfos.DockerBuilderOutputInfos[dockerID].RenderedTags {
		ref, err := name.NewTag(tag)
		if err != nil {
			return errors.Wrapf(err, "failed to parse reference from tag %s", tag)
		}

		// need to resolve what we're pushing:
		// 1. image index pointing to image indexes -> dereference top-level index and push image index
		// 2. image index (with multiple platforms) pointing to manifests -> push image index
		// 3. image index (without multiple platforms) pointing to manifests -> read image from index and push manifest
		// 4. manifest -> re-extract image from tarball and push manifest :(
		idxManifest, err := index.IndexManifest()
		if err != nil {
			return err
		}
		switch idxManifest.MediaType {
		case types.OCIImageIndex:
			// check the type of the inner manifests
			switch idxManifest.Manifests[0].MediaType {
			case types.OCIImageIndex:
				// if we have an image index, go one level down and push that
				distgo.PrintlnOrDryRunPrintln(stdout, fmt.Sprintf("Writing image index for tag %s of docker configuration %s of product %s...", tag, dockerID, productID), dryRun)
				if !dryRun {
					innerIndex, err := index.ImageIndex(idxManifest.Manifests[0].Digest)
					if err != nil {
						return err
					}
					if err := remote.WriteIndex(ref, innerIndex, remote.WithAuthFromKeychain(authn.DefaultKeychain)); err != nil {
						return errors.Wrap(err, "failed to write index to remote")
					}
				}
			case types.OCIManifestSchema1:
				if idxManifest.Manifests[0].Platform != nil {
					// if we have platform information, we should push our current image index
					distgo.PrintlnOrDryRunPrintln(stdout, fmt.Sprintf("Writing image index for tag %s of docker configuration %s of product %s...", tag, dockerID, productID), dryRun)
					if !dryRun {
						if err := remote.WriteIndex(ref, index, remote.WithAuthFromKeychain(authn.DefaultKeychain)); err != nil {
							return errors.Wrap(err, "failed to write index to remote")
						}
					}

				} else {
					// if we don't have platform information, read and push the image
					distgo.PrintlnOrDryRunPrintln(stdout, fmt.Sprintf("Writing image for tag %s of docker configuration %s of product %s...", tag, dockerID, productID), dryRun)
					if !dryRun {
						image, err := index.Image(idxManifest.Manifests[0].Digest)
						if err != nil {
							return err
						}
						if err := remote.Write(ref, image, remote.WithAuthFromKeychain(authn.DefaultKeychain)); err != nil {
							return errors.Wrap(err, "failed to write image to remote")
						}
					}
				}
			}

		case types.OCIManifestSchema1:
			distgo.PrintlnOrDryRunPrintln(stdout, fmt.Sprintf("Writing image for tag %s of docker configuration %s of product %s...", tag, dockerID, productID), dryRun)
			if !dryRun {
				image, err := tarball.ImageFromPath(filepath.Join(productTaskOutputInfo.ProductDockerOCIDistOutputDir(dockerID), "image.tar"), &ref)
				if err != nil {
					return err
				}
				if err := remote.Write(ref, image, remote.WithAuthFromKeychain(authn.DefaultKeychain)); err != nil {
					return errors.Wrap(err, "failed to write image to remote")
				}
			}
		}
	}
	return nil
}

func runDockerDaemonPush(
	productID distgo.ProductID,
	dockerID distgo.DockerID,
	productTaskOutputInfo distgo.ProductTaskOutputInfo,
	dryRun bool,
	stdout io.Writer,
) error {
	distgo.PrintlnOrDryRunPrintln(stdout, fmt.Sprintf("Running Docker push for configuration %s of product %s...", dockerID, productID), dryRun)
	for _, tag := range productTaskOutputInfo.Product.DockerOutputInfos.DockerBuilderOutputInfos[dockerID].RenderedTags {
		cmd := exec.Command("docker", "push", tag)
		if err := distgo.RunCommandWithVerboseOption(cmd, true, dryRun, stdout); err != nil {
			return err
		}
	}
	return nil
}
