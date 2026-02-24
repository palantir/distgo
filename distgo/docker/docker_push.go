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
	"os"
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

// pushResult captures the digest, size, and media type of a pushed image or
// index so that attestations can be associated with the correct subject.
type pushResult struct {
	digest    v1.Hash
	size      int64
	mediaType types.MediaType
}

func PushProducts(projectInfo distgo.ProjectInfo, projectParam distgo.ProjectParam, productDockerIDs []distgo.ProductDockerID, tagKeys []string, dryRun bool, insecure bool, stdout io.Writer) error {
	// determine products that match specified productDockerIDs
	productParams, err := distgo.ProductParamsForDockerProductArgs(projectParam.Products, productDockerIDs...)
	if err != nil {
		return err
	}
	productParams = distgo.ProductParamsForDockerTagKeys(productParams, tagKeys)

	// run push only for specified products
	for _, currParam := range productParams {
		if err := RunPush(projectInfo, currParam, dryRun, insecure, stdout); err != nil {
			return err
		}
	}
	return nil
}

func RunPush(projectInfo distgo.ProjectInfo, productParam distgo.ProductParam, dryRun bool, insecure bool, stdout io.Writer) error {
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
			insecure,
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
	insecure bool,
	stdout io.Writer) (rErr error) {

	// if an OCI artifact exists, push that. Otherwise, default to pushing the artifact in the docker daemon
	if _, err := layout.FromPath(productTaskOutputInfo.ProductDockerOCIDistOutputDir(dockerID)); err == nil {
		return runOCIPush(productID, dockerID, productTaskOutputInfo, dryRun, insecure, stdout)
	}
	return runDockerDaemonPush(productID, dockerID, productTaskOutputInfo, dryRun, insecure, stdout)
}

// ociPushResult captures the outcome of pushing an OCI artifact.
// When an image index is pushed, pushedIndex is set so that VEX attestations
// can be attached to each child manifest (Trivy resolves platform-specific
// images within an index and looks for attestations on those digests).
type ociPushResult struct {
	result pushResult
	// pushedIndex is non-nil when an image index was pushed.
	pushedIndex v1.ImageIndex
}

func runOCIPush(productID distgo.ProductID, dockerID distgo.DockerID, productTaskOutputInfo distgo.ProductTaskOutputInfo, dryRun bool, insecure bool, stdout io.Writer) error {
	index, err := layout.ImageIndexFromPath(productTaskOutputInfo.ProductDockerOCIDistOutputDir(dockerID))
	if err != nil {
		return errors.Wrapf(err, "failed to construct image index from OCI layout at path %s", productTaskOutputInfo.ProductDockerOCIDistOutputDir(dockerID))
	}

	tags := productTaskOutputInfo.Product.DockerOutputInfos.DockerBuilderOutputInfos[dockerID].RenderedTags
	var firstOCIResult ociPushResult
	var firstRef name.Reference

	for i, tag := range tags {
		var opts []name.Option
		if insecure {
			opts = append(opts, name.Insecure)
		}
		ref, err := name.ParseReference(tag, opts...)
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

		var ociResult ociPushResult
		switch idxManifest.MediaType {
		case types.OCIImageIndex:
			ociResult, err = handleImageIndex(index, idxManifest, ref, productID, dockerID, dryRun, stdout)
			if err != nil {
				return errors.Wrapf(err, "failed to publish image index for configuration %s for product %s", dockerID, productID)
			}
		case types.OCIManifestSchema1:
			result, err := handleImageManifest(ref, productID, dockerID, productTaskOutputInfo, dryRun, stdout)
			if err != nil {
				return errors.Wrapf(err, "failed to image manifest for configuration %s for product %s", dockerID, productID)
			}
			ociResult = ociPushResult{result: result}
		default:
			return errors.Errorf("unexpected media type %s for configuration %s for product %s", idxManifest.MediaType, dockerID, productID)
		}

		if i == 0 {
			firstOCIResult = ociResult
			firstRef = ref
		}
	}

	// Attach VEX attestation if a VEX file was produced by vulncheck.
	vexPath := productTaskOutputInfo.ProductVulncheckVEXPath()
	if _, err := os.Stat(vexPath); err == nil && firstRef != nil {
		if err := attachVEXAttestations(firstRef, firstOCIResult, vexPath, dryRun, insecure, stdout); err != nil {
			return errors.Wrapf(err, "failed to attach VEX attestation for configuration %s of product %s", dockerID, productID)
		}
	} else if os.IsNotExist(err) {
		distgo.PrintlnOrDryRunPrintln(stdout, fmt.Sprintf("No VEX file found at %s for configuration %s of product %s; skipping attestation", vexPath, dockerID, productID), dryRun)
	}

	return nil
}

