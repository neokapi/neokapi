// Package credentials manages AI provider configurations on disk and API keys
// in the OS keychain. Configurations are stored in ~/.config/kapi/providers.json;
// API keys are kept in the platform keychain (macOS Keychain, Windows Credential
// Manager, Linux Secret Service).
//
// Both the kapi CLI and Kapi Desktop share this store.
package credentials

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/neokapi/neokapi/core/id"
	"github.com/zalando/go-keyring"
)

const keyringService = "kapi"

// ProviderConfig holds saved AI provider configuration.
// API keys are NOT stored here; they go in the OS keychain.
type ProviderConfig struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ProviderType string `json:"provider_type"` // "anthropic", "openai", "ollama", "azureopenai", "gemini"
	Model        string `json:"model,omitempty"`
	BaseURL      string `json:"base_url,omitempty"`
}

// ProviderConfigWithKey is used for transport (save/test) and is never persisted to disk.
type ProviderConfigWithKey struct {
	ProviderConfig
	APIKey string `json:"api_key"`
}

// Store manages provider configurations on disk and API keys in the OS keychain.
type Store struct {
	mu       sync.RWMutex
	filePath string
	configs  []ProviderConfig
}

// NewStore creates a Store backed by the given JSON file path.
// If the file does not exist, the store starts empty.
func NewStore(filePath string) *Store {
	s := &Store{filePath: filePath}
	s.load()
	return s
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

// List returns all stored provider configs.
func (s *Store) List() []ProviderConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ProviderConfig, len(s.configs))
	copy(out, s.configs)
	return out
}

// Get returns a provider config by ID.
func (s *Store) Get(configID string) (ProviderConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, c := range s.configs {
		if c.ID == configID {
			return c, nil
		}
	}
	return ProviderConfig{}, fmt.Errorf("provider config %q not found", configID)
}

// GetByName returns a provider config by its user-friendly name (case-insensitive).
func (s *Store) GetByName(name string) (ProviderConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, c := range s.configs {
		if strings.EqualFold(c.Name, name) {
			return c, nil
		}
	}
	return ProviderConfig{}, fmt.Errorf("provider config with name %q not found", name)
}

// FindByType returns all provider configs matching the given provider type.
// If providerType is empty, returns all configs.
func (s *Store) FindByType(providerType string) []ProviderConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []ProviderConfig
	for _, c := range s.configs {
		if providerType == "" || strings.EqualFold(c.ProviderType, providerType) {
			out = append(out, c)
		}
	}
	return out
}

// Upsert inserts or updates a provider config, persisting the change to disk.
// If cfg.ID is empty, a new ID is assigned. The returned config carries the
// assigned ID. A non-nil error means the config was NOT persisted; callers must
// surface it (and avoid writing an orphaned keychain secret) rather than report
// success.
func (s *Store) Upsert(cfg ProviderConfig) (ProviderConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if cfg.ID == "" {
		cfg.ID = id.New()
	}

	// Persist config before the caller writes the keychain secret, so a failed
	// write surfaces as an error instead of leaving an orphaned secret behind.
	// On failure roll back to prev; build a fresh slice so the in-place update
	// does not mutate prev's shared backing array.
	prev := s.configs
	for i, c := range s.configs {
		if c.ID == cfg.ID {
			updated := make([]ProviderConfig, len(s.configs))
			copy(updated, s.configs)
			updated[i] = cfg
			s.configs = updated
			if err := s.save(); err != nil {
				s.configs = prev
				return ProviderConfig{}, err
			}
			return cfg, nil
		}
	}

	s.configs = append(s.configs[:len(s.configs):len(s.configs)], cfg)
	if err := s.save(); err != nil {
		s.configs = prev
		return ProviderConfig{}, err
	}
	return cfg, nil
}

// Remove deletes a provider config by ID, persisting the change to disk.
// A non-nil error means the change was NOT persisted.
func (s *Store) Remove(configID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, c := range s.configs {
		if c.ID == configID {
			prev := s.configs
			s.configs = append(s.configs[:i:i], s.configs[i+1:]...)
			if err := s.save(); err != nil {
				s.configs = prev
				return err
			}
			return nil
		}
	}
	return fmt.Errorf("provider config %q not found", configID)
}

// SetAPIKey stores an API key in the OS keychain.
func (s *Store) SetAPIKey(configID, key string) error {
	return keyring.Set(keyringService, configID, key)
}

// GetAPIKey retrieves an API key from the OS keychain.
func (s *Store) GetAPIKey(configID string) (string, error) {
	return keyring.Get(keyringService, configID)
}

// DeleteAPIKey removes an API key from the OS keychain.
func (s *Store) DeleteAPIKey(configID string) error {
	return keyring.Delete(keyringService, configID)
}

func (s *Store) load() {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		s.configs = nil
		return
	}
	var configs []ProviderConfig
	if err := json.Unmarshal(data, &configs); err != nil {
		s.configs = nil
		return
	}
	s.configs = configs
}

func (s *Store) save() error {
	data, err := json.MarshalIndent(s.configs, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal provider configs: %w", err)
	}

	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config dir %q: %w", dir, err)
	}

	tmp := s.filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write provider configs: %w", err)
	}
	if err := os.Rename(tmp, s.filePath); err != nil {
		return fmt.Errorf("persist provider configs to %q: %w", s.filePath, err)
	}
	return nil
}
