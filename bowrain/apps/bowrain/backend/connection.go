package backend

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	venueauth "github.com/neokapi/neokapi/host/venue/auth"

	"github.com/neokapi/neokapi/bowrain/editorclient"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
	"github.com/neokapi/neokapi/host/venue/config"
)

var errNotConnected = errors.New("not connected to server")

// errAuthRequired marks a connection failure no retry can fix: there are no
// usable credentials for the server, so the user has to log in again. The
// reconnect loop stops on it rather than backing off forever against a server
// that will keep refusing.
var errAuthRequired = errors.New("not authenticated")

// connectProbeTimeout bounds the reachability probe a connection attempt makes.
// It is short because the caller is either a user waiting at the connect screen
// or a reconnect attempt on a timer; a probe that outlives its own retry
// interval is worse than a failed one.
const connectProbeTimeout = 10 * time.Second

// DefaultServerURL is the Bowrain SaaS instance URL used when no custom server is specified.
const DefaultServerURL = config.DefaultServerURL

// ConnectionState represents the connection state of the desktop client.
type ConnectionState string

const (
	StateDisconnected ConnectionState = "disconnected"
	StateConnecting   ConnectionState = "connecting"
	StateConnected    ConnectionState = "connected"
	StateOffline      ConnectionState = "offline"
)

// ConnectionInfo is the connection status exposed to the frontend.
type ConnectionInfo struct {
	State     ConnectionState `json:"state"`
	ServerURL string          `json:"server_url,omitempty"`
	UserName  string          `json:"user_name,omitempty"`
	UserEmail string          `json:"user_email,omitempty"`
	Workspace string          `json:"workspace,omitempty"`
}

// pkceResult is what the sign-in redirect delivered: a one-time code to redeem
// with the PKCE verifier, or the reason the sign-in failed.
type pkceResult struct {
	Code string
	Err  error
}

// loginRedirectURI is where the server sends the browser after sign-in. The OS
// routes bowrain:// URLs to this app through the registered protocol handler.
const loginRedirectURI = "bowrain://auth/callback"

// GetConnectionState returns the current connection info.
// If BOWRAIN_TOKEN is set and no connection exists, auto-connects to the
// server (for CI/headless mode where interactive login is not possible).
func (a *App) GetConnectionState() ConnectionInfo {
	a.mu.RLock()
	state := a.connState
	autoConnectDone := a.autoConnectDone
	a.mu.RUnlock()

	// Auto-connect via pre-supplied token (CI/headless mode). Only attempt once.
	if state == StateDisconnected && !autoConnectDone {
		if token := os.Getenv("BOWRAIN_TOKEN"); token != "" {
			a.mu.Lock()
			a.autoConnectDone = true
			a.mu.Unlock()
			serverURL := a.GetDefaultServerURL()
			if err := a.connectWithToken(serverURL, token); err != nil {
				slog.Info("bowrain: auto-connect failed", "error", err)
			}
		}
	}

	a.mu.RLock()
	defer a.mu.RUnlock()
	info := ConnectionInfo{
		State:     a.connState,
		ServerURL: a.serverURL,
		Workspace: a.activeWS,
	}
	if a.authInfo != nil {
		info.UserName = a.authInfo.User.Name
		info.UserEmail = a.authInfo.User.Email
	}
	return info
}

// connectWithToken establishes a server connection using a pre-supplied JWT token.
// Used by CI/headless mode (BOWRAIN_TOKEN env var) to skip interactive OIDC login.
func (a *App) connectWithToken(serverURL, token string) error {
	serverURL = strings.TrimRight(serverURL, "/")

	a.mu.Lock()
	a.remoteHTTP = editorclient.New(serverURL, token)
	a.authInfo = &config.StoredAuth{
		ServerURL:   serverURL,
		AccessToken: token,
		User:        config.StoredUser{Email: "ci@bowrain.cloud", Name: "CI"},
	}
	a.connState = StateConnected
	a.serverURL = serverURL
	a.mu.Unlock()

	// Don't auto-select workspace: the test seeder creates the workspace
	// after the binary starts, so it may not exist yet. The frontend's
	// setupServerApp helper handles workspace selection after seeding.
	slog.Info("bowrain: auto-connected via BOWRAIN_TOKEN", "server_url", serverURL)

	// Notify the frontend so its connection state hook re-fetches and the
	// app transitions out of the connect screen. Without this, the React
	// useEffect that ran connection.refresh() before this auto-connect
	// completed has already set mode="connecting".
	a.emit("connection-state-changed", ConnectionInfo{
		State:     StateConnected,
		ServerURL: serverURL,
		UserName:  "CI",
		UserEmail: "ci@bowrain.cloud",
	})
	return nil
}

