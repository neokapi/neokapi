package backend

// Bowrain auth token storage — boundary-clean reimplementation of the
// keychain + metadata convention used by the kapi-bowrain plugin
// (bowrain/core/config). The desktop app is a clean local authoring tool and
// must NOT import bowrain platform packages (only bowrain/plugin/schema is
// allowed). Interop with `kapi sync` is achieved purely through a shared
// on-disk/keychain *convention*, not a shared Go package:
//
//   - keychain service: "kapi"
//   - access token key:  "bowrain-auth:<serverURL>"
//   - refresh token key:  "bowrain-refresh:<serverURL>"
//   - metadata file:     <BOWRAIN_CONFIG_DIR|UserConfigDir>/bowrain/auth.json
//
// Writing tokens here means a subsequent `kapi sync` (driven by the
// kapi-bowrain plugin) picks up the desktop-provisioned login transparently.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/zalando/go-keyring"
)

// bowrainKeyringService is the OS keychain service identifier. It is
// intentionally the same value the kapi CLI and the kapi-bowrain plugin use so
// users see a single keychain identity for both provider keys and server
// tokens.
const bowrainKeyringService = "kapi"

func bowrainAccessKey(serverURL string) string  { return "bowrain-auth:" + serverURL }
func bowrainRefreshKey(serverURL string) string { return "bowrain-refresh:" + serverURL }

// bowrainStoredUser mirrors the user info persisted alongside the auth
// metadata by the kapi-bowrain plugin.
type bowrainStoredUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// bowrainStoredAuth is the auth payload. Tokens live in the OS keychain;
// everything else is persisted as JSON metadata. The JSON shape matches the
// kapi-bowrain plugin's StoredAuth exactly so the two can read each other's
// state.
type bowrainStoredAuth struct {
	ServerURL    string            `json:"server_url"`
	AccessToken  string            `json:"-"` // keychain
	RefreshToken string            `json:"-"` // keychain
	Expiry       time.Time         `json:"expiry"`
	User         bowrainStoredUser `json:"user"`
}

// bowrainAuthFilePath returns the path to the auth metadata file, honoring
// BOWRAIN_CONFIG_DIR for test/CI isolation just like the plugin.
func bowrainAuthFilePath() string {
	if dir := os.Getenv("BOWRAIN_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "auth.json")
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(configDir, "bowrain", "auth.json")
}

// saveBowrainAuth persists auth credentials: tokens to the OS keychain,
// non-secret metadata to the JSON file.
func saveBowrainAuth(a bowrainStoredAuth) error {
	if a.ServerURL == "" {
		return errors.New("save bowrain auth: server URL required")
	}
	if a.AccessToken != "" {
		if err := keyring.Set(bowrainKeyringService, bowrainAccessKey(a.ServerURL), a.AccessToken); err != nil {
			return fmt.Errorf("save access token to keychain: %w", err)
		}
	}
	if a.RefreshToken != "" {
		if err := keyring.Set(bowrainKeyringService, bowrainRefreshKey(a.ServerURL), a.RefreshToken); err != nil {
			return fmt.Errorf("save refresh token to keychain: %w", err)
		}
	}

	path := bowrainAuthFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create auth dir: %w", err)
	}
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal auth metadata: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

// loadBowrainAuth returns auth credentials for the given server URL, reading
// metadata from disk and tokens from the keychain. A BOWRAIN_AUTH_TOKEN env
// var short-circuits (CI/CD bypass), matching the plugin's resolution order.
func loadBowrainAuth(serverURL string) (*bowrainStoredAuth, error) {
	if token := os.Getenv("BOWRAIN_AUTH_TOKEN"); token != "" {
		return &bowrainStoredAuth{
			ServerURL:   firstNonEmpty(serverURL, os.Getenv("BOWRAIN_SERVER_URL")),
			AccessToken: token,
		}, nil
	}

	data, err := os.ReadFile(bowrainAuthFilePath())
	if err != nil {
		return nil, err
	}
	var a bowrainStoredAuth
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, fmt.Errorf("parse auth metadata: %w", err)
	}
	if a.ServerURL == "" {
		return nil, errors.New("auth metadata missing server URL")
	}
	// If a specific server was requested, only honor stored auth for it.
	if serverURL != "" && a.ServerURL != serverURL {
		return nil, fmt.Errorf("no stored auth for %s (have %s)", serverURL, a.ServerURL)
	}

	access, err := keyring.Get(bowrainKeyringService, bowrainAccessKey(a.ServerURL))
	if err != nil {
		return nil, fmt.Errorf("read access token from keychain: %w", err)
	}
	a.AccessToken = access

	if refresh, err := keyring.Get(bowrainKeyringService, bowrainRefreshKey(a.ServerURL)); err == nil {
		a.RefreshToken = refresh
	}
	return &a, nil
}

// deleteBowrainAuth clears stored credentials for a server URL. Missing
// entries are not an error.
func deleteBowrainAuth(serverURL string) error {
	if serverURL != "" {
		if err := keyring.Delete(bowrainKeyringService, bowrainAccessKey(serverURL)); err != nil && !errors.Is(err, keyring.ErrNotFound) {
			return fmt.Errorf("delete access token: %w", err)
		}
		if err := keyring.Delete(bowrainKeyringService, bowrainRefreshKey(serverURL)); err != nil && !errors.Is(err, keyring.ErrNotFound) {
			return fmt.Errorf("delete refresh token: %w", err)
		}
	}
	if err := os.Remove(bowrainAuthFilePath()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove auth metadata: %w", err)
	}
	return nil
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
