//go:build !js

// Cosign / Sigstore signature verification for plugin tarballs.
//
// Plugins published to the registry are signed with cosign keyless
// (Sigstore Fulcio + Rekor) by the GitHub Actions release workflow.
// The registry pins, per-platform:
//
//   - signature        — URL to a Sigstore bundle (artifact.sigstore.json)
//                        containing the signature, signing certificate,
//                        Rekor entry, and (optionally) RFC 3161 timestamps
//   - cert_identity    — expected SAN on the signing certificate
//                        (typically a GitHub Actions workflow URL)
//   - cert_oidc_issuer — expected OIDC issuer
//                        (e.g. https://token.actions.githubusercontent.com)
//
// VerifyBundle ties a downloaded plugin tarball to the exact workflow
// that produced it. It uses sigstore-go (the lighter-weight verification
// library) rather than pulling in the full cosign CLI, which would
// drag in a much larger dep graph.
//
// We support a single transport: the Sigstore JSON bundle. Newer cosign
// (v2.2+) emits a `*.sigstore.json` bundle by default that carries both
// the signing cert and the signature in one document — much simpler to
// distribute than a `.sig` + `.pem` pair. If a publisher has only the
// legacy split files they can wrap them into a bundle at release time.

package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/tuf"
	"github.com/sigstore/sigstore-go/pkg/verify"
)

// SignatureVerificationError is returned when a downloaded plugin
// tarball fails Sigstore verification. It deliberately wraps the
// underlying error so callers can `errors.Is`/`errors.As` if needed,
// while still surfacing a stable string for users.
type SignatureVerificationError struct {
	Reason string
	Err    error
}

func (e *SignatureVerificationError) Error() string {
	if e.Err == nil {
		return "signature verification failed: " + e.Reason
	}
	return "signature verification failed: " + e.Reason + ": " + e.Err.Error()
}

func (e *SignatureVerificationError) Unwrap() error { return e.Err }

// CosignVerifyOptions configures VerifyBundle. The zero value uses the
// public Sigstore TUF root (production) and fetches it lazily.
type CosignVerifyOptions struct {
	// TrustedMaterial overrides the default Sigstore public-good
	// trusted root. Tests inject a VirtualSigstore here. When nil,
	// VerifyBundle calls fetchPublicGoodTrustedRoot().
	TrustedMaterial root.TrustedMaterial

	// HTTPClient overrides the default http.Client used to download
	// the bundle. Tests inject a stub here.
	HTTPClient *http.Client

	// skipSCT, set only by tests, drops the "Signed Certificate
	// Timestamps" verifier requirement. The in-process VirtualSigstore
	// CA used in unit tests does not embed SCTs in leaf certs, but
	// real cosign / Fulcio always does — so we keep SCT mandatory in
	// production.
	skipSCT bool
}

// VerifyBundle verifies that bundleURL describes a Sigstore bundle that
// signs the artifact whose SHA-256 is artifactSHA256Hex, and that the
// signing certificate matches certIdentity / certIssuer.
//
//   - artifactSHA256Hex is the lowercase hex SHA-256 of the *raw* tarball
//     bytes — the same value VerifySHA256 checks against the registry hash.
//   - certIdentity is matched as the SAN on the signing certificate.
//   - certIssuer is matched as the OIDC issuer extension.
//
// On success, returns nil. On any failure (download, parse, mismatch,
// expired cert, missing TLog entry, …) returns a *SignatureVerificationError.
func VerifyBundle(ctx context.Context, bundleURL, artifactSHA256Hex, certIdentity, certIssuer string, opts CosignVerifyOptions) error {
	if bundleURL == "" {
		return &SignatureVerificationError{Reason: "no signature URL in registry entry"}
	}
	if certIdentity == "" {
		return &SignatureVerificationError{Reason: "no cert_identity in registry entry"}
	}
	if certIssuer == "" {
		return &SignatureVerificationError{Reason: "no cert_oidc_issuer in registry entry"}
	}
	if artifactSHA256Hex == "" {
		return &SignatureVerificationError{Reason: "no sha256 to bind signature to"}
	}

	digest, err := decodeSHA256Hex(artifactSHA256Hex)
	if err != nil {
		return &SignatureVerificationError{Reason: "decode sha256", Err: err}
	}

	// Fetch the bundle.
	client := opts.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	bundleData, err := downloadBundle(ctx, client, bundleURL)
	if err != nil {
		return &SignatureVerificationError{Reason: "download signature", Err: err}
	}

	b, err := parseBundle(bundleData)
	if err != nil {
		return &SignatureVerificationError{Reason: "parse signature bundle", Err: err}
	}

	// Resolve trusted material.
	trusted := opts.TrustedMaterial
	if trusted == nil {
		trusted, err = fetchPublicGoodTrustedRoot()
		if err != nil {
			return &SignatureVerificationError{Reason: "load Sigstore trusted root", Err: err}
		}
	}

	return verifyEntity(b, trusted, digest, certIdentity, certIssuer, opts.skipSCT)
}

