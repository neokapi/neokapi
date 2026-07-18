package billing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/core/id"
)

// Migration ledger — versions 1..6 are live in production; the list is
// append-only. NEVER reuse a version number, even for a migration that was
// later found unnecessary: deployed databases record applied versions and
// silently skip a reused number (see the repo-wide migration-squash
// postmortem). No versions have been retired from this list.
var billingMigrations = []storage.Migration{
	{
		Version:     1,
		Description: "create billing tables",
		SQL: `
			CREATE TABLE subscriptions (
				id                     TEXT PRIMARY KEY,
				workspace_id           TEXT NOT NULL UNIQUE,
				stripe_customer_id     TEXT NOT NULL UNIQUE,
				stripe_subscription_id TEXT,
				plan                   TEXT NOT NULL DEFAULT 'free',
				status                 TEXT NOT NULL DEFAULT 'active',
				seat_count             INTEGER NOT NULL DEFAULT 1,
				current_period_start   TIMESTAMPTZ,
				current_period_end     TIMESTAMPTZ,
				cancel_at              TIMESTAMPTZ,
				created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);

			CREATE TABLE credit_allocations (
				id             TEXT PRIMARY KEY,
				workspace_id   TEXT NOT NULL,
				credits_total  BIGINT NOT NULL,
				credits_used   BIGINT NOT NULL DEFAULT 0,
				week_start     TIMESTAMPTZ NOT NULL,
				week_end       TIMESTAMPTZ NOT NULL,
				source         TEXT NOT NULL DEFAULT 'plan',
				created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				UNIQUE(workspace_id, week_start, source)
			);

			CREATE TABLE credit_ledger (
				id             BIGSERIAL PRIMARY KEY,
				workspace_id   TEXT NOT NULL,
				allocation_id  TEXT,
				amount         BIGINT NOT NULL,
				balance_after  BIGINT NOT NULL,
				operation      TEXT NOT NULL,
				reference_id   TEXT,
				created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX idx_credit_ledger_workspace ON credit_ledger(workspace_id, created_at);

			CREATE TABLE feature_overrides (
				id           TEXT PRIMARY KEY,
				workspace_id TEXT NOT NULL,
				feature      TEXT NOT NULL,
				enabled      BOOLEAN NOT NULL,
				reason       TEXT,
				created_by   TEXT NOT NULL,
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				expires_at   TIMESTAMPTZ,
				UNIQUE(workspace_id, feature)
			);

			CREATE TABLE workspace_notes (
				id           BIGSERIAL PRIMARY KEY,
				workspace_id TEXT NOT NULL,
				author_email TEXT NOT NULL,
				content      TEXT NOT NULL,
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);

			CREATE TABLE billing_events (
				id           BIGSERIAL PRIMARY KEY,
				workspace_id TEXT NOT NULL,
				event_type   TEXT NOT NULL,
				detail       TEXT NOT NULL DEFAULT '',
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX idx_billing_events_type ON billing_events(event_type, created_at);
		`,
	},
	{
		Version:     2,
		Description: "allow empty stripe_customer_id for non-Stripe subscriptions (trials, admin overrides)",
		SQL: `
			ALTER TABLE subscriptions DROP CONSTRAINT IF EXISTS subscriptions_stripe_customer_id_key;
			ALTER TABLE subscriptions ALTER COLUMN stripe_customer_id DROP NOT NULL;
			ALTER TABLE subscriptions ALTER COLUMN stripe_customer_id SET DEFAULT '';
			CREATE UNIQUE INDEX subscriptions_stripe_customer_id_key
				ON subscriptions (stripe_customer_id)
				WHERE stripe_customer_id IS NOT NULL AND stripe_customer_id != '';
		`,
	},
	{
		Version:     3,
		Description: "track processed Stripe webhook events for idempotent delivery",
		SQL: `
			CREATE TABLE processed_stripe_events (
				event_id     TEXT PRIMARY KEY,
				event_type   TEXT NOT NULL,
				processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
		`,
	},
	{
		Version:     4,
		Description: "give local trials an end date so they can expire",
		SQL: `
			ALTER TABLE subscriptions ADD COLUMN trial_ends_at TIMESTAMPTZ;

			-- Trials created before this column existed have no deadline and would
			-- never expire. Date them from their creation so the sweeper picks them
			-- up on the same 14-day terms they were granted under.
			UPDATE subscriptions
			   SET trial_ends_at = created_at + INTERVAL '14 days'
			 WHERE status = 'trialing' AND trial_ends_at IS NULL;

			CREATE INDEX idx_subscriptions_trialing
				ON subscriptions (trial_ends_at)
				WHERE status = 'trialing';
		`,
	},
	{
		Version:     5,
		Description: "idempotency guard for one-time credit-pack grants",
		SQL: `
			-- A purchased credit-pack grant must happen exactly once per Stripe
			-- checkout, even though the webhook can be delivered more than once and
			-- rolls its event marker back on a partial failure. The Stripe session
			-- id lands in reference_id; this makes a second grant for the same
			-- session collide rather than double-credit the workspace.
			CREATE UNIQUE INDEX credit_ledger_purchase_ref
				ON credit_ledger (workspace_id, reference_id)
				WHERE operation = 'purchase' AND reference_id <> '';
		`,
	},
	{
		Version:     6,
		Description: "document the monthly-credits period model on the allocation columns",
		SQL: `
			-- Monthly credits (this migration's epic): plan allocations are now keyed
			-- on the CALENDAR MONTH (UTC) instead of the calendar week, the Free plan
			-- has no recurring allowance (replaced by a one-time source='trial' grant
			-- in the non-expiring sentinel period, like purchased packs), and spend
			-- cascades plan -> trial -> purchased.
			--
			-- Deliberately no data rewrite: renaming week_start/week_end or rewriting
			-- rows is not a zero-downtime change, and none is needed — current-week
			-- plan rows from the weekly era stop matching the month-keyed queries
			-- immediately and simply age out; the workspace receives its month
			-- allocation on next touch, deduped on the month key by
			-- UNIQUE(workspace_id, week_start, source).
			COMMENT ON COLUMN credit_allocations.week_start IS
				'Allocation period start (UTC). plan: calendar-month start (calendar-week Monday before the monthly-credits change); trial/purchased: non-expiring sentinel 1970-01-01. Column name is historical.';
			COMMENT ON COLUMN credit_allocations.week_end IS
				'Allocation period end, exclusive (UTC). plan: next month start; trial/purchased: non-expiring sentinel 9999-01-01. Column name is historical.';
		`,
	},
}

