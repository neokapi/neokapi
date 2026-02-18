package backend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

// --- parseServerURL tests ---

func TestParseServerURLHTTP(t *testing.T) {
	u, err := parseServerURL("http://localhost:8080")
	require.NoError(t, err)
	assert.Equal(t, "http", u.scheme)
	assert.Equal(t, "localhost", u.host)
	assert.Equal(t, "8080", u.port)
}

func TestParseServerURLHTTPS(t *testing.T) {
	u, err := parseServerURL("https://bowrain.example.com")
	require.NoError(t, err)
	assert.Equal(t, "https", u.scheme)
	assert.Equal(t, "bowrain.example.com", u.host)
	assert.Equal(t, "", u.port)
}

func TestParseServerURLHTTPSWithPort(t *testing.T) {
	u, err := parseServerURL("https://bowrain.example.com:8443")
	require.NoError(t, err)
	assert.Equal(t, "https", u.scheme)
	assert.Equal(t, "bowrain.example.com", u.host)
	assert.Equal(t, "8443", u.port)
}

func TestParseServerURLNoScheme(t *testing.T) {
	u, err := parseServerURL("localhost:8080")
	require.NoError(t, err)
	assert.Equal(t, "http", u.scheme)
	assert.Equal(t, "localhost", u.host)
	assert.Equal(t, "8080", u.port)
}

func TestParseServerURLTrailingSlash(t *testing.T) {
	u, err := parseServerURL("http://localhost:8080/")
	require.NoError(t, err)
	assert.Equal(t, "localhost", u.host)
	assert.Equal(t, "8080", u.port)
}

func TestParseServerURLWithPath(t *testing.T) {
	u, err := parseServerURL("http://example.com:8080/some/path")
	require.NoError(t, err)
	assert.Equal(t, "example.com", u.host)
	assert.Equal(t, "8080", u.port)
}

func TestParseServerURLEmptyHost(t *testing.T) {
	_, err := parseServerURL("http://")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty host")
}

// --- discoverGRPCAddr tests ---

func TestDiscoverGRPCAddrHTTP(t *testing.T) {
	addr, useTLS, err := discoverGRPCAddr("http://localhost:8080")
	require.NoError(t, err)
	assert.Equal(t, "localhost:9080", addr)
	assert.False(t, useTLS)
}

func TestDiscoverGRPCAddrHTTPS(t *testing.T) {
	addr, useTLS, err := discoverGRPCAddr("https://bowrain.example.com")
	require.NoError(t, err)
	assert.Equal(t, "bowrain.example.com:1443", addr)
	assert.True(t, useTLS)
}

func TestDiscoverGRPCAddrHTTPNoPort(t *testing.T) {
	addr, useTLS, err := discoverGRPCAddr("http://example.com")
	require.NoError(t, err)
	assert.Equal(t, "example.com:1080", addr)
	assert.False(t, useTLS)
}

func TestDiscoverGRPCAddrHTTPSWithPort(t *testing.T) {
	addr, useTLS, err := discoverGRPCAddr("https://example.com:9000")
	require.NoError(t, err)
	assert.Equal(t, "example.com:10000", addr)
	assert.True(t, useTLS)
}

// --- Auth persistence tests ---

func TestSaveAndLoadDesktopAuth(t *testing.T) {
	keyring.MockInit()
	tmpDir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", tmpDir)

	stored := &storedDesktopAuth{
		ServerURL:   "http://localhost:8080",
		AccessToken: "test-token-123",
		Expiry:      time.Now().Add(time.Hour).Truncate(time.Second),
		User: storedDesktopUser{
			ID:    "user-1",
			Email: "alice@test.com",
			Name:  "Alice",
		},
	}

	err := saveDesktopAuth(stored)
	require.NoError(t, err)

	// Verify file exists.
	path := filepath.Join(tmpDir, "auth.json")
	_, err = os.Stat(path)
	require.NoError(t, err)

	// Load and verify.
	loaded, err := loadDesktopAuth()
	require.NoError(t, err)
	assert.Equal(t, stored.ServerURL, loaded.ServerURL)
	assert.Equal(t, stored.AccessToken, loaded.AccessToken)
	assert.Equal(t, stored.User.Email, loaded.User.Email)
	assert.Equal(t, stored.User.Name, loaded.User.Name)
	assert.WithinDuration(t, stored.Expiry, loaded.Expiry, time.Second)
}

func TestLoadDesktopAuthMissing(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", tmpDir)

	_, err := loadDesktopAuth()
	require.Error(t, err)
}

func TestSaveDesktopAuthPermissions(t *testing.T) {
	keyring.MockInit()
	tmpDir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", tmpDir)

	stored := &storedDesktopAuth{
		ServerURL:   "http://localhost:8080",
		AccessToken: "secret-token",
	}

	err := saveDesktopAuth(stored)
	require.NoError(t, err)

	path := filepath.Join(tmpDir, "auth.json")
	info, err := os.Stat(path)
	require.NoError(t, err)
	// Should be 0600 (owner read/write only).
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

	// Tokens should be in keyring, not in the JSON file.
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "secret-token")

	kr, err := keyring.Get(keyringService, keyringAccessTokenKey)
	require.NoError(t, err)
	assert.Equal(t, "secret-token", kr)
}

