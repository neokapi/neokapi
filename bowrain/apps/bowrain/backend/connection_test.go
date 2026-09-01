package backend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/neokapi/neokapi/host/venue/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

// --- GetDefaultServerURL tests ---

func TestGetDefaultServerURLDefault(t *testing.T) {
	app := newTestApp(t)
	t.Setenv("BOWRAIN_SERVER_URL", "")
	assert.Equal(t, DefaultServerURL, app.GetDefaultServerURL())
}

func TestGetDefaultServerURLEnvOverride(t *testing.T) {
	app := newTestApp(t)
	t.Setenv("BOWRAIN_SERVER_URL", "https://bowrain.mymac")
	assert.Equal(t, "https://bowrain.mymac", app.GetDefaultServerURL())
}

func TestGetDefaultServerURLTrimsTrailingSlash(t *testing.T) {
	app := newTestApp(t)
	t.Setenv("BOWRAIN_SERVER_URL", "https://bowrain.mymac/")
	assert.Equal(t, "https://bowrain.mymac", app.GetDefaultServerURL())
}

// --- Auth persistence tests ---
//
// Desktop credentials now persist through the shared host/venue/config store
// (keychain service "kapi", URL-namespaced keys, metadata at
// $BOWRAIN_CONFIG_DIR/auth.json) — the same scheme the kapi CLI uses, which is
// what makes a desktop login and a CLI login mutually visible. These tests
// exercise that shared store via the desktop's loadDesktopAuth wrapper.

func TestSaveAndLoadDesktopAuth(t *testing.T) {
	keyring.MockInit()
	tmpDir := t.TempDir()
	t.Setenv("BOWRAIN_CONFIG_DIR", tmpDir)

	stored := config.StoredAuth{
		ServerURL:   "http://localhost:8080",
		AccessToken: "test-token-123",
		Expiry:      time.Now().Add(time.Hour).Truncate(time.Second),
		User: config.StoredUser{
			ID:    "user-1",
			Email: "alice@test.com",
			Name:  "Alice",
		},
	}

	err := config.SaveAuth(stored)
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
	t.Setenv("BOWRAIN_CONFIG_DIR", tmpDir)

	_, err := loadDesktopAuth()
	require.Error(t, err)
}

func TestSaveDesktopAuthPermissions(t *testing.T) {
	keyring.MockInit()
	tmpDir := t.TempDir()
	t.Setenv("BOWRAIN_CONFIG_DIR", tmpDir)

	stored := config.StoredAuth{
		ServerURL:   "http://localhost:8080",
		AccessToken: "secret-token",
	}

	err := config.SaveAuth(stored)
	require.NoError(t, err)

	path := filepath.Join(tmpDir, "auth.json")
	info, err := os.Stat(path)
	require.NoError(t, err)
	// Should be 0600 (owner read/write only).
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

	// Tokens should be in the keychain, not in the JSON file.
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "secret-token")

	// The token round-trips through the shared store's keychain.
	loaded, err := loadDesktopAuth()
	require.NoError(t, err)
	assert.Equal(t, "secret-token", loaded.AccessToken)
}

func TestDesktopAuthJSONFormat(t *testing.T) {
	keyring.MockInit()
	tmpDir := t.TempDir()
	t.Setenv("BOWRAIN_CONFIG_DIR", tmpDir)

	stored := config.StoredAuth{
		ServerURL:    "http://localhost:8080",
		AccessToken:  "token",
		RefreshToken: "refresh",
		User:         config.StoredUser{ID: "u1", Email: "a@b.com", Name: "A"},
	}
	err := config.SaveAuth(stored)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(tmpDir, "auth.json"))
	require.NoError(t, err)

	// JSON should have metadata but NOT tokens (those go to the keychain).
	var parsed map[string]any
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:8080", parsed["server_url"])
	assert.Nil(t, parsed["access_token"])  // json:"-" means it won't be serialized
	assert.Nil(t, parsed["refresh_token"]) // json:"-" means it won't be serialized

	// Both tokens round-trip through the shared store's keychain.
	loaded, err := loadDesktopAuth()
	require.NoError(t, err)
	assert.Equal(t, "token", loaded.AccessToken)
	assert.Equal(t, "refresh", loaded.RefreshToken)
}

