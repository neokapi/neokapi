//go:build !js

package registry

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"testing"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/testing/ca"
	"github.com/sigstore/sigstore-go/pkg/tlog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildLegacyCosignBundle signs artifact with the in-process VirtualSigstore
// and assembles the result into the JSON shape that `cosign sign-blob --bundle`
// (the legacy, non-`--new-bundle-format` output) writes:
//
//	{"base64Signature": …, "cert": <base64 PEM>, "rekorBundle": {…}}
//
// It returns the legacy bundle JSON and the artifact's SHA-256 digest.
func buildLegacyCosignBundle(t *testing.T, vs *ca.VirtualSigstore, identity, issuer string, artifact []byte) ([]byte, [32]byte) {
	t.Helper()

	entity, err := vs.Sign(identity, issuer, artifact)
	require.NoError(t, err)

	// Raw artifact signature.
	sc, err := entity.SignatureContent()
	require.NoError(t, err)
	ms, ok := sc.(*bundle.MessageSignature)
	require.True(t, ok, "expected a message-signature entity")
	sigB64 := base64.StdEncoding.EncodeToString(ms.Signature())

	// Signing certificate as base64-encoded PEM (cosign's legacy "cert" field).
	vc, err := entity.VerificationContent()
	require.NoError(t, err)
	cert, ok := vc.(*bundle.Certificate)
	require.True(t, ok, "expected a certificate verification content")
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate().Raw})
	certB64 := base64.StdEncoding.EncodeToString(pemBytes)

	// Transparency-log entry body + metadata.
	entries, err := entity.TlogEntries()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	tle := entries[0].TransparencyLogEntry()
	bodyB64 := base64.StdEncoding.EncodeToString(tle.CanonicalizedBody)
	logIDHex := hex.EncodeToString(tle.LogId.KeyId)

	// Recompute the SET (inclusion promise) over the Rekor payload exactly as
	// the log would have. This mirrors ca.createRekorBundle: Body is the
	// base64-encoded canonicalized body.
	set, err := vs.RekorSignPayload(tlog.RekorPayload{
		Body:           bodyB64,
		IntegratedTime: tle.IntegratedTime,
		LogIndex:       tle.LogIndex,
		LogID:          logIDHex,
	})
	require.NoError(t, err)
	setB64 := base64.StdEncoding.EncodeToString(set)

	legacy := map[string]any{
		"base64Signature": sigB64,
		"cert":            certB64,
		"rekorBundle": map[string]any{
			"SignedEntryTimestamp": setB64,
			"Payload": map[string]any{
				"body":           bodyB64,
				"integratedTime": tle.IntegratedTime,
				"logIndex":       tle.LogIndex,
				"logID":          logIDHex,
			},
		},
	}
	data, err := json.Marshal(legacy)
	require.NoError(t, err)

	return data, sha256.Sum256(artifact)
}

// TestCosignLegacyBundle_Success verifies that a legacy `cosign sign-blob
// --bundle` document — the format some already-released artifacts (e.g. early
// kapi-sat builds) ship — is converted and verifies successfully through the
// same policy as a new-format Sigstore bundle.
func TestCosignLegacyBundle_Success(t *testing.T) {
	vs, err := ca.NewVirtualSigstore()
	require.NoError(t, err)

	const (
		identity = "release-bot@example.com"
		issuer   = "https://accounts.example.com"
	)
	artifact := []byte("kapi-sat-plugin-tarball-contents")

	legacyData, digest := buildLegacyCosignBundle(t, vs, identity, issuer, artifact)

	require.True(t, looksLikeLegacyCosignBundle(legacyData), "should be detected as a legacy cosign bundle")

	b, err := convertLegacyCosignBundle(legacyData, digest[:])
	require.NoError(t, err)

	err = verifyEntity(b, vs, digest[:], identity, issuer, true /* skipSCT */)
	assert.NoError(t, err)
}

// TestCosignLegacyBundle_TamperedSignature ensures a legacy bundle whose
// artifact signature has been altered fails verification.
func TestCosignLegacyBundle_TamperedSignature(t *testing.T) {
	vs, err := ca.NewVirtualSigstore()
	require.NoError(t, err)

	const (
		identity = "release-bot@example.com"
		issuer   = "https://accounts.example.com"
	)
	artifact := []byte("kapi-sat-plugin-tarball-contents")

	legacyData, digest := buildLegacyCosignBundle(t, vs, identity, issuer, artifact)

	// Flip a byte in the decoded signature, keeping it valid base64 of the
	// same length so it survives decoding and fails at signature verification.
	var legacy legacyCosignBundle
	require.NoError(t, json.Unmarshal(legacyData, &legacy))
	sig, err := base64.StdEncoding.DecodeString(legacy.Base64Signature)
	require.NoError(t, err)
	require.NotEmpty(t, sig)
	sig[len(sig)-1] ^= 0xff
	legacy.Base64Signature = base64.StdEncoding.EncodeToString(sig)
	tampered, err := json.Marshal(legacy)
	require.NoError(t, err)

	b, err := convertLegacyCosignBundle(tampered, digest[:])
	require.NoError(t, err) // structurally valid; signature is wrong

	err = verifyEntity(b, vs, digest[:], identity, issuer, true /* skipSCT */)
	require.Error(t, err)
	var sve *SignatureVerificationError
	require.ErrorAs(t, err, &sve)
}

// TestCosignLegacyBundle_WrongDigest ensures the converted bundle is bound to
// the artifact digest: a mismatching digest fails verification.
func TestCosignLegacyBundle_WrongDigest(t *testing.T) {
	vs, err := ca.NewVirtualSigstore()
	require.NoError(t, err)

	const (
		identity = "release-bot@example.com"
		issuer   = "https://accounts.example.com"
	)
	artifact := []byte("kapi-sat-plugin-tarball-contents")

	legacyData, _ := buildLegacyCosignBundle(t, vs, identity, issuer, artifact)
	wrongDigest := sha256.Sum256([]byte("a-different-artifact"))

	b, err := convertLegacyCosignBundle(legacyData, wrongDigest[:])
	require.NoError(t, err)

	err = verifyEntity(b, vs, wrongDigest[:], identity, issuer, true /* skipSCT */)
	require.Error(t, err)
	var sve *SignatureVerificationError
	require.ErrorAs(t, err, &sve)
}

// TestLooksLikeLegacyCosignBundle distinguishes legacy from new-format and junk.
func TestLooksLikeLegacyCosignBundle(t *testing.T) {
	t.Parallel()

	assert.True(t, looksLikeLegacyCosignBundle([]byte(`{"base64Signature":"abc","cert":"def"}`)))
	assert.False(t, looksLikeLegacyCosignBundle([]byte(`{"mediaType":"application/vnd.dev.sigstore.bundle.v0.3+json"}`)))
	assert.False(t, looksLikeLegacyCosignBundle([]byte("not json")))
	assert.False(t, looksLikeLegacyCosignBundle([]byte(`{"base64Signature":""}`)))
}
