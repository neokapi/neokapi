package voice

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/storage"
)

// The profile update bounds its wait for the profile's row lock, and the
// bound ends with that statement: after a write that lands, a write refused
// as stale and a write that timed out, the transaction's lock_timeout is
// what it was before, and the transaction can go on.
func TestUpdateProfileAt_LockBoundEndsWithTheLockStatement(t *testing.T) {
	store := newVoiceStore(t)
	ctx := t.Context()

	p := newTestProfile("ws-bound", "Bound")
	require.NoError(t, store.CreateProfile(ctx, p))
	revs, err := store.Revisions(ctx, []string{p.ID})
	require.NoError(t, err)
	base := revs[p.ID]

	tx, err := store.db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `SET LOCAL lock_timeout = '17s'`)
	require.NoError(t, err)
	bound := store.Bind(tx)
	timeout := func() string {
		t.Helper()
		var v string
		require.NoError(t, tx.QueryRowContext(ctx, `SHOW lock_timeout`).Scan(&v))
		return v
	}

	edited := *p
	edited.Description = "landed"
	require.NoError(t, bound.UpdateProfileAt(ctx, &edited, base))
	assert.Equal(t, "17s", timeout(), "after a write that lands")

	stale := *p
	stale.Description = "stale"
	require.ErrorIs(t, bound.UpdateProfileAt(ctx, &stale, base), storage.ErrStale)
	assert.Equal(t, "17s", timeout(), "after a write refused as stale")

	require.NoError(t, tx.Commit())

	// Another transaction holds the profile; the bounded wait runs out, and
	// the transaction that waited is still usable with its own setting.
	holder, err := store.db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = holder.Rollback() }()
	_, err = holder.ExecContext(ctx, `SELECT 1 FROM voice_profiles WHERE id = $1 FOR UPDATE`, p.ID)
	require.NoError(t, err)

	revs, err = store.Revisions(ctx, []string{p.ID})
	require.NoError(t, err)
	tx, err = store.db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `SET LOCAL lock_timeout = '17s'`)
	require.NoError(t, err)
	waited := *p
	waited.Description = "waited"
	start := time.Now()
	require.ErrorIs(t, store.Bind(tx).UpdateProfileAt(ctx, &waited, revs[p.ID]), storage.ErrBusy)
	assert.Less(t, time.Since(start), 10*time.Second, "the wait is bounded")
	var after string
	require.NoError(t, tx.QueryRowContext(ctx, `SHOW lock_timeout`).Scan(&after),
		"the transaction is usable after a timed-out wait")
	assert.Equal(t, "17s", after, "after a write that timed out")
}
