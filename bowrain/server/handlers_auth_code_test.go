package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	venueauth "github.com/neokapi/neokapi/host/venue/auth"
)

const (
	codeTestRedirect = "bowrain://auth/callback"
	codeTestVerifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
)

func newCodeTestServer(t *testing.T) (*Server, *platauth.User) {
	t.Helper()
	cfg := DefaultConfig()
	cfg.JWTSecret = "test-secret"
	cfg.OIDCIssuerURL = "http://localhost:8180"
	cfg.OIDCClientID = "test-client"
	srv := shutdownOnCleanup(t, NewServer(cfg))
	initTestStores(t, srv)
	user := &platauth.User{ID: "u-code", Email: "code@example.com", Name: "Code User"}
	require.NoError(t, srv.AuthStore.CreateUser(t.Context(), user))
	return srv, user
}

// issueCode runs the end of a code-mode sign-in and returns the redirect it sends.
func issueCode(t *testing.T, srv *Server, userID string) *url.URL {
	t.Helper()
	entry := &desktopAuthEntry{
		RedirectURI:   codeTestRedirect,
		CodeChallenge: venueauth.ComputeCodeChallenge(codeTestVerifier),
		ResponseMode:  desktopResponseModeCode,
		ClientState:   "client-state-1",
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/desktop/callback", nil)
	rec := httptest.NewRecorder()
	c := srv.GetEcho().NewContext(req, rec)
	require.NoError(t, srv.redirectWithLoginCode(c, entry, userID))
	require.Equal(t, http.StatusFound, rec.Code)
	loc, err := url.Parse(rec.Header().Get("Location"))
	require.NoError(t, err)
	return loc
}

func redeem(t *testing.T, srv *Server, code, verifier, redirect string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(desktopTokenRequest{Code: code, CodeVerifier: verifier, RedirectURI: redirect})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/desktop/token", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.GetEcho().ServeHTTP(rec, req)
	return rec
}

func TestDesktopCodeRedirectCarriesNoToken(t *testing.T) {
	srv, user := newCodeTestServer(t)
	loc := issueCode(t, srv, user.ID)
	assert.Equal(t, "bowrain", loc.Scheme)
	q := loc.Query()
	assert.NotEmpty(t, q.Get("code"))
	assert.Equal(t, "client-state-1", q.Get("state"))
	assert.Empty(t, q.Get("token"), "a code-mode redirect must never carry a token")
	assert.Empty(t, q.Get("refresh_token"))
}

func TestDesktopCodeExchangeIssuesTokens(t *testing.T) {
	srv, user := newCodeTestServer(t)
	code := issueCode(t, srv, user.ID).Query().Get("code")

	rec := redeem(t, srv, code, codeTestVerifier, codeTestRedirect)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resp venueauth.TokenResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.AccessToken)
	assert.NotEmpty(t, resp.RefreshToken)
	claims, err := platauth.ValidateToken(resp.AccessToken, "test-secret")
	require.NoError(t, err)
	assert.Equal(t, user.ID, claims.Subject)
}

func TestDesktopCodeExchangeRefusesWrongVerifierAndSpendsTheCode(t *testing.T) {
	srv, user := newCodeTestServer(t)
	code := issueCode(t, srv, user.ID).Query().Get("code")

	rec := redeem(t, srv, code, "wrong-verifier-wrong-verifier-wrong-verifier", codeTestRedirect)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid_grant")
	assert.NotContains(t, rec.Body.String(), code, "a refusal must not echo the code")

	// The first attempt consumed the code, right verifier or not.
	rec = redeem(t, srv, code, codeTestVerifier, codeTestRedirect)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestDesktopCodeExchangeRefusesAReusedCode(t *testing.T) {
	srv, user := newCodeTestServer(t)
	code := issueCode(t, srv, user.ID).Query().Get("code")
	require.Equal(t, http.StatusOK, redeem(t, srv, code, codeTestVerifier, codeTestRedirect).Code)
	rec := redeem(t, srv, code, codeTestVerifier, codeTestRedirect)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid_grant")
}

func TestDesktopCodeExchangeRefusesAnExpiredCode(t *testing.T) {
	srv, user := newCodeTestServer(t)
	entry := &desktopCodeEntry{
		UserID:        user.ID,
		CodeChallenge: venueauth.ComputeCodeChallenge(codeTestVerifier),
		RedirectURI:   codeTestRedirect,
	}
	require.NoError(t, sessionSet(t.Context(), srv.SessionStore, prefixDesktopCode, "expiring", entry, time.Millisecond))
	time.Sleep(5 * time.Millisecond)
	rec := redeem(t, srv, "expiring", codeTestVerifier, codeTestRedirect)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid_grant")
}

func TestDesktopCodeExchangeRefusesAnotherRedirect(t *testing.T) {
	srv, user := newCodeTestServer(t)
	code := issueCode(t, srv, user.ID).Query().Get("code")
	rec := redeem(t, srv, code, codeTestVerifier, "bowrain://other/callback")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid_grant")

	// A redirect outside the allowlist is refused before any code is read.
	rec = redeem(t, srv, "anything", codeTestVerifier, "https://evil.example/callback")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "redirect_uri must be")
}

