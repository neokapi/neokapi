package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var deadlineMigrations = []storage.Migration{
	{Version: 1, Description: "a table", SQL: `CREATE TABLE IF NOT EXISTS migrate_deadline_t (id INTEGER PRIMARY KEY)`},
}

// A migration pass that cannot take the advisory lock fails inside its
// deadline, names the lock it wanted, and says which session holds it. Before
// the pass had a deadline, the same situation hung until the test binary's
// 15-minute alarm killed the whole package, blaming whichever test happened
// to be running.
func TestMigratePostgresNSFailsWithinDeadlineWhenLockIsHeld(t *testing.T) {
	db := pgtest.NewTestDB(t)

	// The holder is a session of its own, on a pool of its own, so the
	// migration's pinned connection and the summary's pool connection do not
	// compete with it. Advisory locks are database-wide, which is what makes a
	// lock taken here visible to a pass on db.
	holderDB, err := storage.OpenPostgres(db.ConnStr())
	require.NoError(t, err)
	t.Cleanup(func() { _ = holderDB.Close() })
	holder, err := holderDB.Conn(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = holder.Close() })

	_, err = holder.ExecContext(t.Context(), "SELECT pg_advisory_lock($1)", storage.MigrationLockKey)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	start := time.Now()
	err = storage.MigratePostgresNSContext(ctx, db, "migrate_deadline_schema_migrations", deadlineMigrations)
	elapsed := time.Since(start)

	require.Error(t, err)
	t.Logf("the pass reported: %v", err)
	assert.Less(t, elapsed, 30*time.Second, "the pass must give up at its deadline, not at the package's")
	assert.GreaterOrEqual(t, elapsed, 2*time.Second, "the pass must wait for the lock until its deadline")
	assert.Contains(t, err.Error(), "acquire migration advisory lock", "the error names the statement that timed out")
	assert.Contains(t, err.Error(), "holds the migration lock", "the error says which session held the lock")
	assert.Contains(t, err.Error(), "pg_advisory_lock", "the holder's last statement is in the summary")

	// Releasing the lock lets the same pass through, and the pass releases
	// the lock behind itself: a second pass on the same database runs.
	var released bool
	require.NoError(t, holder.QueryRowContext(t.Context(), "SELECT pg_advisory_unlock($1)", storage.MigrationLockKey).Scan(&released))
	require.True(t, released)

	require.NoError(t, storage.MigratePostgresNS(db, "migrate_deadline_schema_migrations", deadlineMigrations))
	require.NoError(t, storage.MigratePostgresNS(db, "migrate_deadline_schema_migrations", deadlineMigrations))

	var version int
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT MAX(version) FROM migrate_deadline_schema_migrations").Scan(&version))
	assert.Equal(t, 1, version)
}

// A pass that has already run out of time does not open a connection or take
// the lock.
func TestMigratePostgresNSRefusesAnExpiredContext(t *testing.T) {
	db := pgtest.NewTestDB(t)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := storage.MigratePostgresNSContext(ctx, db, "migrate_expired_schema_migrations", deadlineMigrations)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// The session timeouts the pass sets on its pinned connection do not follow
// that connection back into the pool. With a pool of one, the connection the
// pass used is the only one a later caller can get.
func TestMigratePostgresNSResetsSessionTimeouts(t *testing.T) {
	db := pgtest.NewTestDBWithMaxConns(t, 1)

	require.NoError(t, storage.MigratePostgresNS(db, "migrate_reset_schema_migrations", deadlineMigrations))

	for _, setting := range []string{"lock_timeout", "statement_timeout"} {
		var value string
		require.NoError(t, db.QueryRowContext(t.Context(), "SHOW "+setting).Scan(&value))
		assert.Equal(t, "0", value, "%s must be back at its default after the pass", setting)
	}
}
