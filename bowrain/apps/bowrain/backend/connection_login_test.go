package backend

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	venueauth "github.com/neokapi/neokapi/host/venue/auth"
	"github.com/neokapi/neokapi/host/venue/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

// codeServer stands in for the server's desktop token endpoint. It accepts one
// code, bound to the challenge of the verifier the test hands it, the way
// HandleDesktopTokenExchange does.
type codeServer struct {
	*httptest.Server
	code      string
	challenge string
	redeemed  int
}

func newCodeServer(t *testing.T, code, verifier string) *codeServer {
	t.Helper()
	cs := &codeServer{code: code, challenge: venueauth.ComputeCodeChallenge(verifier)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("POST /api/v1/auth/desktop/token", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Code         string `json:"code"`
			CodeVerifier string `json:"code_verifier"`
			RedirectURI  string `json:"redirect_uri"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
			return
		}
		cs.redeemed++
		if req.Code != cs.code || req.RedirectURI != loginRedirectURI ||
			venueauth.ComputeCodeChallenge(req.CodeVerifier) != cs.challenge || cs.redeemed > 1 {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(venueauth.TokenResponse{
			AccessToken: "access-1", RefreshToken: "refresh-1", TokenType: "Bearer", ExpiresIn: 900,
		})
	})
	mux.HandleFunc("GET /api/v1/auth/me", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer access-1" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "user-1", "email": "alice@test.com", "name": "Alice"})
	})
	cs.Server = httptest.NewServer(mux)
	t.Cleanup(cs.Close)
	return cs
}

func TestDesktopLoginURLAsksForACode(t *testing.T) {
	raw := desktopLoginURL("https://bowrain.example", "challenge-1", "state-1")
	u, err := url.Parse(raw)
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/auth/desktop/login", u.Path)
	q := u.Query()
	assert.Equal(t, "code", q.Get("response_mode"))
	assert.Equal(t, loginRedirectURI, q.Get("redirect_uri"))
	assert.Equal(t, "challenge-1", q.Get("code_challenge"))
	assert.Equal(t, "S256", q.Get("code_challenge_method"))
	assert.Equal(t, "state-1", q.Get("state"))
}

func TestStartLoginHoldsVerifierAndState(t *testing.T) {
	cs := newCodeServer(t, "unused", "unused")
	app := newTestApp(t)
	require.NoError(t, app.StartLogin(cs.URL+"/"))

	app.mu.RLock()
	defer app.mu.RUnlock()
	assert.Equal(t, cs.URL, app.serverURL)
	assert.NotEmpty(t, app.pkceVerifier)
	assert.Len(t, app.pkceState, 32)
	assert.NotNil(t, app.pkceResultCh)
}

func TestLoginRedeemsTheCode(t *testing.T) {
	keyring.MockInit()
	t.Setenv("BOWRAIN_CONFIG_DIR", t.TempDir())

	app := newTestApp(t)
	cs := newCodeServer(t, "code-1", "") // challenge set below, once the verifier exists
	require.NoError(t, app.StartLogin(cs.URL))

	app.mu.RLock()
	verifier, state := app.pkceVerifier, app.pkceState
	app.mu.RUnlock()
	cs.challenge = venueauth.ComputeCodeChallenge(verifier)

	app.HandleAuthURL(loginRedirectURI + "?code=code-1&state=" + state)
	ok, err := app.WaitForLogin()
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, 1, cs.redeemed)

	loaded, err := config.LoadAuth()
	require.NoError(t, err)
	assert.Equal(t, "access-1", loaded.AccessToken)
	assert.Equal(t, "refresh-1", loaded.RefreshToken)
	assert.Equal(t, config.StoredUser{ID: "user-1", Email: "alice@test.com", Name: "Alice"}, loaded.User)

	// The flow is over: its verifier and state are gone.
	app.mu.RLock()
	defer app.mu.RUnlock()
	assert.Empty(t, app.pkceVerifier)
	assert.Empty(t, app.pkceState)
}

func TestExchangeLoginCodeRefused(t *testing.T) {
	cs := newCodeServer(t, "code-1", "right-verifier")
	_, err := exchangeLoginCode(t.Context(), cs.URL, "code-1", "wrong-verifier")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "400")
}

func TestHandleAuthURLDeliversCode(t *testing.T) {
	app := newTestApp(t)
	resultCh := make(chan *pkceResult, 1)
	app.mu.Lock()
	app.pkceResultCh = resultCh
	app.pkceState = "state-1"
	app.mu.Unlock()

	app.HandleAuthURL(loginRedirectURI + "?code=code-1&state=state-1")

	result := <-resultCh
	require.NoError(t, result.Err)
	assert.Equal(t, "code-1", result.Code)
}

func TestHandleAuthURLIgnoresForeignState(t *testing.T) {
	app := newTestApp(t)
	resultCh := make(chan *pkceResult, 1)
	app.mu.Lock()
	app.pkceResultCh = resultCh
	app.pkceState = "state-1"
	app.mu.Unlock()

	app.HandleAuthURL(loginRedirectURI + "?code=code-1&state=someone-else")
	app.HandleAuthURL(loginRedirectURI + "?code=code-1")

	assert.Empty(t, resultCh, "a redirect this flow did not start must not complete it")
}

func TestHandleAuthURLIgnoresTokens(t *testing.T) {
	app := newTestApp(t)
	resultCh := make(chan *pkceResult, 1)
	app.mu.Lock()
	app.pkceResultCh = resultCh
	app.pkceState = "state-1"
	app.mu.Unlock()

	// The token redirect is not accepted: without a code there is nothing to redeem.
	app.HandleAuthURL(loginRedirectURI + "?token=jwt-123&refresh_token=rt-456&state=state-1")

	result := <-resultCh
	require.Error(t, result.Err)
	assert.Contains(t, result.Err.Error(), "no sign-in code")
}

func TestHandleAuthURLError(t *testing.T) {
	app := newTestApp(t)
	resultCh := make(chan *pkceResult, 1)
	app.mu.Lock()
	app.pkceResultCh = resultCh
	app.pkceState = "state-1"
	app.mu.Unlock()

	app.HandleAuthURL(loginRedirectURI + "?error=access_denied")

	result := <-resultCh
	require.Error(t, result.Err)
	assert.Contains(t, result.Err.Error(), "access_denied")
}

func TestHandleAuthURLSecondRedirectDoesNotBlock(t *testing.T) {
	app := newTestApp(t)
	resultCh := make(chan *pkceResult, 1)
	app.mu.Lock()
	app.pkceResultCh = resultCh
	app.pkceState = "state-1"
	app.mu.Unlock()

	app.HandleAuthURL(loginRedirectURI + "?code=code-1&state=state-1")
	app.HandleAuthURL(loginRedirectURI + "?code=code-2&state=state-1")

	assert.Equal(t, "code-1", (<-resultCh).Code)
}

func TestHandleAuthURLNoActiveFlow(t *testing.T) {
	app := newTestApp(t)
	// Should not panic when no login flow is active.
	app.HandleAuthURL(loginRedirectURI + "?code=code-1&state=state-1")
}
