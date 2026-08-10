package billing

import (
	"context"
	"time"
)

// BillingStore persists subscription, credit, and billing-related data.
type BillingStore interface {
	// Subscriptions
	GetSubscription(ctx context.Context, workspaceID string) (*Subscription, error)
	UpsertSubscription(ctx context.Context, sub *Subscription) error
	ListSubscriptions(ctx context.Context, limit, offset int) ([]*Subscription, error)

	// Credits
	GetCurrentAllocation(ctx context.Context, workspaceID string) (*CreditAllocation, error)
	DeductCredits(ctx context.Context, workspaceID string, amount int64, op string, refID string) error
	CheckCredits(ctx context.Context, workspaceID string) (remaining int64, err error)
	GrantCredits(ctx context.Context, workspaceID string, amount int64, source string) error

	// GrantPurchasedCredits grants a one-time credit pack idempotently, keyed on
	// referenceID (the Stripe checkout session id). It returns granted=false when
	// the reference was already granted — a duplicate webhook delivery — so the
	// caller can treat it as success without re-crediting. See the PgBillingStore
	// implementation for why the webhook's marker-rollback makes this necessary.
	GrantPurchasedCredits(ctx context.Context, workspaceID string, amount int64, referenceID string) (granted bool, err error)

	// GrantTrialCredits grants the workspace's one-time trial credits
	// (source='trial', non-expiring). At most one grant per workspace, EVER —
	// keyed on the allocation table's unique constraint — so it is safe to call
	// on every touch (the allocation middleware uses it as a lazy backfill).
	// Returns granted=false when the workspace already has its grant.
	GrantTrialCredits(ctx context.Context, workspaceID string, amount int64) (granted bool, err error)

	// Ledger
	GetLedger(ctx context.Context, workspaceID string, from, to time.Time) ([]LedgerEntry, error)

	// GetLedgerPage returns one page of the ledger, filtered and counted in
	// SQL. It is what a client-facing history view reads: GetLedger returns
	// the whole window and is for callers that need every row.
	GetLedgerPage(ctx context.Context, workspaceID string, q LedgerQuery) (*LedgerPage, error)

	// GetUsageByOperation sums the debits in the window per operation, so the
	// usage breakdown does not depend on how many entries a page carries.
	GetUsageByOperation(ctx context.Context, workspaceID string, from, to time.Time) (map[string]int64, error)

	// GetNetByOperation sums EVERY movement in the window per operation, signed:
	// a debit stays negative, a purchase or grant positive.
	//
	// It exists because GetUsageByOperation answers a narrower question than its
	// key set suggests. Summing `amount < 0` only, it never mentions purchase,
	// grant, expire or plan_reset — so a client deriving the ledger's operation
	// filter from it could not offer the operations the ledger table right below
	// was displaying. This one names every operation the window contains.
	GetNetByOperation(ctx context.Context, workspaceID string, from, to time.Time) (map[string]int64, error)

	// Feature overrides
	GetFeatureOverrides(ctx context.Context, workspaceID string) ([]FeatureOverride, error)
	ListAllFeatureOverrides(ctx context.Context) ([]FeatureOverride, error)
	SetFeatureOverride(ctx context.Context, override *FeatureOverride) error
	DeleteFeatureOverride(ctx context.Context, workspaceID string, feature Feature) error

	// Notes
	ListNotes(ctx context.Context, workspaceID string) ([]WorkspaceNote, error)
	AddNote(ctx context.Context, note *WorkspaceNote) error

	// Upsells
	GetUpsellOpportunities(ctx context.Context) ([]UpsellOpportunity, error)

	// Metrics
	GetPlatformMetrics(ctx context.Context) (*PlatformMetrics, error)

	// Events
	ListBillingEvents(ctx context.Context, limit, offset int, eventType string) ([]BillingEvent, error)
	RecordBillingEvent(ctx context.Context, event *BillingEvent) error

	// Webhook idempotency
	//
	// MarkStripeEventProcessed records that a Stripe webhook event has been
	// processed. It returns alreadyProcessed=true when the event ID was already
	// present (i.e. this is a duplicate delivery), allowing callers to
	// short-circuit before re-applying side effects such as granting credits.
	// The insert uses ON CONFLICT DO NOTHING so concurrent duplicate deliveries
	// race to a single winner.
	MarkStripeEventProcessed(ctx context.Context, eventID, eventType string) (alreadyProcessed bool, err error)

	// UnmarkStripeEvent removes the processed marker for an event ID. It is used
	// to roll back the idempotency claim when downstream processing fails, so a
	// retried delivery is reprocessed instead of being silently skipped.
	UnmarkStripeEvent(ctx context.Context, eventID string) error
}