// GetDefaultServerURL returns the default server URL for the desktop app.
// It checks the BOWRAIN_SERVER_URL environment variable first, falling back
// to the Bowrain SaaS instance URL.
func (a *App) GetDefaultServerURL() string {
	if envURL := os.Getenv("BOWRAIN_SERVER_URL"); envURL != "" {
		return config.NormalizeServerURL(envURL)
	}
	return DefaultServerURL
}

// isConnected returns true when a server connection is active.
func (a *App) isConnected() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.connState == StateConnected && a.remoteHTTP != nil
}

// isOffline returns true when the app has lost its server connection
// and is operating from the local cache with changes queued.
func (a *App) isOffline() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.connState == StateOffline
}

// GetPendingChangesCount returns the number of queued offline changes.
// Exposed to the frontend so it can show a pending sync indicator.
func (a *App) GetPendingChangesCount() int {
	if a.offlineQueue == nil {
		return 0
	}
	return a.offlineQueue.PendingCount()
}

// credentialsFor returns the credentials to present to serverURL, preferring the
// ones already held in memory over a keychain read. The in-memory copy is what
// makes a session that never touched the keychain reconnectable: a BOWRAIN_TOKEN
// auto-connect (headless and recording runs) holds its token only in authInfo,
// and a reconnect that consulted the keychain alone would find nothing for the
// server it is connected to and give up.
//
// An access token past its expiry is still usable when a refresh token sits
// beside it: the transport spends the refresh on the first 401. Only a missing
// token, a token for another server, or an expired one with nothing to refresh
// it is errAuthRequired.
func (a *App) credentialsFor(serverURL string) (*config.StoredAuth, error) {
	a.mu.RLock()
	var cached *config.StoredAuth
	if a.authInfo != nil {
		copied := *a.authInfo
		cached = &copied
	}
	a.mu.RUnlock()

	if cached != nil && config.NormalizeServerURL(cached.ServerURL) == serverURL && cached.AccessToken != "" {
		return cached, nil
	}

	stored, err := loadDesktopAuth()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errAuthRequired, err)
	}
	if config.NormalizeServerURL(stored.ServerURL) != serverURL || stored.AccessToken == "" {
		return nil, fmt.Errorf("%w: no stored credentials for %s", errAuthRequired, serverURL)
	}
	if !stored.Expiry.IsZero() && time.Now().After(stored.Expiry) && stored.RefreshToken == "" {
		return nil, fmt.Errorf("%w: the access token for %s expired and there is no refresh token", errAuthRequired, serverURL)
	}
	return stored, nil
}

// ConnectToServer establishes a REST/SSE connection to the given server URL
// using stored credentials. The URL should be the HTTP base URL
// (e.g. "http://localhost:8080").
//
// The connection is confirmed with a round trip before it is reported as one:
// a stored token proves the user logged in once, not that the server is
// reachable now. Reporting a connection the network cannot carry sends the next
// write straight into the offline queue and the reconnect loop straight into a
// success it did not have.
//
// A failed attempt leaves an offline app offline. Offline is a working copy with
// a queue behind it; disconnected is the sign-in screen, and demoting between
// them on a network error throws the user out of the app mid-outage.
func (a *App) ConnectToServer(serverURL string) error {
	return a.connect(context.Background(), serverURL)
}

