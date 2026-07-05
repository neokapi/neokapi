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
// We accept two on-the-wire transports for the signature:
//
//   - New-format Sigstore bundle (media type application/vnd.dev.sigstore.bundle…).
//     Newer cosign (v2.2+) emits this with `--new-bundle-format`; it carries the
//     signing cert, signature, and Rekor entry in one self-describing document.
//     This is the format every plugin SHOULD publish going forward.
//
//   - Legacy cosign bundle. `cosign sign-blob --bundle` (without
//     `--new-bundle-format`) emits a JSON object of the shape
//     {"base64Signature": …, "cert": <base64 PEM>, "rekorBundle": {…}}.
//     This predates the Sigstore protobuf bundle and is NOT parseable by
//     sigstore-go's bundle.Bundle (it rejects the unknown "base64Signature"
//     field). Some already-released artifacts (e.g. early kapi-sat builds) only
//     have this legacy bundle, and we cannot re-sign published releases, so the
//     verifier converts a legacy bundle into an equivalent *bundle.Bundle and
//     verifies it with the same security policy as the new format.

package registry

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	protobundle "github.com/sigstore/protobuf-specs/gen/pb-go/bundle/v1"
	protocommon "github.com/sigstore/protobuf-specs/gen/pb-go/common/v1"
	protorekor "github.com/sigstore/protobuf-specs/gen/pb-go/rekor/v1"
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

	// A legacy `cosign sign-blob --bundle` document carries the artifact
	// signature in a "base64Signature" field that sigstore-go's bundle parser
	// rejects. Detect it up front and convert it to a *bundle.Bundle, binding
	// in the artifact digest the message signature needs.
	var b *bundle.Bundle
	if looksLikeLegacyCosignBundle(bundleData) {
		b, err = convertLegacyCosignBundle(bundleData, digest)
	} else {
		b, err = parseBundle(bundleData)
	}
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

// ---- legacy cosign `sign-blob --bundle` support ----
//
// The legacy bundle predates the Sigstore protobuf bundle. It is what
// `cosign sign-blob --bundle FILE` writes (without `--new-bundle-format`).
// See sigstore/cosign's cosign.LocalSignedPayload / bundle.RekorBundle.

// legacyCosignBundle mirrors cosign.LocalSignedPayload.
type legacyCosignBundle struct {
	// Base64Signature is the base64-encoded raw signature over the artifact.
	Base64Signature string `json:"base64Signature"`
	// Cert is the base64-encoded PEM block of the signing (leaf) certificate.
	Cert string `json:"cert"`
	// RekorBundle is the transparency-log inclusion bundle (SET + payload).
	RekorBundle *legacyRekorBundle `json:"rekorBundle"`
}

// legacyRekorBundle mirrors cosign/pkg/cosign/bundle.RekorBundle. Field names
// (SignedEntryTimestamp, Payload) match cosign's struct, which has no JSON tags.
type legacyRekorBundle struct {
	// SignedEntryTimestamp is the base64-encoded SET (inclusion promise).
	SignedEntryTimestamp string `json:"SignedEntryTimestamp"`
	Payload              struct {
		// Body is the base64-encoded canonicalized Rekor entry body.
		Body           string `json:"body"`
		IntegratedTime int64  `json:"integratedTime"`
		LogIndex       int64  `json:"logIndex"`
		LogID          string `json:"logID"` //nolint:tagliatelle // matches cosign wire format
	} `json:"Payload"`
}

