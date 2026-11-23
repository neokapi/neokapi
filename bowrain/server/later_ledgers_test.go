package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/terms"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
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
