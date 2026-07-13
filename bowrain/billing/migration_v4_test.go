package billing

import (
	"testing"
	"time"

	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Migration v4 has to run against a table that already holds trials — production
// has been handing out trials with no deadline since the first workspace was
// created. Those rows have no trial_ends_at, and a sweep keyed on
// `trial_ends_at <= now()` would skip them forever, leaving the exact population
// this epic exists to fix permanently on Pro.
//
// This applies the migrations in two stages against a real Postgres — v1..v3,
// then insert legacy trials, then v4 — which is the sequence a live database
// actually goes through.
func TestMigrationV4_BackfillsExistingTrials(t *testing.T) {
	db := pgtest.NewTestDB(t)
	ctx := t.Context()

	// ExpireTrials (called at the end) keeps the auth-owned workspaces.plan cache
	// in step; create the minimal stand-in the real deployment always has.
	_, err := db.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS workspaces (id TEXT PRIMARY KEY, plan TEXT NOT NULL DEFAULT 'free')`)
	require.NoError(t, err)

	// Stage 1: the schema as it exists in production today.
	require.NoError(t, storage.MigratePostgresNS(db, "billing_schema_migrations", billingMigrations[:3]))

	// A trial created 20 days ago (already past a 14-day window) and one created
	// yesterday, both without a deadline — because the column did not exist.
	// These mirror what UpsertSubscription writes for real trial rows: empty (not
	// NULL) stripe ids, and zero-time period columns (Go's zero time.Time
	// serializes to '0001-01-01', never NULL) — so this exercises the migration
	// against production-shaped data, not a hand-idealized row.
	_, err = db.ExecContext(ctx,
		`INSERT INTO subscriptions
		   (id, workspace_id, stripe_customer_id, stripe_subscription_id, plan, status, seat_count,
		    current_period_start, current_period_end, created_at)
		 VALUES
		   ('s1', 'ws-old', '', '', 'pro', 'trialing', 1, '0001-01-01', '0001-01-01', NOW() - INTERVAL '20 days'),
		   ('s2', 'ws-new', '', '', 'pro', 'trialing', 1, '0001-01-01', '0001-01-01', NOW() - INTERVAL '1 day'),
		   ('s3', 'ws-paid', 'cus_1', 'sub_1', 'team', 'active', 5, '0001-01-01', '0001-01-01', NOW() - INTERVAL '60 days')`)
	require.NoError(t, err)

	// Stage 2: this epic's migration.
	require.NoError(t, storage.MigratePostgresNS(db, "billing_schema_migrations", billingMigrations))

	store := &PgBillingStore{db: db}

	// The old trial is dated from its creation, so it is already due and the very
	// first sweep collects it.
	old, err := store.GetSubscription(ctx, "ws-old")
	require.NoError(t, err)
	require.NotNil(t, old.TrialEndsAt, "a legacy trial must be given a deadline, or it never ends")
	assert.True(t, old.TrialEndsAt.Before(time.Now().UTC()), "a trial started 20 days ago is already over")

	// The recent one keeps the remainder of its 14 days rather than being cut short.
	fresh, err := store.GetSubscription(ctx, "ws-new")
	require.NoError(t, err)
	require.NotNil(t, fresh.TrialEndsAt)
	assert.True(t, fresh.TrialEndsAt.After(time.Now().UTC()), "a trial started yesterday still has days left")

	// A paying subscription is untouched: it is not trialing, so it gets no deadline.
	paid, err := store.GetSubscription(ctx, "ws-paid")
	require.NoError(t, err)
	assert.Nil(t, paid.TrialEndsAt)

	expired, err := store.ExpireTrials(ctx, time.Now().UTC(), 100)
	require.NoError(t, err)
	require.Len(t, expired, 1)
	assert.Equal(t, "ws-old", expired[0].WorkspaceID)
}
