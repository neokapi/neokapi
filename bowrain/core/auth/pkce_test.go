package auth

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateCodeVerifier(t *testing.T) {
	v, err := GenerateCodeVerifier()
	require.NoError(t, err)

	// 32 bytes → 43 base64url characters (no padding).
	assert.Len(t, v, 43)

	// Must be valid base64url.
	_, err = base64.RawURLEncoding.DecodeString(v)
	require.NoError(t, err)

	// Two verifiers must be different.
	v2, err := GenerateCodeVerifier()
	require.NoError(t, err)
	assert.NotEqual(t, v, v2)
}

func TestComputeCodeChallenge(t *testing.T) {
	// RFC 7636 Appendix B test vector.
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := ComputeCodeChallenge(verifier)

	assert.Equal(t, "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM", challenge)

	// Must be valid base64url.
	_, err := base64.RawURLEncoding.DecodeString(challenge)
	require.NoError(t, err)
}
