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

func (s *PgBillingStore) DeductCredits(ctx context.Context, workspaceID string, amount int64, op string, refID string) error {
	now := time.Now().UTC()
	ws := WeekStart(now)
	we := WeekEnd(now)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rollback is a no-op after commit

	// Update allocation.
	var allocID string
	var creditsTotal, creditsUsed int64
	err = tx.QueryRowContext(ctx,
		`UPDATE credit_allocations
		 SET credits_used = credits_used + $1
		 WHERE workspace_id = $2 AND week_start = $3 AND week_end = $4 AND source = 'plan'
		 RETURNING id, credits_total, credits_used`,
		amount, workspaceID, ws, we).
		Scan(&allocID, &creditsTotal, &creditsUsed)
	if err != nil {
		return fmt.Errorf("deduct credits: %w", err)
	}

	balanceAfter := creditsTotal - creditsUsed

	// Insert ledger entry.
	_, err = tx.ExecContext(ctx,
		`INSERT INTO credit_ledger (workspace_id, allocation_id, amount, balance_after, operation, reference_id)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		workspaceID, allocID, -amount, balanceAfter, op, refID)
	if err != nil {
		return fmt.Errorf("insert ledger entry: %w", err)
	}

	return tx.Commit()
}

func (s *PgBillingStore) CheckCredits(ctx context.Context, workspaceID string) (int64, error) {
	alloc, err := s.GetCurrentAllocation(ctx, workspaceID)
	if err != nil {
		return 0, err
	}
	return alloc.CreditsTotal - alloc.CreditsUsed, nil
}

func (s *PgBillingStore) GrantCredits(ctx context.Context, workspaceID string, amount int64, source string) error {
	now := time.Now().UTC()
	ws := WeekStart(now)
	we := WeekEnd(now)
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

	// Active workspaces (with at least one member).
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT workspace_id) FROM credit_allocations
		 WHERE week_start <= NOW() AND week_end > NOW()`).Scan(&m.ActiveWorkspaces)
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
