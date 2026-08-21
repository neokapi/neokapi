//go:build integration

package migrations_test

import (
	"testing"

	"github.com/neokapi/neokapi/bowrain/migrations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The drift types were renamed identifier AND stored value, so rows written
// before the rename keep the former spelling. Nothing errors and nothing today
// reads the value exactly, which is precisely why this needs asserting: the
// failure it prevents is silent, in a consumer that does not exist yet.
func TestDriftRowsCarryTheVoiceSpelling(t *testing.T) {
	db := freshDatabase(t)
	ctx := t.Context()

	require.NoError(t, migrations.Apply(db, nil))

	// Rows as they were written before the rename, inserted under the ledger's
	// back so the backfill has something to find on its next pass.
	_, err := db.ExecContext(ctx, `
		INSERT INTO activities (id, event_id, workspace_id, actor_id, actor_name, type, created_at)
		VALUES ('a1', 'e1', 'ws1', 'u1', 'Ada', 'brand.drift', NOW());
		INSERT INTO notifications (id, user_id, type, title, created_at)
		VALUES ('n1', 'u1', 'brand.drift', 'Voice drift', NOW());`)
	require.NoError(t, err)

	// Re-applying the baseline set is how a deploy reaches an existing
	// database; the backfill must be part of what that does.
	// Rewind the ledger to before the backfill so the next Apply replays it.
	// Every version at or above it is dropped rather than just the backfill's
	// own: the loop skips anything at or below the current version, so removing
	// one row in the middle would leave the backfill unreachable the moment a
	// later migration exists. The replayed migrations are idempotent by design.
	_, err = db.ExecContext(ctx, `DELETE FROM store_schema_migrations WHERE version >= 28`)
	require.NoError(t, err)
	require.NoError(t, migrations.Apply(db, nil))

	var activityType, notificationType string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT type FROM activities WHERE id = 'a1'`).Scan(&activityType))
	require.NoError(t, db.QueryRowContext(ctx, `SELECT type FROM notifications WHERE id = 'n1'`).Scan(&notificationType))
	assert.Equal(t, "voice.drift", activityType)
	assert.Equal(t, "voice.drift", notificationType)

	// Idempotent: a second pass has nothing left to find.
	require.NoError(t, migrations.Apply(db, nil))
}
