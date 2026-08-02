package billing

import (
	"testing"
	"time"

	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The trial sweep must collect a trial whose deadline has passed and leave
// everything else alone.
//
// This test used to drive the schema in two stages — v1..v3, insert deadline-less
// trials, then v4 — to prove v4's backfill dated the legacy rows production had
// been accumulating since the first workspace. The billing migrations are now a
// single consolidated baseline and that backfill is deliberately not carried
// (see the ledger on Migrations): it repaired rows written before trial_ends_at
// existed, production has long since been repaired by it, and a database built
// from the baseline has no such rows. There is no longer a schema state in which
// a trial can exist without a deadline, so the staged sequence cannot be built.
//
// What remains worth guarding is the behaviour the backfill existed to enable,
// so that is what this now tests directly.
func TestExpireTrials_CollectsOnlyOverdueTrials(t *testing.T) {
	db := pgtest.NewTestDB(t)
	ctx := t.Context()

	// ExpireTrials keeps the auth-owned workspaces.plan cache in step; create the
	// minimal stand-in the real deployment always has.
	_, err := db.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS workspaces (id TEXT PRIMARY KEY, plan TEXT NOT NULL DEFAULT 'free')`)
	require.NoError(t, err)

	require.NoError(t, storage.MigratePostgresNS(db, "billing_schema_migrations", Migrations))

	// Production-shaped rows rather than hand-idealized ones: trial subscriptions
	// carry empty (not NULL) stripe ids, and zero-time period columns — Go's zero
	// time.Time serializes to '0001-01-01', never NULL.
	_, err = db.ExecContext(ctx,
		`INSERT INTO subscriptions
		   (id, workspace_id, stripe_customer_id, stripe_subscription_id, plan, status, seat_count,
		    current_period_start, current_period_end, trial_ends_at, created_at)
		 VALUES
		   ('s1', 'ws-old',  '',      '',      'pro',  'trialing', 1, '0001-01-01', '0001-01-01', NOW() - INTERVAL '6 days',  NOW() - INTERVAL '20 days'),
		   ('s2', 'ws-new',  '',      '',      'pro',  'trialing', 1, '0001-01-01', '0001-01-01', NOW() + INTERVAL '13 days', NOW() - INTERVAL '1 day'),
		   ('s3', 'ws-paid', 'cus_1', 'sub_1', 'team', 'active',   5, '0001-01-01', '0001-01-01', NULL,                       NOW() - INTERVAL '60 days')`)
	require.NoError(t, err)

	store := &PgBillingStore{db: db}

	// A trial past its deadline is already due, so the very first sweep takes it.
	old, err := store.GetSubscription(ctx, "ws-old")
	require.NoError(t, err)
	require.NotNil(t, old.TrialEndsAt, "a trial must carry a deadline, or it never ends")
	assert.True(t, old.TrialEndsAt.Before(time.Now().UTC()))

	// One with time left keeps its remaining days rather than being cut short.
	fresh, err := store.GetSubscription(ctx, "ws-new")
	require.NoError(t, err)
	require.NotNil(t, fresh.TrialEndsAt)
	assert.True(t, fresh.TrialEndsAt.After(time.Now().UTC()))

	// A paying subscription is untouched: it is not trialing, so it has no deadline.
	paid, err := store.GetSubscription(ctx, "ws-paid")
	require.NoError(t, err)
	assert.Nil(t, paid.TrialEndsAt)

	expired, err := store.ExpireTrials(ctx, time.Now().UTC(), 100)
	require.NoError(t, err)
	require.Len(t, expired, 1, "only the overdue trial is collected")
	assert.Equal(t, "ws-old", expired[0].WorkspaceID)
}
