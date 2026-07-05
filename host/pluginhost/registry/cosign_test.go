package registry

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sigstore/sigstore-go/pkg/testing/ca"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVerifyEntity_Success exercises the success path: an artifact
// signed by the in-process VirtualSigstore verifies against the same
// VirtualSigstore as the trusted root, with a matching cert identity.
func TestVerifyEntity_Success(t *testing.T) {
	vs, err := ca.NewVirtualSigstore()
	require.NoError(t, err)

	const (
		identity = "release-bot@example.com"
		issuer   = "https://accounts.example.com"
	)

	artifact := []byte("hello-cosign-test-artifact")
	digest := sha256.Sum256(artifact)

	entity, err := vs.Sign(identity, issuer, artifact)
	require.NoError(t, err)

	err = verifyEntity(entity, vs, digest[:], identity, issuer, true)
	assert.NoError(t, err)
}

// TestVerifyEntity_WrongCertIdentity ensures we reject signatures
// produced by a different signer than the registry pinned.
func TestVerifyEntity_WrongCertIdentity(t *testing.T) {
	vs, err := ca.NewVirtualSigstore()
	require.NoError(t, err)

	const (
		actualIdentity   = "release-bot@example.com"
		issuer           = "https://accounts.example.com"
		expectedIdentity = "different-bot@example.com"
	)

	artifact := []byte("hello-cosign-test-artifact")
	digest := sha256.Sum256(artifact)

	entity, err := vs.Sign(actualIdentity, issuer, artifact)
	require.NoError(t, err)

	err = verifyEntity(entity, vs, digest[:], expectedIdentity, issuer, true)
	require.Error(t, err)

	var sve *SignatureVerificationError
	require.ErrorAs(t, err, &sve)
	assert.Contains(t, sve.Error(), "Sigstore verification")
}

// TestVerifyEntity_WrongIssuer ensures we reject signatures whose OIDC
// issuer is different from what the registry pinned.
func TestVerifyEntity_WrongIssuer(t *testing.T) {
	vs, err := ca.NewVirtualSigstore()
	require.NoError(t, err)

	const (
		identity       = "release-bot@example.com"
		actualIssuer   = "https://accounts.example.com"
		expectedIssuer = "https://accounts.different.com"
	)

	artifact := []byte("hello-cosign-test-artifact")
	digest := sha256.Sum256(artifact)

	entity, err := vs.Sign(identity, actualIssuer, artifact)
	require.NoError(t, err)

	err = verifyEntity(entity, vs, digest[:], identity, expectedIssuer, true)
	require.Error(t, err)

	var sve *SignatureVerificationError
	require.ErrorAs(t, err, &sve)
}

// TestVerifyEntity_WrongDigest ensures the verifier rejects when the
// supplied artifact digest doesn't match the signed artifact.
func TestVerifyEntity_WrongDigest(t *testing.T) {
	vs, err := ca.NewVirtualSigstore()
	require.NoError(t, err)

	const (
		identity = "release-bot@example.com"
		issuer   = "https://accounts.example.com"
	)

	signedArtifact := []byte("hello-cosign-test-artifact")
	otherArtifact := []byte("a-completely-different-artifact")
	otherDigest := sha256.Sum256(otherArtifact)

	entity, err := vs.Sign(identity, issuer, signedArtifact)
	require.NoError(t, err)

	err = verifyEntity(entity, vs, otherDigest[:], identity, issuer, true)
	require.Error(t, err)

	var sve *SignatureVerificationError
	require.ErrorAs(t, err, &sve)
}

// TestVerifyEntity_DifferentTrustedRoot ensures a bundle signed by one
// CA cannot be verified against an unrelated trusted root.
func TestVerifyEntity_DifferentTrustedRoot(t *testing.T) {
	vsSigner, err := ca.NewVirtualSigstore()
	require.NoError(t, err)
	vsTrusted, err := ca.NewVirtualSigstore()
	require.NoError(t, err)

	const (
		identity = "release-bot@example.com"
		issuer   = "https://accounts.example.com"
	)

	artifact := []byte("hello-cosign-test-artifact")
	digest := sha256.Sum256(artifact)

	entity, err := vsSigner.Sign(identity, issuer, artifact)
	require.NoError(t, err)

	err = verifyEntity(entity, vsTrusted, digest[:], identity, issuer, true)
	require.Error(t, err)
}