// PgBillingStore implements BillingStore using PostgreSQL.
type PgBillingStore struct {
	db *storage.PgDB
}

// NewPgBillingStore creates a PostgreSQL-backed BillingStore and runs migrations.
func NewPgBillingStore(db *storage.PgDB) (*PgBillingStore, error) {
	if err := storage.MigratePostgresNS(db, "billing_schema_migrations", billingMigrations); err != nil {
		return nil, fmt.Errorf("migrate billing schema: %w", err)
	}
	return &PgBillingStore{db: db}, nil
}

// ---------------------------------------------------------------------------
// Subscriptions
// ---------------------------------------------------------------------------

func (s *PgBillingStore) GetSubscription(ctx context.Context, workspaceID string) (*Subscription, error) {
	var sub Subscription
	var plan, status string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, workspace_id, stripe_customer_id, stripe_subscription_id,
		        plan, status, seat_count, current_period_start, current_period_end,
		        cancel_at, trial_ends_at, created_at, updated_at
		 FROM subscriptions WHERE workspace_id = $1`, workspaceID).
		Scan(&sub.ID, &sub.WorkspaceID, &sub.StripeCustomerID, &sub.StripeSubscriptionID,
			&plan, &status, &sub.SeatCount, &sub.CurrentPeriodStart, &sub.CurrentPeriodEnd,
			&sub.CancelAt, &sub.TrialEndsAt, &sub.CreatedAt, &sub.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get subscription: %w", err)
	}
	sub.Plan = Plan(plan)
	sub.Status = status
	return &sub, nil
}

func (s *PgBillingStore) UpsertSubscription(ctx context.Context, sub *Subscription) error {
	if sub.ID == "" {
		sub.ID = id.New()
	}
	now := time.Now().UTC()
	if sub.CreatedAt.IsZero() {
		sub.CreatedAt = now
	}
	sub.UpdatedAt = now

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO subscriptions
			(id, workspace_id, stripe_customer_id, stripe_subscription_id,
			 plan, status, seat_count, current_period_start, current_period_end,
			 cancel_at, trial_ends_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		 ON CONFLICT (workspace_id) DO UPDATE SET
			stripe_customer_id = EXCLUDED.stripe_customer_id,
			stripe_subscription_id = EXCLUDED.stripe_subscription_id,
			plan = EXCLUDED.plan,
			status = EXCLUDED.status,
			seat_count = EXCLUDED.seat_count,
			current_period_start = EXCLUDED.current_period_start,
			current_period_end = EXCLUDED.current_period_end,
			cancel_at = EXCLUDED.cancel_at,
			trial_ends_at = EXCLUDED.trial_ends_at,
			updated_at = EXCLUDED.updated_at`,
		sub.ID, sub.WorkspaceID, sub.StripeCustomerID, sub.StripeSubscriptionID,
		string(sub.Plan), sub.Status, sub.SeatCount,
		sub.CurrentPeriodStart, sub.CurrentPeriodEnd,
		sub.CancelAt, sub.TrialEndsAt, sub.CreatedAt, sub.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert subscription: %w", err)
	}
	return nil
}

// ExpiredTrial identifies a workspace whose local trial the sweeper just ended.
type ExpiredTrial struct {
	WorkspaceID      string
	StripeCustomerID string
}

