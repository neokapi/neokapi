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
		        cancel_at, created_at, updated_at
		 FROM subscriptions WHERE workspace_id = $1`, workspaceID).
		Scan(&sub.ID, &sub.WorkspaceID, &sub.StripeCustomerID, &sub.StripeSubscriptionID,
			&plan, &status, &sub.SeatCount, &sub.CurrentPeriodStart, &sub.CurrentPeriodEnd,
			&sub.CancelAt, &sub.CreatedAt, &sub.UpdatedAt)
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
			 cancel_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 ON CONFLICT (workspace_id) DO UPDATE SET
			stripe_customer_id = EXCLUDED.stripe_customer_id,
			stripe_subscription_id = EXCLUDED.stripe_subscription_id,
			plan = EXCLUDED.plan,
			status = EXCLUDED.status,
			seat_count = EXCLUDED.seat_count,
			current_period_start = EXCLUDED.current_period_start,
			current_period_end = EXCLUDED.current_period_end,
			cancel_at = EXCLUDED.cancel_at,
			updated_at = EXCLUDED.updated_at`,
		sub.ID, sub.WorkspaceID, sub.StripeCustomerID, sub.StripeSubscriptionID,
		string(sub.Plan), sub.Status, sub.SeatCount,
		sub.CurrentPeriodStart, sub.CurrentPeriodEnd,
		sub.CancelAt, sub.CreatedAt, sub.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert subscription: %w", err)
	}
	return nil
}

