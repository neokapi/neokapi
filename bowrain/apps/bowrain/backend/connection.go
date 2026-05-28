package backend

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/zalando/go-keyring"
)

var errNotConnected = errors.New("not connected to server")

// DefaultServerURL is the Bowrain SaaS instance URL used when no custom server is specified.
const DefaultServerURL = "https://bowrain.cloud"

const (
	keyringServiceBase     = "bowrain"
	keyringAccessTokenKey  = "access-token"
	keyringRefreshTokenKey = "refresh-token"
)

// keyringService returns the OS keychain service name for the desktop auth
// tokens. It is namespaced by the config dir so an isolated
// BOWRAIN_DESKTOP_CONFIG_DIR (tests, or an alternate/dogfood instance) gets its
// own token slot instead of reading or clobbering the user's real login — the
// same env var that already isolates auth.json now isolates the tokens too. The
// default config dir keeps the bare "bowrain" service for backward
// compatibility with existing installs.
func keyringService() string {
	if dir := os.Getenv("BOWRAIN_DESKTOP_CONFIG_DIR"); dir != "" {
		return keyringServiceBase + ":" + dir
	}
	return keyringServiceBase
}

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

// storedDesktopAuth holds non-secret auth metadata persisted at <UserConfigDir>/bowrain-desktop/auth.json.
// Tokens are stored in the OS keychain (macOS Keychain, Windows Credential Manager, etc.).
type storedDesktopAuth struct {
	ServerURL string            `json:"server_url"`
	Expiry    time.Time         `json:"expiry"`
	User      storedDesktopUser `json:"user"`

	// In-memory only — loaded from keyring, never serialized to disk.
	AccessToken  string `json:"-"`
	RefreshToken string `json:"-"`
}

type storedDesktopUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
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

	grpcAddr, useTLS, err := discoverGRPCAddr(serverURL)
	if err != nil {
		return fmt.Errorf("discover gRPC: %w", err)
	}

	client, err := NewServerClient(grpcAddr, token, useTLS)
	if err != nil {
		return fmt.Errorf("gRPC connect: %w", err)
	}

	a.mu.Lock()
	a.remote = client
	a.authInfo = &storedDesktopAuth{
		ServerURL:   serverURL,
		AccessToken: token,
		User:        storedDesktopUser{Email: "ci@bowrain.cloud", Name: "CI"},
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
		return strings.TrimRight(envURL, "/")
	}
	return DefaultServerURL
}

// isConnected returns true when a server connection is active.
func (a *App) isConnected() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.connState == StateConnected && a.remote != nil
}

// isOffline returns true when the app has lost its server connection
// and is operating from the local cache with changes queued.
func (a *App) isOffline() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.connState == StateOffline
}

// enqueue adds a mutation to the offline queue. Silently logs on failure.
func (a *App) enqueue(operation string, payload any) {
	if a.offlineQueue == nil {
		return
	}
	if err := a.offlineQueue.Enqueue(operation, payload); err != nil {
		slog.Info("bowrain: failed to enqueue", "id", operation, "error", err)
	}
}

// GetPendingChangesCount returns the number of queued offline changes.
// Exposed to the frontend so it can show a pending sync indicator.
func (a *App) GetPendingChangesCount() int {
	if a.offlineQueue == nil {
		return 0
	}
	return a.offlineQueue.PendingCount()
}

