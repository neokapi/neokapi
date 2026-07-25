package billing

import (
	"context"
	"testing"
	"time"

	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTrialTestStore(t *testing.T) *PgBillingStore {
	t.Helper()
	db := pgtest.NewTestDB(t)
	// ExpireTrials keeps workspaces.plan in step with the downgrade in the same
	// statement (see its doc). That table is owned by the auth package and exists
	// alongside the billing tables in every real deployment; create a minimal
	// stand-in so these unit tests exercise the real cross-table SQL.
	_, err := db.ExecContext(context.Background(),
		`CREATE TABLE IF NOT EXISTS workspaces (id TEXT PRIMARY KEY, plan TEXT NOT NULL DEFAULT 'free')`)
	require.NoError(t, err)
	store, err := NewPgBillingStore(db)
	require.NoError(t, err)
	return store
}

// seedWorkspace inserts a workspace cache row so ExpireTrials's plan-cache sync
// has something to update.
func seedWorkspace(t *testing.T, store *PgBillingStore, id, plan string) {
	t.Helper()
	_, err := store.db.ExecContext(context.Background(),
		`INSERT INTO workspaces (id, plan) VALUES ($1, $2)
		 ON CONFLICT (id) DO UPDATE SET plan = EXCLUDED.plan`, id, plan)
	require.NoError(t, err)
}

// workspacePlan reads back the cached plan for assertions.
func workspacePlan(t *testing.T, store *PgBillingStore, id string) string {
	t.Helper()
	var plan string
	require.NoError(t, store.db.QueryRowContext(context.Background(),
		`SELECT plan FROM workspaces WHERE id = $1`, id).Scan(&plan))
	return plan
}

// SetupTrial must give the trial a deadline. Without one there is nothing for the
// sweeper to act on and the trial is effectively permanent — which is what every
// signup got before epic 005.
func TestSetupTrial_WritesDeadline(t *testing.T) {
	store := newTrialTestStore(t)
	ctx := t.Context()

	SetupTrial(ctx, store, "ws-1")

	sub, err := store.GetSubscription(ctx, "ws-1")
	require.NoError(t, err)
	assert.Equal(t, PlanPro, sub.Plan)
	assert.Equal(t, "trialing", sub.Status)
	require.NotNil(t, sub.TrialEndsAt, "a trial without an end date never ends")
	assert.WithinDuration(t, time.Now().UTC().AddDate(0, 0, DefaultTrialDays), *sub.TrialEndsAt, time.Minute)
}

// Two workspaces must both get a trial. The schema once declared
// stripe_customer_id as NOT NULL UNIQUE while SetupTrial inserted ”, so the
// second workspace's insert collided — and SetupTrial only logs, so the second
// workspace silently got no subscription at all. Migration v2 fixed the schema
// (partial unique index); this pins it, because the failure is invisible.
func TestSetupTrial_TwoWorkspacesBothGetTrials(t *testing.T) {
	store := newTrialTestStore(t)
	ctx := t.Context()

	SetupTrial(ctx, store, "ws-1")
	SetupTrial(ctx, store, "ws-2")

	for _, ws := range []string{"ws-1", "ws-2"} {
		sub, err := store.GetSubscription(ctx, ws)
		require.NoError(t, err, "workspace %s got no subscription row", ws)
		assert.Equal(t, "trialing", sub.Status)
		assert.Empty(t, sub.StripeCustomerID, "a local trial has no Stripe customer")
	}
}

// The sweep downgrades trials that are due and leaves live ones alone.
func TestExpireTrials_OnlyDueTrials(t *testing.T) {
	store := newTrialTestStore(t)
	ctx := t.Context()

	past := time.Now().UTC().Add(-time.Hour)
	future := time.Now().UTC().Add(72 * time.Hour)
	require.NoError(t, store.UpsertSubscription(ctx, &Subscription{
		WorkspaceID: "ws-expired", Plan: PlanPro, Status: "trialing", SeatCount: 1, TrialEndsAt: &past,
	}))
	require.NoError(t, store.UpsertSubscription(ctx, &Subscription{
		WorkspaceID: "ws-live", Plan: PlanPro, Status: "trialing", SeatCount: 1, TrialEndsAt: &future,
	}))
	// A paying Pro customer is not a trial and must never be swept, even though
	// its plan matches.
	require.NoError(t, store.UpsertSubscription(ctx, &Subscription{
		WorkspaceID: "ws-paid", Plan: PlanPro, Status: "active", SeatCount: 1,
		StripeCustomerID: "cus_paid", StripeSubscriptionID: "sub_paid",
	}))

	expired, err := store.ExpireTrials(ctx, time.Now().UTC(), 100)
	require.NoError(t, err)
	require.Len(t, expired, 1)
	assert.Equal(t, "ws-expired", expired[0].WorkspaceID)

	got, err := store.GetSubscription(ctx, "ws-expired")
	require.NoError(t, err)
	assert.Equal(t, PlanFree, got.Plan)
	assert.Equal(t, "active", got.Status)
	assert.Nil(t, got.TrialEndsAt)

	live, err := store.GetSubscription(ctx, "ws-live")
	require.NoError(t, err)
	assert.Equal(t, PlanPro, live.Plan)
	assert.Equal(t, "trialing", live.Status)

	paid, err := store.GetSubscription(ctx, "ws-paid")
	require.NoError(t, err)
	assert.Equal(t, PlanPro, paid.Plan)
	assert.Equal(t, "active", paid.Status)
}

// A second sweep finds nothing: the downgrade is a state transition, so it must
// not re-fire (which would re-record trial_expired events forever).
func TestExpireTrials_IsIdempotent(t *testing.T) {
	store := newTrialTestStore(t)
	ctx := t.Context()

	past := time.Now().UTC().Add(-time.Hour)
	require.NoError(t, store.UpsertSubscription(ctx, &Subscription{
		WorkspaceID: "ws-1", Plan: PlanPro, Status: "trialing", SeatCount: 1, TrialEndsAt: &past,
	}))

	first, err := store.ExpireTrials(ctx, time.Now().UTC(), 100)
	require.NoError(t, err)
	require.Len(t, first, 1)

	second, err := store.ExpireTrials(ctx, time.Now().UTC(), 100)
	require.NoError(t, err)
	assert.Empty(t, second, "an already-downgraded trial must not be swept twice")
}

// A trial that converts to a paid subscription is no longer a trial: the checkout
// webhook's write clears the deadline, so the sweeper cannot later downgrade a
// paying customer.
func TestExpireTrials_ConvertedTrialIsNotSwept(t *testing.T) {
	store := newTrialTestStore(t)
	ctx := t.Context()

	SetupTrial(ctx, store, "ws-1")

	// What handleCheckoutCompleted writes on conversion.
	require.NoError(t, store.UpsertSubscription(ctx, &Subscription{
		WorkspaceID: "ws-1", Plan: PlanTeam, Status: "active", SeatCount: 3,
		StripeCustomerID: "cus_1", StripeSubscriptionID: "sub_1",
	}))

	sub, err := store.GetSubscription(ctx, "ws-1")
	require.NoError(t, err)
	require.Nil(t, sub.TrialEndsAt, "conversion must clear the trial deadline")

	// Even sweeping far in the future finds nothing.
	expired, err := store.ExpireTrials(ctx, time.Now().UTC().AddDate(1, 0, 0), 100)
	require.NoError(t, err)
	assert.Empty(t, expired)
}

// The sweep downgrades the workspace's cached plan (so authorization and the
// monthly-credit grant stop treating it as Pro) and records the audit event.
func TestTrialSweeper_SyncsPlanAndRecordsEvent(t *testing.T) {
	store := newTrialTestStore(t)
	ctx := t.Context()

	seedWorkspace(t, store, "ws-1", string(PlanPro))
	past := time.Now().UTC().Add(-time.Hour)
	require.NoError(t, store.UpsertSubscription(ctx, &Subscription{
		WorkspaceID: "ws-1", Plan: PlanPro, Status: "trialing", SeatCount: 1, TrialEndsAt: &past,
	}))

	NewTrialSweeper(store, time.Hour).sweep(ctx)

	// The plan cache — what the hot path actually reads — must be Free now, or
	// the workspace keeps Pro limits (and, once no longer trialing, would draw
	// the Pro monthly allowance) for nothing.
	assert.Equal(t, string(PlanFree), workspacePlan(t, store, "ws-1"))

	events, err := store.ListBillingEvents(ctx, 10, 0, "trial_expired")
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "ws-1", events[0].WorkspaceID)
}