func (s *PgBillingStore) ListSubscriptions(ctx context.Context, limit, offset int) ([]*Subscription, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, workspace_id, stripe_customer_id, stripe_subscription_id,
		        plan, status, seat_count, current_period_start, current_period_end,
		        cancel_at, created_at, updated_at
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
			&sub.CancelAt, &sub.CreatedAt, &sub.UpdatedAt); err != nil {
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

// GetCurrentAllocation returns the current week's PLAN allocation only. It is
// the weekly-allowance bucket that drives EnsureWeeklyAllocation (has this week
// been granted yet?) and the low-credit threshold notifications. It deliberately
// excludes purchased packs — spendable balance (plan + purchased) is
// CheckCredits, and spending cascades across both in DeductCredits (Epic 004).
func (s *PgBillingStore) GetCurrentAllocation(ctx context.Context, workspaceID string) (*CreditAllocation, error) {
	now := time.Now().UTC()
	ws := WeekStart(now)
	we := WeekEnd(now)

	var alloc CreditAllocation
	err := s.db.QueryRowContext(ctx,
		`SELECT id, workspace_id, credits_total, credits_used, week_start, week_end, source, created_at
		 FROM credit_allocations
		 WHERE workspace_id = $1 AND week_start = $2 AND week_end = $3 AND source = 'plan'`,
		workspaceID, ws, we).
		Scan(&alloc.ID, &alloc.WorkspaceID, &alloc.CreditsTotal, &alloc.CreditsUsed,
			&alloc.WeekStart, &alloc.WeekEnd, &alloc.Source, &alloc.CreatedAt)
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
// sources: draw from the weekly plan allowance first, then non-expiring
// purchased packs (Epic 004). Any overage once both are exhausted is recorded
// against the plan bucket (or purchased when no plan bucket exists) so the debt
// is never lost. One immutable ledger entry is written per bucket touched.
func (s *PgBillingStore) DeductCredits(ctx context.Context, workspaceID string, amount int64, op string, refID string) error {
	if amount <= 0 {
		return nil
	}

	now := time.Now().UTC()
	ws := WeekStart(now)
	we := WeekEnd(now)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rollback is a no-op after commit

	// Lock both buckets FOR UPDATE in a fixed order (plan, then purchased) so
	// concurrent deductions on the same workspace serialize deterministically.
	plan, err := lockAllocation(ctx, tx,
		`SELECT id, credits_total, credits_used FROM credit_allocations
		 WHERE workspace_id = $1 AND week_start = $2 AND week_end = $3 AND source = 'plan' FOR UPDATE`,
		workspaceID, ws, we)
	if err != nil {
		return fmt.Errorf("lock plan allocation: %w", err)
	}
	purchased, err := lockAllocation(ctx, tx,
		`SELECT id, credits_total, credits_used FROM credit_allocations
		 WHERE workspace_id = $1 AND source = 'purchased' FOR UPDATE`,
		workspaceID)
	if err != nil {
		return fmt.Errorf("lock purchased allocation: %w", err)
	}
	if !plan.found && !purchased.found {
		return fmt.Errorf("deduct credits: no allocation for workspace %s", workspaceID)
	}

	// Cascade the charge: plan first, then purchased.
	remaining := amount
	var planDelta, purchasedDelta int64
	if plan.found {
		d := min(remaining, plan.avail())
		planDelta += d
		remaining -= d
	}
	if purchased.found && remaining > 0 {
		d := min(remaining, purchased.avail())
		purchasedDelta += d
		remaining -= d
	}
	if remaining > 0 {
		// Both buckets exhausted; record the overage so it isn't lost. Prefer
		// the plan bucket (it refills weekly); fall back to purchased.
		if plan.found {
			planDelta += remaining
		} else {
			purchasedDelta += remaining
		}
	}

	if planDelta != 0 {
		if err := s.applyDeduction(ctx, tx, workspaceID, plan, planDelta, op, refID); err != nil {
			return err
		}
	}
	if purchasedDelta != 0 {
		if err := s.applyDeduction(ctx, tx, workspaceID, purchased, purchasedDelta, op, refID); err != nil {
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

// CheckCredits returns the workspace's total spendable balance: this week's
// remaining plan allowance plus all non-expiring purchased credits (Epic 004).
// When the workspace has neither a plan allocation for the week nor any
// purchased credits, it returns the "no allocation" error so callers (the
// enqueue pre-check, QuotaGuard) degrade to allowing the request.
func (s *PgBillingStore) CheckCredits(ctx context.Context, workspaceID string) (int64, error) {
	now := time.Now().UTC()
	ws := WeekStart(now)
	we := WeekEnd(now)

	var planRemaining int64
	planErr := s.db.QueryRowContext(ctx,
		`SELECT credits_total - credits_used FROM credit_allocations
		 WHERE workspace_id = $1 AND week_start = $2 AND week_end = $3 AND source = 'plan'`,
		workspaceID, ws, we).Scan(&planRemaining)
	if planErr != nil && !errors.Is(planErr, sql.ErrNoRows) {
		return 0, fmt.Errorf("get plan credits: %w", planErr)
	}

	var purchasedRemaining int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(credits_total - credits_used), 0) FROM credit_allocations
		 WHERE workspace_id = $1 AND source = 'purchased'`,
		workspaceID).Scan(&purchasedRemaining); err != nil {
		return 0, fmt.Errorf("sum purchased credits: %w", err)
	}

	if errors.Is(planErr, sql.ErrNoRows) {
		if purchasedRemaining != 0 {
			return purchasedRemaining, nil
		}
		// No plan allocation and no purchased credits: preserve the historical
		// "no allocation" signal so callers degrade gracefully.
		return 0, fmt.Errorf("check credits: %w", planErr)
	}
	return planRemaining + purchasedRemaining, nil
}

func (s *PgBillingStore) GrantCredits(ctx context.Context, workspaceID string, amount int64, source string) error {
	// Plan grants land in the current week; purchased packs land in the
	// non-expiring sentinel week so they survive weekly rollovers (Epic 004).
	ws, we := allocationWeek(source, time.Now().UTC())
	allocID := id.New()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rollback is a no-op after commit

	_, err = tx.ExecContext(ctx,
		`INSERT INTO credit_allocations (id, workspace_id, credits_total, credits_used, week_start, week_end, source)
		 VALUES ($1, $2, $3, 0, $4, $5, $6)
		 ON CONFLICT (workspace_id, week_start, source) DO UPDATE SET
			credits_total = credit_allocations.credits_total + EXCLUDED.credits_total`,
		allocID, workspaceID, amount, ws, we, source)
	if err != nil {
		return fmt.Errorf("grant credits: %w", err)
	}

	// Insert ledger entry.
	_, err = tx.ExecContext(ctx,
		`INSERT INTO credit_ledger (workspace_id, allocation_id, amount, balance_after, operation, reference_id)
		 VALUES ($1, $2, $3, $4, $5, '')`,
		workspaceID, allocID, amount, amount, "grant")
	if err != nil {
		return fmt.Errorf("insert ledger entry: %w", err)
	}

	return tx.Commit()
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

	// Active workspaces: those with a current weekly plan allocation. Restricted
	// to source='plan' so non-expiring purchased packs (which carry a sentinel
	// week spanning "now") don't inflate the count (Epic 004).
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT workspace_id) FROM credit_allocations
		 WHERE week_start <= NOW() AND week_end > NOW() AND source = 'plan'`).Scan(&m.ActiveWorkspaces)
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

	// Credit utilization.
	now := time.Now().UTC()
	ws := WeekStart(now)
	we := WeekEnd(now)
	err = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(
			AVG(CASE WHEN credits_total > 0 THEN (credits_used::float / credits_total) * 100 ELSE 0 END),
			0)
		 FROM credit_allocations
		 WHERE week_start = $1 AND week_end = $2 AND source = 'plan'`, ws, we).Scan(&m.CreditUtilizationPct)
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
