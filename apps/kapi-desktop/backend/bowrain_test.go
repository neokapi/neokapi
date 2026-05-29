package backend

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	bowrainschema "github.com/neokapi/neokapi/bowrain/plugin/schema"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

// --- Recipe server: block writing / reading ---

func TestWriteAndReadServerBlock(t *testing.T) {
	app := NewApp()
	tab := newTestProject(t, app, "ServerBlock")
	op := app.getOpenProject(tab.ID)
	require.NotNil(t, op)

	// Initially no server block.
	spec, err := readServerSpec(op.Project)
	require.NoError(t, err)
	assert.Nil(t, spec)

	// Write a valid compound project URL (publish-time shape). A bare server
	// URL is intentionally NOT a valid server: block (the schema requires a
	// project ID), so writeServerBlock always receives a full URL.
	full := bowrainschema.FormatProjectURL("https://bowrain.example.com", "acme", "proj123")
	err = app.writeServerBlock(tab.ID, op, &bowrainschema.ServerSpec{URL: full})
	require.NoError(t, err)

	// Read it back from the in-memory project.
	spec, err = readServerSpec(op.Project)
	require.NoError(t, err)
	require.NotNil(t, spec)
	assert.Equal(t, full, spec.URL)

	// It must round-trip through disk: reload the saved recipe and confirm the
	// server: block survived (the bowrain schema decoder validates on load).
	reloaded, err := project.Load(op.Path)
	require.NoError(t, err)
	rspec, err := readServerSpec(reloaded)
	require.NoError(t, err)
	require.NotNil(t, rspec)
	assert.Equal(t, full, rspec.URL)
	assert.Equal(t, "proj123", rspec.ProjectID())
}

func TestWriteBareServerBlockRejected(t *testing.T) {
	app := NewApp()
	tab := newTestProject(t, app, "BareServerBlock")
	op := app.getOpenProject(tab.ID)
	require.NotNil(t, op)

	// A bare server URL has no project ID — the schema rejects it.
	err := app.writeServerBlock(tab.ID, op, &bowrainschema.ServerSpec{URL: "https://bowrain.example.com"})
	require.Error(t, err)
}

func TestWriteServerBlockWithProjectURLValidates(t *testing.T) {
	app := NewApp()
	tab := newTestProject(t, app, "ServerBlockClaimed")
	op := app.getOpenProject(tab.ID)
	require.NotNil(t, op)

	full := bowrainschema.FormatProjectURL("https://bowrain.example.com", "acme", "proj123")
	err := app.writeServerBlock(tab.ID, op, &bowrainschema.ServerSpec{URL: full})
	require.NoError(t, err)

	spec, err := readServerSpec(op.Project)
	require.NoError(t, err)
	require.NotNil(t, spec)
	assert.Equal(t, "https://bowrain.example.com", spec.ServerURL())
	assert.Equal(t, "acme", spec.Workspace())
	assert.Equal(t, "proj123", spec.ProjectID())
}

func TestGetBowrainConnectionDisconnected(t *testing.T) {
	app := NewApp()
	tab := newTestProject(t, app, "Disconnected")
	conn, err := app.GetBowrainConnection(tab.ID)
	require.NoError(t, err)
	require.NotNil(t, conn)
	assert.False(t, conn.Connected)
	assert.False(t, conn.Authenticated)
	assert.Empty(t, conn.ServerURL)
}

func TestGetBowrainConnectionConnected(t *testing.T) {
	keyring.MockInit()
	t.Setenv("BOWRAIN_CONFIG_DIR", t.TempDir())

	app := NewApp()
	tab := newTestProject(t, app, "Connected")
	op := app.getOpenProject(tab.ID)
	require.NotNil(t, op)

	full := bowrainschema.FormatProjectURL("https://bowrain.example.com", "acme", "proj123")
	require.NoError(t, app.writeServerBlock(tab.ID, op, &bowrainschema.ServerSpec{URL: full}))

	// Store a token for that server.
	require.NoError(t, saveBowrainAuth(bowrainStoredAuth{
		ServerURL:   "https://bowrain.example.com",
		AccessToken: "tok-123",
		User:        bowrainStoredUser{Email: "dev@example.com"},
	}))

	conn, err := app.GetBowrainConnection(tab.ID)
	require.NoError(t, err)
	assert.True(t, conn.Connected)
	assert.Equal(t, "https://bowrain.example.com", conn.ServerURL)
	assert.Equal(t, "proj123", conn.ProjectID)
	assert.True(t, conn.Authenticated)
	assert.Equal(t, "dev@example.com", conn.UserEmail)
}