// The cache downgrade is atomic with the subscription downgrade: ExpireTrials
// updates both in one statement, so a workspace can never be left on the Pro
// plan cache (and Pro limits) after its trial has ended.
func TestExpireTrials_UpdatesPlanCacheAtomically(t *testing.T) {
	store := newTrialTestStore(t)
	ctx := t.Context()

	seedWorkspace(t, store, "ws-1", string(PlanPro))
	past := time.Now().UTC().Add(-time.Hour)
	require.NoError(t, store.UpsertSubscription(ctx, &Subscription{
		WorkspaceID: "ws-1", Plan: PlanPro, Status: "trialing", SeatCount: 1, TrialEndsAt: &past,
	}))

	expired, err := store.ExpireTrials(ctx, time.Now().UTC(), 100)
	require.NoError(t, err)
	require.Len(t, expired, 1)

	sub, err := store.GetSubscription(ctx, "ws-1")
	require.NoError(t, err)
	assert.Equal(t, PlanFree, sub.Plan)
	assert.Equal(t, string(PlanFree), workspacePlan(t, store, "ws-1"), "the plan cache must fall to Free with the subscription")
}

// Run sweeps once at startup rather than waiting a full interval, so a worker
// that was down across a trial boundary catches up on boot.
func TestTrialSweeper_SweepsOnStartup(t *testing.T) {
	store := newTrialTestStore(t)
	ctx := t.Context()

	past := time.Now().UTC().Add(-time.Hour)
	require.NoError(t, store.UpsertSubscription(ctx, &Subscription{
		WorkspaceID: "ws-1", Plan: PlanPro, Status: "trialing", SeatCount: 1, TrialEndsAt: &past,
	}))

	// An interval far longer than the test: only the startup sweep can fire.
	runCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	err := NewTrialSweeper(store, time.Hour).Run(runCtx)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	sub, err := store.GetSubscription(ctx, "ws-1")
	require.NoError(t, err)
	assert.Equal(t, PlanFree, sub.Plan)
}