// ExpireTrials downgrades every workspace whose local trial has run out — the
// subscription AND the denormalized workspaces.plan cache — in ONE statement, and
// returns the workspaces it downgraded.
//
// Both writes must be atomic. The workspaces.plan cache is what the hot path
// reads (WorkspaceAccessMiddleware → PlanGuard, and the monthly-credit grant):
// if the subscription were downgraded but the cache write failed as a separate
// step, the row would leave `trialing` and never be re-swept, leaving the
// workspace on Pro limits (and, once its subscription is no longer `trialing`,
// eligible for the Pro monthly credit grant) for nothing. Doing both in one
// statement means a failure rolls back both and the next tick retries.
//
// This is the one place the billing store writes the auth-owned workspaces table
// directly. The alternative — a separate cache-sync call after the downgrade —
// is exactly the two-write drift above; and syncing the cache *before* the
// downgrade would let a workspace that converts to a paid plan mid-sweep get its
// cache clobbered to free. Atomicity is the only correct option, and billing is
// the authority the cache mirrors.
//
// The row is claimed with FOR UPDATE SKIP LOCKED and the subscription UPDATE
// re-asserts `status = 'trialing'`, so two sweepers can never both downgrade the
// same workspace, and a checkout converting the trial mid-sweep serializes on the
// same row lock (the re-check then excludes it, so the paid plan wins). The
// workspace's one-time trial credit grant is left alone — it is the workspace's
// grant regardless of plan, and clawing credits back would fail running jobs.
func (s *PgBillingStore) ExpireTrials(ctx context.Context, now time.Time, limit int) ([]ExpiredTrial, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`WITH due AS (
			SELECT id FROM subscriptions
			 WHERE status = 'trialing'
			   AND trial_ends_at IS NOT NULL
			   AND trial_ends_at <= $1
			 ORDER BY trial_ends_at
			 LIMIT $2
			 FOR UPDATE SKIP LOCKED
		 ),
		 downgraded AS (
			UPDATE subscriptions s
			   SET plan = $3, status = 'active', trial_ends_at = NULL, updated_at = NOW()
			   FROM due
			  WHERE s.id = due.id AND s.status = 'trialing'
			 RETURNING s.workspace_id, s.stripe_customer_id
		 ),
		 synced AS (
			-- Data-modifying CTEs always run to completion even when the final
			-- query does not read them, so this keeps the plan cache in step
			-- within the same transaction as the downgrade.
			UPDATE workspaces w
			   SET plan = $3
			   FROM downgraded
			  WHERE w.id = downgraded.workspace_id AND w.plan IS DISTINCT FROM $3
		 )
		 SELECT workspace_id, stripe_customer_id FROM downgraded`,
		now.UTC(), limit, string(PlanFree))
	if err != nil {
		return nil, fmt.Errorf("expire trials: %w", err)
	}
	defer rows.Close()

	var expired []ExpiredTrial
	for rows.Next() {
		var e ExpiredTrial
		var customerID sql.NullString
		if err := rows.Scan(&e.WorkspaceID, &customerID); err != nil {
			return nil, fmt.Errorf("scan expired trial: %w", err)
		}
		e.StripeCustomerID = customerID.String
		expired = append(expired, e)
	}
	return expired, rows.Err()
}

func (s *PgBillingStore) ListSubscriptions(ctx context.Context, limit, offset int) ([]*Subscription, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, workspace_id, stripe_customer_id, stripe_subscription_id,
		        plan, status, seat_count, current_period_start, current_period_end,
		        cancel_at, trial_ends_at, created_at, updated_at
		 FROM subscriptions ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	defer rows.Close()

	var result []*Subscription
	for rows.Next() {
		var sub Subscription
		var plan, status string
		if err := rows.Scan(&sub.ID, &sub.WorkspaceID, &sub.StripeCustomerID, &sub.StripeSubscriptionID,
			&plan, &status, &sub.SeatCount, &sub.CurrentPeriodStart, &sub.CurrentPeriodEnd,
			&sub.CancelAt, &sub.TrialEndsAt, &sub.CreatedAt, &sub.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan subscription: %w", err)
		}
		sub.Plan = Plan(plan)
		sub.Status = status
		result = append(result, &sub)
	}
	return result, rows.Err()
}

// ---------------------------------------------------------------------------
// Credits
// ---------------------------------------------------------------------------

