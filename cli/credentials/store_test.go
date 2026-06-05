package credentials

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mustUpsert upserts cfg and fails the test if persistence errors.
func mustUpsert(t *testing.T, s *Store, cfg ProviderConfig) ProviderConfig {
	t.Helper()
	got, err := s.Upsert(cfg)
	require.NoError(t, err)
	return got
}

func TestNewStore_Empty(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "providers.json"))
	assert.Empty(t, s.List())
}

func TestUpsert_AutoAssignsID(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "providers.json"))
	cfg, err := s.Upsert(ProviderConfig{
		Name:         "My Anthropic",
		ProviderType: "anthropic",
		Model:        "claude-sonnet-4-5-20250514",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, cfg.ID)
	assert.Equal(t, "My Anthropic", cfg.Name)
	require.Len(t, s.List(), 1)
	assert.Equal(t, cfg.ID, s.List()[0].ID)
}

func TestUpsert_UpdatesExisting(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "providers.json"))
	cfg, err := s.Upsert(ProviderConfig{Name: "Original", ProviderType: "openai"})
	require.NoError(t, err)

	cfg.Name = "Updated"
	_, err = s.Upsert(cfg)
	require.NoError(t, err)

	list := s.List()
	require.Len(t, list, 1)
	assert.Equal(t, "Updated", list[0].Name)
}

func TestGet_ByID(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "providers.json"))
	cfg, err := s.Upsert(ProviderConfig{Name: "Test", ProviderType: "anthropic"})
	require.NoError(t, err)

	got, err := s.Get(cfg.ID)
	require.NoError(t, err)
	assert.Equal(t, "Test", got.Name)

	_, err = s.Get("nonexistent")
	assert.ErrorContains(t, err, "not found")
}

func TestGetByName_CaseInsensitive(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "providers.json"))
	mustUpsert(t, s, ProviderConfig{Name: "My OpenAI Key", ProviderType: "openai"})

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
	assert.ErrorContains(t, err, "not found")
}

func TestFindByType_FiltersCorrectly(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "providers.json"))
	mustUpsert(t, s, ProviderConfig{Name: "A", ProviderType: "anthropic"})
	mustUpsert(t, s, ProviderConfig{Name: "B", ProviderType: "openai"})
	mustUpsert(t, s, ProviderConfig{Name: "C", ProviderType: "anthropic"})
	mustUpsert(t, s, ProviderConfig{Name: "D", ProviderType: "gemini"})

	assert.Len(t, s.FindByType("anthropic"), 2)
	assert.Len(t, s.FindByType("openai"), 1)
	assert.Len(t, s.FindByType("gemini"), 1)
	assert.Empty(t, s.FindByType("ollama"))
	assert.Len(t, s.FindByType(""), 4, "empty type returns all")
}

func TestRemove(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "providers.json"))
	cfg := mustUpsert(t, s, ProviderConfig{Name: "To Delete", ProviderType: "ollama"})

	require.NoError(t, s.Remove(cfg.ID))
	assert.Empty(t, s.List())
	assert.Error(t, s.Remove("nonexistent"))
}

func TestPersistence_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "providers.json")

	s1 := NewStore(path)
	mustUpsert(t, s1, ProviderConfig{ID: "test-id", Name: "Persisted", ProviderType: "anthropic", Model: "claude-3"})

	s2 := NewStore(path)
	list := s2.List()
	require.Len(t, list, 1)
	assert.Equal(t, "test-id", list[0].ID)
	assert.Equal(t, "Persisted", list[0].Name)
	assert.Equal(t, "anthropic", list[0].ProviderType)
	assert.Equal(t, "claude-3", list[0].Model)
}

func TestDefaultPath(t *testing.T) {
	// Isolate from a developer's environment that may set KAPI_CONFIG_DIR.
	t.Setenv("KAPI_CONFIG_DIR", "")
	path := DefaultPath()
	assert.Contains(t, path, "kapi")
	assert.Contains(t, path, "providers.json")
}

