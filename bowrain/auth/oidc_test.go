package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// oidcTestServer is a minimal OIDC identity provider for tests. It serves a
// discovery document and a JWKS endpoint, and can mint RS256-signed tokens
// that go-oidc's verifier accepts.
type oidcTestServer struct {
	srv    *httptest.Server
	key    *rsa.PrivateKey
	keyID  string
	issuer string
}

// newOIDCTestServer stands up an httptest OIDC provider with a freshly
// generated RSA signing key. The discovery issuer is the server's own URL so
// that go-oidc's issuer check (issuer in discovery == requested URL) passes.
func newOIDCTestServer(t *testing.T) *oidcTestServer {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	ts := &oidcTestServer{key: key, keyID: "test-key-1"}

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	ts.srv = srv
	ts.issuer = srv.URL

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                ts.issuer,
			"authorization_endpoint":                ts.issuer + "/auth",
			"token_endpoint":                        ts.issuer + "/token",
			"jwks_uri":                              ts.issuer + "/jwks",
			"userinfo_endpoint":                     ts.issuer + "/userinfo",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})

	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		jwks := jose.JSONWebKeySet{
			Keys: []jose.JSONWebKey{{
				Key:       key.Public(),
				KeyID:     ts.keyID,
				Algorithm: "RS256",
				Use:       "sig",
			}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jwks)
	})

	return ts
}

// tokenClaims are the registered claims used in test tokens.
type tokenClaims struct {
	Issuer   string   `json:"iss"`
	Subject  string   `json:"sub"`
	Audience []string `json:"aud"`
	Expiry   int64    `json:"exp"`
	IssuedAt int64    `json:"iat"`
}

// signToken signs an RS256 JWT with the server's key and key ID.
func (ts *oidcTestServer) signToken(t *testing.T, claims tokenClaims) string {
	t.Helper()
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: ts.key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", ts.keyID),
	)
	require.NoError(t, err)
	raw, err := jwt.Signed(signer).Claims(claims).Serialize()
	require.NoError(t, err)
	return raw
}

func TestNewOIDCVerifier_Construction(t *testing.T) {
	ts := newOIDCTestServer(t)
	ctx := context.Background()

	v, err := NewOIDCVerifier(ctx, ts.issuer, "my-client")
	require.NoError(t, err)
	require.NotNil(t, v)
}

func TestNewOIDCVerifier_BadIssuer(t *testing.T) {
	ctx := context.Background()
	// A URL that 404s on discovery should fail provider construction.
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)

	_, err := NewOIDCVerifier(ctx, srv.URL, "my-client")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create OIDC provider")
}

func TestNewOIDCVerifier_IssuerMismatch(t *testing.T) {
	// go-oidc rejects a provider whose discovery `issuer` differs from the
	// requested URL (defends against issuer-confusion). Stand up a server whose
	// discovery advertises a different issuer.
	ctx := context.Background()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":   "https://evil.example.com",
			"jwks_uri": srv.URL + "/jwks",
		})
	})

	_, err := NewOIDCVerifier(ctx, srv.URL, "my-client")
	require.Error(t, err)
}

func TestOIDCVerifier_VerifyValidToken(t *testing.T) {
	ts := newOIDCTestServer(t)
	ctx := context.Background()

	v, err := NewOIDCVerifier(ctx, ts.issuer, "my-client")
	require.NoError(t, err)

	now := time.Now()
	raw := ts.signToken(t, tokenClaims{
		Issuer:   ts.issuer,
		Subject:  "user-123",
		Audience: []string{"my-client"},
		Expiry:   now.Add(time.Hour).Unix(),
		IssuedAt: now.Unix(),
	})

	idToken, err := v.Verify(ctx, raw)
	require.NoError(t, err)
	assert.Equal(t, "user-123", idToken.Subject)
	assert.Equal(t, ts.issuer, idToken.Issuer)
}

func TestOIDCVerifier_RejectsWrongAudience(t *testing.T) {
	ts := newOIDCTestServer(t)
	ctx := context.Background()

	v, err := NewOIDCVerifier(ctx, ts.issuer, "my-client")
	require.NoError(t, err)

	now := time.Now()
	raw := ts.signToken(t, tokenClaims{
		Issuer:   ts.issuer,
		Subject:  "user-123",
		Audience: []string{"some-other-client"}, // not my-client
		Expiry:   now.Add(time.Hour).Unix(),
		IssuedAt: now.Unix(),
	})

	_, err = v.Verify(ctx, raw)
	require.Error(t, err)
}