// attachVEXAttestations attaches VEX attestations to the pushed artifact. When
// an image index was pushed, the attestation is attached to each platform image
// manifest individually because Trivy resolves the platform-specific image
// within an index and looks for attestations on that image's digest. Pushes
// are done concurrently to reduce wall-clock time.
func attachVEXAttestations(ref name.Reference, ociResult ociPushResult, vexPath string, dryRun bool, insecure bool, stdout io.Writer) error {
	if ociResult.pushedIndex == nil {
		// Single image — attach directly.
		return attachVEXAttestation(ref, ociResult.result.digest, vexPath, dryRun, insecure, stdout)
	}

	// Image index — attach to each platform image manifest so Trivy can
	// discover the attestation after resolving to a platform-specific image.
	// Skip non-platform entries (e.g. Docker buildx attestation manifests).
	idxManifest, err := ociResult.pushedIndex.IndexManifest()
	if err != nil {
		return errors.Wrap(err, "failed to read pushed index manifest for attestation")
	}

	var platformDescs []v1.Descriptor
	for _, desc := range idxManifest.Manifests {
		if desc.Annotations != nil {
			if _, ok := desc.Annotations["vnd.docker.reference.type"]; ok {
				continue
			}
		}
		platformDescs = append(platformDescs, desc)
	}

	errs := make(chan error, len(platformDescs))
	for _, desc := range platformDescs {
		go func(d v1.Descriptor) {
			errs <- attachVEXAttestation(ref, d.Digest, vexPath, dryRun, insecure, stdout)
		}(desc)
	}
	for range platformDescs {
		if err := <-errs; err != nil {
			return err
		}
	}
	return nil
}

func handleImageIndex(index v1.ImageIndex, idxManifest *v1.IndexManifest, ref name.Reference, productID distgo.ProductID, dockerID distgo.DockerID, dryRun bool, stdout io.Writer) (ociPushResult, error) {
	manifestMetadata, err := manifestMetadataFromIndexManifest(idxManifest)
	if err != nil {
		return ociPushResult{}, errors.Wrap(err, "encountered unexpected index manifest state")
	}

	switch manifestMetadata.mediaType {
	case types.OCIImageIndex:
		// if we have an image index, go one level down and push that
		innerIndex, err := index.ImageIndex(manifestMetadata.digest)
		if err != nil {
			return ociPushResult{}, errors.Wrapf(err, "failed to read image index digest %s from OCI layout", manifestMetadata.digest)
		}
		result, err := writeIndex(innerIndex, ref, productID, dockerID, dryRun, stdout)
		if err != nil {
			return ociPushResult{}, errors.Wrapf(err, "failed to write image index for tag %s of configuration %s for product %s", ref, dockerID, productID)
		}
		return ociPushResult{result: result, pushedIndex: innerIndex}, nil
	case types.OCIManifestSchema1:
		if manifestMetadata.hasPlatformInfo {
			// if we have platform information, we should push our current image index
			result, err := writeIndex(index, ref, productID, dockerID, dryRun, stdout)
			if err != nil {
				return ociPushResult{}, errors.Wrapf(err, "failed to write image index for tag %s of configuration %s for product %s", ref, dockerID, productID)
			}
			return ociPushResult{result: result, pushedIndex: index}, nil
		}

		if len(idxManifest.Manifests) != 1 {
			return ociPushResult{}, errors.New("unexpected number of image manifests present in image index without platform information")
		}
		image, err := index.Image(manifestMetadata.digest)
		if err != nil {
			return ociPushResult{}, errors.Wrapf(err, "failed to read image digest %s from OCI layout", manifestMetadata.digest)
		}
		result, err := writeImage(image, ref, productID, dockerID, dryRun, stdout)
		if err != nil {
			return ociPushResult{}, errors.Wrapf(err, "failed to write image for tag %s of configuration %s for product %s", ref, dockerID, productID)
		}
		return ociPushResult{result: result}, nil
	default:
		return ociPushResult{}, errors.Errorf("unexpected media type %s for configuration %s for product %s", idxManifest.MediaType, dockerID, productID)
	}
}

