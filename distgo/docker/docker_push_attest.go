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
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/palantir/distgo/distgo"
	"github.com/pkg/errors"
)

const (
	inTotoPayloadType    = "application/vnd.in-toto+json"
	inTotoStatementType  = "https://in-toto.io/Statement/v1"
	openVEXPredicateType = "https://openvex.dev/ns"
)

// dsseEnvelope is a Dead Simple Signing Envelope as defined by
// https://github.com/secure-systems-lab/dsse. This matches the schema used
// by go-securesystemslib/dsse.Envelope; we define our own struct to avoid
// adding a dependency just for JSON serialization.
type dsseEnvelope struct {
	PayloadType string          `json:"payloadType"`
	Payload     string          `json:"payload"`
	Signatures  []dsseSignature `json:"signatures"`
}

type dsseSignature struct {
	KeyID string `json:"keyid"`
	Sig   string `json:"sig"`
}

// inTotoStatement is an in-toto Statement v1 as defined by
// https://github.com/in-toto/attestation/blob/main/spec/v1/statement.md.
type inTotoStatement struct {
	Type          string          `json:"_type"`
	Subject       []inTotoSubject `json:"subject"`
	PredicateType string          `json:"predicateType"`
	Predicate     json.RawMessage `json:"predicate"`
}

type inTotoSubject struct {
	Name   string            `json:"name"`
	Digest map[string]string `json:"digest"`
}

// attachVEXAttestation reads a VEX document from vexPath, wraps it in an
// unsigned DSSE envelope containing an in-toto Statement, builds an OCI
// attestation image, and pushes it to the same repository as ref using the
// OCI referrers API (via the manifest Subject field).
func attachVEXAttestation(
	ref name.Reference,
	subjectDigest v1.Hash,
	subjectSize int64,
	subjectMediaType types.MediaType,
	vexPath string,
	dryRun bool,
	insecure bool,
	stdout io.Writer,
) error {
	distgo.PrintlnOrDryRunPrintln(stdout, fmt.Sprintf("Attaching VEX attestation from %s to %s...", vexPath, ref), dryRun)
	if dryRun {
		return nil
	}

	vexBytes, err := os.ReadFile(vexPath)
	if err != nil {
		return errors.Wrapf(err, "failed to read VEX file %s", vexPath)
	}

	dsseBytes, err := buildDSSEEnvelope(ref.String(), subjectDigest, vexBytes)
	if err != nil {
		return errors.Wrap(err, "failed to build DSSE envelope")
	}

	subjectDesc := v1.Descriptor{
		Digest:    subjectDigest,
		Size:      subjectSize,
		MediaType: subjectMediaType,
	}

	attestImg, err := buildAttestationImage(subjectDesc, dsseBytes)
	if err != nil {
		return errors.Wrap(err, "failed to build attestation image")
	}

	var nameOpts []name.Option
	if insecure {
		nameOpts = append(nameOpts, name.Insecure)
	}

	// Push using the subject digest as the tag to associate via referrers.
	// The Subject field in the manifest handles the OCI 1.1 referrers API
	// association; we push by digest so we don't clobber any existing tags.
	attestDigest, err := attestImg.Digest()
	if err != nil {
		return errors.Wrap(err, "failed to compute attestation image digest")
	}
	digestRef, err := name.NewDigest(ref.Context().String()+"@"+attestDigest.String(), nameOpts...)
	if err != nil {
		return errors.Wrapf(err, "failed to create digest reference for attestation")
	}

	if err := remote.Write(digestRef, attestImg, remote.WithAuthFromKeychain(authn.DefaultKeychain)); err != nil {
		return errors.Wrap(err, "failed to push VEX attestation image")
	}

	distgo.PrintlnOrDryRunPrintln(stdout, fmt.Sprintf("Attached VEX attestation %s to %s", attestDigest, ref), false)
	return nil
}

// buildDSSEEnvelope constructs an unsigned DSSE envelope containing an in-toto
// Statement with the OpenVEX document as the predicate.
func buildDSSEEnvelope(subjectRef string, subjectDigest v1.Hash, vexBytes []byte) ([]byte, error) {
	stmt := inTotoStatement{
		Type: inTotoStatementType,
		Subject: []inTotoSubject{
			{
				Name: subjectRef,
				Digest: map[string]string{
					subjectDigest.Algorithm: subjectDigest.Hex,
				},
			},
		},
		PredicateType: openVEXPredicateType,
		Predicate:     json.RawMessage(vexBytes),
	}

	stmtBytes, err := json.Marshal(stmt)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal in-toto statement")
	}

	env := dsseEnvelope{
		PayloadType: inTotoPayloadType,
		Payload:     base64.StdEncoding.EncodeToString(stmtBytes),
		Signatures:  []dsseSignature{},
	}

	envBytes, err := json.Marshal(env)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal DSSE envelope")
	}
	return envBytes, nil
}

// buildAttestationImage constructs an OCI image that stores a DSSE envelope
// as a single layer, suitable for discovery via the OCI referrers API. The
// image has:
//   - Manifest media type: application/vnd.oci.image.manifest.v1+json
//   - Config media type: application/vnd.in-toto+json
//   - Subject descriptor pointing to the attested image
//   - Single layer with media type application/vnd.in-toto+json
func buildAttestationImage(subjectDesc v1.Descriptor, dsseBytes []byte) (v1.Image, error) {
	img := empty.Image

	// Set manifest media type to OCI manifest.
	img = mutate.MediaType(img, types.OCIManifestSchema1)

	// Set config media type to in-toto payload type.
	img = mutate.ConfigMediaType(img, types.MediaType(inTotoPayloadType))

	// Add the DSSE envelope as a single layer.
	layer := newStaticLayer(dsseBytes, types.MediaType(inTotoPayloadType))
	var err error
	img, err = mutate.AppendLayers(img, layer)
	if err != nil {
		return nil, errors.Wrap(err, "failed to append attestation layer")
	}

	// Set the subject descriptor to associate with the attested image.
	result := mutate.Subject(img, subjectDesc)
	attestImg, ok := result.(v1.Image)
	if !ok {
		return nil, errors.New("mutate.Subject did not return a v1.Image")
	}

	return attestImg, nil
}

// staticLayer is a minimal v1.Layer implementation for raw (non-tarball)
// byte content. go-containerregistry/pkg/v1/static is not vendored, so we
// provide our own implementation.
type staticLayer struct {
	content   []byte
	mediaType types.MediaType
}

func newStaticLayer(content []byte, mediaType types.MediaType) *staticLayer {
	return &staticLayer{
		content:   content,
		mediaType: mediaType,
	}
}

func (l *staticLayer) Digest() (v1.Hash, error) {
	h := sha256.Sum256(l.content)
	return v1.Hash{
		Algorithm: "sha256",
		Hex:       hex.EncodeToString(h[:]),
	}, nil
}

func (l *staticLayer) DiffID() (v1.Hash, error) {
	return l.Digest()
}

func (l *staticLayer) Compressed() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(l.content)), nil
}

func (l *staticLayer) Uncompressed() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(l.content)), nil
}

func (l *staticLayer) Size() (int64, error) {
	return int64(len(l.content)), nil
}

func (l *staticLayer) MediaType() (types.MediaType, error) {
	return l.mediaType, nil
}