// TestDefaultPath_HonorsKapiConfigDir verifies that DefaultPath uses
// KAPI_CONFIG_DIR as the base directory when set, so isolated (dogfood/test)
// kapi invocations never read or write the developer's real providers.json.
func TestDefaultPath_HonorsKapiConfigDir(t *testing.T) {
	t.Run("set uses KAPI_CONFIG_DIR as base", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("KAPI_CONFIG_DIR", dir)
		assert.Equal(t, filepath.Join(dir, "providers.json"), DefaultPath())
	})

	t.Run("empty falls back to user config dir", func(t *testing.T) {
		t.Setenv("KAPI_CONFIG_DIR", "")
		path := DefaultPath()
		// An empty value must not be treated as a base; the parent dir is "kapi".
		assert.Equal(t, "kapi", filepath.Base(filepath.Dir(path)))
		assert.Equal(t, "providers.json", filepath.Base(path))
	})
}

func TestLoadInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "providers.json")
	require.NoError(t, os.WriteFile(path, []byte("not json"), 0o644))
	s := NewStore(path)
	assert.Empty(t, s.List())
}

func TestMultipleProviders(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "providers.json"))
	mustUpsert(t, s, ProviderConfig{Name: "A", ProviderType: "anthropic"})
	mustUpsert(t, s, ProviderConfig{Name: "B", ProviderType: "openai"})
	mustUpsert(t, s, ProviderConfig{Name: "C", ProviderType: "ollama"})
	assert.Len(t, s.List(), 3)
}

func TestUpsert_PreservesBaseURL(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "providers.json"))
	cfg := mustUpsert(t, s, ProviderConfig{
		Name:         "Custom",
		ProviderType: "openai",
		BaseURL:      "https://my-proxy.example.com/v1",
	})

	got, err := s.Get(cfg.ID)
	require.NoError(t, err)
	assert.Equal(t, "https://my-proxy.example.com/v1", got.BaseURL)
}

// TestUpsert_PropagatesSaveError verifies that when the config cannot be
// persisted, Upsert returns an error (rather than falsely reporting success),
// and rolls back in-memory state so the store stays consistent with disk.
func TestUpsert_PropagatesSaveError(t *testing.T) {
	// Make the parent path a regular file so MkdirAll on the config dir fails:
	// blocker is a file, so blocker/providers.json cannot be created.
	base := t.TempDir()
	blocker := filepath.Join(base, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))

	s := NewStore(filepath.Join(blocker, "providers.json"))

	_, err := s.Upsert(ProviderConfig{Name: "Doomed", ProviderType: "anthropic"})
	require.Error(t, err)

	// In-memory state must be rolled back: nothing was persisted, so the store
	// must not pretend the config exists (which would orphan a keychain secret).
	assert.Empty(t, s.List(), "failed save must not leave the config in memory")
}

// TestUpsert_UpdateRollsBackOnSaveError verifies an in-place update is rolled
// back to its previous value when persistence fails.
func TestUpsert_UpdateRollsBackOnSaveError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "providers.json")
	s := NewStore(path)
	cfg := mustUpsert(t, s, ProviderConfig{Name: "Before", ProviderType: "openai"})

	// Make the next save fail by removing write permission on the config dir.
	dir := filepath.Dir(path)
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	cfg.Name = "After"
	_, err := s.Upsert(cfg)
	require.Error(t, err)

	require.NoError(t, os.Chmod(dir, 0o700))
	got, err := s.Get(cfg.ID)
	require.NoError(t, err)
	assert.Equal(t, "Before", got.Name, "failed save must roll back the update")
}

// TestRemove_PropagatesSaveError verifies Remove surfaces persistence failures
// and rolls back its in-memory deletion.
func TestRemove_PropagatesSaveError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "providers.json")
	s := NewStore(path)
	cfg := mustUpsert(t, s, ProviderConfig{Name: "Keep", ProviderType: "ollama"})

	// Remove write permission on the config dir so the atomic write/rename fails.
	dir := filepath.Dir(path)
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	err := s.Remove(cfg.ID)
	require.Error(t, err)

	// Deletion must be rolled back since it was never persisted.
	require.NoError(t, os.Chmod(dir, 0o700))
	assert.Len(t, s.List(), 1, "failed save must not drop the config from memory")
}