func desktopLogin(t *testing.T, srv *Server, params url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/desktop/login?"+params.Encode(), nil)
	rec := httptest.NewRecorder()
	srv.GetEcho().ServeHTTP(rec, req)
	return rec
}

func TestDesktopLoginCodeModeRequiresS256AndState(t *testing.T) {
	srv, _ := newCodeTestServer(t)
	challenge := venueauth.ComputeCodeChallenge(codeTestVerifier)
	base := func() url.Values {
		return url.Values{
			"redirect_uri":          {codeTestRedirect},
			"code_challenge":        {challenge},
			"code_challenge_method": {"S256"},
			"state":                 {"client-state-1"},
			"response_mode":         {"code"},
		}
	}

	plain := base()
	plain.Set("code_challenge_method", "plain")
	assert.Equal(t, http.StatusBadRequest, desktopLogin(t, srv, plain).Code)

	noMethod := base()
	noMethod.Del("code_challenge_method")
	rec := desktopLogin(t, srv, noMethod)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "requires code_challenge_method=S256")

	badChallenge := base()
	badChallenge.Set("code_challenge", "not-a-challenge")
	assert.Equal(t, http.StatusBadRequest, desktopLogin(t, srv, badChallenge).Code)

	noState := base()
	noState.Del("state")
	rec = desktopLogin(t, srv, noState)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "requires a state")

	unknown := base()
	unknown.Set("response_mode", "fragment")
	assert.Equal(t, http.StatusBadRequest, desktopLogin(t, srv, unknown).Code)

	// Valid parameters pass validation and reach OIDC discovery, which has no
	// provider to find in this test.
	assert.NotEqual(t, http.StatusBadRequest, desktopLogin(t, srv, base()).Code)
}

func TestDesktopLoginWithoutResponseModeKeepsTheTokenRedirect(t *testing.T) {
	srv, _ := newCodeTestServer(t)
	// The request of desktop releases that predate the code redirect: no
	// response_mode and no state. It still passes validation.
	rec := desktopLogin(t, srv, url.Values{
		"redirect_uri":          {"http://127.0.0.1:12345/callback"},
		"code_challenge":        {"test-challenge"},
		"code_challenge_method": {"S256"},
	})
	assert.NotEqual(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

// The Bowrain desktop app's request since it moved to the code redirect: the
// session entry the callback reads must carry the mode, the client's state and
// the PKCE challenge, and the identity provider must see the server's own state.
func TestDesktopLoginCodeModeStoresTheClientRequest(t *testing.T) {
	var oidcURL string
	mockOIDC := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"issuer":"` + oidcURL + `","authorization_endpoint":"` + oidcURL +
			`/auth","token_endpoint":"` + oidcURL + `/token","jwks_uri":"` + oidcURL + `/certs"}`))
	}))
	defer mockOIDC.Close()
	oidcURL = mockOIDC.URL

	cfg := DefaultConfig()
	cfg.JWTSecret = "test-secret"
	cfg.OIDCIssuerURL = oidcURL
	cfg.OIDCClientID = "test-client"
	srv := shutdownOnCleanup(t, NewServer(cfg))
	initTestStores(t, srv)

	challenge := venueauth.ComputeCodeChallenge(codeTestVerifier)
	rec := desktopLogin(t, srv, url.Values{
		"redirect_uri":          {codeTestRedirect},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {"client-state-1"},
		"response_mode":         {"code"},
	})
	require.Equal(t, http.StatusFound, rec.Code, rec.Body.String())

	loc, err := url.Parse(rec.Header().Get("Location"))
	require.NoError(t, err)
	upstreamState := loc.Query().Get("state")
	require.NotEmpty(t, upstreamState)
	assert.NotEqual(t, "client-state-1", upstreamState, "the identity provider sees the server's state, not the client's")
	assert.NotEqual(t, challenge, loc.Query().Get("code_challenge"), "the server runs its own PKCE with the identity provider")

	entry, err := sessionGet[desktopAuthEntry](t.Context(), srv.SessionStore, prefixDesktopAuth, upstreamState)
	require.NoError(t, err)
	assert.Equal(t, desktopResponseModeCode, entry.ResponseMode)
	assert.Equal(t, "client-state-1", entry.ClientState)
	assert.Equal(t, codeTestRedirect, entry.RedirectURI)
	assert.Equal(t, challenge, entry.CodeChallenge)
}