// GetCurrentAllocation returns the current month's PLAN allocation only. It is
// the monthly-allowance bucket that drives EnsureMonthlyAllocation (has this
// month been granted yet?) and the low-credit threshold notifications. It
// deliberately excludes the non-expiring buckets — spendable balance
// (plan + trial + purchased) is CheckCredits, and spending cascades across all
// three in DeductCredits (Epic 004).
func (s *PgBillingStore) GetCurrentAllocation(ctx context.Context, workspaceID string) (*CreditAllocation, error) {
	now := time.Now().UTC()
	ms := MonthStart(now)
	me := MonthEnd(now)

	var alloc CreditAllocation
	err := s.db.QueryRowContext(ctx,
		`SELECT id, workspace_id, credits_total, credits_used, week_start, week_end, source, created_at
		 FROM credit_allocations
		 WHERE workspace_id = $1 AND week_start = $2 AND week_end = $3 AND source = 'plan'`,
		workspaceID, ms, me).
		Scan(&alloc.ID, &alloc.WorkspaceID, &alloc.CreditsTotal, &alloc.CreditsUsed,
			&alloc.PeriodStart, &alloc.PeriodEnd, &alloc.Source, &alloc.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get current allocation: %w", err)
	}
	return &alloc, nil
}

// allocationRow is a locked credit_allocations row being drawn from in DeductCredits.
type allocationRow struct {
	id    string
	total int64
	used  int64
	found bool
}

// avail is the spendable remainder in this bucket, floored at zero (a bucket
// already driven negative by a prior overage does not absorb more here).
func (a allocationRow) avail() int64 {
	return max(a.total-a.used, 0)
}

