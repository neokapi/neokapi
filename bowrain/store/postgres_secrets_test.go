package store

import (
	"encoding/base64"
	"strings"
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testCipher(t *testing.T) *crypto.Cipher {
	t.Helper()
	c, err := crypto.NewCipher(base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	require.NoError(t, err)
	return c
}

// TestConnectorConfigEncryptedAtRest verifies connector credentials are sealed
// in the connector_config column when a secrets cipher is configured, and still
// round-trip through the store API.
func TestConnectorConfigEncryptedAtRest(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	s.SetSecretsCipher(testCipher(t))

	coll := &platstore.Collection{
		ProjectID: p.ID,
		Name:      "WordPress",
		ConnectorConfig: map[string]string{
			"base_url": "https://wp.example.com",
			"password": "hunter2-secret",
		},
	}
	require.NoError(t, s.CreateCollection(ctx, coll))

	// Round-trips through the store API.
	got, err := s.GetCollection(ctx, p.ID, coll.ID)
	require.NoError(t, err)
	assert.Equal(t, "hunter2-secret", got.ConnectorConfig["password"])
	assert.Equal(t, "https://wp.example.com", got.ConnectorConfig["base_url"])

	// The raw column is sealed: prefixed, and the plaintext secret is absent.
	var raw string
	require.NoError(t, s.SQLDB().QueryRowContext(ctx,
		`SELECT connector_config FROM collections WHERE id=$1`, coll.ID).Scan(&raw))
	assert.True(t, strings.HasPrefix(raw, "enc:v1:"), "connector_config must be sealed at rest")
	assert.NotContains(t, raw, "hunter2-secret", "plaintext secret must not be stored")

	// Update re-seals and still round-trips (secret rotation).
	got.ConnectorConfig["password"] = "rotated-secret"
	require.NoError(t, s.UpdateCollection(ctx, got))
	got2, err := s.GetCollection(ctx, p.ID, coll.ID)
	require.NoError(t, err)
	assert.Equal(t, "rotated-secret", got2.ConnectorConfig["password"])
}

// TestConnectorConfigLegacyPlaintextReadable verifies a pre-encryption
// (plaintext) row is read transparently once a cipher is configured, and is
// re-sealed on its next write (lazy migration, no migration step).
func TestConnectorConfigLegacyPlaintextReadable(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	// Write WITHOUT a cipher => stored as plaintext JSON (a legacy row).
	coll := &platstore.Collection{
		ProjectID:       p.ID,
		Name:            "Legacy",
		ConnectorConfig: map[string]string{"token": "legacy-plain"},
	}
	require.NoError(t, s.CreateCollection(ctx, coll))

	// Enable encryption and read: the legacy plaintext must still open.
	s.SetSecretsCipher(testCipher(t))
	got, err := s.GetCollection(ctx, p.ID, coll.ID)
	require.NoError(t, err)
	assert.Equal(t, "legacy-plain", got.ConnectorConfig["token"])

	// Re-saving now seals it.
	require.NoError(t, s.UpdateCollection(ctx, got))
	var raw string
	require.NoError(t, s.SQLDB().QueryRowContext(ctx,
		`SELECT connector_config FROM collections WHERE id=$1`, coll.ID).Scan(&raw))
	assert.True(t, strings.HasPrefix(raw, "enc:v1:"), "re-saved config should be sealed")
}