func TestOIDCVerifier_RejectsExpiredToken(t *testing.T) {
	ts := newOIDCTestServer(t)
	ctx := context.Background()

	v, err := NewOIDCVerifier(ctx, ts.issuer, "my-client")
	require.NoError(t, err)

	past := time.Now().Add(-2 * time.Hour)
	raw := ts.signToken(t, tokenClaims{
		Issuer:   ts.issuer,
		Subject:  "user-123",
		Audience: []string{"my-client"},
		Expiry:   past.Add(time.Hour).Unix(), // expired an hour ago
		IssuedAt: past.Unix(),
	})

	_, err = v.Verify(ctx, raw)
	require.Error(t, err)
}

func TestOIDCVerifier_RejectsWrongSignature(t *testing.T) {
	ts := newOIDCTestServer(t)
	ctx := context.Background()

	v, err := NewOIDCVerifier(ctx, ts.issuer, "my-client")
	require.NoError(t, err)

	// Sign with a different key that isn't published in the server's JWKS.
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: otherKey},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", ts.keyID),
	)
	require.NoError(t, err)
	now := time.Now()
	raw, err := jwt.Signed(signer).Claims(tokenClaims{
		Issuer:   ts.issuer,
		Subject:  "user-123",
		Audience: []string{"my-client"},
		Expiry:   now.Add(time.Hour).Unix(),
		IssuedAt: now.Unix(),
	}).Serialize()
	require.NoError(t, err)

	_, err = v.Verify(ctx, raw)
	require.Error(t, err)
}

func TestNewOIDCAccessTokenVerifier_SkipsAudienceCheck(t *testing.T) {
	// Keycloak access tokens carry aud="account", not the client ID. The
	// access-token verifier must accept them via SkipClientIDCheck while still
	// enforcing issuer + signature.
	ts := newOIDCTestServer(t)
	ctx := context.Background()

	v, err := NewOIDCAccessTokenVerifier(ctx, ts.issuer)
	require.NoError(t, err)

	now := time.Now()
	raw := ts.signToken(t, tokenClaims{
		Issuer:   ts.issuer,
		Subject:  "user-123",
		Audience: []string{"account"}, // not a client ID
		Expiry:   now.Add(time.Hour).Unix(),
		IssuedAt: now.Unix(),
	})

	idToken, err := v.Verify(ctx, raw)
	require.NoError(t, err)
	assert.Equal(t, "user-123", idToken.Subject)
}

func TestNewOIDCAccessTokenVerifier_StillChecksIssuer(t *testing.T) {
	ts := newOIDCTestServer(t)
	ctx := context.Background()

	v, err := NewOIDCAccessTokenVerifier(ctx, ts.issuer)
	require.NoError(t, err)

	now := time.Now()
	raw := ts.signToken(t, tokenClaims{
		Issuer:   "https://evil.example.com", // wrong issuer
		Subject:  "user-123",
		Audience: []string{"account"},
		Expiry:   now.Add(time.Hour).Unix(),
		IssuedAt: now.Unix(),
	})

	_, err = v.Verify(ctx, raw)
	require.Error(t, err)
}

func TestNewOAuth2Config_ResolvesEndpoints(t *testing.T) {
	ts := newOIDCTestServer(t)
	ctx := context.Background()

	cfg := OIDCConfig{
		IssuerURL:    ts.issuer,
		ClientID:     "my-client",
		ClientSecret: "shh",
		RedirectURL:  "https://app.example.com/callback",
	}
	oc, err := NewOAuth2Config(ctx, cfg)
	require.NoError(t, err)

	assert.Equal(t, "my-client", oc.ClientID)
	assert.Equal(t, "shh", oc.ClientSecret)
	assert.Equal(t, "https://app.example.com/callback", oc.RedirectURL)
	assert.Equal(t, ts.issuer+"/auth", oc.Endpoint.AuthURL)
	assert.Equal(t, ts.issuer+"/token", oc.Endpoint.TokenURL)
	assert.Contains(t, oc.Scopes, "openid")
	assert.Contains(t, oc.Scopes, "profile")
	assert.Contains(t, oc.Scopes, "email")
}

func TestNewOAuth2Config_BadIssuer(t *testing.T) {
	ctx := context.Background()
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)

	_, err := NewOAuth2Config(ctx, OIDCConfig{IssuerURL: srv.URL, ClientID: "c"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create OIDC provider")
}
