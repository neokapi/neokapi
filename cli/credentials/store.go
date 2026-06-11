// Package credentials manages AI provider configurations on disk and API keys
// in the OS keychain. Configurations are stored in ~/.config/kapi/providers.json;
// API keys are kept in the platform keychain (macOS Keychain, Windows Credential
// Manager, Linux Secret Service).
//
// Both the kapi CLI and Kapi Desktop share this store. The implementation
// lives in the framework's core/credentials package; this package binds it to
// the kapi config path and keychain service.
package credentials

import (
	"os"
	"path/filepath"

	corecreds "github.com/neokapi/neokapi/core/credentials"
)

const keyringService = "kapi"

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

// DefaultPath returns the default config file path (~/.config/kapi/providers.json).
//
// When KAPI_CONFIG_DIR is set it is used as the kapi config directory directly
// (mirroring cli/config and cli/resource), so providers.json lands under it.
// This keeps isolated (dogfood/test) kapi invocations from reading or writing
// the developer's real ~/.config/kapi/providers.json. An empty KAPI_CONFIG_DIR
// falls back to os.UserConfigDir()/kapi.
func DefaultPath() string {
	if dir := os.Getenv("KAPI_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "providers.json")
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(configDir, "kapi", "providers.json")
}
