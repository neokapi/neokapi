package backend

import (
	"context"
	"errors"
	"fmt"
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

// pkceResult is the result received from the local PKCE callback server.
type pkceResult struct {
	AccessToken  string
	RefreshToken string
	UserEmail    string
	UserName     string
	Err          error
}

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

	// Don't auto-select workspace — the test seeder creates the workspace
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

// GetFailedChangesCount returns the number of queued offline changes the
// server permanently rejected on replay (4xx). Exposed to the frontend so it
// can surface that some offline edits did not apply.
func (a *App) GetFailedChangesCount() int {
	if a.offlineQueue == nil {
		return 0
	}
	return a.offlineQueue.FailedCount()
}

// ConnectToServer establishes a REST/SSE connection to the given server URL
// using stored credentials. The URL should be the HTTP base URL
// (e.g. "http://localhost:8080").
func (a *App) ConnectToServer(serverURL string) error {
	serverURL = config.NormalizeServerURL(serverURL)
	a.mu.Lock()
	a.connState = StateConnecting
	a.serverURL = serverURL
	a.mu.Unlock()

	// Load stored auth for this server.
	stored, err := loadDesktopAuth()
	if err != nil || stored.ServerURL != serverURL || stored.AccessToken == "" {
		a.mu.Lock()
		a.connState = StateDisconnected
		a.mu.Unlock()
		return errors.New("not authenticated. Use StartLogin first")
	}

	// Check if token has expired.
	if !stored.Expiry.IsZero() && time.Now().After(stored.Expiry) {
		a.mu.Lock()
		a.connState = StateDisconnected
		a.mu.Unlock()
		return errors.New("token expired. Log in again")
	}

	editorClient := editorclient.New(serverURL, stored.AccessToken)
	a.wireRemoteRefresh(editorClient, serverURL, stored)

	a.mu.Lock()
	a.remoteHTTP = editorClient
	a.authInfo = stored
	a.connState = StateConnected
	a.mu.Unlock()

	return nil
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

// StartLogin begins an authorization code + PKCE flow against the server.
// It starts a local HTTP server to receive the callback, generates PKCE
// parameters, and opens the system browser to the server's desktop login
// endpoint. The frontend should call WaitForLogin to block until the
// callback is received.
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

	// Generate PKCE code verifier + challenge.
	verifier, err := venueauth.GenerateCodeVerifier()
	if err != nil {
		return fmt.Errorf("generate PKCE verifier: %w", err)
	}
	challenge := venueauth.ComputeCodeChallenge(verifier)

	resultCh := make(chan *pkceResult, 1)

	a.mu.Lock()
	a.serverURL = serverURL
	a.pkceVerifier = verifier
	a.pkceResultCh = resultCh
	a.mu.Unlock()

	// Build the desktop login URL with bowrain:// as the redirect URI.
	loginURL := fmt.Sprintf("%s/api/v1/auth/desktop/login?redirect_uri=%s&code_challenge=%s&code_challenge_method=S256",
		serverURL,
		url.QueryEscape("bowrain://auth/callback"),
		url.QueryEscape(challenge),
	)

	// Open the system browser for OIDC login.
	// After authentication, the server redirects to bowrain://auth/callback?token=...
	// which the OS routes back to this app via the registered URL protocol handler.
	if a.app != nil {
		_ = a.app.Browser.OpenURL(loginURL)
	}

	return nil
}

// WaitForLogin blocks until the PKCE callback is received or a timeout occurs.
// Returns true when authentication succeeds.
func (a *App) WaitForLogin() (bool, error) {
	a.mu.RLock()
	resultCh := a.pkceResultCh
	serverURL := a.serverURL
	a.mu.RUnlock()

	if resultCh == nil {
		return false, errors.New("no active login flow. Call StartLogin first")
	}

	// Wait for the callback with a 10-minute timeout.
	select {
	case result := <-resultCh:
		// Shut down the local server.
		a.cleanupPKCE()

		if result.Err != nil {
			return false, result.Err
		}

		// Save tokens to the shared host/venue/config store (OS keychain +
		// non-secret metadata), so a desktop login and a kapi CLI login share
		// one set of credentials.
		stored := &config.StoredAuth{
			ServerURL:    serverURL,
			AccessToken:  result.AccessToken,
			RefreshToken: result.RefreshToken,
			Expiry:       time.Now().Add(24 * time.Hour),
			User: config.StoredUser{
				Email: result.UserEmail,
				Name:  result.UserName,
			},
		}

		// Fetch full user info (including ID) from the server.
		if user, err := apiclient.FetchUser(context.Background(), serverURL, result.AccessToken); err == nil {
			stored.User = config.StoredUser{ID: user.ID, Email: user.Email, Name: user.Name}
		}

		if err := config.SaveAuth(*stored); err != nil {
			return true, fmt.Errorf("save auth: %w", err)
		}

		a.mu.Lock()
		a.authInfo = stored
		a.mu.Unlock()

		return true, nil

	case <-time.After(10 * time.Minute):
		a.cleanupPKCE()
		return false, errors.New("login timed out")
	}
}

// HandleAuthURL processes a bowrain:// URL received via the OS protocol handler.
// Expected format: bowrain://auth/callback?token=...&refresh_token=...&user=...&name=...
func (a *App) HandleAuthURL(rawURL string) {
	a.mu.RLock()
	resultCh := a.pkceResultCh
	a.mu.RUnlock()

	if resultCh == nil {
		slog.Info("bowrain: received auth URL but no login flow is active")
		return
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		resultCh <- &pkceResult{Err: fmt.Errorf("invalid auth URL: %w", err)}
		return
	}

	q := parsed.Query()
	token := q.Get("token")
	if token == "" {
		errMsg := q.Get("error")
		if errMsg == "" {
			errMsg = "no token received"
		}
		resultCh <- &pkceResult{Err: fmt.Errorf("auth failed: %s", errMsg)}
		return
	}

	resultCh <- &pkceResult{
		AccessToken:  token,
		RefreshToken: q.Get("refresh_token"),
		UserEmail:    q.Get("user"),
		UserName:     q.Get("name"),
	}
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

	// A disconnect is a state change the gate must observe — sign-out and a
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
// Desktop credentials live in the shared host/venue/config store — the same
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
