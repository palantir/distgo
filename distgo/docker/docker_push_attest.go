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
// unsigned DSSE envelope containing an in-toto Statement, and pushes it using
// cosign's tag-based attestation scheme (sha256-<hex>.att). This is the
// discovery mechanism used by Trivy's --vex oci via the openvex/discovery
// library, which calls cosign.FetchAttestations internally.
//
// If an existing attestation image already exists at the tag (e.g. from a
// previous cosign attest), the new layer is appended to preserve existing
// attestations.
func attachVEXAttestation(
	ref name.Reference,
	subjectDigest v1.Hash,
	vexPath string,
	dryRun bool,
	insecure bool,
	stdout io.Writer,
) error {
	// Build the cosign-compatible .att tag for this subject digest.
	var nameOpts []name.Option
	if insecure {
		nameOpts = append(nameOpts, name.Insecure)
	}
	attTag, err := name.NewTag(
		fmt.Sprintf("%s:%s-%s.att", ref.Context().String(), subjectDigest.Algorithm, subjectDigest.Hex),
		nameOpts...,
	)
	if err != nil {
		return errors.Wrap(err, "failed to create attestation tag")
	}

	distgo.PrintlnOrDryRunPrintln(stdout, fmt.Sprintf("Attaching VEX attestation from %s to %s...", vexPath, attTag), dryRun)
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

	layer := newStaticLayer(dsseBytes, types.MediaType("application/vnd.dsse.envelope.v1+json"))

	remoteOpts := []remote.Option{remote.WithAuthFromKeychain(authn.DefaultKeychain)}

	// Try to fetch an existing attestation image at this tag so we can
	// append rather than replace (preserves attestations from cosign, etc.).
	var attestImg v1.Image
	existingImg, err := remote.Image(attTag, remoteOpts...)
	if err == nil {
		attestImg, err = mutate.AppendLayers(existingImg, layer)
		if err != nil {
			return errors.Wrap(err, "failed to append attestation layer to existing image")
		}
	} else {
		img := empty.Image
		img = mutate.MediaType(img, types.OCIManifestSchema1)
		attestImg, err = mutate.AppendLayers(img, layer)
		if err != nil {
			return errors.Wrap(err, "failed to create attestation image")
		}
	}

	if err := remote.Write(attTag, attestImg, remoteOpts...); err != nil {
		return errors.Wrap(err, "failed to push VEX attestation image")
	}

	distgo.PrintlnOrDryRunPrintln(stdout, fmt.Sprintf("Attached VEX attestation to %s", attTag), false)
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
