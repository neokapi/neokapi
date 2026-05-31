package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/cli/credentials"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStoreEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "providers.json")
	s := credentials.NewStore(path)
	assert.Empty(t, s.List())
}

func TestUpsertAndList(t *testing.T) {
	dir := t.TempDir()
	s := credentials.NewStore(filepath.Join(dir, "providers.json"))

	cfg, err := s.Upsert(credentials.ProviderConfig{
		Name:         "My Anthropic",
		ProviderType: "anthropic",
		Model:        "claude-sonnet-4-5-20241022",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, cfg.ID, "should auto-assign an ID")
	assert.Equal(t, "My Anthropic", cfg.Name)

	list := s.List()
	require.Len(t, list, 1)
	assert.Equal(t, cfg.ID, list[0].ID)
}

func TestUpsertUpdate(t *testing.T) {
	dir := t.TempDir()
	s := credentials.NewStore(filepath.Join(dir, "providers.json"))

	cfg, err := s.Upsert(credentials.ProviderConfig{
		Name:         "Original",
		ProviderType: "openai",
	})
	require.NoError(t, err)

	cfg.Name = "Updated"
	_, err = s.Upsert(cfg)
	require.NoError(t, err)

	list := s.List()
	require.Len(t, list, 1)
	assert.Equal(t, "Updated", list[0].Name)
}

func TestGet(t *testing.T) {
	dir := t.TempDir()
	s := credentials.NewStore(filepath.Join(dir, "providers.json"))

	cfg, err := s.Upsert(credentials.ProviderConfig{
		Name:         "Test",
		ProviderType: "anthropic",
	})
	require.NoError(t, err)

	got, err := s.Get(cfg.ID)
	require.NoError(t, err)
	assert.Equal(t, "Test", got.Name)

	_, err = s.Get("nonexistent")
	assert.Error(t, err)
}

func TestRemove(t *testing.T) {
	dir := t.TempDir()
	s := credentials.NewStore(filepath.Join(dir, "providers.json"))

	cfg, err := s.Upsert(credentials.ProviderConfig{
		Name:         "To Delete",
		ProviderType: "ollama",
	})
	require.NoError(t, err)

	require.NoError(t, s.Remove(cfg.ID))
	assert.Empty(t, s.List())

	assert.Error(t, s.Remove("nonexistent"))
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "providers.json")

	// Create and save.
	s1 := credentials.NewStore(path)
	_, err := s1.Upsert(credentials.ProviderConfig{
		ID:           "test-id",
		Name:         "Persisted",
		ProviderType: "anthropic",
	})
	require.NoError(t, err)

	// Reopen and verify.
	s2 := credentials.NewStore(path)
	list := s2.List()
	require.Len(t, list, 1)
	assert.Equal(t, "test-id", list[0].ID)
	assert.Equal(t, "Persisted", list[0].Name)
}

func TestDefaultPath(t *testing.T) {
	path := credentials.DefaultPath()
	assert.Contains(t, path, "kapi")
	assert.Contains(t, path, "providers.json")
}

func TestLoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "providers.json")
	require.NoError(t, os.WriteFile(path, []byte("not json"), 0o644))

	s := credentials.NewStore(path)
	assert.Empty(t, s.List(), "invalid JSON should result in empty store")
}

func TestMultipleProviders(t *testing.T) {
	dir := t.TempDir()
	s := credentials.NewStore(filepath.Join(dir, "providers.json"))

	_, err := s.Upsert(credentials.ProviderConfig{Name: "A", ProviderType: "anthropic"})
	require.NoError(t, err)
	_, err = s.Upsert(credentials.ProviderConfig{Name: "B", ProviderType: "openai"})
	require.NoError(t, err)
	_, err = s.Upsert(credentials.ProviderConfig{Name: "C", ProviderType: "ollama"})
	require.NoError(t, err)

	assert.Len(t, s.List(), 3)
}