// DeductCredits charges amount against a workspace's credits, cascading across
// sources: draw from the monthly plan allowance first, then the one-time trial
// grant, then non-expiring purchased packs (Epic 004). The order is deliberate:
// the expiring bucket goes first (unspent plan credits evaporate at month
// rollover anyway), the promotional trial grant next, and credits the customer
// paid real money for are preserved the longest. Any overage once every bucket
// is exhausted is recorded against the plan bucket (or the first non-expiring
// bucket that exists) so the debt is never lost. One immutable ledger entry is
// written per bucket touched.
func (s *PgBillingStore) DeductCredits(ctx context.Context, workspaceID string, amount int64, op string, refID string) error {
	if amount <= 0 {
		return nil
	}

	now := time.Now().UTC()
	ms := MonthStart(now)
	me := MonthEnd(now)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rollback is a no-op after commit

	// Lock the buckets FOR UPDATE in a fixed order (plan, trial, purchased) so
	// concurrent deductions on the same workspace serialize deterministically.
	plan, err := lockAllocation(ctx, tx,
		`SELECT id, credits_total, credits_used FROM credit_allocations
		 WHERE workspace_id = $1 AND week_start = $2 AND week_end = $3 AND source = 'plan' FOR UPDATE`,
		workspaceID, ms, me)
	if err != nil {
		return fmt.Errorf("lock plan allocation: %w", err)
	}
	trial, err := lockAllocation(ctx, tx,
		`SELECT id, credits_total, credits_used FROM credit_allocations
		 WHERE workspace_id = $1 AND source = 'trial' FOR UPDATE`,
		workspaceID)
	if err != nil {
		return fmt.Errorf("lock trial allocation: %w", err)
	}
	purchased, err := lockAllocation(ctx, tx,
		`SELECT id, credits_total, credits_used FROM credit_allocations
		 WHERE workspace_id = $1 AND source = 'purchased' FOR UPDATE`,
		workspaceID)
	if err != nil {
		return fmt.Errorf("lock purchased allocation: %w", err)
	}
	if !plan.found && !trial.found && !purchased.found {
		return fmt.Errorf("deduct credits: no allocation for workspace %s", workspaceID)
	}

	// Cascade the charge across the buckets in spend order, then record any
	// overage on the first bucket that exists so the debt is never lost (plan
	// preferred — it refills monthly, so the debt nets against the refill).
	buckets := []*allocationRow{&plan, &trial, &purchased}
	deltas := make([]int64, len(buckets))
	remaining := amount
	for i, b := range buckets {
		if b.found && remaining > 0 {
			d := min(remaining, b.avail())
			deltas[i] += d
			remaining -= d
		}
	}
	if remaining > 0 {
		for i, b := range buckets {
			if b.found {
				deltas[i] += remaining
				break
			}
		}
	}

	for i, b := range buckets {
		if deltas[i] == 0 {
			continue
		}
		if err := s.applyDeduction(ctx, tx, workspaceID, *b, deltas[i], op, refID); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// lockAllocation runs a `SELECT ... FOR UPDATE` returning a single allocation
// row. A missing row yields found=false (not an error), so callers can cascade
// across whichever buckets exist.
func lockAllocation(ctx context.Context, tx *sql.Tx, query string, args ...any) (allocationRow, error) {
	var a allocationRow
	err := tx.QueryRowContext(ctx, query, args...).Scan(&a.id, &a.total, &a.used)
	switch {
	case err == nil:
		a.found = true
		return a, nil
	case errors.Is(err, sql.ErrNoRows):
		return a, nil
	default:
		return a, err
	}
}

// applyDeduction adds delta to a bucket's credits_used and appends the matching
// (debit) ledger entry, within the caller's transaction.
func (s *PgBillingStore) applyDeduction(ctx context.Context, tx *sql.Tx, workspaceID string, bucket allocationRow, delta int64, op, refID string) error {
	newUsed := bucket.used + delta
	if _, err := tx.ExecContext(ctx,
		`UPDATE credit_allocations SET credits_used = $1 WHERE id = $2`, newUsed, bucket.id); err != nil {
		return fmt.Errorf("update allocation: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO credit_ledger (workspace_id, allocation_id, amount, balance_after, operation, reference_id)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		workspaceID, bucket.id, -delta, bucket.total-newUsed, op, refID); err != nil {
		return fmt.Errorf("insert ledger entry: %w", err)
	}
	return nil
}

// CheckCredits returns the workspace's total spendable balance: this month's
// remaining plan allowance plus all non-expiring credits (the one-time trial
// grant and purchased packs, Epic 004).
//
// Only a workspace with NO credit rows at all yields the "no allocation" error
// (callers — the enqueue pre-check, QuotaGuard — degrade to allowing the
// request). A workspace whose buckets exist but are spent returns the real,
// possibly zero-or-negative balance and IS blocked: with the Free plan's
// recurring allowance gone, "trial grant fully spent" is the steady state of a
// free workspace and must read as out-of-credits, not as unmetered.
func (s *PgBillingStore) CheckCredits(ctx context.Context, workspaceID string) (int64, error) {
	now := time.Now().UTC()
	ms := MonthStart(now)
	me := MonthEnd(now)

	var planRemaining int64
	planErr := s.db.QueryRowContext(ctx,
		`SELECT credits_total - credits_used FROM credit_allocations
		 WHERE workspace_id = $1 AND week_start = $2 AND week_end = $3 AND source = 'plan'`,
		workspaceID, ms, me).Scan(&planRemaining)
	if planErr != nil && !errors.Is(planErr, sql.ErrNoRows) {
		return 0, fmt.Errorf("get plan credits: %w", planErr)
	}

	var nonExpiringRows int
	var nonExpiringRemaining int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(credits_total - credits_used), 0) FROM credit_allocations
		 WHERE workspace_id = $1 AND source IN ('trial', 'purchased')`,
		workspaceID).Scan(&nonExpiringRows, &nonExpiringRemaining); err != nil {
		return 0, fmt.Errorf("sum non-expiring credits: %w", err)
	}

	if errors.Is(planErr, sql.ErrNoRows) {
		if nonExpiringRows == 0 {
			return 0, errNoAllocation
		}
		return nonExpiringRemaining, nil
	}
	return planRemaining + nonExpiringRemaining, nil
}

// GrantPurchasedCredits grants one purchased credit pack, exactly once per Stripe
// checkout, and records its billing event — all in a single transaction keyed on
// referenceID (the Stripe session id).
//
// This exists because the webhook handler rolls its idempotency marker back on
// ANY dispatch error, so a grant that commits while a later, separate write fails
// would be re-applied by Stripe's retry — a $5 pack crediting 400K. Folding the
// grant and its event into one transaction means a retry re-runs an
// all-or-nothing unit, and the unique index on (workspace_id, reference_id) for
// purchase rows makes even a concurrent double-delivery collide instead of
// double-crediting. Returns false (no error) when this session was already
// granted, so the caller treats the duplicate as success.
func (s *PgBillingStore) GrantPurchasedCredits(ctx context.Context, workspaceID string, amount int64, referenceID string) (bool, error) {
	if referenceID == "" {
		return false, errors.New("grant purchased credits: reference id is required for idempotency")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rollback is a no-op after commit

	// Already granted for this checkout? A duplicate delivery lands here.
	var already bool
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM credit_ledger
		 WHERE workspace_id = $1 AND reference_id = $2 AND operation = 'purchase')`,
		workspaceID, referenceID).Scan(&already); err != nil {
		return false, fmt.Errorf("check existing grant: %w", err)
	}
	if already {
		return false, tx.Commit()
	}

	// Add to the non-expiring purchased bucket (sentinel period).
	allocID := id.New()
	var total, used int64
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO credit_allocations (id, workspace_id, credits_total, credits_used, week_start, week_end, source)
		 VALUES ($1, $2, $3, 0, $4, $5, $6)
		 ON CONFLICT (workspace_id, week_start, source) DO UPDATE SET
			credits_total = credit_allocations.credits_total + EXCLUDED.credits_total
		 RETURNING id, credits_total, credits_used`,
		allocID, workspaceID, amount, nonExpiringPeriodStart, nonExpiringPeriodEnd, SourcePurchased).
		Scan(&allocID, &total, &used); err != nil {
		return false, fmt.Errorf("grant purchased credits: %w", err)
	}

	// The ledger row carries the reference; the unique index makes a racing
	// second delivery fail here (unique violation) rather than double-grant.
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO credit_ledger (workspace_id, allocation_id, amount, balance_after, operation, reference_id)
		 VALUES ($1, $2, $3, $4, 'purchase', $5)`,
		workspaceID, allocID, amount, total-used, referenceID); err != nil {
		return false, fmt.Errorf("record purchase ledger entry: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO billing_events (workspace_id, event_type, detail)
		 VALUES ($1, 'credits_purchased', $2)`,
		workspaceID, fmt.Sprintf("Credit pack purchased, +%d credits", amount)); err != nil {
		return false, fmt.Errorf("record purchase event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit purchase: %w", err)
	}
	return true, nil
}

func (s *PgBillingStore) GrantCredits(ctx context.Context, workspaceID string, amount int64, source string) error {
	// Plan grants land in the current calendar month; trial grants and
	// purchased packs land in the non-expiring sentinel period so they survive
	// monthly rollovers (Epic 004).
	ps, pe := allocationPeriod(source, time.Now().UTC())
	allocID := id.New()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rollback is a no-op after commit

	// Conflict semantics differ by bucket. A PLAN allowance is granted at most
	// once per period: two racing EnsureMonthlyAllocation calls (or a re-grant
	// within the weekly→monthly transition month) must dedupe on the month key,
	// not stack — DO NOTHING. Non-expiring buckets share one sentinel-keyed row
	// per source, so repeated grants there ACCUMULATE.
	var inserted bool
	if source == SourcePlan {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO credit_allocations (id, workspace_id, credits_total, credits_used, week_start, week_end, source)
			 VALUES ($1, $2, $3, 0, $4, $5, $6)
			 ON CONFLICT (workspace_id, week_start, source) DO NOTHING`,
			allocID, workspaceID, amount, ps, pe, source)
		if err != nil {
			return fmt.Errorf("grant credits: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("grant credits rows affected: %w", err)
		}
		inserted = n > 0
	} else {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO credit_allocations (id, workspace_id, credits_total, credits_used, week_start, week_end, source)
			 VALUES ($1, $2, $3, 0, $4, $5, $6)
			 ON CONFLICT (workspace_id, week_start, source) DO UPDATE SET
				credits_total = credit_allocations.credits_total + EXCLUDED.credits_total`,
			allocID, workspaceID, amount, ps, pe, source); err != nil {
			return fmt.Errorf("grant credits: %w", err)
		}
		inserted = true
	}

	if inserted {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO credit_ledger (workspace_id, allocation_id, amount, balance_after, operation, reference_id)
			 VALUES ($1, $2, $3, $4, $5, '')`,
			workspaceID, allocID, amount, amount, "grant"); err != nil {
			return fmt.Errorf("insert ledger entry: %w", err)
		}
	}

	return tx.Commit()
}