func TestDisconnectBowrainPublished(t *testing.T) {
	keyring.MockInit()
	t.Setenv("BOWRAIN_CONFIG_DIR", t.TempDir())

	app := NewApp()
	tab := newTestProject(t, app, "ToDisconnect")
	op := app.getOpenProject(tab.ID)
	full := bowrainschema.FormatProjectURL("https://bowrain.example.com", "acme", "p1")
	require.NoError(t, app.writeServerBlock(tab.ID, op, &bowrainschema.ServerSpec{URL: full}))
	require.NoError(t, saveBowrainAuth(bowrainStoredAuth{ServerURL: "https://bowrain.example.com", AccessToken: "tok"}))

	require.NoError(t, app.DisconnectBowrain(tab.ID))

	conn, err := app.GetBowrainConnection(tab.ID)
	require.NoError(t, err)
	assert.False(t, conn.Connected)
	assert.False(t, conn.Authenticated)

	// Recipe on disk no longer has a server: block.
	reloaded, err := project.Load(op.Path)
	require.NoError(t, err)
	spec, err := readServerSpec(reloaded)
	require.NoError(t, err)
	assert.Nil(t, spec)
}

// TestConnectedNotPublished covers the intermediate state: authenticated (token
// + server in auth metadata) but no server: block in the recipe yet.
func TestConnectedNotPublished(t *testing.T) {
	keyring.MockInit()
	t.Setenv("BOWRAIN_CONFIG_DIR", t.TempDir())

	app := NewApp()
	tab := newTestProject(t, app, "ConnectedNotPublished")

	require.NoError(t, saveBowrainAuth(bowrainStoredAuth{
		ServerURL:   "https://bowrain.example.com",
		AccessToken: "tok",
		User:        bowrainStoredUser{Email: "dev@example.com"},
	}))

	conn, err := app.GetBowrainConnection(tab.ID)
	require.NoError(t, err)
	assert.True(t, conn.Connected, "authenticated counts as connected")
	assert.True(t, conn.Authenticated)
	assert.Equal(t, "https://bowrain.example.com", conn.ServerURL)
	assert.Empty(t, conn.ProjectID, "no server-side project yet")

	// Disconnect clears the stored auth even with no recipe server block.
	require.NoError(t, app.DisconnectBowrain(tab.ID))
	conn, err = app.GetBowrainConnection(tab.ID)
	require.NoError(t, err)
	assert.False(t, conn.Authenticated)
}

// --- Auth storage round-trip ---

func TestBowrainAuthRoundTrip(t *testing.T) {
	keyring.MockInit()
	t.Setenv("BOWRAIN_CONFIG_DIR", t.TempDir())

	in := bowrainStoredAuth{
		ServerURL:    "https://bowrain.example.com",
		AccessToken:  "access-xyz",
		RefreshToken: "refresh-abc",
		User:         bowrainStoredUser{ID: "u1", Email: "a@b.com", Name: "Dev"},
	}
	require.NoError(t, saveBowrainAuth(in))

	out, err := loadBowrainAuth("https://bowrain.example.com")
	require.NoError(t, err)
	assert.Equal(t, "access-xyz", out.AccessToken)
	assert.Equal(t, "refresh-abc", out.RefreshToken)
	assert.Equal(t, "a@b.com", out.User.Email)

	// Keychain keys must match the kapi-bowrain plugin convention so a later
	// `kapi sync` finds the same token.
	got, err := keyring.Get("kapi", "bowrain-auth:https://bowrain.example.com")
	require.NoError(t, err)
	assert.Equal(t, "access-xyz", got)
}

func TestBowrainAuthEnvBypass(t *testing.T) {
	t.Setenv("BOWRAIN_AUTH_TOKEN", "ci-token")
	t.Setenv("BOWRAIN_SERVER_URL", "https://ci.example.com")

	out, err := loadBowrainAuth("")
	require.NoError(t, err)
	assert.Equal(t, "ci-token", out.AccessToken)
	assert.Equal(t, "https://ci.example.com", out.ServerURL)
}

// --- OAuth / PKCE URL construction ---

