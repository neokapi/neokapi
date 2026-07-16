package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- test doubles -----------------------------------------------------------

type fakeCredentialManager struct {
	inApp       bool
	accountURL  string
	listResult  []auth.Passkey
	listErr     error
	startResult json.RawMessage
	startErr    error
	finishErr   error
	deleteErr   error

	lastAccessToken string
	lastDeleteID    string
}

func (f *fakeCredentialManager) InApp() bool             { return f.inApp }
func (f *fakeCredentialManager) AccountURL(string) string { return f.accountURL }
func (f *fakeCredentialManager) ListPasskeys(_ context.Context, at string) ([]auth.Passkey, error) {
	f.lastAccessToken = at
	return f.listResult, f.listErr
}
func (f *fakeCredentialManager) RegisterStart(_ context.Context, at string) (json.RawMessage, error) {
	f.lastAccessToken = at
	return f.startResult, f.startErr
}
func (f *fakeCredentialManager) RegisterFinish(_ context.Context, at string, _ json.RawMessage) error {
	f.lastAccessToken = at
	return f.finishErr
}
func (f *fakeCredentialManager) DeletePasskey(_ context.Context, at, id string) error {
	f.lastAccessToken = at
	f.lastDeleteID = id
	return f.deleteErr
}

type fakeUpstreamTokenStore struct {
	token     string
	expiresAt time.Time
	has       bool
	deleted   bool
}

func (f *fakeUpstreamTokenStore) PutRefreshToken(_ context.Context, _, _, tok string, exp time.Time) error {
	f.token, f.expiresAt, f.has = tok, exp, true
	return nil
}
func (f *fakeUpstreamTokenStore) GetRefreshToken(_ context.Context, _, _ string) (string, time.Time, error) {
	if !f.has {
		return "", time.Time{}, auth.ErrUpstreamTokenNotFound
	}
	return f.token, f.expiresAt, nil
}
func (f *fakeUpstreamTokenStore) DeleteRefreshToken(_ context.Context, _, _ string) error {
	f.deleted = true
	f.has = false
	return nil
}
func (f *fakeUpstreamTokenStore) Close() error { return nil }

// newMemSession returns a MemorySessionStore closed at test end (goleak-clean).
func newMemSession(t *testing.T) *MemorySessionStore {
	t.Helper()
	ss := NewMemorySessionStore()
	t.Cleanup(func() { _ = ss.Close() })
	return ss
}

func newPasskeyCtx(t *testing.T, method, target, body, userID string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if userID != "" {
		c.Set("user_id", userID)
	}
	return c, rec
}

// --- guard behaviors --------------------------------------------------------