// connect is ConnectToServer with a caller-supplied context. The reconnect loop
// passes its own, so cancelling it (Disconnect, shutdown) aborts a probe in
// flight instead of leaving it to time out.
func (a *App) connect(ctx context.Context, serverURL string) error {
	serverURL = config.NormalizeServerURL(serverURL)

	a.mu.Lock()
	previous := a.connState
	a.serverURL = serverURL
	if previous == StateDisconnected {
		a.connState = StateConnecting
	}
	a.mu.Unlock()

	restore := func() {
		a.mu.Lock()
		if a.connState == StateConnecting {
			a.connState = previous
		}
		a.mu.Unlock()
	}

	stored, err := a.credentialsFor(serverURL)
	if err != nil {
		restore()
		return err
	}

	editorClient := editorclient.New(serverURL, stored.AccessToken)
	a.wireRemoteRefresh(editorClient, serverURL, stored)

	probeCtx, cancel := context.WithTimeout(ctx, connectProbeTimeout)
	defer cancel()
	if err := editorClient.Ping(probeCtx); err != nil {
		restore()
		if rejectedSession(err) {
			return fmt.Errorf("%w: the server rejected the stored session: %w", errAuthRequired, err)
		}
		return fmt.Errorf("reach %s: %w", serverURL, err)
	}

	a.mu.Lock()
	a.remoteHTTP = editorClient
	a.authInfo = stored
	a.connState = StateConnected
	a.mu.Unlock()

	return nil
}

// rejectedSession reports whether an error is the server refusing the
// credentials (401/403) rather than the network refusing the request. The
// transport has already spent its refresh attempt by the time such an error
// surfaces, so re-presenting the same token can only be refused again.
func rejectedSession(err error) bool {
	var statusErr *apiclient.StatusError
	if !errors.As(err, &statusErr) {
		return false
	}
	return statusErr.StatusCode == http.StatusUnauthorized || statusErr.StatusCode == http.StatusForbidden
}

// wireRemoteRefresh configures the REST/SSE editor client to auto-refresh its
// access token on 401 and persist the rotated tokens through the shared
// host/venue/config store, keeping the cached auth in sync so a refresh
// triggered by any surface (including the CLI) is visible.
func (a *App) wireRemoteRefresh(c *editorclient.EditorClient, serverURL string, stored *config.StoredAuth) {
	if stored == nil || stored.RefreshToken == "" {
		return
	}
	c.SetRefreshToken(stored.RefreshToken, func(newAccess, newRefresh string) {
		a.mu.Lock()
		if a.authInfo != nil {
			a.authInfo.AccessToken = newAccess
			a.authInfo.RefreshToken = newRefresh
		}
		updated := a.authInfo
		a.mu.Unlock()
		if updated != nil {
			_ = config.SaveAuth(*updated)
		}
	})
}

// StartLogin begins an authorization code + PKCE flow against the server and
// opens the system browser at the server's desktop sign-in endpoint. The
// redirect back to the app carries a one-time code, never a token: any app can
// register the bowrain:// scheme, so the code is useless without the PKCE
// verifier this app holds. The frontend calls WaitForLogin to block until the
// redirect arrives and the code is redeemed.
func (a *App) StartLogin(serverURL string) error {
	serverURL = strings.TrimRight(serverURL, "/")

	// Verify this is a valid Bowrain server before opening the browser.
	client := &http.Client{Timeout: 5 * time.Second}
	healthReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, serverURL+"/api/v1/health", nil)
	if err != nil {
		return fmt.Errorf("build health request: %w", err)
	}
	resp, err := client.Do(healthReq)
	if err != nil {
		return fmt.Errorf("cannot reach server: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("not a valid Bowrain server (health check returned %d)", resp.StatusCode)
	}

	verifier, err := venueauth.GenerateCodeVerifier()
	if err != nil {
		return fmt.Errorf("generate PKCE verifier: %w", err)
	}
	state, err := randomState()
	if err != nil {
		return fmt.Errorf("generate sign-in state: %w", err)
	}

	resultCh := make(chan *pkceResult, 1)

	a.mu.Lock()
	a.serverURL = serverURL
	a.pkceVerifier = verifier
	a.pkceState = state
	a.pkceResultCh = resultCh
	a.mu.Unlock()

	if a.app != nil {
		_ = a.app.Browser.OpenURL(desktopLoginURL(serverURL, venueauth.ComputeCodeChallenge(verifier), state))
	}

	return nil
}

// desktopLoginURL is the server's sign-in endpoint, asking for the one-time
// code redirect (response_mode=code) bound to the PKCE challenge and state.
func desktopLoginURL(serverURL, challenge, state string) string {
	q := url.Values{
		"redirect_uri":          {loginRedirectURI},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {state},
		"response_mode":         {"code"},
	}
	return serverURL + "/api/v1/auth/desktop/login?" + q.Encode()
}

