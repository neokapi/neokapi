package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// StoredAuth is the auth token persisted at ~/.config/bowrain/auth.json.
type StoredAuth struct {
	ServerURL    string     `json:"server_url"`
	AccessToken  string     `json:"access_token"`
	RefreshToken string     `json:"refresh_token,omitempty"`
	Expiry       time.Time  `json:"expiry"`
	User         StoredUser `json:"user"`
}

// StoredUser is the user info stored alongside the token.
type StoredUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// AuthFilePath returns the path to the auth token file.
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

// SaveAuth persists auth credentials to disk.
func SaveAuth(a StoredAuth) error {
	path := AuthFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// LoadAuth returns auth credentials. It checks the BOWRAIN_AUTH_TOKEN
// environment variable first (for CI/CD), then falls back to persisted
// credentials on disk. When using the env var, BOWRAIN_SERVER_URL should
// also be set so server URL validation succeeds.
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
		return nil, err
	}
	return &a, nil
}