func handleImageManifest(ref name.Reference, productID distgo.ProductID, dockerID distgo.DockerID, productTaskOutputInfo distgo.ProductTaskOutputInfo, dryRun bool, stdout io.Writer) (pushResult, error) {
	path := filepath.Join(productTaskOutputInfo.ProductDockerOCIDistOutputDir(dockerID), "image.tar")
	image, err := tarball.ImageFromPath(path, nil)
	if err != nil {
		return pushResult{}, errors.Wrapf(err, "failed to read image from path %s", path)
	}
	result, err := writeImage(image, ref, productID, dockerID, dryRun, stdout)
	if err != nil {
		return pushResult{}, errors.Wrapf(err, "failed to write image for tag %s of configuration %s for product %s", ref, dockerID, productID)
	}
	return result, nil
}

func writeIndex(index v1.ImageIndex, ref name.Reference, productID distgo.ProductID, dockerID distgo.DockerID, dryRun bool, stdout io.Writer) (pushResult, error) {
	distgo.PrintlnOrDryRunPrintln(stdout, fmt.Sprintf("Writing image index for tag %s of docker configuration %s of product %s...", ref, dockerID, productID), dryRun)

	digest, err := index.Digest()
	if err != nil {
		return pushResult{}, errors.Wrap(err, "failed to compute index digest")
	}
	manifest, err := index.RawManifest()
	if err != nil {
		return pushResult{}, errors.Wrap(err, "failed to get index manifest")
	}
	mt, err := index.MediaType()
	if err != nil {
		return pushResult{}, errors.Wrap(err, "failed to get index media type")
	}

	if !dryRun {
		if err := remote.WriteIndex(ref, index, remote.WithAuthFromKeychain(authn.DefaultKeychain)); err != nil {
			return pushResult{}, errors.Wrap(err, "failed to write image index to remote")
		}
	}
	return pushResult{
		digest:    digest,
		size:      int64(len(manifest)),
		mediaType: mt,
	}, nil
}

func writeImage(image v1.Image, ref name.Reference, productID distgo.ProductID, dockerID distgo.DockerID, dryRun bool, stdout io.Writer) (pushResult, error) {
	distgo.PrintlnOrDryRunPrintln(stdout, fmt.Sprintf("Writing image for tag %s of docker configuration %s of product %s...", ref, dockerID, productID), dryRun)

	digest, err := image.Digest()
	if err != nil {
		return pushResult{}, errors.Wrap(err, "failed to compute image digest")
	}
	manifest, err := image.RawManifest()
	if err != nil {
		return pushResult{}, errors.Wrap(err, "failed to get image manifest")
	}
	mt, err := image.MediaType()
	if err != nil {
		return pushResult{}, errors.Wrap(err, "failed to get image media type")
	}

	if !dryRun {
		if err := remote.Write(ref, image, remote.WithAuthFromKeychain(authn.DefaultKeychain)); err != nil {
			return pushResult{}, errors.Wrap(err, "failed to write image to remote")
		}
	}
	return pushResult{
		digest:    digest,
		size:      int64(len(manifest)),
		mediaType: mt,
	}, nil
}

