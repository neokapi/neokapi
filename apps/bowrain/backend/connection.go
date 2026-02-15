package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gokapi/gokapi/core/auth"
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

// DeviceAuthInfo is returned when starting a login flow.
type DeviceAuthInfo struct {
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	// deviceCode is kept private — the frontend only needs userCode + URI.
	deviceCode string
	interval   int
}

// storedDesktopAuth is the auth token persisted at ~/.config/gokapi/auth.json.
// Matches the CLI's StoredAuth format so both tools share the same token.
type storedDesktopAuth struct {
	ServerURL    string           `json:"server_url"`
	AccessToken  string           `json:"access_token"`
	RefreshToken string           `json:"refresh_token,omitempty"`
	Expiry       time.Time        `json:"expiry"`
	User         storedDesktopUser `json:"user"`
}

type storedDesktopUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
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

// ConnectToServer establishes a gRPC connection to the given server URL.
// The URL should be the HTTP base URL (e.g. "http://localhost:8080").
// gRPC port is discovered from the server health endpoint.
func (a *App) ConnectToServer(serverURL string) error {
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

// StartLogin begins a device authorization flow against the server.
func (a *App) StartLogin(serverURL string) (*DeviceAuthInfo, error) {
	client := &auth.DeviceFlowClient{
		DeviceAuthURL: serverURL + "/api/v1/auth/device/start",
		TokenURL:      serverURL + "/api/v1/auth/device/poll",
		ClientID:      "gokapi-desktop",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := client.StartDeviceAuth(ctx)
	if err != nil {
		return nil, fmt.Errorf("device auth start: %w", err)
	}

	a.mu.Lock()
	a.serverURL = serverURL
	a.deviceFlowClient = client
	a.mu.Unlock()

	return &DeviceAuthInfo{
		UserCode:        resp.UserCode,
		VerificationURI: resp.VerificationURI,
		ExpiresIn:       resp.ExpiresIn,
		deviceCode:      resp.DeviceCode,
		interval:        resp.Interval,
	}, nil
}

// PollLogin polls for the device auth token. Returns true when authorized.
func (a *App) PollLogin(deviceCode string, interval int) (bool, error) {
	a.mu.RLock()
	client := a.deviceFlowClient
	serverURL := a.serverURL
	a.mu.RUnlock()

	if client == nil {
		return false, fmt.Errorf("no active login flow — call StartLogin first")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(interval+5)*time.Second)
	defer cancel()

	token, pending, err := client.TryToken(ctx, deviceCode)
	if err != nil {
		return false, err
	}
	if pending {
		return false, nil
	}

	// Success — save token and connect.
	stored := &storedDesktopAuth{
		ServerURL:    serverURL,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		Expiry:       time.Now().Add(time.Duration(token.ExpiresIn) * time.Second),
	}

	// Fetch user info.
	if user, err := fetchDesktopUserInfo(serverURL, token.AccessToken); err == nil {
		stored.User = *user
	}

	if err := saveDesktopAuth(stored); err != nil {
		return true, fmt.Errorf("save auth: %w", err)
	}

	a.mu.Lock()
	a.authInfo = stored
	a.deviceFlowClient = nil
	a.mu.Unlock()

	return true, nil
}

// CancelLogin cancels any active device auth flow.
func (a *App) CancelLogin() {
	a.mu.Lock()
	a.deviceFlowClient = nil
	a.mu.Unlock()
}

// Logout removes stored auth and disconnects.
func (a *App) Logout() {
	a.Disconnect()
	_ = os.Remove(desktopAuthFilePath())
}

// Disconnect closes the server connection.
func (a *App) Disconnect() {
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

// --- Auth persistence (shared with CLI at ~/.config/gokapi/auth.json) ---

func desktopAuthFilePath() string {
	if dir := os.Getenv("KAPI_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "auth.json")
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(configDir, "gokapi", "auth.json")
}

func saveDesktopAuth(a *storedDesktopAuth) error {
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
