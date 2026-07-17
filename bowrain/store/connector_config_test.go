package store_test

import (
	"context"
	"database/sql"
	"encoding/base64"
	"path/filepath"
	"testing"
	"time"

	"github.com/neokapi/neokapi/bowrain/crypto"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestConnectorStore returns a ConnectorConfigStore over a freshly migrated
// SQLite database (the connector_configs table ships in the baseline SQLite
// migrations). A nil cipher exercises the plaintext pass-through path. The raw
// *sql.DB is returned too so a test can read the stored column and prove the
// secret is sealed at rest.
func newTestConnectorStore(t *testing.T, cipher *crypto.Cipher) (*bstore.ConnectorConfigStore, *sql.DB) {
	t.Helper()
	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "connectors.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	return bstore.NewConnectorConfigStore(cs.DB(), cipher), cs.DB()
}

func testCipher(t *testing.T) *crypto.Cipher {
	t.Helper()
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	c, err := crypto.NewCipher(key)
	require.NoError(t, err)
	return c
}

func TestConnectorConfigCRUDAndSealing(t *testing.T) {
	store, db := newTestConnectorStore(t, testCipher(t))
	ctx := context.Background()
	const wsID = "ws-acme"

	// Add a WordPress connector with a secret password.
	saved, err := store.Upsert(ctx, &bstore.ConnectorConfig{
		ID:          "wp-1",
		WorkspaceID: wsID,
		Type:        "wordpress",
		Name:        "Blog",
		Config: map[string]string{
			"url":      "https://blog.example.com",
			"username": "editor",
			"password": "hunter2",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "wp-1", saved.ID)
	assert.Equal(t, "wordpress", saved.Type)
	// The returned view redacts the secret but keeps the key and non-secrets.
	assert.Empty(t, saved.Config["password"], "Upsert must redact the secret in its returned view")
	assert.Equal(t, "https://blog.example.com", saved.Config["url"])
	assert.Equal(t, "editor", saved.Config["username"])

	// The secret must be sealed in the stored column, never plaintext.
	var stored string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT config FROM connector_configs WHERE workspace_id=$1 AND id=$2`, wsID, "wp-1").Scan(&stored))
	assert.NotContains(t, stored, "hunter2", "the password must not be stored in plaintext")
	assert.Contains(t, stored, "enc:v1:", "the password must be sealed with the cipher")
	assert.Contains(t, stored, "editor", "non-secret fields stay in plaintext")

	// Get decrypts the secret for re-instantiation.
	got, err := store.Get(ctx, wsID, "wp-1")
	require.NoError(t, err)
	assert.Equal(t, "hunter2", got.Config["password"])
	assert.Equal(t, "https://blog.example.com", got.Config["url"])

	// List redacts secrets.
	list, err := store.List(ctx, wsID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Empty(t, list[0].Config["password"], "List must not return the secret")
	assert.Equal(t, "editor", list[0].Config["username"])
}

func TestConnectorConfigPreserveBlankSecret(t *testing.T) {
	store, _ := newTestConnectorStore(t, testCipher(t))
	ctx := context.Background()
	const wsID = "ws-acme"

	_, err := store.Upsert(ctx, &bstore.ConnectorConfig{
		ID: "wp-1", WorkspaceID: wsID, Type: "wordpress", Name: "Blog",
		Config: map[string]string{"url": "https://blog.example.com", "username": "editor", "password": "hunter2"},
	})
	require.NoError(t, err)

	// Update with a changed URL but a BLANK password: the stored secret is kept.
	_, err = store.Upsert(ctx, &bstore.ConnectorConfig{
		ID: "wp-1", WorkspaceID: wsID, Type: "wordpress", Name: "Blog (renamed)",
		Config: map[string]string{"url": "https://new.example.com", "username": "editor", "password": ""},
	})
	require.NoError(t, err)

	got, err := store.Get(ctx, wsID, "wp-1")
	require.NoError(t, err)
	assert.Equal(t, "hunter2", got.Config["password"], "a blank secret must preserve the stored credential")
	assert.Equal(t, "https://new.example.com", got.Config["url"], "non-secret fields are updated")

	// Omitting the secret key entirely also preserves it.
	_, err = store.Upsert(ctx, &bstore.ConnectorConfig{
		ID: "wp-1", WorkspaceID: wsID, Type: "wordpress", Name: "Blog",
		Config: map[string]string{"url": "https://new.example.com", "username": "editor"},
	})
	require.NoError(t, err)
	got, err = store.Get(ctx, wsID, "wp-1")
	require.NoError(t, err)
	assert.Equal(t, "hunter2", got.Config["password"], "an omitted secret must preserve the stored credential")

	// A non-empty secret replaces the stored one.
	_, err = store.Upsert(ctx, &bstore.ConnectorConfig{
		ID: "wp-1", WorkspaceID: wsID, Type: "wordpress", Name: "Blog",
		Config: map[string]string{"url": "https://new.example.com", "username": "editor", "password": "s3cret-new"},
	})
	require.NoError(t, err)
	got, err = store.Get(ctx, wsID, "wp-1")
	require.NoError(t, err)
	assert.Equal(t, "s3cret-new", got.Config["password"], "a non-empty secret must replace the stored credential")
}

func TestConnectorConfigListAllAndDelete(t *testing.T) {
	store, _ := newTestConnectorStore(t, testCipher(t))
	ctx := context.Background()

	// Two workspaces, three connectors — the boot-time rehydration read.
	_, err := store.Upsert(ctx, &bstore.ConnectorConfig{
		ID: "wp-1", WorkspaceID: "ws-1", Type: "wordpress",
		Config: map[string]string{"url": "https://a.example.com", "password": "pw-a"},
	})
	require.NoError(t, err)
	_, err = store.Upsert(ctx, &bstore.ConnectorConfig{
		ID: "figma-1", WorkspaceID: "ws-1", Type: "figma",
		Config: map[string]string{"file_key": "abc", "token": "tok-figma"},
	})
	require.NoError(t, err)
	_, err = store.Upsert(ctx, &bstore.ConnectorConfig{
		ID: "hs-1", WorkspaceID: "ws-2", Type: "hubspot",
		Config: map[string]string{"api_key": "key-hs"},
	})
	require.NoError(t, err)

	all, err := store.ListAll(ctx)
	require.NoError(t, err)
	require.Len(t, all, 3, "ListAll must span every workspace")
	// ListAll decrypts secrets (rehydration needs the real credential).
	byID := map[string]bstore.ConnectorConfig{}
	for _, c := range all {
		byID[c.ID] = c
	}
	assert.Equal(t, "pw-a", byID["wp-1"].Config["password"])
	assert.Equal(t, "tok-figma", byID["figma-1"].Config["token"])
	assert.Equal(t, "key-hs", byID["hs-1"].Config["api_key"])

	// Delete is workspace-scoped: wrong workspace is a not-found no-op.
	err = store.Delete(ctx, "ws-2", "wp-1")
	require.ErrorIs(t, err, bstore.ErrConnectorConfigNotFound, "cross-workspace delete must not match")
	_, err = store.Get(ctx, "ws-1", "wp-1")
	require.NoError(t, err, "the connector must survive a cross-workspace delete attempt")

	// Delete in the owning workspace removes the row.
	require.NoError(t, store.Delete(ctx, "ws-1", "wp-1"))
	_, err = store.Get(ctx, "ws-1", "wp-1")
	assert.ErrorIs(t, err, bstore.ErrConnectorConfigNotFound)
}

func TestConnectorConfigTouchLastSync(t *testing.T) {
	store, _ := newTestConnectorStore(t, nil)
	ctx := context.Background()
	const wsID = "ws-acme"

	_, err := store.Upsert(ctx, &bstore.ConnectorConfig{
		ID: "wp-1", WorkspaceID: wsID, Type: "wordpress", Name: "Blog",
		Config: map[string]string{"url": "https://blog.example.com"},
	})
	require.NoError(t, err)

	// Never synced → zero last-sync (the status path renders this as "never synced").
	got, err := store.Get(ctx, wsID, "wp-1")
	require.NoError(t, err)
	assert.True(t, got.LastSyncAt.IsZero(), "a fresh connector has no last-sync")

	// Stamp a sync; the stored value round-trips to the second (RFC3339).
	ts := time.Date(2026, 7, 18, 9, 30, 0, 0, time.UTC)
	require.NoError(t, store.TouchLastSync(ctx, wsID, "wp-1", ts))
	got, err = store.Get(ctx, wsID, "wp-1")
	require.NoError(t, err)
	assert.True(t, ts.Equal(got.LastSyncAt), "last-sync must persist the stamped time, got %s", got.LastSyncAt)

	// Cross-workspace touch matches nothing and must not stamp the row.
	require.NoError(t, store.TouchLastSync(ctx, "ws-other", "wp-1", ts.Add(time.Hour)))
	got, err = store.Get(ctx, wsID, "wp-1")
	require.NoError(t, err)
	assert.True(t, ts.Equal(got.LastSyncAt), "a cross-workspace touch must not change the row")
}

func TestConnectorConfigCrossWorkspaceIsolation(t *testing.T) {
	store, _ := newTestConnectorStore(t, testCipher(t))
	ctx := context.Background()

	_, err := store.Upsert(ctx, &bstore.ConnectorConfig{
		ID: "wp-1", WorkspaceID: "ws-1", Type: "wordpress",
		Config: map[string]string{"url": "https://a.example.com", "password": "pw-a"},
	})
	require.NoError(t, err)

	// A different workspace cannot read it (indistinguishable from missing).
	_, err = store.Get(ctx, "ws-2", "wp-1")
	require.ErrorIs(t, err, bstore.ErrConnectorConfigNotFound)

	// A different workspace's List does not reveal it.
	list, err := store.List(ctx, "ws-2")
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestConnectorConfigNilCipherRedactsButPassesThrough(t *testing.T) {
	// A nil cipher stores secrets in plaintext, but redaction is independent of
	// encryption: List/Upsert views must still blank the secret.
	store, db := newTestConnectorStore(t, nil)
	ctx := context.Background()
	const wsID = "ws-acme"

	saved, err := store.Upsert(ctx, &bstore.ConnectorConfig{
		ID: "hs-1", WorkspaceID: wsID, Type: "hubspot", Name: "Marketing",
		Config: map[string]string{"api_key": "key-plain"},
	})
	require.NoError(t, err)
	assert.Empty(t, saved.Config["api_key"], "the secret is redacted even with no cipher")

	// Stored plaintext (no cipher), but Get still returns it for rehydration.
	var stored string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT config FROM connector_configs WHERE workspace_id=$1 AND id=$2`, wsID, "hs-1").Scan(&stored))
	assert.Contains(t, stored, "key-plain", "with no cipher the secret is stored as plaintext")

	got, err := store.Get(ctx, wsID, "hs-1")
	require.NoError(t, err)
	assert.Equal(t, "key-plain", got.Config["api_key"])
}