type manifestMetadata struct {
	digest          v1.Hash
	mediaType       types.MediaType
	hasPlatformInfo bool
}

func manifestMetadataFromIndexManifest(indexManifest *v1.IndexManifest) (manifestMetadata, error) {
	if len(indexManifest.Manifests) == 0 {
		return manifestMetadata{}, errors.New("image index contains no inner manifests")
	}

	mediaTypes := make(map[types.MediaType]struct{})
	for _, manifest := range indexManifest.Manifests {
		mediaTypes[manifest.MediaType] = struct{}{}
	}

	if len(mediaTypes) != 1 {
		return manifestMetadata{}, errors.Errorf("image index contained mixed inner manifest types: %s", maps.Keys(mediaTypes))
	}

	mediaType := maps.Keys(mediaTypes)[0]
	digest := indexManifest.Manifests[0].Digest
	switch mediaType {
	case types.OCIImageIndex:
		return manifestMetadata{digest: digest, mediaType: mediaType, hasPlatformInfo: false}, nil
	case types.OCIManifestSchema1:
		platformInfo := indexManifest.Manifests[0].Platform != nil
		return manifestMetadata{digest: digest, mediaType: mediaType, hasPlatformInfo: platformInfo}, nil
	default:
		return manifestMetadata{}, errors.Errorf("unexpected media type %s", mediaType)
	}
}

func runDockerDaemonPush(
	productID distgo.ProductID,
	dockerID distgo.DockerID,
	productTaskOutputInfo distgo.ProductTaskOutputInfo,
	dryRun bool,
	insecure bool,
	stdout io.Writer,
) error {
	distgo.PrintlnOrDryRunPrintln(stdout, fmt.Sprintf("Running Docker push for configuration %s of product %s...", dockerID, productID), dryRun)

	tags := productTaskOutputInfo.Product.DockerOutputInfos.DockerBuilderOutputInfos[dockerID].RenderedTags
	var firstRef name.Reference

	for i, tag := range tags {
		cmd := exec.Command("docker", "push", tag)
		if err := distgo.RunCommandWithVerboseOption(cmd, true, dryRun, stdout); err != nil {
			return err
		}

		if i == 0 {
			var opts []name.Option
			if insecure {
				opts = append(opts, name.Insecure)
			}
			ref, err := name.ParseReference(tag, opts...)
			if err != nil {
				return errors.Wrapf(err, "failed to parse reference from tag %s", tag)
			}
			firstRef = ref
		}
	}

	// Attach VEX attestation if a VEX file was produced by vulncheck.
	vexPath := productTaskOutputInfo.ProductVulncheckVEXPath()
	if _, err := os.Stat(vexPath); err == nil && firstRef != nil {
		// Resolve the remote descriptor to get the digest for the image that
		// was pushed via the Docker daemon.
		remoteOpts := []remote.Option{remote.WithAuthFromKeychain(authn.DefaultKeychain)}
		desc, err := remote.Head(firstRef, remoteOpts...)
		if err != nil && !dryRun {
			return errors.Wrapf(err, "failed to resolve remote digest for %s", firstRef)
		}
		if desc != nil {
			if err := attachVEXAttestation(firstRef, desc.Digest, vexPath, dryRun, insecure, stdout); err != nil {
				return errors.Wrapf(err, "failed to attach VEX attestation for configuration %s of product %s", dockerID, productID)
			}
		}
	} else if os.IsNotExist(err) {
		distgo.PrintlnOrDryRunPrintln(stdout, fmt.Sprintf("No VEX file found at %s for configuration %s of product %s; skipping attestation", vexPath, dockerID, productID), dryRun)
	}

	return nil
}
