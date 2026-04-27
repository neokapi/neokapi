package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/zalando/go-keyring"
)

// keyringService is the OS keychain service identifier shared with the
// kapi CLI's `cli/credentials` store. Using a single service value means
// users see a single keychain prompt per machine for both kapi (LLM
// provider keys) and bowrain (server tokens).
const keyringService = "kapi"

// keyringAccessKey returns the keychain key for the access token of a
// given bowrain server URL. Keys are namespaced with a `bowrain-auth:`
// prefix so they don't collide with kapi's UUID-keyed provider entries.
func keyringAccessKey(serverURL string) string {
	return "bowrain-auth:" + serverURL
}

// keyringRefreshKey returns the keychain key for the refresh token.
func keyringRefreshKey(serverURL string) string {
	return "bowrain-refresh:" + serverURL
}

// StoredAuth is the bowrain auth payload — server URL, tokens, expiry,
// and user info. Tokens are kept in the OS keychain; everything else is
// persisted as JSON metadata at the path returned by AuthFilePath().
type StoredAuth struct {
	ServerURL    string     `json:"server_url"`
	AccessToken  string     `json:"-"` // keychain
	RefreshToken string     `json:"-"` // keychain
	Expiry       time.Time  `json:"expiry"`
	User         StoredUser `json:"user"`
}

// StoredUser is the user info stored alongside the auth metadata.
type StoredUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// AuthFilePath returns the path to the auth metadata file. The file
// holds non-secret fields only; access and refresh tokens live in the OS
// keychain.
func AuthFilePath() string {
	if dir := os.Getenv("BOWRAIN_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "auth.json")
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(configDir, "bowrain", "auth.json")
}

// SaveAuth persists auth credentials. Tokens go to the OS keychain;
// non-secret metadata (server URL, expiry, user info) goes to the
// metadata JSON file.
func SaveAuth(a StoredAuth) error {
	if a.ServerURL == "" {
		return errors.New("save auth: server URL required")
	}

	if a.AccessToken != "" {
		if err := keyring.Set(keyringService, keyringAccessKey(a.ServerURL), a.AccessToken); err != nil {
			return fmt.Errorf("save access token to keychain: %w", err)
		}
	}
	if a.RefreshToken != "" {
		if err := keyring.Set(keyringService, keyringRefreshKey(a.ServerURL), a.RefreshToken); err != nil {
			return fmt.Errorf("save refresh token to keychain: %w", err)
		}
	}

	path := AuthFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create auth dir: %w", err)
	}
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal auth metadata: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

// LoadAuth returns auth credentials. Resolution order:
//
//  1. BOWRAIN_AUTH_TOKEN env var (CI/CD bypass — pairs with BOWRAIN_SERVER_URL).
//  2. Metadata file + tokens from the OS keychain.
func LoadAuth() (*StoredAuth, error) {
	if token := os.Getenv("BOWRAIN_AUTH_TOKEN"); token != "" {
		return &StoredAuth{
			ServerURL:   os.Getenv("BOWRAIN_SERVER_URL"),
			AccessToken: token,
		}, nil
	}
	data, err := os.ReadFile(AuthFilePath())
	if err != nil {
		return nil, err
	}
	var a StoredAuth
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, fmt.Errorf("parse auth metadata: %w", err)
	}
	if a.ServerURL == "" {
		return nil, errors.New("auth metadata missing server URL")
	}

	access, err := keyring.Get(keyringService, keyringAccessKey(a.ServerURL))
	if err != nil {
		return nil, fmt.Errorf("read access token from keychain: %w", err)
	}
	a.AccessToken = access

	if refresh, err := keyring.Get(keyringService, keyringRefreshKey(a.ServerURL)); err == nil {
		a.RefreshToken = refresh
	}

	return &a, nil
}

// DeleteAuth clears stored credentials for the given server URL: both
// keychain entries and the metadata file. Missing entries are not an
// error.
func DeleteAuth(serverURL string) error {
	if serverURL != "" {
		if err := keyring.Delete(keyringService, keyringAccessKey(serverURL)); err != nil && !errors.Is(err, keyring.ErrNotFound) {
			return fmt.Errorf("delete access token: %w", err)
		}
		if err := keyring.Delete(keyringService, keyringRefreshKey(serverURL)); err != nil && !errors.Is(err, keyring.ErrNotFound) {
			return fmt.Errorf("delete refresh token: %w", err)
		}
	}
	if err := os.Remove(AuthFilePath()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove auth metadata: %w", err)
	}
	return nil
}
