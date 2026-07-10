package store

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/bowrain/crypto"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestProviderStore returns a ProviderConfigStore over a freshly migrated
// test schema. Passing a nil cipher exercises the plaintext pass-through path.
func newTestProviderStore(t *testing.T, cipher *crypto.Cipher) *ProviderConfigStore {
	t.Helper()
	db := pgtest.NewTestDB(t)
	// NewPostgresStoreFromDB runs the store baseline migrations, which create the
	// provider_configs table (Version 2).
	_, err := NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	return NewProviderConfigStore(db.DB, cipher)
}

func TestProviderConfigCRUD(t *testing.T) {
	s := newTestProviderStore(t, testCipher(t))
	ctx := t.Context()
	const wsID = "ws-acme"

	// Create with an API key.
	saved, err := s.Upsert(ctx, &ProviderConfig{
		WorkspaceID:   wsID,
		WorkspaceSlug: "acme",
		Name:          "OpenAI (prod)",
		Type:          "openai",
		Model:         "gpt-4o",
		BaseURL:       "https://api.openai.com/v1",
		APIKey:        "sk-secret-1",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, saved.ID)
	assert.Equal(t, "OpenAI (prod)", saved.Name)
	assert.Equal(t, "openai", saved.Type)
	assert.Empty(t, saved.APIKey, "Upsert must not echo the API key back")

	id := saved.ID

	// Get returns the decrypted key.
	got, err := s.Get(ctx, wsID, id)
	require.NoError(t, err)
	assert.Equal(t, "sk-secret-1", got.APIKey)
	assert.Equal(t, "gpt-4o", got.Model)

	// List omits the key.
	list, err := s.List(ctx, wsID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Empty(t, list[0].APIKey, "List must never carry API keys")
	assert.Equal(t, "OpenAI (prod)", list[0].Name)

	// Upsert with an empty key (rename/model edit) preserves the stored key.
	_, err = s.Upsert(ctx, &ProviderConfig{
		WorkspaceID:   wsID,
		WorkspaceSlug: "acme",
		Name:          "OpenAI (prod)",
		Type:          "openai",
		Model:         "gpt-4o-mini",
		APIKey:        "", // unchanged
	})
	require.NoError(t, err)
	got, err = s.Get(ctx, wsID, id)
	require.NoError(t, err)
	assert.Equal(t, "sk-secret-1", got.APIKey, "empty api key on update must preserve the stored key")
	assert.Equal(t, "gpt-4o-mini", got.Model, "model must update")

	// Rotate the key.
	_, err = s.Upsert(ctx, &ProviderConfig{
		WorkspaceID:   wsID,
		WorkspaceSlug: "acme",
		Name:          "OpenAI (prod)",
		Type:          "openai",
		APIKey:        "sk-secret-2",
	})
	require.NoError(t, err)
	got, err = s.Get(ctx, wsID, id)
	require.NoError(t, err)
	assert.Equal(t, "sk-secret-2", got.APIKey)

	// Delete.
	require.NoError(t, s.Delete(ctx, wsID, id))
	_, err = s.Get(ctx, wsID, id)
	assert.ErrorIs(t, err, ErrProviderConfigNotFound)
}

func TestProviderConfigSealedAtRest(t *testing.T) {
	s := newTestProviderStore(t, testCipher(t))
	ctx := t.Context()

	saved, err := s.Upsert(ctx, &ProviderConfig{
		WorkspaceID: "ws-1",
		Name:        "Anthropic",
		Type:        "anthropic",
		APIKey:      "sk-ant-hunter2",
	})
	require.NoError(t, err)

	// The raw column is sealed: prefixed, and the plaintext key is absent.
	var raw []byte
	require.NoError(t, s.db.QueryRowContext(ctx,
		`SELECT api_key_sealed FROM provider_configs WHERE id=$1`, saved.ID).Scan(&raw))
	assert.True(t, strings.HasPrefix(string(raw), "enc:v1:"), "api key must be sealed at rest")
	assert.NotContains(t, string(raw), "sk-ant-hunter2", "plaintext key must not be stored")

	// It still round-trips through Get.
	got, err := s.Get(ctx, "ws-1", saved.ID)
	require.NoError(t, err)
	assert.Equal(t, "sk-ant-hunter2", got.APIKey)
}

func TestProviderConfigPlaintextWithoutCipher(t *testing.T) {
	// A nil cipher (no BOWRAIN_SECRETS_KEY) stores keys as plaintext but still
	// round-trips — mirrors the connector-secret pass-through behavior.
	s := newTestProviderStore(t, nil)
	ctx := t.Context()

	saved, err := s.Upsert(ctx, &ProviderConfig{
		WorkspaceID: "ws-1",
		Name:        "Gemini",
		Type:        "gemini",
		APIKey:      "plain-key",
	})
	require.NoError(t, err)

	got, err := s.Get(ctx, "ws-1", saved.ID)
	require.NoError(t, err)
	assert.Equal(t, "plain-key", got.APIKey)
}

func TestProviderConfigCrossTenantIsolation(t *testing.T) {
	s := newTestProviderStore(t, testCipher(t))
	ctx := t.Context()

	const (
		aID, aSlug = "ws-a-id", "team-a"
		bID, bSlug = "ws-b-id", "team-b"
	)

	a, err := s.Upsert(ctx, &ProviderConfig{
		WorkspaceID: aID, WorkspaceSlug: aSlug,
		Name: "A's OpenAI", Type: "openai", APIKey: "sk-a-secret",
	})
	require.NoError(t, err)

	// Workspace B has its own, unrelated config.
	_, err = s.Upsert(ctx, &ProviderConfig{
		WorkspaceID: bID, WorkspaceSlug: bSlug,
		Name: "B's OpenAI", Type: "openai", APIKey: "sk-b-secret",
	})
	require.NoError(t, err)

	// B cannot see A's config via List.
	bList, err := s.List(ctx, bID)
	require.NoError(t, err)
	require.Len(t, bList, 1)
	assert.Equal(t, "B's OpenAI", bList[0].Name)

	// B cannot Get A's config by id.
	_, err = s.Get(ctx, bID, a.ID)
	require.ErrorIs(t, err, ErrProviderConfigNotFound)

	// B cannot Resolve A's config, even naming A's id.
	_, err = s.Resolve(ctx, bID, a.ID)
	require.ErrorIs(t, err, ErrProviderConfigNotFound)

	// B cannot Delete A's config; A's row survives.
	err = s.Delete(ctx, bID, a.ID)
	require.ErrorIs(t, err, ErrProviderConfigNotFound)
	got, err := s.Get(ctx, aID, a.ID)
	require.NoError(t, err)
	assert.Equal(t, "sk-a-secret", got.APIKey)

	// A unique (workspace_id, name) does not clash across tenants — both "OpenAI"
	// names coexist because they belong to different workspaces.
	aList, err := s.List(ctx, aID)
	require.NoError(t, err)
	assert.Len(t, aList, 1)
}

func TestProviderConfigResolveByWorkspaceID(t *testing.T) {
	s := newTestProviderStore(t, testCipher(t))
	ctx := t.Context()

	const wsID, wsSlug = "ws-durable-id", "acme"
	saved, err := s.Upsert(ctx, &ProviderConfig{
		WorkspaceID: wsID, WorkspaceSlug: wsSlug,
		Name: "OpenAI", Type: "openai", APIKey: "sk-secret",
	})
	require.NoError(t, err)

	// Resolve by durable workspace id (the worker persists it on the job).
	got, err := s.Resolve(ctx, wsID, saved.ID)
	require.NoError(t, err)
	assert.Equal(t, "sk-secret", got.APIKey)

	// A wrong id does not resolve.
	_, err = s.Resolve(ctx, "nope-id", saved.ID)
	require.ErrorIs(t, err, ErrProviderConfigNotFound)

	// An empty scope never resolves (guards against a blank/anon job leaking a config).
	_, err = s.Resolve(ctx, "", saved.ID)
	assert.ErrorIs(t, err, ErrProviderConfigNotFound)
}

// TestProviderConfigResolveIgnoresStaleSlug is the security regression for
// BLOCKER 2: the workspace slug must never authorize secret retrieval. A config
// stores a snapshot slug taken at Upsert; if that slug is later freed and
// re-acquired by a DIFFERENT workspace, that workspace supplying the victim's
// config id — even naming the shared slug — must not decrypt the victim's key.
// Resolution is by durable workspace_id only.
func TestProviderConfigResolveIgnoresStaleSlug(t *testing.T) {
	s := newTestProviderStore(t, testCipher(t))
	ctx := t.Context()

	const sharedSlug = "reused-slug"

	// Victim workspace saves a config; its row snapshots sharedSlug.
	victim, err := s.Upsert(ctx, &ProviderConfig{
		WorkspaceID: "ws-victim", WorkspaceSlug: sharedSlug,
		Name: "Victim OpenAI", Type: "openai", APIKey: "sk-victim-secret",
	})
	require.NoError(t, err)

	// Later, an attacker workspace ends up holding the same slug (slugs are
	// reusable after release). The attacker knows the victim's config id and
	// supplies its own workspace_id — which does not own the row.
	_, err = s.Resolve(ctx, "ws-attacker", victim.ID)
	require.ErrorIs(t, err, ErrProviderConfigNotFound,
		"a foreign workspace_id must not resolve the victim's config")

	// The victim can still resolve its own config by its durable id.
	got, err := s.Resolve(ctx, "ws-victim", victim.ID)
	require.NoError(t, err)
	assert.Equal(t, "sk-victim-secret", got.APIKey)
}