// looksLikeLegacyCosignBundle reports whether data is a legacy cosign bundle
// rather than a new-format Sigstore protobuf bundle. A legacy bundle carries a
// non-empty "base64Signature"; a Sigstore bundle never has that field.
func looksLikeLegacyCosignBundle(data []byte) bool {
	var probe struct {
		Base64Signature string `json:"base64Signature"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return false
	}
	return probe.Base64Signature != ""
}

// convertLegacyCosignBundle converts a legacy cosign bundle into an equivalent
// sigstore-go *bundle.Bundle so the standard verifier can consume it. It
// produces a v0.1 bundle (single leaf certificate, message signature, one
// Rekor v1 transparency-log entry with an inclusion promise / SET).
//
// artifactDigest is the SHA-256 of the artifact; the legacy bundle does not
// carry the message digest, so we supply it here. The verifier checks this
// digest both against the policy's expected digest and against the Rekor entry
// body, so binding the wrong digest cannot pass verification.
func convertLegacyCosignBundle(data, artifactDigest []byte) (*bundle.Bundle, error) {
	var legacy legacyCosignBundle
	if err := json.Unmarshal(data, &legacy); err != nil {
		return nil, fmt.Errorf("decode legacy cosign bundle: %w", err)
	}
	if legacy.Base64Signature == "" {
		return nil, errors.New("legacy cosign bundle: empty base64Signature")
	}
	if legacy.Cert == "" {
		return nil, errors.New("legacy cosign bundle: missing signing certificate")
	}
	if legacy.RekorBundle == nil {
		return nil, errors.New("legacy cosign bundle: missing rekorBundle (transparency-log entry)")
	}

	sig, err := base64.StdEncoding.DecodeString(legacy.Base64Signature)
	if err != nil {
		return nil, fmt.Errorf("legacy cosign bundle: decode base64Signature: %w", err)
	}

	certDER, err := decodeLegacyCert(legacy.Cert)
	if err != nil {
		return nil, fmt.Errorf("legacy cosign bundle: %w", err)
	}

	set, err := base64.StdEncoding.DecodeString(legacy.RekorBundle.SignedEntryTimestamp)
	if err != nil {
		return nil, fmt.Errorf("legacy cosign bundle: decode SignedEntryTimestamp: %w", err)
	}
	if len(set) == 0 {
		return nil, errors.New("legacy cosign bundle: empty SignedEntryTimestamp (no inclusion promise)")
	}

	canonicalBody, err := base64.StdEncoding.DecodeString(legacy.RekorBundle.Payload.Body)
	if err != nil {
		return nil, fmt.Errorf("legacy cosign bundle: decode rekor body: %w", err)
	}
	logID, err := hex.DecodeString(legacy.RekorBundle.Payload.LogID)
	if err != nil {
		return nil, fmt.Errorf("legacy cosign bundle: decode logID: %w", err)
	}

	kind, version, err := rekorBodyKindVersion(canonicalBody)
	if err != nil {
		return nil, fmt.Errorf("legacy cosign bundle: %w", err)
	}

	mediaType, err := bundle.MediaTypeString("0.1")
	if err != nil {
		return nil, fmt.Errorf("legacy cosign bundle: media type: %w", err)
	}

	pb := &protobundle.Bundle{
		MediaType: mediaType,
		VerificationMaterial: &protobundle.VerificationMaterial{
			Content: &protobundle.VerificationMaterial_Certificate{
				Certificate: &protocommon.X509Certificate{RawBytes: certDER},
			},
			TlogEntries: []*protorekor.TransparencyLogEntry{
				{
					LogIndex:          legacy.RekorBundle.Payload.LogIndex,
					LogId:             &protocommon.LogId{KeyId: logID},
					KindVersion:       &protorekor.KindVersion{Kind: kind, Version: version},
					IntegratedTime:    legacy.RekorBundle.Payload.IntegratedTime,
					InclusionPromise:  &protorekor.InclusionPromise{SignedEntryTimestamp: set},
					CanonicalizedBody: canonicalBody,
				},
			},
		},
		Content: &protobundle.Bundle_MessageSignature{
			MessageSignature: &protocommon.MessageSignature{
				MessageDigest: &protocommon.HashOutput{
					Algorithm: protocommon.HashAlgorithm_SHA2_256,
					Digest:    artifactDigest,
				},
				Signature: sig,
			},
		},
	}

	return bundle.NewBundle(pb)
}

// decodeLegacyCert decodes the legacy bundle "cert" field (base64-encoded PEM)
// into DER certificate bytes for protocommon.X509Certificate.
func decodeLegacyCert(certField string) ([]byte, error) {
	pemBytes, err := base64.StdEncoding.DecodeString(certField)
	if err != nil {
		return nil, fmt.Errorf("decode cert base64: %w", err)
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("cert is not valid PEM")
	}
	if block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("cert PEM block is %q, want CERTIFICATE", block.Type)
	}
	return block.Bytes, nil
}

// rekorBodyKindVersion extracts the kind + apiVersion from a canonicalized
// Rekor entry body. The Sigstore bundle requires these on each tlog entry to
// reconstruct the entry during verification.
func rekorBodyKindVersion(canonicalBody []byte) (kind, version string, err error) {
	var head struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
	}
	if err := json.Unmarshal(canonicalBody, &head); err != nil {
		return "", "", fmt.Errorf("parse rekor body kind/version: %w", err)
	}
	if head.Kind == "" || head.APIVersion == "" {
		return "", "", errors.New("rekor body missing kind/apiVersion")
	}
	return head.Kind, head.APIVersion, nil
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
