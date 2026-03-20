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
}