// GrantTrialCredits grants the workspace's one-time trial credits: a
// non-expiring source='trial' allocation created at most once per workspace,
// EVER, keyed on the UNIQUE(workspace_id, week_start, source) constraint with
// the sentinel period. The grant, its ledger row, and its billing event are one
// transaction, so the audit trail cannot drift from the money.
//
// The duplicate fast path is a single read (no transaction): the allocation
// middleware calls this on every touch of a workspace with no recurring
// allowance, so the already-granted case must stay hot-path cheap. Two
// concurrent first grants race to the insert; DO NOTHING makes the loser a
// no-op (granted=false) rather than a double grant.
func (s *PgBillingStore) GrantTrialCredits(ctx context.Context, workspaceID string, amount int64) (bool, error) {
	var already bool
	if err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM credit_allocations
		 WHERE workspace_id = $1 AND source = 'trial')`,
		workspaceID).Scan(&already); err != nil {
		return false, fmt.Errorf("check trial grant: %w", err)
	}
	if already {
		return false, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rollback is a no-op after commit

	allocID := id.New()
	res, err := tx.ExecContext(ctx,
		`INSERT INTO credit_allocations (id, workspace_id, credits_total, credits_used, week_start, week_end, source)
		 VALUES ($1, $2, $3, 0, $4, $5, $6)
		 ON CONFLICT (workspace_id, week_start, source) DO NOTHING`,
		allocID, workspaceID, amount, nonExpiringPeriodStart, nonExpiringPeriodEnd, SourceTrial)
	if err != nil {
		return false, fmt.Errorf("grant trial credits: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("grant trial credits rows affected: %w", err)
	}
	if n == 0 {
		// A concurrent grant won the race; this call is a no-op success.
		return false, tx.Commit()
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO credit_ledger (workspace_id, allocation_id, amount, balance_after, operation, reference_id)
		 VALUES ($1, $2, $3, $4, 'trial_grant', '')`,
		workspaceID, allocID, amount, amount); err != nil {
		return false, fmt.Errorf("record trial grant ledger entry: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO billing_events (workspace_id, event_type, detail)
		 VALUES ($1, 'trial_credits_granted', $2)`,
		workspaceID, fmt.Sprintf("One-time trial grant, +%d credits", amount)); err != nil {
		return false, fmt.Errorf("record trial grant event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit trial grant: %w", err)
	}
	return true, nil
}

// ---------------------------------------------------------------------------
// Ledger
// ---------------------------------------------------------------------------

func (s *PgBillingStore) GetLedger(ctx context.Context, workspaceID string, from, to time.Time) ([]LedgerEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, workspace_id, allocation_id, amount, balance_after, operation, reference_id, created_at
		 FROM credit_ledger
		 WHERE workspace_id = $1 AND created_at >= $2 AND created_at < $3
		 ORDER BY created_at DESC`,
		workspaceID, from, to)
	if err != nil {
		return nil, fmt.Errorf("get ledger: %w", err)
	}
	defer rows.Close()

	var entries []LedgerEntry
	for rows.Next() {
		var e LedgerEntry
		var allocID sql.NullString
		var refID sql.NullString
		if err := rows.Scan(&e.ID, &e.WorkspaceID, &allocID, &e.Amount, &e.BalanceAfter,
			&e.Operation, &refID, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan ledger entry: %w", err)
		}
		e.AllocationID = allocID.String
		e.ReferenceID = refID.String
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// ---------------------------------------------------------------------------
// Feature Overrides
// ---------------------------------------------------------------------------

func (s *PgBillingStore) GetFeatureOverrides(ctx context.Context, workspaceID string) ([]FeatureOverride, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, workspace_id, feature, enabled, reason, created_by, created_at, expires_at
		 FROM feature_overrides
		 WHERE workspace_id = $1 AND (expires_at IS NULL OR expires_at > NOW())
		 ORDER BY created_at`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("get feature overrides: %w", err)
	}
	defer rows.Close()

	var overrides []FeatureOverride
	for rows.Next() {
		var o FeatureOverride
		var feat, reason sql.NullString
		if err := rows.Scan(&o.ID, &o.WorkspaceID, &feat, &o.Enabled, &reason,
			&o.CreatedBy, &o.CreatedAt, &o.ExpiresAt); err != nil {
			return nil, fmt.Errorf("scan feature override: %w", err)
		}
		o.Feature = Feature(feat.String)
		o.Reason = reason.String
		overrides = append(overrides, o)
	}
	return overrides, rows.Err()
}

func (s *PgBillingStore) ListAllFeatureOverrides(ctx context.Context) ([]FeatureOverride, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, workspace_id, feature, enabled, reason, created_by, created_at, expires_at
		 FROM feature_overrides
		 WHERE expires_at IS NULL OR expires_at > NOW()
		 ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list all feature overrides: %w", err)
	}
	defer rows.Close()

	var overrides []FeatureOverride
	for rows.Next() {
		var o FeatureOverride
		var feat, reason sql.NullString
		if err := rows.Scan(&o.ID, &o.WorkspaceID, &feat, &o.Enabled, &reason,
			&o.CreatedBy, &o.CreatedAt, &o.ExpiresAt); err != nil {
			return nil, fmt.Errorf("scan feature override: %w", err)
		}
		o.Feature = Feature(feat.String)
		o.Reason = reason.String
		overrides = append(overrides, o)
	}
	return overrides, rows.Err()
}

func (s *PgBillingStore) SetFeatureOverride(ctx context.Context, override *FeatureOverride) error {
	if override.ID == "" {
		override.ID = id.New()
	}
	if override.CreatedAt.IsZero() {
		override.CreatedAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO feature_overrides (id, workspace_id, feature, enabled, reason, created_by, created_at, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (workspace_id, feature) DO UPDATE SET
			enabled = EXCLUDED.enabled,
			reason = EXCLUDED.reason,
			created_by = EXCLUDED.created_by,
			created_at = EXCLUDED.created_at,
			expires_at = EXCLUDED.expires_at`,
		override.ID, override.WorkspaceID, string(override.Feature), override.Enabled,
		override.Reason, override.CreatedBy, override.CreatedAt, override.ExpiresAt)
	if err != nil {
		return fmt.Errorf("set feature override: %w", err)
	}
	return nil
}

func (s *PgBillingStore) DeleteFeatureOverride(ctx context.Context, workspaceID string, feature Feature) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM feature_overrides WHERE workspace_id = $1 AND feature = $2`,
		workspaceID, string(feature))
	if err != nil {
		return fmt.Errorf("delete feature override: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("feature override not found")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Notes
// ---------------------------------------------------------------------------

func (s *PgBillingStore) ListNotes(ctx context.Context, workspaceID string) ([]WorkspaceNote, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, workspace_id, author_email, content, created_at
		 FROM workspace_notes WHERE workspace_id = $1
		 ORDER BY created_at DESC`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list notes: %w", err)
	}
	defer rows.Close()

	var notes []WorkspaceNote
	for rows.Next() {
		var n WorkspaceNote
		if err := rows.Scan(&n.ID, &n.WorkspaceID, &n.AuthorEmail, &n.Content, &n.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan note: %w", err)
		}
		notes = append(notes, n)
	}
	return notes, rows.Err()
}

