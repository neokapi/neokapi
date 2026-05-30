package server

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

// googleJWKSFixture stands up a JWKS endpoint backed by a freshly generated RSA
// key and returns the verifier (pointed at it) plus a token minter.
func googleJWKSFixture(t *testing.T, audiences, saEmails []string) (*googleTokenVerifier, func(claims jwt.MapClaims) string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	const kid = "test-key"
	jwks := jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
		Key: &key.PublicKey, KeyID: kid, Algorithm: "RS256", Use: "sig",
	}}}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jwks)
	}))
	t.Cleanup(srv.Close)

	v := newGoogleTokenVerifier(audiences, saEmails)
	v.certsURL = srv.URL

	mint := func(claims jwt.MapClaims) string {
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		tok.Header["kid"] = kid
		signed, serr := tok.SignedString(key)
		require.NoError(t, serr)
		return signed
	}
	return v, mint
}

func validClaims() jwt.MapClaims {
	return jwt.MapClaims{
		"iss":   "https://accounts.google.com",
		"aud":   "https://addin.bowrain.cloud",
		"email": "bowrain@addon.iam.gserviceaccount.com",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Add(-time.Minute).Unix(),
	}
}

func TestGoogleVerifyAcceptsValidToken(t *testing.T) {
	v, mint := googleJWKSFixture(t, []string{"https://addin.bowrain.cloud"}, nil)
	require.NoError(t, v.verify(t.Context(), mint(validClaims())))
}

func TestGoogleVerifyEnforcesAudience(t *testing.T) {
	v, mint := googleJWKSFixture(t, []string{"https://addin.bowrain.cloud"}, nil)
	c := validClaims()
	c["aud"] = "https://evil.example.com"
	require.Error(t, v.verify(t.Context(), mint(c)))
}

func TestGoogleVerifyEnforcesIssuer(t *testing.T) {
	v, mint := googleJWKSFixture(t, []string{"https://addin.bowrain.cloud"}, nil)
	c := validClaims()
	c["iss"] = "https://accounts.evil.com"
	require.Error(t, v.verify(t.Context(), mint(c)))
}

func TestGoogleVerifyRejectsExpired(t *testing.T) {
	v, mint := googleJWKSFixture(t, []string{"https://addin.bowrain.cloud"}, nil)
	c := validClaims()
	c["exp"] = time.Now().Add(-time.Hour).Unix()
	require.Error(t, v.verify(t.Context(), mint(c)))
}

func TestGoogleVerifyEnforcesServiceAccountEmail(t *testing.T) {
	v, mint := googleJWKSFixture(t, []string{"https://addin.bowrain.cloud"},
		[]string{"bowrain@addon.iam.gserviceaccount.com"})
	require.NoError(t, v.verify(t.Context(), mint(validClaims())))

	c := validClaims()
	c["email"] = "attacker@evil.com"
	require.Error(t, v.verify(t.Context(), mint(c)))
}

func TestGoogleVerifyAcceptsArrayAudience(t *testing.T) {
	v, mint := googleJWKSFixture(t, []string{"123456789"}, nil)
	c := validClaims()
	c["aud"] = []any{"https://other", "123456789"}
	require.NoError(t, v.verify(t.Context(), mint(c)))
}

func TestGoogleVerifyRejectsMissingKid(t *testing.T) {
	v, _ := googleJWKSFixture(t, []string{"https://addin.bowrain.cloud"}, nil)
	// A token with no kid header should be rejected.
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, validClaims())
	signed, err := tok.SignedString([]byte("secret"))
	require.NoError(t, err)
	require.Error(t, v.verify(t.Context(), signed))
}
