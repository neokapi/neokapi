package credentials

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

// newStore returns a Store backed by a file under t.TempDir, so no test in this
// package can read or write the developer's real ~/.config/kapi/providers.json.
func newStore(t *testing.T) *Store {
	t.Helper()
	return NewStore(filepath.Join(t.TempDir(), "providers.json"), "kapi-test")
}

// mockKeyring swaps go-keyring's global provider for the in-memory one, so the
// keychain assertions below never reach the OS keychain. go-keyring exposes no
// way to restore the real provider, which is precisely what we want: once a
// test binary in this package has called it, nothing can fall back to the real
// keychain by accident.
func mockKeyring(t *testing.T) {
	t.Helper()
	keyring.MockInit()
}

func mustUpsert(t *testing.T, s *Store, cfg ProviderConfig) ProviderConfig {
	t.Helper()
	out, err := s.Upsert(cfg)
	require.NoError(t, err)
	return out
}

// --- default exclusivity ---------------------------------------------------

func TestSetDefaultStaysExclusiveWithinProviderType(t *testing.T) {
	s := newStore(t)

	a := mustUpsert(t, s, ProviderConfig{Name: "A", ProviderType: "openai"})
	b := mustUpsert(t, s, ProviderConfig{Name: "B", ProviderType: "openai"})
	other := mustUpsert(t, s, ProviderConfig{Name: "C", ProviderType: "anthropic"})

	require.NoError(t, s.SetDefault(a.ID))
	require.NoError(t, s.SetDefault(b.ID))

	byID := func(id string) ProviderConfig {
		c, err := s.Get(id)
		require.NoError(t, err)
		return c
	}
	assert.False(t, byID(a.ID).Default, "the default moved to B within openai")
	assert.True(t, byID(b.ID).Default)
	assert.False(t, byID(other.ID).Default, "a different provider type is unaffected")
}

// --- on-disk representation ------------------------------------------------

func TestSaveWritesOwnerOnlyPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "providers.json")
	s := NewStore(path, "kapi-test")

	mustUpsert(t, s, ProviderConfig{Name: "A", ProviderType: "anthropic"})

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(),
		"provider configs are owner-only; a group- or world-readable file leaks endpoints and model choices")

	// The atomic-rename temp file must not be left behind.
	_, err = os.Stat(path + ".tmp")
	assert.True(t, os.IsNotExist(err), "the .tmp staging file should be renamed away, not left on disk")
}

func TestSaveCreatesMissingConfigDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "deeper", "providers.json")
	s := NewStore(path, "kapi-test")

	mustUpsert(t, s, ProviderConfig{Name: "A", ProviderType: "anthropic"})

	_, err := os.Stat(path)
	require.NoError(t, err, "save should create the parent directory")
}

// unwritableStore returns a Store whose save() cannot succeed, because a parent
// path component is a regular file rather than a directory. That fails MkdirAll
// with ENOTDIR for any uid, so the test behaves the same as root (as in many CI
// containers) as it does for an ordinary user.
func unwritableStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("not a directory"), 0o600))
	return NewStore(filepath.Join(blocker, "providers.json"), "kapi-test")
}

func TestUpsertRollsBackWhenSaveFails(t *testing.T) {
	s := unwritableStore(t)

	_, err := s.Upsert(ProviderConfig{Name: "A", ProviderType: "anthropic"})
	require.Error(t, err, "an unpersistable config must be reported, not silently accepted")
	assert.Empty(t, s.List(), "a failed save must leave no phantom config in memory")
}

func TestUpsertUpdateRollsBackWhenSaveFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "providers.json")
	s := NewStore(path, "kapi-test")
	cfg := mustUpsert(t, s, ProviderConfig{Name: "A", ProviderType: "anthropic", Model: "old"})

	// Replace the config file's directory with something save() cannot write
	// through, then attempt an update.
	require.NoError(t, os.RemoveAll(dir))
	require.NoError(t, os.WriteFile(dir, []byte("not a directory"), 0o600))

	cfg.Model = "new"
	_, err := s.Upsert(cfg)
	require.Error(t, err)

	got, err := s.Get(cfg.ID)
	require.NoError(t, err)
	assert.Equal(t, "old", got.Model, "a failed save must roll the in-memory config back")
}

func TestRemoveRollsBackWhenSaveFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "providers.json")
	s := NewStore(path, "kapi-test")
	cfg := mustUpsert(t, s, ProviderConfig{Name: "A", ProviderType: "anthropic"})

	require.NoError(t, os.RemoveAll(dir))
	require.NoError(t, os.WriteFile(dir, []byte("not a directory"), 0o600))

	require.Error(t, s.Remove(cfg.ID))
	assert.Len(t, s.List(), 1, "a config that could not be removed on disk is still present")
}

func TestSetDefaultRollsBackWhenSaveFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "providers.json")
	s := NewStore(path, "kapi-test")
	cfg := mustUpsert(t, s, ProviderConfig{Name: "A", ProviderType: "anthropic"})

	require.NoError(t, os.RemoveAll(dir))
	require.NoError(t, os.WriteFile(dir, []byte("not a directory"), 0o600))

	require.Error(t, s.SetDefault(cfg.ID))

	got, err := s.Get(cfg.ID)
	require.NoError(t, err)
	assert.False(t, got.Default, "a failed save must roll the flag back")
}