// verifyEntity does the actual sigstore-go verify dance against an
// already-parsed entity (a real bundle in production, a *ca.TestEntity
// in unit tests). Both implement verify.SignedEntity.
//
// Verifier configuration mirrors what cosign verify-blob uses for
// keyless verification:
//
//   - SCT (signed certificate timestamps) threshold 1
//   - Transparency log (Rekor) threshold 1
//   - Either RFC 3161 signed timestamps OR integrated Rekor timestamps
//     (threshold 1 — observer timestamps satisfies whichever is present)
//
// skipSCT is honoured only for tests against the in-process
// VirtualSigstore CA, which does not embed SCTs.
func verifyEntity(entity verify.SignedEntity, trusted root.TrustedMaterial, artifactDigest []byte, certIdentity, certIssuer string, skipSCT bool) error {
	verifierOpts := []verify.VerifierOption{
		verify.WithTransparencyLog(1),
		verify.WithObserverTimestamps(1),
	}
	if !skipSCT {
		verifierOpts = append(verifierOpts, verify.WithSignedCertificateTimestamps(1))
	}
	v, err := verify.NewVerifier(trusted, verifierOpts...)
	if err != nil {
		return &SignatureVerificationError{Reason: "build verifier", Err: err}
	}

	certID, err := verify.NewShortCertificateIdentity(certIssuer, "", certIdentity, "")
	if err != nil {
		return &SignatureVerificationError{Reason: "build cert identity policy", Err: err}
	}

	policy := verify.NewPolicy(
		verify.WithArtifactDigest("sha256", artifactDigest),
		verify.WithCertificateIdentity(certID),
	)
	if _, err := v.Verify(entity, policy); err != nil {
		return &SignatureVerificationError{Reason: "Sigstore verification", Err: err}
	}
	return nil
}

func parseBundle(data []byte) (*bundle.Bundle, error) {
	var b bundle.Bundle
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, err
	}
	return &b, nil
}

func decodeSHA256Hex(s string) ([]byte, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if len(s) != sha256.Size*2 {
		return nil, fmt.Errorf("expected %d-char sha256 hex, got %d", sha256.Size*2, len(s))
	}
	out, err := hex.DecodeString(s)
	if err != nil {
		return nil, err
	}
	return out, nil
}

const maxBundleBytes = 4 * 1024 * 1024 // 4 MiB is generous for a bundle

func downloadBundle(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bundle %s: HTTP %d", url, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBundleBytes+1))
	if err != nil {
		return nil, fmt.Errorf("bundle %s: %w", url, err)
	}
	if len(data) > maxBundleBytes {
		return nil, fmt.Errorf("bundle %s: exceeds %d-byte size limit", url, maxBundleBytes)
	}
	return data, nil
}

// ---- public-good Sigstore trusted root, fetched & cached ----

var (
	trustedRootOnce sync.Once
	trustedRoot     root.TrustedMaterial
	trustedRootErr  error
)

func fetchPublicGoodTrustedRoot() (root.TrustedMaterial, error) {
	trustedRootOnce.Do(func() {
		opts := tuf.DefaultOptions()
		// DefaultOptions picks the Sigstore production TUF repo + cache dir.
		tr, err := root.FetchTrustedRootWithOptions(opts)
		if err != nil {
			trustedRootErr = err
			return
		}
		trustedRoot = tr
	})
	return trustedRoot, trustedRootErr
}