// ConnectToServer establishes a gRPC connection to the given server URL.
// The URL should be the HTTP base URL (e.g. "http://localhost:8080").
// gRPC port is discovered from the server health endpoint.
func (a *App) ConnectToServer(serverURL string) error {
	serverURL = strings.TrimRight(serverURL, "/")
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
		return fmt.Errorf("not authenticated — use StartLogin first")
	}

	// Check if token has expired.
	if !stored.Expiry.IsZero() && time.Now().After(stored.Expiry) {
		a.mu.Lock()
		a.connState = StateDisconnected
		a.mu.Unlock()
		return fmt.Errorf("token expired — please log in again")
	}

	// Determine gRPC address. Convention: gRPC port = HTTP port + 1000.
	// Can be overridden via the health endpoint in the future.
	grpcAddr, useTLS, err := discoverGRPCAddr(serverURL)
	if err != nil {
		a.mu.Lock()
		a.connState = StateDisconnected
		a.mu.Unlock()
		return fmt.Errorf("discover gRPC: %w", err)
	}

	client, err := NewServerClient(grpcAddr, stored.AccessToken, useTLS)
	if err != nil {
		a.mu.Lock()
		a.connState = StateDisconnected
		a.mu.Unlock()
		return fmt.Errorf("gRPC connect: %w", err)
	}

	a.mu.Lock()
	a.remote = client
	a.authInfo = stored
	a.connState = StateConnected
	a.mu.Unlock()

	return nil
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
	resp, err := client.Get(serverURL + "/api/v1/health")
	if err != nil {
		return fmt.Errorf("cannot reach server: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("not a valid Bowrain server (health check returned %d)", resp.StatusCode)
	}

	// Generate PKCE code verifier + challenge.
	verifier, err := platauth.GenerateCodeVerifier()
	if err != nil {
		return fmt.Errorf("generate PKCE verifier: %w", err)
	}
	challenge := platauth.ComputeCodeChallenge(verifier)

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
		return false, fmt.Errorf("no active login flow — call StartLogin first")
	}

	// Wait for the callback with a 10-minute timeout.
	select {
	case result := <-resultCh:
		// Shut down the local server.
		a.cleanupPKCE()

		if result.Err != nil {
			return false, result.Err
		}

		// Save tokens to keychain and metadata to disk.
		stored := &storedDesktopAuth{
			ServerURL:    serverURL,
			AccessToken:  result.AccessToken,
			RefreshToken: result.RefreshToken,
			Expiry:       time.Now().Add(24 * time.Hour),
			User: storedDesktopUser{
				Email: result.UserEmail,
				Name:  result.UserName,
			},
		}

		// Fetch full user info (including ID) from the server.
		if user, err := fetchDesktopUserInfo(serverURL, result.AccessToken); err == nil {
			stored.User = *user
		}

		if err := saveDesktopAuth(stored); err != nil {
			return true, fmt.Errorf("save auth: %w", err)
		}

		a.mu.Lock()
		a.authInfo = stored
		a.mu.Unlock()

		return true, nil

	case <-time.After(10 * time.Minute):
		a.cleanupPKCE()
		return false, fmt.Errorf("login timed out")
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
	for i := 0; i < len(segments)-1; i++ {
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
	a.Disconnect()
	_ = os.Remove(desktopAuthFilePath())
	// Remove tokens from keyring (best-effort).
	_ = keyring.Delete(keyringService(),keyringAccessTokenKey)
	_ = keyring.Delete(keyringService(),keyringRefreshTokenKey)
}

// Disconnect closes the server connection.
func (a *App) Disconnect() {
	a.stopReconnect()
	a.StopWatching()

	a.mu.Lock()
	defer a.mu.Unlock()

	if a.remote != nil {
		a.remote.Close()
		a.remote = nil
	}
	a.connState = StateDisconnected
	a.authInfo = nil
	a.activeWS = ""
}

// GetServerWorkspaces returns workspaces from the connected server.
func (a *App) GetServerWorkspaces() ([]WorkspaceInfo, error) {
	if !a.isConnected() {
		return nil, fmt.Errorf("not connected")
	}
	return a.remote.ListWorkspaces()
}

// SelectWorkspace sets the active workspace for all subsequent operations.
func (a *App) SelectWorkspace(slug string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.activeWS = slug
	return nil
}

// --- Auth persistence ---
// Non-secret metadata: <UserConfigDir>/bowrain-desktop/auth.json
// Tokens: OS keychain via go-keyring

func desktopAuthFilePath() string {
	return filepath.Join(desktopConfigDir(), "auth.json")
}

func saveDesktopAuth(a *storedDesktopAuth) error {
	// Save tokens to OS keychain.
	if a.AccessToken != "" {
		if err := keyring.Set(keyringService(),keyringAccessTokenKey, a.AccessToken); err != nil {
			return fmt.Errorf("save access token to keyring: %w", err)
		}
	}
	if a.RefreshToken != "" {
		if err := keyring.Set(keyringService(),keyringRefreshTokenKey, a.RefreshToken); err != nil {
			return fmt.Errorf("save refresh token to keyring: %w", err)
		}
	}

	// Save non-secret metadata to disk.
	path := desktopAuthFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func loadDesktopAuth() (*storedDesktopAuth, error) {
	data, err := os.ReadFile(desktopAuthFilePath())
	if err != nil {
		return nil, err
	}
	var a storedDesktopAuth
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, err
	}

	// Load tokens from keyring.
	a.AccessToken, _ = keyring.Get(keyringService(),keyringAccessTokenKey)
	a.RefreshToken, _ = keyring.Get(keyringService(),keyringRefreshTokenKey)

	return &a, nil
}

// fetchDesktopUserInfo calls /api/v1/auth/me to get user details from the server.
func fetchDesktopUserInfo(serverURL, token string) (*storedDesktopUser, error) {
	req, err := http.NewRequest(http.MethodGet, serverURL+"/api/v1/auth/me", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("auth/me returned %d: %s", resp.StatusCode, body)
	}

	var user storedDesktopUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, err
	}
	return &user, nil
}

