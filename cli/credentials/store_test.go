package credentials

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStore_Empty(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "providers.json"))
	assert.Empty(t, s.List())
}

func TestUpsert_AutoAssignsID(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "providers.json"))
	cfg := s.Upsert(ProviderConfig{
		Name:         "My Anthropic",
		ProviderType: "anthropic",
		Model:        "claude-sonnet-4-5-20250514",
	})
	assert.NotEmpty(t, cfg.ID)
	assert.Equal(t, "My Anthropic", cfg.Name)
	require.Len(t, s.List(), 1)
	assert.Equal(t, cfg.ID, s.List()[0].ID)
}

func TestUpsert_UpdatesExisting(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "providers.json"))
	cfg := s.Upsert(ProviderConfig{Name: "Original", ProviderType: "openai"})

	cfg.Name = "Updated"
	s.Upsert(cfg)

	list := s.List()
	require.Len(t, list, 1)
	assert.Equal(t, "Updated", list[0].Name)
}

func TestGet_ByID(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "providers.json"))
	cfg := s.Upsert(ProviderConfig{Name: "Test", ProviderType: "anthropic"})

	got, err := s.Get(cfg.ID)
	require.NoError(t, err)
	assert.Equal(t, "Test", got.Name)

	_, err = s.Get("nonexistent")
	assert.Error(t, err)
}

func TestGetByName_CaseInsensitive(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "providers.json"))
	s.Upsert(ProviderConfig{Name: "My OpenAI Key", ProviderType: "openai"})

	// Exact match.
	cfg, err := s.GetByName("My OpenAI Key")
	require.NoError(t, err)
	assert.Equal(t, "openai", cfg.ProviderType)

	// Case-insensitive.
	cfg, err = s.GetByName("my openai key")
	require.NoError(t, err)
	assert.Equal(t, "openai", cfg.ProviderType)

	// Not found.
	_, err = s.GetByName("nonexistent")
	assert.Error(t, err)
}

func TestFindByType_FiltersCorrectly(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "providers.json"))
	s.Upsert(ProviderConfig{Name: "A", ProviderType: "anthropic"})
	s.Upsert(ProviderConfig{Name: "B", ProviderType: "openai"})
	s.Upsert(ProviderConfig{Name: "C", ProviderType: "anthropic"})
	s.Upsert(ProviderConfig{Name: "D", ProviderType: "gemini"})

	assert.Len(t, s.FindByType("anthropic"), 2)
	assert.Len(t, s.FindByType("openai"), 1)
	assert.Len(t, s.FindByType("gemini"), 1)
	assert.Empty(t, s.FindByType("ollama"))
	assert.Len(t, s.FindByType(""), 4, "empty type returns all")
}

func TestRemove(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "providers.json"))
	cfg := s.Upsert(ProviderConfig{Name: "To Delete", ProviderType: "ollama"})

	require.NoError(t, s.Remove(cfg.ID))
	assert.Empty(t, s.List())
	assert.Error(t, s.Remove("nonexistent"))
}

func TestPersistence_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "providers.json")

	s1 := NewStore(path)
	s1.Upsert(ProviderConfig{ID: "test-id", Name: "Persisted", ProviderType: "anthropic", Model: "claude-3"})

	s2 := NewStore(path)
	list := s2.List()
	require.Len(t, list, 1)
	assert.Equal(t, "test-id", list[0].ID)
	assert.Equal(t, "Persisted", list[0].Name)
	assert.Equal(t, "anthropic", list[0].ProviderType)
	assert.Equal(t, "claude-3", list[0].Model)
}

func TestDefaultPath(t *testing.T) {
	path := DefaultPath()
	assert.Contains(t, path, "kapi")
	assert.Contains(t, path, "providers.json")
}

func TestLoadInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "providers.json")
	require.NoError(t, os.WriteFile(path, []byte("not json"), 0o644))
	s := NewStore(path)
	assert.Empty(t, s.List())
}

func TestMultipleProviders(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "providers.json"))
	s.Upsert(ProviderConfig{Name: "A", ProviderType: "anthropic"})
	s.Upsert(ProviderConfig{Name: "B", ProviderType: "openai"})
	s.Upsert(ProviderConfig{Name: "C", ProviderType: "ollama"})
	assert.Len(t, s.List(), 3)
}

func TestUpsert_PreservesBaseURL(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "providers.json"))
	cfg := s.Upsert(ProviderConfig{
		Name:         "Custom",
		ProviderType: "openai",
		BaseURL:      "https://my-proxy.example.com/v1",
	})

	got, err := s.Get(cfg.ID)
	require.NoError(t, err)
	assert.Equal(t, "https://my-proxy.example.com/v1", got.BaseURL)
}
