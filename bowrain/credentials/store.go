package credentials

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/neokapi/neokapi/core/id"
	"github.com/zalando/go-keyring"
)

const keyringService = "bowrain"

// ProviderConfig holds saved AI provider configuration.
// API keys are NOT stored here; they go in the OS keychain.
type ProviderConfig struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ProviderType string `json:"provider_type"` // "anthropic", "openai", "ollama"
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

// DefaultPath returns the default config file path (e.g. ~/.config/bowrain/providers.json on Linux).
// Used by bowrain-server, bowrain-worker, and bowrain desktop app.
func DefaultPath() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(configDir, "bowrain", "providers.json")
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
func (s *Store) Get(id string) (ProviderConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, c := range s.configs {
		if c.ID == id {
			return c, nil
		}
	}
	return ProviderConfig{}, fmt.Errorf("provider config %q not found", id)
}

// Upsert inserts or updates a provider config. If cfg.ID is empty, a new UUID is assigned.
// Returns the (possibly updated) config.
func (s *Store) Upsert(cfg ProviderConfig) ProviderConfig {
	s.mu.Lock()
	defer s.mu.Unlock()

	if cfg.ID == "" {
		cfg.ID = id.New()
	}

	for i, c := range s.configs {
		if c.ID == cfg.ID {
			s.configs[i] = cfg
			s.save()
			return cfg
		}
	}

	s.configs = append(s.configs, cfg)
	s.save()
	return cfg
}

// Remove deletes a provider config by ID.
func (s *Store) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, c := range s.configs {
		if c.ID == id {
			s.configs = append(s.configs[:i], s.configs[i+1:]...)
			s.save()
			return nil
		}
	}
	return fmt.Errorf("provider config %q not found", id)
}

// SetAPIKey stores an API key in the OS keychain.
func (s *Store) SetAPIKey(id, key string) error {
	return keyring.Set(keyringService, id, key)
}

// GetAPIKey retrieves an API key from the OS keychain.
func (s *Store) GetAPIKey(id string) (string, error) {
	return keyring.Get(keyringService, id)
}

// DeleteAPIKey removes an API key from the OS keychain.
func (s *Store) DeleteAPIKey(id string) error {
	return keyring.Delete(keyringService, id)
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

func (s *Store) save() {
	data, err := json.MarshalIndent(s.configs, "", "  ")
	if err != nil {
		return
	}

	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}

	// Atomic write: temp file + rename
	tmp := s.filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, s.filePath)
}