// TestVerifyBundle_MissingFields covers the user-facing input checks in
// the public VerifyBundle entry point.
func TestVerifyBundle_MissingFields(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		bundleURL    string
		sha256Hex    string
		certIdentity string
		certIssuer   string
		wantContains string
	}{
		{
			name:         "no signature URL",
			bundleURL:    "",
			sha256Hex:    strings.Repeat("0", 64),
			certIdentity: "x@y",
			certIssuer:   "https://issuer",
			wantContains: "no signature URL",
		},
		{
			name:         "no cert_identity",
			bundleURL:    "https://example.com/sig",
			sha256Hex:    strings.Repeat("0", 64),
			certIdentity: "",
			certIssuer:   "https://issuer",
			wantContains: "no cert_identity",
		},
		{
			name:         "no cert_oidc_issuer",
			bundleURL:    "https://example.com/sig",
			sha256Hex:    strings.Repeat("0", 64),
			certIdentity: "x@y",
			certIssuer:   "",
			wantContains: "no cert_oidc_issuer",
		},
		{
			name:         "no sha256",
			bundleURL:    "https://example.com/sig",
			sha256Hex:    "",
			certIdentity: "x@y",
			certIssuer:   "https://issuer",
			wantContains: "no sha256",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := VerifyBundle(t.Context(), tc.bundleURL, tc.sha256Hex, tc.certIdentity, tc.certIssuer, CosignVerifyOptions{})
			require.Error(t, err)
			var sve *SignatureVerificationError
			require.ErrorAs(t, err, &sve)
			assert.Contains(t, err.Error(), tc.wantContains)
		})
	}
}

// TestDecodeSHA256Hex covers the small hex helper.
func TestDecodeSHA256Hex(t *testing.T) {
	t.Parallel()

	want := make([]byte, 32)
	for i := range want {
		want[i] = byte(i)
	}
	gotHex := hex.EncodeToString(want)

	got, err := decodeSHA256Hex(gotHex)
	require.NoError(t, err)
	assert.Equal(t, want, got)

	// Mixed case OK.
	got2, err := decodeSHA256Hex(strings.ToUpper(gotHex))
	require.NoError(t, err)
	assert.Equal(t, want, got2)

	// Wrong length.
	_, err = decodeSHA256Hex("abcd")
	require.Error(t, err)

	// Bad hex.
	_, err = decodeSHA256Hex(strings.Repeat("z", 64))
	require.Error(t, err)
}

// TestParseBundle_RejectsGarbage ensures we don't silently accept
// non-bundle JSON or arbitrary bytes.
func TestParseBundle_RejectsGarbage(t *testing.T) {
	t.Parallel()

	cases := [][]byte{
		[]byte(""),                                // empty
		[]byte("not even json"),                   // garbage
		[]byte(`{"hello":"world"}`),               // wrong shape
		[]byte(`{"mediaType":"application/foo"}`), // wrong media type
	}
	for i, data := range cases {
		_, err := parseBundle(data)
		require.Errorf(t, err, "case %d: expected error parsing %q", i, data)
	}
}

// TestVerifyBundle_HTTPNotFound exercises the HTTP path: a 404 from the
// signature URL surfaces as a SignatureVerificationError with the
// download reason.
func TestVerifyBundle_HTTPNotFound(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	defer srv.Close()

	vs, err := ca.NewVirtualSigstore()
	require.NoError(t, err)

	digest := strings.Repeat("ab", 32)
	err = VerifyBundle(t.Context(), srv.URL+"/sig.json", digest, "x@y", "https://issuer", CosignVerifyOptions{
		HTTPClient:      srv.Client(),
		TrustedMaterial: vs,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "download signature")
	assert.Contains(t, err.Error(), "HTTP 404")
}

// TestVerifyBundle_HTTPGarbageBody exercises the parse path: HTTP 200
// but the body isn't a valid bundle.
func TestVerifyBundle_HTTPGarbageBody(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not-a-bundle"))
	}))
	defer srv.Close()

	vs, err := ca.NewVirtualSigstore()
	require.NoError(t, err)

	digest := strings.Repeat("ab", 32)
	err = VerifyBundle(t.Context(), srv.URL+"/sig.json", digest, "x@y", "https://issuer", CosignVerifyOptions{
		HTTPClient:      srv.Client(),
		TrustedMaterial: vs,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse signature bundle")
}

// TestSignatureVerificationError_Unwrap checks the error chain behaves.
func TestSignatureVerificationError_Unwrap(t *testing.T) {
	inner := errors.New("inner cause")
	sve := &SignatureVerificationError{Reason: "wrapped", Err: inner}
	require.ErrorIs(t, sve, inner)
	assert.Contains(t, sve.Error(), "wrapped")
	assert.Contains(t, sve.Error(), "inner cause")

	bare := &SignatureVerificationError{Reason: "bare"}
	assert.Contains(t, bare.Error(), "bare")
	require.NoError(t, bare.Unwrap())
}