// TestListReturnsACopy guards the defensive copy in List: a caller that mutates
// the returned slice must not reach into the store's state.
func TestListReturnsACopy(t *testing.T) {
	s := newStore(t)
	cfg := mustUpsert(t, s, ProviderConfig{Name: "A", ProviderType: "anthropic"})

	list := s.List()
	require.Len(t, list, 1)
	list[0].Name = "mutated"

	got, err := s.Get(cfg.ID)
	require.NoError(t, err)
	assert.Equal(t, "A", got.Name, "List must hand out a copy, not the store's backing array")
}

// --- lookups ---------------------------------------------------------------

func TestGetByNameIsCaseInsensitive(t *testing.T) {
	s := newStore(t)
	cfg := mustUpsert(t, s, ProviderConfig{Name: "My Anthropic", ProviderType: "anthropic"})

	got, err := s.GetByName("my anthropic")
	require.NoError(t, err)
	assert.Equal(t, cfg.ID, got.ID)

	_, err = s.GetByName("nope")
	assert.Error(t, err)
}

func TestFindByType(t *testing.T) {
	s := newStore(t)
	mustUpsert(t, s, ProviderConfig{Name: "A", ProviderType: "openai"})
	mustUpsert(t, s, ProviderConfig{Name: "B", ProviderType: "OpenAI"})
	mustUpsert(t, s, ProviderConfig{Name: "C", ProviderType: "anthropic"})

	tests := []struct {
		name         string
		providerType string
		want         int
	}{
		{"case-insensitive match", "openai", 2},
		{"other type", "anthropic", 1},
		{"empty returns all", "", 3},
		{"unknown type", "gemini", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Len(t, s.FindByType(tt.providerType), tt.want)
		})
	}
}

// --- keychain --------------------------------------------------------------

func TestAPIKeyRoundTrip(t *testing.T) {
	mockKeyring(t)
	s := newStore(t)
	cfg := mustUpsert(t, s, ProviderConfig{Name: "A", ProviderType: "anthropic"})

	require.NoError(t, s.SetAPIKey(cfg.ID, "sk-secret"))

	got, err := s.GetAPIKey(cfg.ID)
	require.NoError(t, err)
	assert.Equal(t, "sk-secret", got)

	require.NoError(t, s.DeleteAPIKey(cfg.ID))
	_, err = s.GetAPIKey(cfg.ID)
	assert.ErrorIs(t, err, keyring.ErrNotFound)
}

func TestAPIKeysNeverReachTheConfigFile(t *testing.T) {
	mockKeyring(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "providers.json")
	s := NewStore(path, "kapi-test")

	cfg := mustUpsert(t, s, ProviderConfig{Name: "A", ProviderType: "anthropic"})
	require.NoError(t, s.SetAPIKey(cfg.ID, "sk-must-not-be-on-disk"))

	// Re-save so any leak would have been flushed.
	_, err := s.Upsert(ProviderConfig{Name: "B", ProviderType: "openai"})
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "sk-must-not-be-on-disk",
		"API keys live in the keychain; the JSON file must never carry one")
}

func TestStoresAreIsolatedByKeyringService(t *testing.T) {
	mockKeyring(t)
	kapi := NewStore(filepath.Join(t.TempDir(), "providers.json"), "kapi")
	bowrain := NewStore(filepath.Join(t.TempDir(), "providers.json"), "bowrain")

	require.NoError(t, kapi.SetAPIKey("shared-id", "kapi-key"))
	require.NoError(t, bowrain.SetAPIKey("shared-id", "bowrain-key"))

	got, err := kapi.GetAPIKey("shared-id")
	require.NoError(t, err)
	assert.Equal(t, "kapi-key", got, "the keychain service name must scope each host's keys")
}

// --- load ------------------------------------------------------------------

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int
	}{
		{"missing file starts empty", "", 0},
		{"invalid JSON starts empty", "not json", 0},
		{"an object where an array is expected starts empty", `{"id":"x"}`, 0},
		{"empty array", `[]`, 0},
		{"one config", `[{"id":"x","name":"A","provider_type":"anthropic"}]`, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "providers.json")
			if tt.content != "" {
				require.NoError(t, os.WriteFile(path, []byte(tt.content), 0o600))
			}
			s := NewStore(path, "kapi-test")
			assert.Len(t, s.List(), tt.want)
		})
	}
}

func TestPersistenceAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "providers.json")

	s1 := NewStore(path, "kapi-test")
	cfg := mustUpsert(t, s1, ProviderConfig{
		Name:         "Persisted",
		ProviderType: "anthropic",
		Model:        "claude-sonnet-4-5",
		BaseURL:      "https://example.invalid",
	})
	require.NoError(t, s1.SetDefault(cfg.ID))

	s2 := NewStore(path, "kapi-test")
	got, err := s2.Get(cfg.ID)
	require.NoError(t, err)
	assert.Equal(t, cfg.Name, got.Name)
	assert.Equal(t, "claude-sonnet-4-5", got.Model)
	assert.Equal(t, "https://example.invalid", got.BaseURL)
	assert.True(t, got.Default)
}