func TestHandleAccountSecurity(t *testing.T) {
	t.Run("503 when credential manager unconfigured", func(t *testing.T) {
		s := &Server{}
		c, rec := newPasskeyCtx(t, http.MethodGet, "/account/security", "", "user-1")
		require.NoError(t, s.HandleAccountSecurity(c))
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	})
	t.Run("in-app provider", func(t *testing.T) {
		s := &Server{CredentialManager: &fakeCredentialManager{inApp: true}}
		c, rec := newPasskeyCtx(t, http.MethodGet, "/account/security", "", "user-1")
		require.NoError(t, s.HandleAccountSecurity(c))
		assert.Equal(t, http.StatusOK, rec.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Equal(t, true, body["in_app"])
		assert.Empty(t, body["account_url"])
	})
	t.Run("console provider exposes account_url", func(t *testing.T) {
		s := &Server{CredentialManager: &fakeCredentialManager{inApp: false, accountURL: "https://auth/realms/bowrain/account"}}
		c, rec := newPasskeyCtx(t, http.MethodGet, "/account/security", "", "user-1")
		require.NoError(t, s.HandleAccountSecurity(c))
		assert.Equal(t, http.StatusOK, rec.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Equal(t, false, body["in_app"])
		assert.Equal(t, "https://auth/realms/bowrain/account", body["account_url"])
	})
	t.Run("401 without user", func(t *testing.T) {
		s := &Server{CredentialManager: &fakeCredentialManager{inApp: true}}
		c, rec := newPasskeyCtx(t, http.MethodGet, "/account/security", "", "")
		require.NoError(t, s.HandleAccountSecurity(c))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

func TestHandleListPasskeys_Guards(t *testing.T) {
	t.Run("503 when unconfigured", func(t *testing.T) {
		s := &Server{}
		c, rec := newPasskeyCtx(t, http.MethodGet, "/account/passkeys", "", "user-1")
		require.NoError(t, s.HandleListPasskeys(c))
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	})
	t.Run("401 without user", func(t *testing.T) {
		s := &Server{CredentialManager: &fakeCredentialManager{inApp: true}}
		c, rec := newPasskeyCtx(t, http.MethodGet, "/account/passkeys", "", "")
		require.NoError(t, s.HandleListPasskeys(c))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
	t.Run("409 account_console when not in-app", func(t *testing.T) {
		s := &Server{CredentialManager: &fakeCredentialManager{inApp: false, accountURL: "https://console"}}
		c, rec := newPasskeyCtx(t, http.MethodGet, "/account/passkeys", "", "user-1")
		require.NoError(t, s.HandleListPasskeys(c))
		assert.Equal(t, http.StatusConflict, rec.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Equal(t, "account_console_required", body["error"])
		assert.Equal(t, "https://console", body["account_url"])
	})
	t.Run("409 reauth_required when no upstream token", func(t *testing.T) {
		s := &Server{CredentialManager: &fakeCredentialManager{inApp: true}, UpstreamTokens: &fakeUpstreamTokenStore{}}
		c, rec := newPasskeyCtx(t, http.MethodGet, "/account/passkeys", "", "user-1")
		require.NoError(t, s.HandleListPasskeys(c))
		assert.Equal(t, http.StatusConflict, rec.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Equal(t, "reauth_required", body["error"])
	})
}

// --- freshCognitoAccessToken (token-endpoint refresh) -----------------------

// oidcTestServer serves an OIDC discovery doc and a token endpoint that returns
// tokenStatus/tokenBody. It also records the last token-request form + auth.
type oidcTestServer struct {
	srv         *httptest.Server
	tokenStatus int
	tokenBody   string
	lastForm    url.Values
	lastUser    string
	lastPass    string
}

func newOIDCTestServer(t *testing.T) *oidcTestServer {
	t.Helper()
	ots := &oidcTestServer{tokenStatus: http.StatusOK, tokenBody: `{"access_token":"AT-minted","token_type":"Bearer","expires_in":3600}`}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		base := ots.srv.URL
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"issuer":"` + base + `","authorization_endpoint":"` + base + `/auth","token_endpoint":"` + base + `/token","jwks_uri":"` + base + `/jwks"}`))
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		ots.lastForm = r.PostForm
		ots.lastUser, ots.lastPass, _ = r.BasicAuth()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(ots.tokenStatus)
		_, _ = w.Write([]byte(ots.tokenBody))
	})
	ots.srv = httptest.NewServer(mux)
	t.Cleanup(ots.srv.Close)
	return ots
}

func TestFreshCognitoAccessToken(t *testing.T) {
	t.Run("no store -> reauth", func(t *testing.T) {
		s := &Server{}
		_, err := s.freshCognitoAccessToken(context.Background(), "user-1")
		require.ErrorIs(t, err, errReauthRequired)
	})

	t.Run("no stored token -> reauth", func(t *testing.T) {
		s := &Server{UpstreamTokens: &fakeUpstreamTokenStore{}}
		_, err := s.freshCognitoAccessToken(context.Background(), "user-1")
		require.ErrorIs(t, err, errReauthRequired)
	})

	t.Run("mints access token from refresh token", func(t *testing.T) {
		ots := newOIDCTestServer(t)
		store := &fakeUpstreamTokenStore{token: "refresh-abc", expiresAt: time.Now().Add(time.Hour), has: true}
		s := &Server{
			Config:         Config{OIDCIssuerURL: ots.srv.URL, OIDCClientID: "client-id", OIDCClientSecret: "client-secret"},
			UpstreamTokens: store,
		}
		at, err := s.freshCognitoAccessToken(context.Background(), "user-1")
		require.NoError(t, err)
		assert.Equal(t, "AT-minted", at)
		// Correct grant + confidential-client Basic auth.
		assert.Equal(t, "refresh_token", ots.lastForm.Get("grant_type"))
		assert.Equal(t, "refresh-abc", ots.lastForm.Get("refresh_token"))
		assert.Equal(t, "client-id", ots.lastUser)
		assert.Equal(t, "client-secret", ots.lastPass)
	})

	t.Run("invalid_grant prunes token and returns reauth", func(t *testing.T) {
		ots := newOIDCTestServer(t)
		ots.tokenStatus = http.StatusBadRequest
		ots.tokenBody = `{"error":"invalid_grant"}`
		store := &fakeUpstreamTokenStore{token: "refresh-expired", expiresAt: time.Now().Add(time.Hour), has: true}
		s := &Server{
			Config:         Config{OIDCIssuerURL: ots.srv.URL, OIDCClientID: "client-id", OIDCClientSecret: "client-secret"},
			UpstreamTokens: store,
		}
		_, err := s.freshCognitoAccessToken(context.Background(), "user-1")
		require.ErrorIs(t, err, errReauthRequired)
		assert.True(t, store.deleted, "dead refresh token should be pruned")
	})
}

func TestHandleRegisterFinish_NonceSingleUse(t *testing.T) {
	s := &Server{
		CredentialManager: &fakeCredentialManager{inApp: true},
		UpstreamTokens:    &fakeUpstreamTokenStore{}, // no token → token mint fails after nonce check
		SessionStore:      newMemSession(t),
	}
	ctx := context.Background()
	require.NoError(t, s.SessionStore.Set(ctx, prefixPasskeyNonce+"user-1", []byte("nonce-abc"), time.Minute))

	body := `{"nonce":"nonce-abc","attestation":{"id":"cred-1"}}`

	// First call: nonce is valid and consumed; token mint then fails → 409 reauth.
	c1, rec1 := newPasskeyCtx(t, http.MethodPost, "/account/passkeys/register/finish", body, "user-1")
	require.NoError(t, s.HandleRegisterFinish(c1))
	assert.Equal(t, http.StatusConflict, rec1.Code)
	var b1 map[string]any
	require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &b1))
	assert.Equal(t, "reauth_required", b1["error"])

	// Second call with the same nonce: it was consumed → 400 invalid nonce.
	c2, rec2 := newPasskeyCtx(t, http.MethodPost, "/account/passkeys/register/finish", body, "user-1")
	require.NoError(t, s.HandleRegisterFinish(c2))
	assert.Equal(t, http.StatusBadRequest, rec2.Code)
}

func TestHandleRegisterFinish_BadNonce(t *testing.T) {
	s := &Server{
		CredentialManager: &fakeCredentialManager{inApp: true},
		UpstreamTokens:    &fakeUpstreamTokenStore{token: "r", expiresAt: time.Now().Add(time.Hour), has: true},
		SessionStore:      newMemSession(t),
	}
	// No nonce seeded → any nonce is invalid, and the token mint is never reached.
	body := `{"nonce":"whatever","attestation":{"id":"cred-1"}}`
	c, rec := newPasskeyCtx(t, http.MethodPost, "/account/passkeys/register/finish", body, "user-1")
	require.NoError(t, s.HandleRegisterFinish(c))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleDeletePasskey_Guards(t *testing.T) {
	s := &Server{CredentialManager: &fakeCredentialManager{inApp: true}, UpstreamTokens: &fakeUpstreamTokenStore{}}
	// Missing id → echo would not route, but calling directly with empty param:
	c, rec := newPasskeyCtx(t, http.MethodDelete, "/account/passkeys/", "", "user-1")
	require.NoError(t, s.HandleDeletePasskey(c))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