// TestMigrateLegacyDesktopAuth verifies the one-time migration off the legacy
// desktop-only keychain scheme ("bowrain" service + bowrain-desktop/auth.json)
// into the shared host/venue/config store.
func TestMigrateLegacyDesktopAuth(t *testing.T) {
	keyring.MockInit()
	legacyDir := t.TempDir()
	sharedDir := t.TempDir()
	t.Setenv("BOWRAIN_DESKTOP_CONFIG_DIR", legacyDir)
	t.Setenv("BOWRAIN_CONFIG_DIR", sharedDir)

	// Seed the legacy scheme: metadata file + keychain tokens.
	legacyMeta := `{"server_url":"http://localhost:8080","expiry":"2099-01-01T00:00:00Z","user":{"id":"u1","email":"a@b.com","name":"Alice"}}`
	require.NoError(t, os.WriteFile(legacyDesktopAuthFilePath(), []byte(legacyMeta), 0o600))
	require.NoError(t, keyring.Set(legacyKeyringService(), legacyAccessTokenKey, "legacy-access"))
	require.NoError(t, keyring.Set(legacyKeyringService(), legacyRefreshTokenKey, "legacy-refresh"))

	// loadDesktopAuth triggers the migration, then reads from the shared store.
	loaded, err := loadDesktopAuth()
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:8080", loaded.ServerURL)
	assert.Equal(t, "legacy-access", loaded.AccessToken)
	assert.Equal(t, "legacy-refresh", loaded.RefreshToken)
	assert.Equal(t, "Alice", loaded.User.Name)

	// Shared metadata now exists; legacy file is cleaned up.
	_, err = os.Stat(filepath.Join(sharedDir, "auth.json"))
	require.NoError(t, err)
	_, err = os.Stat(legacyDesktopAuthFilePath())
	assert.True(t, os.IsNotExist(err), "legacy auth.json should be removed")

	// Legacy keychain entries are cleared.
	_, err = keyring.Get(legacyKeyringService(), legacyAccessTokenKey)
	assert.Error(t, err)
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
	app.authInfo = &config.StoredAuth{User: config.StoredUser{Name: "Alice"}}
	app.mu.Unlock()

	app.Disconnect()

	info := app.GetConnectionState()
	assert.Equal(t, StateDisconnected, info.State)
	assert.Empty(t, info.Workspace)
	assert.False(t, app.isConnected())
}

// Disconnect must emit connection-state-changed so the desktop launch gate
// observes a sign-out / rejected session and demotes to the connect screen
// (rather than sitting on a stale "connected" snapshot).
func TestDisconnectEmitsStateChange(t *testing.T) {
	app := newTestApp(t)
	app.mu.Lock()
	app.connState = StateConnected
	app.serverURL = "http://localhost:8080"
	app.mu.Unlock()

	var events []ConnectionInfo
	InjectEventSink(app, func(name string, data any) {
		if name == "connection-state-changed" {
			if ci, ok := data.(ConnectionInfo); ok {
				events = append(events, ci)
			}
		}
	})

	app.Disconnect()

	require.Len(t, events, 1)
	assert.Equal(t, StateDisconnected, events[0].State)
}

