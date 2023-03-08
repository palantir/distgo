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
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/palantir/distgo/distgo"
	"github.com/pkg/errors"
	"golang.org/x/exp/maps"
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
		ref, err := name.ParseReference(tag)
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
			return errors.Wrap(err, "failed to read index manifest")
		}
		switch idxManifest.MediaType {
		case types.OCIImageIndex:
			return handleImageIndex(index, idxManifest, ref, productID, dockerID, dryRun, stdout)
		case types.OCIManifestSchema1:
			return handleImageManifest(ref, productID, dockerID, productTaskOutputInfo, dryRun, stdout)
		default:
			return errors.Errorf("unexpected media type %s for configuration %s for product %s", idxManifest.MediaType, dockerID, productID)
		}
	}
	return nil
}

func handleImageIndex(index v1.ImageIndex, idxManifest *v1.IndexManifest, ref name.Reference, productID distgo.ProductID, dockerID distgo.DockerID, dryRun bool, stdout io.Writer) error {
	// check the type of the inner manifests
	manifestMetadata, err := manifestMetadataFromIndexManifest(idxManifest)
	if err != nil {
		return errors.Wrap(err, "encountered unexpected index manifest state")
	}

	switch manifestMetadata.mediaType {
	case types.OCIImageIndex:
		// if we have an image index, go one level down and push that
		innerIndex, err := index.ImageIndex(idxManifest.Manifests[0].Digest)
		if err != nil {
			return errors.Wrapf(err, "failed to read image index digest %s from OCI layout", idxManifest.Manifests[0].Digest)
		}
		if err := writeIndex(innerIndex, ref, productID, dockerID, dryRun, stdout); err != nil {
			return errors.Wrapf(err, "failed to write image index for tag %s of configuration %s for product %s", ref, dockerID, productID)
		}
		return nil
	case types.OCIManifestSchema1:
		if manifestMetadata.containsPlatformInfo {
			// if we have platform information, we should push our current image index
			if err := writeIndex(index, ref, productID, dockerID, dryRun, stdout); err != nil {
				return errors.Wrapf(err, "failed to write image index for tag %s of configuration %s for product %s", ref, dockerID, productID)
			}
			return nil
		}

		if len(idxManifest.Manifests) != 1 {
			return errors.New("unexpected number of image manifests present in image index without platform information")
		}
		image, err := index.Image(idxManifest.Manifests[0].Digest)
		if err != nil {
			return errors.Wrapf(err, "failed to read image digest %s from OCI layout", idxManifest.Manifests[0].Digest)
		}
		if err := writeImage(image, ref, productID, dockerID, dryRun, stdout); err != nil {
			return errors.Wrapf(err, "failed to write image for tag %s of configuration %s for product %s", ref, dockerID, productID)
		}
		return nil
	default:
		return errors.Errorf("unexpected media type %s for configuration %s for product %s", idxManifest.MediaType, dockerID, productID)
	}
}

func handleImageManifest(ref name.Reference, productID distgo.ProductID, dockerID distgo.DockerID, productTaskOutputInfo distgo.ProductTaskOutputInfo, dryRun bool, stdout io.Writer) error {
	path := filepath.Join(productTaskOutputInfo.ProductDockerOCIDistOutputDir(dockerID), "image.tar")
	image, err := tarball.ImageFromPath(path, nil)
	if err != nil {
		return errors.Wrapf(err, "failed to read image from path %s", path)
	}
	if err := writeImage(image, ref, productID, dockerID, dryRun, stdout); err != nil {
		return errors.Wrapf(err, "failed to write image for tag %s of configuration %s for product %s", ref, dockerID, productID)
	}
	return nil
}

func writeIndex(index v1.ImageIndex, ref name.Reference, productID distgo.ProductID, dockerID distgo.DockerID, dryRun bool, stdout io.Writer) error {
	distgo.PrintlnOrDryRunPrintln(stdout, fmt.Sprintf("Writing image index for tag %s of docker configuration %s of product %s...", ref, dockerID, productID), dryRun)
	if !dryRun {
		if err := remote.WriteIndex(ref, index, remote.WithAuthFromKeychain(authn.DefaultKeychain)); err != nil {
			return errors.Wrap(err, "failed to write image index to remote")
		}
	}
	return nil
}

func writeImage(image v1.Image, ref name.Reference, productID distgo.ProductID, dockerID distgo.DockerID, dryRun bool, stdout io.Writer) error {
	distgo.PrintlnOrDryRunPrintln(stdout, fmt.Sprintf("Writing image for tag %s of docker configuration %s of product %s...", ref, dockerID, productID), dryRun)
	if !dryRun {
		if err := remote.Write(ref, image, remote.WithAuthFromKeychain(authn.DefaultKeychain)); err != nil {
			return errors.Wrap(err, "failed to write image to remote")
		}
	}
	return nil
}

type manifestMetadata struct {
	mediaType            types.MediaType
	containsPlatformInfo bool
}

func manifestMetadataFromIndexManifest(indexManifest *v1.IndexManifest) (manifestMetadata, error) {
	if len(indexManifest.Manifests) == 0 {
		return manifestMetadata{}, errors.New("image index contains no inner manifests")
	}

	var mediaTypes map[types.MediaType]struct{}
	for _, manifest := range indexManifest.Manifests {
		mediaTypes[manifest.MediaType] = struct{}{}
	}

	if len(mediaTypes) != 1 {
		return manifestMetadata{}, errors.Errorf("image index contained mixed inner manifest types: %s", maps.Keys(mediaTypes))
	}

	mediaType := maps.Keys(mediaTypes)[0]
	switch mediaType {
	case types.OCIImageIndex:
		platformInfo := indexManifest.Manifests[0].Platform != nil
		return manifestMetadata{mediaType: mediaType, containsPlatformInfo: platformInfo}, nil
	case types.OCIManifestSchema1:
		return manifestMetadata{mediaType: mediaType, containsPlatformInfo: false}, nil
	default:
		return manifestMetadata{}, errors.Errorf("unexpected media type %s", mediaType)
	}
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