func TestDesktopAuthJSONFormat(t *testing.T) {
	keyring.MockInit()
	tmpDir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", tmpDir)

	stored := &storedDesktopAuth{
		ServerURL:    "http://localhost:8080",
		AccessToken:  "token",
		RefreshToken: "refresh",
		User:         storedDesktopUser{ID: "u1", Email: "a@b.com", Name: "A"},
	}
	err := saveDesktopAuth(stored)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(tmpDir, "auth.json"))
	require.NoError(t, err)

	// JSON should have metadata but NOT tokens (those go to keyring).
	var parsed map[string]any
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:8080", parsed["server_url"])
	assert.Nil(t, parsed["access_token"])  // json:"-" means it won't be serialized
	assert.Nil(t, parsed["refresh_token"]) // json:"-" means it won't be serialized

	// Tokens should be in keyring.
	at, err := keyring.Get(keyringService, keyringAccessTokenKey)
	require.NoError(t, err)
	assert.Equal(t, "token", at)
	rt, err := keyring.Get(keyringService, keyringRefreshTokenKey)
	require.NoError(t, err)
	assert.Equal(t, "refresh", rt)
}

// --- Connection state tests ---

func TestGetConnectionStateDefault(t *testing.T) {
	app := newTestApp(t)
	info := app.GetConnectionState()
	assert.Equal(t, StateDisconnected, info.State)
	assert.Empty(t, info.ServerURL)
	assert.Empty(t, info.UserName)
}

func TestIsConnectedDefault(t *testing.T) {
	app := newTestApp(t)
	assert.False(t, app.isConnected())
}

func TestSelectWorkspace(t *testing.T) {
	app := newTestApp(t)

	err := app.SelectWorkspace("my-workspace")
	require.NoError(t, err)

	app.mu.RLock()
	ws := app.activeWS
	app.mu.RUnlock()
	assert.Equal(t, "my-workspace", ws)
}

func TestDisconnectResetsState(t *testing.T) {
	app := newTestApp(t)

	// Simulate some connected state.
	app.mu.Lock()
	app.connState = StateConnected
	app.serverURL = "http://localhost:8080"
	app.activeWS = "ws-1"
	app.authInfo = &storedDesktopAuth{User: storedDesktopUser{Name: "Alice"}}
	app.mu.Unlock()

	app.Disconnect()

	info := app.GetConnectionState()
	assert.Equal(t, StateDisconnected, info.State)
	assert.Empty(t, info.Workspace)
	assert.False(t, app.isConnected())
}

func TestLogoutRemovesAuthFile(t *testing.T) {
	keyring.MockInit()
	tmpDir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", tmpDir)

	// Save auth first.
	stored := &storedDesktopAuth{
		ServerURL:   "http://localhost:8080",
		AccessToken: "token",
	}
	err := saveDesktopAuth(stored)
	require.NoError(t, err)

	path := filepath.Join(tmpDir, "auth.json")
	_, err = os.Stat(path)
	require.NoError(t, err)

	app := newTestApp(t)
	app.Logout()

	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err))
}

func TestConnectToServerNoStoredAuth(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", tmpDir)

	app := newTestApp(t)
	err := app.ConnectToServer("http://localhost:8080")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not authenticated")

	// State should be disconnected after failed connect.
	assert.Equal(t, StateDisconnected, app.GetConnectionState().State)
}

func TestConnectToServerExpiredToken(t *testing.T) {
	keyring.MockInit()
	tmpDir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", tmpDir)

	// Save expired auth.
	stored := &storedDesktopAuth{
		ServerURL:   "http://localhost:8080",
		AccessToken: "expired-token",
		Expiry:      time.Now().Add(-time.Hour),
	}
	err := saveDesktopAuth(stored)
	require.NoError(t, err)

	app := newTestApp(t)
	err = app.ConnectToServer("http://localhost:8080")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token expired")
}

func TestCancelLogin(t *testing.T) {
	app := newTestApp(t)

	// Verify CancelLogin does not panic when no active flow.
	app.CancelLogin()
}

func TestTryAutoConnectNoAuth(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", tmpDir)

	app := newTestApp(t)
	app.TryAutoConnect()
	// Should remain disconnected without stored auth.
	assert.Equal(t, StateDisconnected, app.GetConnectionState().State)
}

func TestTryAutoConnectExpiredAuth(t *testing.T) {
	keyring.MockInit()
	tmpDir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", tmpDir)

	stored := &storedDesktopAuth{
		ServerURL:   "http://localhost:8080",
		AccessToken: "expired",
		Expiry:      time.Now().Add(-time.Hour),
	}
	err := saveDesktopAuth(stored)
	require.NoError(t, err)

	app := newTestApp(t)
	app.TryAutoConnect()
	// Should remain disconnected with expired token.
	assert.Equal(t, StateDisconnected, app.GetConnectionState().State)
}
