package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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

func (f *fakeCredentialManager) InApp() bool              { return f.inApp }
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

// newMemSession returns a MemorySessionStore closed at test end (goleak-clean).
func newMemSession(t *testing.T) *MemorySessionStore {
	t.Helper()
	ss := NewMemorySessionStore()
	t.Cleanup(func() { _ = ss.Close() })
	return ss
}

// elevated seeds a live elevated access token for userID.
func elevated(t *testing.T, s *Server, userID, token string) {
	t.Helper()
	require.NoError(t, s.storeElevatedToken(context.Background(), userID, token, time.Time{}))
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
	t.Run("409 elevation_required when not elevated", func(t *testing.T) {
		s := &Server{CredentialManager: &fakeCredentialManager{inApp: true}, SessionStore: newMemSession(t)}
		c, rec := newPasskeyCtx(t, http.MethodGet, "/account/passkeys", "", "user-1")
		require.NoError(t, s.HandleListPasskeys(c))
		assert.Equal(t, http.StatusConflict, rec.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Equal(t, "elevation_required", body["error"])
		assert.Equal(t, elevatePath, body["elevate_url"])
	})
	t.Run("200 with elevated token", func(t *testing.T) {
		fake := &fakeCredentialManager{inApp: true, listResult: []auth.Passkey{{ID: "c1", Name: "phone"}}}
		s := &Server{CredentialManager: fake, SessionStore: newMemSession(t)}
		elevated(t, s, "user-1", "AT-elev")
		c, rec := newPasskeyCtx(t, http.MethodGet, "/account/passkeys", "", "user-1")
		require.NoError(t, s.HandleListPasskeys(c))
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "AT-elev", fake.lastAccessToken)
	})
	t.Run("scope error re-elevates and clears the stale token", func(t *testing.T) {
		fake := &fakeCredentialManager{
			inApp:   true,
			listErr: errors.New("cognito list webauthn credentials: NotAuthorizedException: Access Token does not have required scopes"),
		}
		s := &Server{CredentialManager: fake, SessionStore: newMemSession(t)}
		elevated(t, s, "user-1", "AT-stale")
		c, rec := newPasskeyCtx(t, http.MethodGet, "/account/passkeys", "", "user-1")
		require.NoError(t, s.HandleListPasskeys(c))
		assert.Equal(t, http.StatusConflict, rec.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Equal(t, "elevation_required", body["error"])
		// The stale token must be dropped so the next attempt re-elevates.
		_, err := s.elevatedCognitoAccessToken(context.Background(), "user-1")
		require.ErrorIs(t, err, errElevationRequired)
	})
}

// --- elevated token store ---------------------------------------------------

func TestElevatedTokenRoundTrip(t *testing.T) {
	s := &Server{SessionStore: newMemSession(t)}
	ctx := context.Background()

	_, err := s.elevatedCognitoAccessToken(ctx, "user-1")
	require.ErrorIs(t, err, errElevationRequired)

	require.NoError(t, s.storeElevatedToken(ctx, "user-1", "AT-1", time.Now().Add(time.Hour)))
	got, err := s.elevatedCognitoAccessToken(ctx, "user-1")
	require.NoError(t, err)
	assert.Equal(t, "AT-1", got)

	// Bounded by the token's own (past) expiry → refused.
	require.Error(t, s.storeElevatedToken(ctx, "user-2", "AT-2", time.Now().Add(-time.Minute)))
}

// --- helpers ----------------------------------------------------------------

func TestIsScopeError(t *testing.T) {
	assert.True(t, isScopeError(errors.New("NotAuthorizedException: Access Token does not have required scopes")))
	assert.False(t, isScopeError(errors.New("NotAuthorizedException: user is disabled")))
	assert.False(t, isScopeError(errors.New("some other error")))
	assert.False(t, isScopeError(nil))
}

func TestSanitizeReturnPath(t *testing.T) {
	assert.Equal(t, "/settings", sanitizeReturnPath("/settings"))
	assert.Equal(t, "/w/1/user-settings?tab=security", sanitizeReturnPath("/w/1/user-settings?tab=security"))
	assert.Equal(t, "/", sanitizeReturnPath(""))
	assert.Equal(t, "/", sanitizeReturnPath("//evil.com"))
	assert.Equal(t, "/", sanitizeReturnPath("https://evil.com"))
	assert.Equal(t, "/", sanitizeReturnPath("/\\evil"))
}

// --- elevate handler guards -------------------------------------------------

func TestHandleSecurityElevate_Guards(t *testing.T) {
	t.Run("503 when credential manager unconfigured", func(t *testing.T) {
		s := &Server{}
		c, rec := newPasskeyCtx(t, http.MethodGet, elevatePath, "", "user-1")
		require.NoError(t, s.HandleSecurityElevate(c))
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	})
	t.Run("401 without user", func(t *testing.T) {
		s := &Server{CredentialManager: &fakeCredentialManager{inApp: true}}
		c, rec := newPasskeyCtx(t, http.MethodGet, elevatePath, "", "")
		require.NoError(t, s.HandleSecurityElevate(c))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
	t.Run("409 account_console for non-in-app provider", func(t *testing.T) {
		s := &Server{CredentialManager: &fakeCredentialManager{inApp: false, accountURL: "https://console"}}
		c, rec := newPasskeyCtx(t, http.MethodGet, elevatePath, "", "user-1")
		require.NoError(t, s.HandleSecurityElevate(c))
		assert.Equal(t, http.StatusConflict, rec.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Equal(t, "account_console_required", body["error"])
	})
}

func TestHandleSecurityElevateCallback_Guards(t *testing.T) {
	t.Run("401 without user", func(t *testing.T) {
		s := &Server{SessionStore: newMemSession(t)}
		c, rec := newPasskeyCtx(t, http.MethodGet, "/account/security/elevate/callback?state=x&code=y", "", "")
		require.NoError(t, s.HandleSecurityElevateCallback(c))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
	t.Run("unknown state redirects to root with elevated=0", func(t *testing.T) {
		s := &Server{SessionStore: newMemSession(t)}
		c, rec := newPasskeyCtx(t, http.MethodGet, "/account/security/elevate/callback?state=missing&code=y", "", "user-1")
		require.NoError(t, s.HandleSecurityElevateCallback(c))
		assert.Equal(t, http.StatusFound, rec.Code)
		assert.Contains(t, rec.Header().Get("Location"), "elevated=0")
	})
	t.Run("state bound to another user fails closed", func(t *testing.T) {
		s := &Server{SessionStore: newMemSession(t)}
		ctx := context.Background()
		entry := &elevateEntry{CodeVerifier: "v", Nonce: "n", UserID: "other-user", Return: "/settings"}
		require.NoError(t, sessionSet(ctx, s.SessionStore, prefixElevateState, "st-1", entry, time.Minute))
		c, rec := newPasskeyCtx(t, http.MethodGet, "/account/security/elevate/callback?state=st-1&code=y", "", "user-1")
		require.NoError(t, s.HandleSecurityElevateCallback(c))
		assert.Equal(t, http.StatusFound, rec.Code)
		assert.Contains(t, rec.Header().Get("Location"), "/settings?elevated=0")
	})
}

// --- register finish nonce lifecycle ----------------------------------------

func TestHandleRegisterFinish_NonceSingleUse(t *testing.T) {
	// No elevated token → after the nonce is consumed, the token check yields
	// 409 elevation_required.
	s := &Server{
		CredentialManager: &fakeCredentialManager{inApp: true},
		SessionStore:      newMemSession(t),
	}
	ctx := context.Background()
	require.NoError(t, s.SessionStore.Set(ctx, prefixPasskeyNonce+"user-1", []byte("nonce-abc"), time.Minute))

	body := `{"nonce":"nonce-abc","attestation":{"id":"cred-1"}}`

	// First call: nonce valid and consumed; not elevated → 409 elevation_required.
	c1, rec1 := newPasskeyCtx(t, http.MethodPost, "/account/passkeys/register/finish", body, "user-1")
	require.NoError(t, s.HandleRegisterFinish(c1))
	assert.Equal(t, http.StatusConflict, rec1.Code)
	var b1 map[string]any
	require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &b1))
	assert.Equal(t, "elevation_required", b1["error"])

	// Second call with the same nonce: it was consumed → 400 invalid nonce.
	c2, rec2 := newPasskeyCtx(t, http.MethodPost, "/account/passkeys/register/finish", body, "user-1")
	require.NoError(t, s.HandleRegisterFinish(c2))
	assert.Equal(t, http.StatusBadRequest, rec2.Code)
}

func TestHandleRegisterFinish_BadNonce(t *testing.T) {
	s := &Server{
		CredentialManager: &fakeCredentialManager{inApp: true},
		SessionStore:      newMemSession(t),
	}
	elevated(t, s, "user-1", "AT-elev")
	// No nonce seeded → any nonce is invalid, and the manager is never reached.
	body := `{"nonce":"whatever","attestation":{"id":"cred-1"}}`
	c, rec := newPasskeyCtx(t, http.MethodPost, "/account/passkeys/register/finish", body, "user-1")
	require.NoError(t, s.HandleRegisterFinish(c))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleRegisterFinish_Success(t *testing.T) {
	fake := &fakeCredentialManager{inApp: true}
	s := &Server{CredentialManager: fake, SessionStore: newMemSession(t)}
	elevated(t, s, "user-1", "AT-elev")
	ctx := context.Background()
	require.NoError(t, s.SessionStore.Set(ctx, prefixPasskeyNonce+"user-1", []byte("nonce-ok"), time.Minute))

	body := `{"nonce":"nonce-ok","attestation":{"id":"cred-1"}}`
	c, rec := newPasskeyCtx(t, http.MethodPost, "/account/passkeys/register/finish", body, "user-1")
	require.NoError(t, s.HandleRegisterFinish(c))
	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "AT-elev", fake.lastAccessToken)
}

func TestHandleDeletePasskey_Guards(t *testing.T) {
	s := &Server{CredentialManager: &fakeCredentialManager{inApp: true}, SessionStore: newMemSession(t)}
	// Missing id → echo would not route, but calling directly with empty param:
	c, rec := newPasskeyCtx(t, http.MethodDelete, "/account/passkeys/", "", "user-1")
	require.NoError(t, s.HandleDeletePasskey(c))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