func (s *PgBillingStore) AddNote(ctx context.Context, note *WorkspaceNote) error {
	if note.CreatedAt.IsZero() {
		note.CreatedAt = time.Now().UTC()
	}
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO workspace_notes (workspace_id, author_email, content, created_at)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		note.WorkspaceID, note.AuthorEmail, note.Content, note.CreatedAt).
		Scan(&note.ID)
	if err != nil {
		return fmt.Errorf("add note: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Upsells
// ---------------------------------------------------------------------------

func (s *PgBillingStore) GetUpsellOpportunities(ctx context.Context) ([]UpsellOpportunity, error) {
	// Delegate to the upsell detection queries.
	return detectUpsells(ctx, s.db.DB)
}

// ---------------------------------------------------------------------------
// Metrics
// ---------------------------------------------------------------------------

func (s *PgBillingStore) GetPlatformMetrics(ctx context.Context) (*PlatformMetrics, error) {
	var m PlatformMetrics

	// Active workspaces: those that actually consumed credits this month (a
	// debit ledger entry). Allocation presence no longer works as an activity
	// proxy: with the Free tier's recurring allowance gone, only paid plans
	// have current-period plan allocations, and the non-expiring trial /
	// purchased rows carry a sentinel period spanning "now" that would count
	// every workspace ever created (Epic 004).
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT workspace_id) FROM credit_ledger
		 WHERE created_at >= $1 AND amount < 0`, MonthStart(time.Now().UTC())).Scan(&m.ActiveWorkspaces)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("active workspaces: %w", err)
	}

	// MRR from subscriptions: Pro=$25, Team=$20*seats.
	err = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(
			CASE plan
				WHEN 'pro' THEN 25.0
				WHEN 'team' THEN 20.0 * seat_count
				ELSE 0
			END
		), 0) FROM subscriptions WHERE status = 'active'`).Scan(&m.MRR)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("mrr: %w", err)
	}

	// Credit utilization across this month's plan allocations (paid plans —
	// Free has no recurring allocation to utilize).
	now := time.Now().UTC()
	ms := MonthStart(now)
	me := MonthEnd(now)
	err = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(
			AVG(CASE WHEN credits_total > 0 THEN (credits_used::float / credits_total) * 100 ELSE 0 END),
			0)
		 FROM credit_allocations
		 WHERE week_start = $1 AND week_end = $2 AND source = 'plan'`, ms, me).Scan(&m.CreditUtilizationPct)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("credit utilization: %w", err)
	}

	return &m, nil
}

// ---------------------------------------------------------------------------
// Events
// ---------------------------------------------------------------------------

func (s *PgBillingStore) ListBillingEvents(ctx context.Context, limit, offset int, eventType string) ([]BillingEvent, error) {
	var rows *sql.Rows
	var err error
	if eventType != "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, workspace_id, event_type, detail, created_at
			 FROM billing_events WHERE event_type = $1
			 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
			eventType, limit, offset)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, workspace_id, event_type, detail, created_at
			 FROM billing_events
			 ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
			limit, offset)
	}
	if err != nil {
		return nil, fmt.Errorf("list billing events: %w", err)
	}
	defer rows.Close()

	var events []BillingEvent
	for rows.Next() {
		var e BillingEvent
		if err := rows.Scan(&e.ID, &e.WorkspaceID, &e.EventType, &e.Detail, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan billing event: %w", err)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func (s *PgBillingStore) RecordBillingEvent(ctx context.Context, event *BillingEvent) error {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO billing_events (workspace_id, event_type, detail, created_at)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		event.WorkspaceID, event.EventType, event.Detail, event.CreatedAt).
		Scan(&event.ID)
	if err != nil {
		return fmt.Errorf("record billing event: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Webhook idempotency
// ---------------------------------------------------------------------------

func (s *PgBillingStore) MarkStripeEventProcessed(ctx context.Context, eventID, eventType string) (bool, error) {
	// Insert-first with ON CONFLICT DO NOTHING: the first delivery inserts a row
	// (one row affected, alreadyProcessed=false); duplicate deliveries conflict
	// on the primary key and affect zero rows (alreadyProcessed=true).
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO processed_stripe_events (event_id, event_type)
		 VALUES ($1, $2)
		 ON CONFLICT (event_id) DO NOTHING`,
		eventID, eventType)
	if err != nil {
		return false, fmt.Errorf("mark stripe event processed: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("mark stripe event processed rows affected: %w", err)
	}
	return n == 0, nil
}

func (s *PgBillingStore) UnmarkStripeEvent(ctx context.Context, eventID string) error {
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM processed_stripe_events WHERE event_id = $1`, eventID); err != nil {
		return fmt.Errorf("unmark stripe event: %w", err)
	}
	return nil
}
