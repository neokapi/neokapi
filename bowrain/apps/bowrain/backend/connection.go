package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	platauth "github.com/gokapi/gokapi/platform/auth"
	"github.com/zalando/go-keyring"
)

var errNotConnected = errors.New("not connected to server")

const (
	keyringService         = "bowrain"
	keyringAccessTokenKey  = "access-token"
	keyringRefreshTokenKey = "refresh-token"
)

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

// storedDesktopAuth holds non-secret auth metadata persisted at ~/.config/bowrain/auth.json.
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
func (a *App) GetConnectionState() ConnectionInfo {
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
		log.Printf("bowrain: failed to enqueue %s: %v", operation, err)
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

	// Generate PKCE code verifier + challenge.
	verifier, err := platauth.GenerateCodeVerifier()
	if err != nil {
		return fmt.Errorf("generate PKCE verifier: %w", err)
	}
	challenge := platauth.ComputeCodeChallenge(verifier)

	// Start a local HTTP server on a random available port.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("start callback server: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	resultCh := make(chan *pkceResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		token := q.Get("token")
		refreshToken := q.Get("refresh_token")
		userEmail := q.Get("user")
		userName := q.Get("name")

		if token == "" {
			errMsg := q.Get("error")
			if errMsg == "" {
				errMsg = "no token received"
			}
			resultCh <- &pkceResult{Err: fmt.Errorf("auth failed: %s", errMsg)}
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, `<!DOCTYPE html><html><body style="font-family:system-ui;text-align:center;padding:60px">
<h1 style="color:#e53e3e">Authentication Failed</h1><p>%s</p><p>You can close this tab.</p></body></html>`, errMsg)
			return
		}

		resultCh <- &pkceResult{
			AccessToken:  token,
			RefreshToken: refreshToken,
			UserEmail:    userEmail,
			UserName:     userName,
		}

		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body style="font-family:system-ui;text-align:center;padding:60px">
<h1 style="color:#58a6ff">Sign-in Complete</h1><p>You can close this tab and return to Bowrain.</p></body></html>`)
	})

	srv := &http.Server{Handler: mux}
	go func() {
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			resultCh <- &pkceResult{Err: fmt.Errorf("callback server: %w", err)}
		}
	}()

	a.mu.Lock()
	a.serverURL = serverURL
	a.pkceServer = srv
	a.pkceVerifier = verifier
	a.pkceResultCh = resultCh
	a.mu.Unlock()

	// Build the desktop login URL and open in the system browser.
	callbackURL := fmt.Sprintf("http://127.0.0.1:%d/callback", port)
	loginURL := fmt.Sprintf("%s/api/v1/auth/desktop/login?redirect_uri=%s&code_challenge=%s&code_challenge_method=S256",
		serverURL,
		url.QueryEscape(callbackURL),
		url.QueryEscape(challenge),
	)

	// Open the browser. We use the Wails runtime Browser.OpenURL from the
	// frontend side, but provide the URL for the frontend to open.
	// Actually, for simplicity, open it from Go directly.
	openBrowser(loginURL)

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

// CancelLogin cancels any active PKCE login flow.
func (a *App) CancelLogin() {
	a.cleanupPKCE()
}

// cleanupPKCE shuts down the local callback server and clears PKCE state.
func (a *App) cleanupPKCE() {
	a.mu.Lock()
	srv := a.pkceServer
	a.pkceServer = nil
	a.pkceVerifier = ""
	a.pkceResultCh = nil
	a.mu.Unlock()

	if srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}
}

// Logout removes stored auth and disconnects.
func (a *App) Logout() {
	a.Disconnect()
	_ = os.Remove(desktopAuthFilePath())
	// Remove tokens from keyring (best-effort).
	_ = keyring.Delete(keyringService, keyringAccessTokenKey)
	_ = keyring.Delete(keyringService, keyringRefreshTokenKey)
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
// Non-secret metadata: ~/.config/bowrain/auth.json
// Tokens: OS keychain via go-keyring

func desktopAuthFilePath() string {
	if dir := os.Getenv("KAPI_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "auth.json")
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(configDir, "bowrain", "auth.json")
}

func saveDesktopAuth(a *storedDesktopAuth) error {
	// Save tokens to OS keychain.
	if a.AccessToken != "" {
		if err := keyring.Set(keyringService, keyringAccessTokenKey, a.AccessToken); err != nil {
			return fmt.Errorf("save access token to keyring: %w", err)
		}
	}
	if a.RefreshToken != "" {
		if err := keyring.Set(keyringService, keyringRefreshTokenKey, a.RefreshToken); err != nil {
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
	a.AccessToken, _ = keyring.Get(keyringService, keyringAccessTokenKey)
	a.RefreshToken, _ = keyring.Get(keyringService, keyringRefreshTokenKey)

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
// Convention: gRPC port = HTTP port + 1000 (e.g., 8080 → 9080).
func discoverGRPCAddr(serverURL string) (addr string, useTLS bool, err error) {
	// Parse the URL to extract host and port.
	u, err := parseServerURL(serverURL)
	if err != nil {
		return "", false, err
	}

	useTLS = u.scheme == "https"

	// Default ports based on scheme.
	httpPort := u.port
	if httpPort == "" {
		if useTLS {
			httpPort = "443"
		} else {
			httpPort = "80"
		}
	}

	// Derive gRPC port: HTTP port + 1000.
	var portNum int
	if _, err := fmt.Sscanf(httpPort, "%d", &portNum); err != nil {
		return "", false, fmt.Errorf("invalid port %q in URL", httpPort)
	}
	grpcPort := portNum + 1000

	return fmt.Sprintf("%s:%d", u.host, grpcPort), useTLS, nil
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

// openBrowser opens the given URL in the system's default browser.
func openBrowser(url string) {
	// Use exec.Command for cross-platform browser opening.
	// On macOS: open URL
	// On Linux: xdg-open URL
	// On Windows: rundll32 url.dll,FileProtocolHandler URL
	cmd := browserCommand(url)
	if cmd != nil {
		_ = cmd.Start()
	}
}