func TestPKCEChallenge(t *testing.T) {
	v, err := generateCodeVerifier()
	require.NoError(t, err)
	// 32 random bytes -> 43 base64url chars (no padding).
	assert.Len(t, v, 43)
	assert.NotContains(t, v, "=")

	c := computeCodeChallenge(v)
	// Verify it is BASE64URL(SHA256(verifier)).
	h := sha256.Sum256([]byte(v))
	assert.Equal(t, base64.RawURLEncoding.EncodeToString(h[:]), c)

	// Two verifiers differ.
	v2, _ := generateCodeVerifier()
	assert.NotEqual(t, v, v2)
}

func TestBuildDesktopLoginURL(t *testing.T) {
	loginURL := buildDesktopLoginURL("https://bowrain.example.com/", "http://127.0.0.1:54321/callback", "challengeABC")
	u, err := url.Parse(loginURL)
	require.NoError(t, err)
	assert.Equal(t, "https", u.Scheme)
	assert.Equal(t, "bowrain.example.com", u.Host)
	assert.Equal(t, "/api/v1/auth/desktop/login", u.Path)

	q := u.Query()
	assert.Equal(t, "http://127.0.0.1:54321/callback", q.Get("redirect_uri"))
	assert.Equal(t, "challengeABC", q.Get("code_challenge"))
	assert.Equal(t, "S256", q.Get("code_challenge_method"))
}

func TestNormalizeServerURL(t *testing.T) {
	cases := map[string]string{
		"https://bowrain.example.com":    "https://bowrain.example.com",
		"https://bowrain.example.com/":   "https://bowrain.example.com",
		"bowrain.example.com":            "https://bowrain.example.com",
		"http://localhost:8080":          "http://localhost:8080",
		"  https://x.example.com/path  ": "https://x.example.com",
		"":                               "",
	}
	for in, want := range cases {
		assert.Equal(t, want, normalizeServerURL(in), "input %q", in)
	}
}

// --- REST client (httptest) ---

func TestRESTCreateAnonymousProject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/projects/anonymous", r.URL.Path)
		var req anonymousProjectRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "My App", req.Name)
		assert.Equal(t, "en-US", req.DefaultSourceLanguage)
		assert.Equal(t, []string{"fr-FR"}, req.TargetLanguages)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(anonymousProjectResponse{ProjectID: "p1", ClaimToken: "claim-xyz"})
	}))
	defer srv.Close()

	client := &bowrainRESTClient{ServerURL: srv.URL, HTTPClient: srv.Client()}
	out, err := client.CreateAnonymousProject(context.Background(), anonymousProjectRequest{
		Name:                  "My App",
		DefaultSourceLanguage: "en-US",
		TargetLanguages:       []string{"fr-FR"},
	})
	require.NoError(t, err)
	assert.Equal(t, "p1", out.ProjectID)
	assert.Equal(t, "claim-xyz", out.ClaimToken)
}

func TestRESTClaimProject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/projects/claim", r.URL.Path)
		assert.Equal(t, "Bearer tok-abc", r.Header.Get("Authorization"))
		var body map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "claim-xyz", body["claim_token"])
		_ = json.NewEncoder(w).Encode(claimResponse{ProjectID: "p1", WorkspaceSlug: "acme"})
	}))
	defer srv.Close()

	client := &bowrainRESTClient{ServerURL: srv.URL, HTTPClient: srv.Client()}
	out, err := client.ClaimProject(context.Background(), "tok-abc", "claim-xyz")
	require.NoError(t, err)
	assert.Equal(t, "p1", out.ProjectID)
	assert.Equal(t, "acme", out.WorkspaceSlug)
}

