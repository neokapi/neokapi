package credentials

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

func TestMain(m *testing.M) {
	keyring.MockInit()
	os.Exit(m.Run())
}

func tempStorePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "providers.json")
}

func TestUpsertAndGet(t *testing.T) {
	s := NewStore(tempStorePath(t))

	cfg := ProviderConfig{
		Name:         "My Anthropic",
		ProviderType: "anthropic",
		Model:        "claude-sonnet-4-20250514",
	}

	saved := s.Upsert(cfg)
	assert.NotEmpty(t, saved.ID)
	assert.Equal(t, "My Anthropic", saved.Name)

	got, err := s.Get(saved.ID)
	require.NoError(t, err)
	assert.Equal(t, saved, got)
}

func TestUpsertUpdate(t *testing.T) {
	s := NewStore(tempStorePath(t))

	cfg := s.Upsert(ProviderConfig{
		Name:         "Test",
		ProviderType: "openai",
		Model:        "gpt-4o",
	})

	cfg.Model = "gpt-4o-mini"
	updated := s.Upsert(cfg)
	assert.Equal(t, cfg.ID, updated.ID)
	assert.Equal(t, "gpt-4o-mini", updated.Model)

	// Should still be one entry
	assert.Len(t, s.List(), 1)
}

func TestList(t *testing.T) {
	s := NewStore(tempStorePath(t))
	assert.Empty(t, s.List())

	s.Upsert(ProviderConfig{Name: "A", ProviderType: "anthropic"})
	s.Upsert(ProviderConfig{Name: "B", ProviderType: "openai"})
	assert.Len(t, s.List(), 2)
}

func TestRemove(t *testing.T) {
	s := NewStore(tempStorePath(t))

	cfg := s.Upsert(ProviderConfig{Name: "Temp", ProviderType: "ollama"})
	require.Len(t, s.List(), 1)

	err := s.Remove(cfg.ID)
	require.NoError(t, err)
	assert.Empty(t, s.List())

	// Remove non-existent returns error
	err = s.Remove("no-such-id")
	assert.Error(t, err)
}

func TestGetNotFound(t *testing.T) {
	s := NewStore(tempStorePath(t))
	_, err := s.Get("missing")
	assert.Error(t, err)
}

func TestPersistence(t *testing.T) {
	path := tempStorePath(t)

	s1 := NewStore(path)
	s1.Upsert(ProviderConfig{Name: "Persist", ProviderType: "anthropic", Model: "claude-sonnet-4-20250514"})
	require.Len(t, s1.List(), 1)

	// Re-open from same file
	s2 := NewStore(path)
	configs := s2.List()
	require.Len(t, configs, 1)
	assert.Equal(t, "Persist", configs[0].Name)
	assert.Equal(t, "anthropic", configs[0].ProviderType)
}

func TestKeyringSetGetDelete(t *testing.T) {
	s := NewStore(tempStorePath(t))

	cfg := s.Upsert(ProviderConfig{Name: "KeyTest", ProviderType: "anthropic"})

	err := s.SetAPIKey(cfg.ID, "sk-test-12345")
	require.NoError(t, err)

	key, err := s.GetAPIKey(cfg.ID)
	require.NoError(t, err)
	assert.Equal(t, "sk-test-12345", key)

	err = s.DeleteAPIKey(cfg.ID)
	require.NoError(t, err)

	_, err = s.GetAPIKey(cfg.ID)
	assert.Error(t, err)
}

func TestMissingFileCreatesEmptyStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent", "providers.json")
	s := NewStore(path)
	assert.Empty(t, s.List())

	// Upsert should create the directory and file
	s.Upsert(ProviderConfig{Name: "New", ProviderType: "openai"})
	assert.Len(t, s.List(), 1)

	// File should exist now
	_, err := os.Stat(path)
	assert.NoError(t, err)
}

func TestDefaultPath(t *testing.T) {
	p := DefaultPath()
	assert.Contains(t, p, "providers.json")
	assert.Contains(t, p, "bowrain")

	// On Linux the path includes .config; on macOS it uses ~/Library/Application Support.
	configDir, _ := os.UserConfigDir()
	assert.NotEmpty(t, p)
	assert.Contains(t, p, configDir)
}
