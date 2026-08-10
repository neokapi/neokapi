package memory

import (
	"testing"

	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/stretchr/testify/require"
)

// TestPostgresStore_TwoLiveSchemasBothBuildTheFuzzyIndex is the regression test
// for a migration that depended on which test happened to run first.
//
// The content memory's Postgres baseline needs pg_trgm's gin_trgm_ops operator
// class. An extension belongs to a database but is INSTALLED INTO one schema,
// and `CREATE EXTENSION IF NOT EXISTS pg_trgm` with no schema puts it in the
// current one — under pgtest that is the caller's private test schema, which is
// dropped when its test ends. So the second live schema found the extension
// already present (IF NOT EXISTS, database-wide), skipped creating it, and then
// could not see gin_trgm_ops, which lives in the first schema:
//
//	ERROR: operator class "gin_trgm_ops" does not exist for access method "gin"
//
// Sequential tests never noticed — each dropped the extension with its schema
// and the next one recreated it. The full parallel suite, where several test
// schemas are alive at once, failed at NewPostgresStoreFromDB whenever it lost
// the draw. Two overlapping stores are all it takes to pin.
func TestPostgresStore_TwoLiveSchemasBothBuildTheFuzzyIndex(t *testing.T) {
	first := pgtest.NewTestDB(t)
	_, err := NewPostgresStoreFromDB(first, "ws-first")
	require.NoError(t, err)

	// The second schema migrates while the first is still alive — the shape a
	// parallel suite has, and the one a sequential suite never produces.
	second := pgtest.NewTestDB(t)
	_, err = NewPostgresStoreFromDB(second, "ws-second")
	require.NoError(t, err, "a second live schema could not build its fuzzy index")
}