// randomState returns an unguessable value for the sign-in state parameter.
func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// WaitForLogin blocks until the sign-in redirect is received or a timeout
// occurs, then redeems the one-time code for tokens. Returns true when
// authentication succeeds.
func (a *App) WaitForLogin() (bool, error) {
	a.mu.RLock()
	resultCh := a.pkceResultCh
	serverURL := a.serverURL
	verifier := a.pkceVerifier
	a.mu.RUnlock()

	if resultCh == nil {
		return false, errors.New("no active login flow. Call StartLogin first")
	}

	var result *pkceResult
	select {
	case result = <-resultCh:
		a.cleanupPKCE()
	case <-time.After(10 * time.Minute):
		a.cleanupPKCE()
		return false, errors.New("login timed out")
	}
	if result.Err != nil {
		return false, result.Err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tokens, err := exchangeLoginCode(ctx, serverURL, result.Code, verifier)
	if err != nil {
		return false, err
	}

	// Save tokens to the shared host/venue/config store (OS keychain +
	// non-secret metadata), so a desktop login and a kapi CLI login share
	// one set of credentials.
	stored := &config.StoredAuth{
		ServerURL:    serverURL,
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		Expiry:       time.Now().Add(24 * time.Hour),
	}

	// The redirect carries no identity, so the user comes from the server.
	if user, err := apiclient.FetchUser(ctx, serverURL, tokens.AccessToken); err == nil {
		stored.User = config.StoredUser{ID: user.ID, Email: user.Email, Name: user.Name}
	}

	if err := config.SaveAuth(*stored); err != nil {
		return true, fmt.Errorf("save auth: %w", err)
	}

	a.mu.Lock()
	a.authInfo = stored
	a.mu.Unlock()

	return true, nil
}

// exchangeLoginCode redeems a one-time sign-in code at the server's desktop
// token endpoint. The tokens travel only in the response body.
func exchangeLoginCode(ctx context.Context, serverURL, code, verifier string) (*venueauth.TokenResponse, error) {
	body, err := json.Marshal(map[string]string{
		"code":          code,
		"code_verifier": verifier,
		"redirect_uri":  loginRedirectURI,
	})
	if err != nil {
		return nil, fmt.Errorf("encode sign-in code: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+"/api/v1/auth/desktop/token", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("redeem sign-in code: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, apiclient.NewStatusError("redeem sign-in code", resp.StatusCode, respBody)
	}
	var tokens venueauth.TokenResponse
	if err := json.Unmarshal(respBody, &tokens); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}
	if tokens.AccessToken == "" {
		return nil, errors.New("redeem sign-in code: the server returned no access token")
	}
	return &tokens, nil
}

// HandleAuthURL processes a bowrain:// URL received via the OS protocol handler.
// Expected format: bowrain://auth/callback?code=...&state=...
func (a *App) HandleAuthURL(rawURL string) {
	a.mu.RLock()
	resultCh := a.pkceResultCh
	wantState := a.pkceState
	a.mu.RUnlock()

	if resultCh == nil {
		slog.Info("bowrain: received auth URL but no login flow is active")
		return
	}

	deliver := func(r *pkceResult) {
		// The channel holds one result; a second redirect for the same flow
		// is dropped rather than blocking the protocol handler.
		select {
		case resultCh <- r:
		default:
			slog.Info("bowrain: ignored a second auth URL for the active login flow")
		}
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		deliver(&pkceResult{Err: fmt.Errorf("invalid auth URL: %w", err)})
		return
	}

	q := parsed.Query()
	if errMsg := q.Get("error"); errMsg != "" {
		deliver(&pkceResult{Err: fmt.Errorf("auth failed: %s", errMsg)})
		return
	}
	if wantState == "" || subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(wantState)) != 1 {
		// A redirect this flow did not start: ignore it and keep waiting.
		slog.Info("bowrain: ignored an auth URL whose state does not match the active login flow")
		return
	}
	code := q.Get("code")
	if code == "" {
		deliver(&pkceResult{Err: errors.New("auth failed: no sign-in code received")})
		return
	}
	deliver(&pkceResult{Code: code})
}

