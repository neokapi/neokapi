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

	// Ledger
	GetLedger(ctx context.Context, workspaceID string, from, to time.Time) ([]LedgerEntry, error)

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
