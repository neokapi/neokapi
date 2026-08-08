package host

import (
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/host/credentials"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

func newTestCredStore(t *testing.T) *credentials.Store {
	t.Helper()
	// Swap go-keyring's global provider for the in-memory one so key writes
	// never reach the OS keychain (and pass on a headless CI runner).
	keyring.MockInit()
	return credentials.NewStore(filepath.Join(t.TempDir(), "providers.json"))
}

// TestSaveCredential_RejectsUnknownProvider is finding 5: the desktop used to
// persist a typo'd provider type because only the CLI validated. The shared
// host path rejects it, so neither surface can.
func TestSaveCredential_RejectsUnknownProvider(t *testing.T) {
	store := newTestCredStore(t)

	_, err := SaveCredential(store, credentials.ProviderConfig{
		Name:         "oops",
		ProviderType: "antropic", // typo
	}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown provider")

	// Nothing was persisted.
	assert.Empty(t, store.List())
}

// TestSaveCredential_PersistsValidProvider confirms the happy path upserts the
// config and stores the key.
func TestSaveCredential_PersistsValidProvider(t *testing.T) {
	store := newTestCredStore(t)

	cfg, err := SaveCredential(store, credentials.ProviderConfig{
		Name:         "my-anthropic",
		ProviderType: "anthropic",
	}, "sk-test-key")
	require.NoError(t, err)
	assert.NotEmpty(t, cfg.ID)
	require.Len(t, store.List(), 1)

	usable, err := TestCredential(store, cfg)
	require.NoError(t, err)
	assert.True(t, usable, "a stored key makes the credential usable")
}

// TestTestCredential_KeylessProviderPasses is the other half of the fork: a
// keyless provider is usable with no key, so both surfaces report it usable
// rather than one saying "empty".
func TestTestCredential_KeylessProviderPasses(t *testing.T) {
	store := newTestCredStore(t)

	cfg, err := SaveCredential(store, credentials.ProviderConfig{
		Name:         "local-ollama",
		ProviderType: "ollama",
	}, "")
	require.NoError(t, err)

	usable, err := TestCredential(store, cfg)
	require.NoError(t, err)
	assert.True(t, usable, "keyless providers need no key to be usable")
}