// discoverGRPCAddr derives the gRPC address from the server URL.
// gRPC is served on the same port as HTTP via h2c protocol multiplexing.
//
// For bowrain.cloud domains, the gRPC endpoint uses the grpc.{domain}
// subdomain which routes directly to the Container App (bypassing Azure
// Front Door, which only supports HTTP/1.1 to origins).
func discoverGRPCAddr(serverURL string) (addr string, useTLS bool, err error) {
	u, err := parseServerURL(serverURL)
	if err != nil {
		return "", false, err
	}

	useTLS = u.scheme == "https"
	host := grpcHost(u.host)

	port := u.port
	if port == "" {
		if useTLS {
			port = "443"
		} else {
			port = "80"
		}
	}

	return fmt.Sprintf("%s:%s", host, port), useTLS, nil
}

// grpcHost returns the gRPC hostname for a given server hostname.
// For bowrain.cloud domains fronted by Azure Front Door (which downgrades
// to HTTP/1.1), gRPC uses a dedicated subdomain that routes directly to
// the Container App with full HTTP/2 support.
func grpcHost(host string) string {
	switch host {
	case "bowrain.cloud":
		return "grpc.bowrain.cloud"
	case "dev.bowrain.cloud":
		return "grpc.dev.bowrain.cloud"
	default:
		return host
	}
}

type parsedURL struct {
	scheme string
	host   string
	port   string
}

func parseServerURL(serverURL string) (*parsedURL, error) {
	// Simple URL parsing for scheme://host:port patterns.
	scheme := "http"
	rest := serverURL

	if len(rest) >= 8 && rest[:8] == "https://" {
		scheme = "https"
		rest = rest[8:]
	} else if len(rest) >= 7 && rest[:7] == "http://" {
		rest = rest[7:]
	}

	// Strip trailing slash and path.
	for i := 0; i < len(rest); i++ {
		if rest[i] == '/' {
			rest = rest[:i]
			break
		}
	}

	host := rest
	port := ""
	// Check for host:port.
	for i := len(rest) - 1; i >= 0; i-- {
		if rest[i] == ':' {
			host = rest[:i]
			port = rest[i+1:]
			break
		}
	}

	if host == "" {
		return nil, fmt.Errorf("empty host in URL %q", serverURL)
	}

	return &parsedURL{scheme: scheme, host: host, port: port}, nil
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