func TestRESTErrorSurfaced(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"name and default_source_language are required"}`))
	}))
	defer srv.Close()

	client := &bowrainRESTClient{ServerURL: srv.URL, HTTPClient: srv.Client()}
	_, err := client.CreateAnonymousProject(context.Background(), anonymousProjectRequest{
		Name:                  "x",
		DefaultSourceLanguage: "en",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 400")
}

// --- Desktop login loopback (end-to-end with a fake server) ---

// TestRunDesktopLoginLoopback drives the real loopback flow against a fake
// "bowrain server" that immediately redirects the browser-opened login URL
// back to the desktop's loopback callback with tokens — exactly as the real
// HandleDesktopLogin -> HandleDesktopCallback pair does.
func TestRunDesktopLoginLoopback(t *testing.T) {
	keyring.MockInit()
	t.Setenv("BOWRAIN_CONFIG_DIR", t.TempDir())

	// Stub the browser: instead of launching a real browser, simulate it
	// navigating to the login URL. The fake server then drives the loopback
	// callback (mirroring the real HandleDesktopLogin -> HandleDesktopCallback
	// redirect chain). The GET runs in a goroutine so OpenURL returns promptly.
	orig := openBrowser
	openBrowser = func(loginURL string) error {
		go func() {
			resp, err := http.Get(loginURL)
			_ = err
			if resp != nil {
				_ = resp.Body.Close()
			}
		}() //nolint:errcheck
		return nil
	}
	t.Cleanup(func() { openBrowser = orig })

	var fakeServer *httptest.Server
	fakeServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/desktop/login" {
			http.NotFound(w, r)
			return
		}
		// Validate the desktop supplied PKCE + a loopback redirect_uri.
		q := r.URL.Query()
		require.NotEmpty(t, q.Get("code_challenge"))
		assert.Equal(t, "S256", q.Get("code_challenge_method"))
		redirect := q.Get("redirect_uri")
		require.True(t, strings.HasPrefix(redirect, "http://127.0.0.1:"), "redirect must be loopback: %s", redirect)

		// Simulate the server completing OIDC and redirecting back with tokens.
		target, _ := url.Parse(redirect)
		rq := target.Query()
		rq.Set("token", "server-issued-jwt")
		rq.Set("refresh_token", "server-issued-refresh")
		rq.Set("user", "dev@example.com")
		rq.Set("name", "Dev Example")
		target.RawQuery = rq.Encode()
		if resp, err := http.Get(target.String()); err == nil { //nolint:noctx // test helper drives the loopback callback
			_ = resp.Body.Close()
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer fakeServer.Close()

	app := NewApp()
	auth, err := app.runDesktopLogin(context.Background(), fakeServer.URL)
	require.NoError(t, err)
	assert.Equal(t, "server-issued-jwt", auth.AccessToken)
	assert.Equal(t, "server-issued-refresh", auth.RefreshToken)
	assert.Equal(t, "dev@example.com", auth.User.Email)
	assert.Equal(t, "Dev Example", auth.User.Name)
	assert.Equal(t, fakeServer.URL, auth.ServerURL)
}

// TestPublishBowrainEndToEnd drives the publish step against a fake server that
// serves the anonymous-create and claim endpoints, then asserts the recipe's
// server: block records the resulting compound URL.
func TestPublishBowrainEndToEnd(t *testing.T) {
	keyring.MockInit()
	t.Setenv("BOWRAIN_CONFIG_DIR", t.TempDir())

	fakeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/projects/anonymous":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(anonymousProjectResponse{ProjectID: "proj42", ClaimToken: "claim42"})
		case "/api/v1/projects/claim":
			assert.Equal(t, "Bearer jwt-token", r.Header.Get("Authorization"))
			_ = json.NewEncoder(w).Encode(claimResponse{ProjectID: "proj42", WorkspaceSlug: "acme"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer fakeServer.Close()

	app := NewApp()
	tab := newTestProject(t, app, "PublishE2E")

	// Simulate a prior Connect: token in keychain + server in auth metadata.
	require.NoError(t, saveBowrainAuth(bowrainStoredAuth{
		ServerURL:   fakeServer.URL,
		AccessToken: "jwt-token",
		User:        bowrainStoredUser{Email: "dev@example.com"},
	}))

	// Point the publish client at the fake server's transport.
	res, err := app.PublishBowrain(tab.ID)
	require.NoError(t, err)
	assert.Equal(t, "proj42", res.ProjectID)
	assert.True(t, res.Delegated)
	assert.NotEmpty(t, res.SyncHint)
	assert.Equal(t, bowrainschema.FormatProjectURL(fakeServer.URL, "acme", "proj42"), res.ProjectURL)
	assert.Empty(t, res.ClaimToken, "claim token consumed once claimed")

	// The recipe now carries a valid server: block, and reloads cleanly.
	op := app.getOpenProject(tab.ID)
	reloaded, err := project.Load(op.Path)
	require.NoError(t, err)
	spec, err := readServerSpec(reloaded)
	require.NoError(t, err)
	require.NotNil(t, spec)
	assert.Equal(t, "proj42", spec.ProjectID())
	assert.Equal(t, "acme", spec.Workspace())
}