// HandleDeepLink processes a deep link web URL (after stripping the "bowrain:" prefix).
// The URL is a standard web URL like https://bowrain.cloud/ws/acme/projects/proj_123.
// It parses path components and emits a "deep-link-project" event to the frontend.
func (a *App) HandleDeepLink(webURL string) {
	parsed, err := url.Parse(webURL)
	if err != nil {
		slog.Info("bowrain: invalid deep link URL", "error", err)
		return
	}

	// Reconstruct the server URL (scheme + host).
	serverURL := parsed.Scheme + "://" + parsed.Host

	// Parse path: /ws/{workspace}/projects/{projectId}[/files/{fileId}]
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")

	var workspace, projectID string
	for i := range len(segments) - 1 {
		switch segments[i] {
		case "ws":
			workspace = segments[i+1]
		case "projects":
			projectID = segments[i+1]
		}
	}

	if projectID == "" {
		slog.Info("bowrain: deep link missing project ID:", "value", webURL)
		return
	}

	a.emit("deep-link-project", map[string]string{
		"project_id": projectID,
		"server_url": serverURL,
		"workspace":  workspace,
	})
}

// CancelLogin cancels any active PKCE login flow.
func (a *App) CancelLogin() {
	a.cleanupPKCE()
}

// cleanupPKCE clears PKCE state.
func (a *App) cleanupPKCE() {
	a.mu.Lock()
	a.pkceVerifier = ""
	a.pkceState = ""
	a.pkceResultCh = nil
	a.mu.Unlock()
}

// Logout removes stored auth and disconnects.
func (a *App) Logout() {
	a.mu.RLock()
	serverURL := a.serverURL
	a.mu.RUnlock()
	if serverURL == "" {
		if stored, err := loadDesktopAuth(); err == nil {
			serverURL = stored.ServerURL
		}
	}
	a.Disconnect()
	// Clear credentials from the shared host/venue/config store (keychain +
	// metadata). Missing entries are not an error.
	_ = config.DeleteAuth(serverURL)
}

// Disconnect closes the server connection.
func (a *App) Disconnect() {
	a.stopReconnect()
	a.StopWatching()

	a.mu.Lock()
	a.remoteHTTP = nil
	a.connState = StateDisconnected
	a.authInfo = nil
	a.activeWS = ""
	a.mu.Unlock()

	// A disconnect is a state change the gate must observe: sign-out and a
	// rejected session demote to the connect screen. emitConnectionState takes
	// the lock, so emit after releasing it.
	a.emitConnectionState()
}

// editorRemote returns the REST editor client and active workspace slug under
// the lock. The client is nil when disconnected; callers gate on isConnected.
func (a *App) editorRemote() (*editorclient.EditorClient, string) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.remoteHTTP, a.activeWS
}

// GetServerWorkspaces returns workspaces from the connected server.
func (a *App) GetServerWorkspaces() ([]WorkspaceInfo, error) {
	if !a.isConnected() {
		return nil, errors.New("not connected")
	}
	client, _ := a.editorRemote()
	ws, err := client.ListWorkspaces(context.Background())
	if err != nil {
		return nil, err
	}
	return editorWorkspacesToInfos(ws), nil
}

// SelectWorkspace sets the active workspace for all subsequent operations.
func (a *App) SelectWorkspace(slug string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.activeWS = slug
	return nil
}

// --- Auth persistence ---
//
// Desktop credentials live in the shared host/venue/config store, the same
// OS keychain scheme (service "kapi", URL-namespaced keys) and metadata file
// (~/.config/bowrain/auth.json) the kapi CLI + kapi-bowrain plugin use. This is
// what makes a desktop login and a CLI login mutually visible. loadDesktopAuth
// first runs a one-time migration off the legacy desktop-only scheme (see
// desktopauth_migrate.go).

func loadDesktopAuth() (*config.StoredAuth, error) {
	migrateLegacyDesktopAuth()
	return config.LoadAuth()
}

// TryAutoConnect attempts to reconnect using stored auth on startup.
func (a *App) TryAutoConnect() {
	stored, err := loadDesktopAuth()
	if err != nil || stored.AccessToken == "" {
		return
	}

	// Don't auto-connect with expired tokens.
	if !stored.Expiry.IsZero() && time.Now().After(stored.Expiry) {
		return
	}

	// Try connecting silently.
	if err := a.ConnectToServer(stored.ServerURL); err != nil {
		return
	}
}