func TestLogoutRemovesAuthFile(t *testing.T) {
	keyring.MockInit()
	tmpDir := t.TempDir()
	t.Setenv("BOWRAIN_CONFIG_DIR", tmpDir)

	// Save auth first.
	stored := config.StoredAuth{
		ServerURL:   "http://localhost:8080",
		AccessToken: "token",
	}
	err := config.SaveAuth(stored)
	require.NoError(t, err)

	path := filepath.Join(tmpDir, "auth.json")
	_, err = os.Stat(path)
	require.NoError(t, err)

	app := newTestApp(t)
	app.mu.Lock()
	app.serverURL = "http://localhost:8080"
	app.mu.Unlock()
	app.Logout()

	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err))
}

func TestConnectToServerNoStoredAuth(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("BOWRAIN_CONFIG_DIR", tmpDir)

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
	t.Setenv("BOWRAIN_CONFIG_DIR", tmpDir)

	// Save expired auth.
	stored := config.StoredAuth{
		ServerURL:   "http://localhost:8080",
		AccessToken: "expired-token",
		Expiry:      time.Now().Add(-time.Hour),
	}
	err := config.SaveAuth(stored)
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
	t.Setenv("BOWRAIN_CONFIG_DIR", tmpDir)

	app := newTestApp(t)
	app.TryAutoConnect()
	// Should remain disconnected without stored auth.
	assert.Equal(t, StateDisconnected, app.GetConnectionState().State)
}

func TestHandleAuthURL(t *testing.T) {
	app := newTestApp(t)

	// Set up an active login flow.
	resultCh := make(chan *pkceResult, 1)
	app.mu.Lock()
	app.pkceResultCh = resultCh
	app.mu.Unlock()

	// Simulate receiving a bowrain:// URL.
	app.HandleAuthURL("bowrain://auth/callback?token=jwt-123&refresh_token=rt-456&user=alice@test.com&name=Alice")

	result := <-resultCh
	require.NoError(t, result.Err)
	assert.Equal(t, "jwt-123", result.AccessToken)
	assert.Equal(t, "rt-456", result.RefreshToken)
	assert.Equal(t, "alice@test.com", result.UserEmail)
	assert.Equal(t, "Alice", result.UserName)
}

func TestHandleAuthURLNoToken(t *testing.T) {
	app := newTestApp(t)

	resultCh := make(chan *pkceResult, 1)
	app.mu.Lock()
	app.pkceResultCh = resultCh
	app.mu.Unlock()

	app.HandleAuthURL("bowrain://auth/callback?error=access_denied")

	result := <-resultCh
	require.Error(t, result.Err)
	assert.Contains(t, result.Err.Error(), "access_denied")
}

func TestHandleAuthURLNoActiveFlow(t *testing.T) {
	app := newTestApp(t)
	// Should not panic when no login flow is active.
	app.HandleAuthURL("bowrain://auth/callback?token=jwt-123")
}

func TestHandleDeepLinkValid(t *testing.T) {
	app := newTestApp(t)
	// Should not panic. Without a.app, event emission is skipped.
	app.HandleDeepLink("https://example.com/ws/my-ws/projects/proj_123")
}

func TestHandleDeepLinkInvalid(t *testing.T) {
	app := newTestApp(t)
	// Should not panic on invalid URL.
	app.HandleDeepLink("://invalid")
}

func TestHandleDeepLinkMissingID(t *testing.T) {
	app := newTestApp(t)
	// URL with no project segment should be handled gracefully.
	app.HandleDeepLink("https://example.com/ws/my-ws")
}

func TestTryAutoConnectExpiredAuth(t *testing.T) {
	keyring.MockInit()
	tmpDir := t.TempDir()
	t.Setenv("BOWRAIN_CONFIG_DIR", tmpDir)

	stored := config.StoredAuth{
		ServerURL:   "http://localhost:8080",
		AccessToken: "expired",
		Expiry:      time.Now().Add(-time.Hour),
	}
	err := config.SaveAuth(stored)
	require.NoError(t, err)

	app := newTestApp(t)
	app.TryAutoConnect()
	// Should remain disconnected with expired token.
	assert.Equal(t, StateDisconnected, app.GetConnectionState().State)
}
