package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/terms"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/bowrain/voice"
	corestorage "github.com/neokapi/neokapi/core/storage"
)

// A ledger migrated only on first workspace access is still compared at
// start: a terms ledger the code disagrees with fails the start, naming both
// sides, rather than passing the health check and failing requests later.
// (The test database arrives with every schema applied.)
func TestCheckLaterLedgers_RefusesAtStart(t *testing.T) {
	db := pgtest.NewTestDB(t)
	ctx := t.Context()
	require.NoError(t, checkLaterLedgers(db), "a terms ledger that agrees with the code starts")

	first := terms.Migrations[0]
	_, err := db.ExecContext(ctx, `UPDATE tb_schema_migrations SET description = 'a migration this version never was', identity = '' WHERE version = $1`, first.Version)
	require.NoError(t, err)

	err = checkLaterLedgers(db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "check the terms ledger")
	assert.Contains(t, err.Error(), `is recorded as "a migration this version never was"`)
	assert.Contains(t, err.Error(), first.Description)
}

// A database a newer release migrated fails the start whichever ledger moved
// ahead, including the voice ledger, whose store the server would otherwise
// run without, so this build never serves a schema it does not know.
func TestCheckLaterLedgers_RefusesADatabaseANewerReleaseMigrated(t *testing.T) {
	db := pgtest.NewTestDB(t)
	ctx := t.Context()
	known := 0
	for _, m := range voice.Migrations {
		known = max(known, m.Version)
	}
	_, err := db.ExecContext(ctx, `INSERT INTO voice_schema_migrations (version, description, identity) VALUES ($1, 'from a newer server', 'newer')`, known+1)
	require.NoError(t, err)

	err = checkLaterLedgers(db)
	require.ErrorIs(t, err, corestorage.ErrNewerDatabase)
	assert.Contains(t, err.Error(), "check the voice ledger")
	assert.Contains(t, err.Error(), "deploy bowrain-server and bowrain-worker at a version at least as new as the one that migrated it. An older version cannot be started against this database")
}
