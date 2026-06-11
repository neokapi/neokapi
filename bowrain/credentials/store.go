// Package credentials manages AI provider configurations on disk and API keys
// in the OS keychain for the bowrain server, worker, and desktop app.
// Configurations are stored in ~/.config/bowrain/providers.json; API keys are
// kept in the platform keychain under the "bowrain" service.
//
// The implementation lives in the framework's core/credentials package; this
// package binds it to the bowrain config path and keychain service.
package credentials

import (
	"os"
	"path/filepath"

	corecreds "github.com/neokapi/neokapi/core/credentials"
)

const keyringService = "bowrain"

// ProviderConfig holds saved AI provider configuration.
// API keys are NOT stored here; they go in the OS keychain.
type ProviderConfig = corecreds.ProviderConfig

// ProviderConfigWithKey is used for transport (save/test) and is never persisted to disk.
type ProviderConfigWithKey = corecreds.ProviderConfigWithKey

// Store manages provider configurations on disk and API keys in the OS keychain.
type Store = corecreds.Store

// NewStore creates a Store backed by the given JSON file path.
// If the file does not exist, the store starts empty.
func NewStore(filePath string) *Store {
	return corecreds.NewStore(filePath, keyringService)
}

// DefaultPath returns the default config file path (e.g. ~/.config/bowrain/providers.json on Linux).
// Used by bowrain-server, bowrain-worker, and bowrain desktop app.
func DefaultPath() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(configDir, "bowrain", "providers.json")
}
