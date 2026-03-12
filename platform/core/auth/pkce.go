package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// GenerateCodeVerifier creates a cryptographically random PKCE code verifier
// per RFC 7636. The verifier is 43 characters of URL-safe base64.
func GenerateCodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// ComputeCodeChallenge computes the S256 code challenge from a code verifier
// per RFC 7636 section 4.2: BASE64URL(SHA256(verifier)).
func ComputeCodeChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}
