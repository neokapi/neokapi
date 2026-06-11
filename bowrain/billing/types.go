package billing

import "time"

// Subscription represents a workspace's billing subscription.
// The source of truth is Stripe; this is a local cache for fast access.
type Subscription struct {
	ID                   string     `json:"id"`
	WorkspaceID          string     `json:"workspace_id"`
	StripeCustomerID     string     `json:"stripe_customer_id"`
	StripeSubscriptionID string     `json:"stripe_subscription_id,omitempty"`
	Plan                 Plan       `json:"plan"`
	Status               string     `json:"status"` // active, past_due, canceled, trialing
	SeatCount            int        `json:"seat_count"`
	CurrentPeriodStart   time.Time  `json:"current_period_start,omitzero"`
	CurrentPeriodEnd     time.Time  `json:"current_period_end,omitzero"`
	CancelAt             *time.Time `json:"cancel_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// CreditAllocation tracks weekly credit usage for a workspace.
type CreditAllocation struct {
	ID           string    `json:"id"`
	WorkspaceID  string    `json:"workspace_id"`
	CreditsTotal int64     `json:"credits_total"`
	CreditsUsed  int64     `json:"credits_used"`
	WeekStart    time.Time `json:"week_start"`
	WeekEnd      time.Time `json:"week_end"`
	Source       string    `json:"source"` // "plan" or "purchased"
	CreatedAt    time.Time `json:"created_at"`
}

// LedgerEntry is an immutable record of a credit transaction.
type LedgerEntry struct {
	ID           int64     `json:"id"`
	WorkspaceID  string    `json:"workspace_id"`
	AllocationID string    `json:"allocation_id,omitempty"`
	Amount       int64     `json:"amount"` // negative = debit, positive = credit
	BalanceAfter int64     `json:"balance_after"`
	Operation    string    `json:"operation"` // ai_translation, bravo_message, bravo_container, purchase, grant, expire
	ReferenceID  string    `json:"reference_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// FeatureOverride allows per-workspace feature grants or revocations
// that override the plan matrix. Managed via the control plane.
type FeatureOverride struct {
	ID          string     `json:"id"`
	WorkspaceID string     `json:"workspace_id"`
	Feature     Feature    `json:"feature"`
	Enabled     bool       `json:"enabled"`
	Reason      string     `json:"reason,omitempty"`
	CreatedBy   string     `json:"created_by"` // admin email
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

// WorkspaceNote is an internal note attached to a workspace by an admin.
type WorkspaceNote struct {
	ID          int64     `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	AuthorEmail string    `json:"author_email"`
	Content     string    `json:"content"`
	CreatedAt   time.Time `json:"created_at"`
}

// UpsellOpportunity represents a workspace that may be a candidate for upgrade.
type UpsellOpportunity struct {
	WorkspaceID   string    `json:"workspace_id"`
	WorkspaceName string    `json:"workspace_name"`
	CurrentPlan   Plan      `json:"current_plan"`
	Signal        string    `json:"signal"` // credit_exhaustion, seat_pressure, feature_gate_hits, high_usage, dormant_paid
	Score         int       `json:"score"`  // priority score (higher = more urgent)
	Detail        string    `json:"detail"` // human-readable detail
	SuggestedPlan Plan      `json:"suggested_plan"`
	DetectedAt    time.Time `json:"detected_at"`
}

// PlatformMetrics holds platform-wide KPIs for the admin dashboard.
type PlatformMetrics struct {
	MRR                  float64 `json:"mrr"`
	ActiveWorkspaces     int     `json:"active_workspaces"`
	NewSignups7d         int     `json:"new_signups_7d"`
	NewSignups30d        int     `json:"new_signups_30d"`
	CreditUtilizationPct float64 `json:"credit_utilization_pct"`
	ChurnRate            float64 `json:"churn_rate"`
}

// BillingEvent represents a billing-related event for the admin event feed.
type BillingEvent struct {
	ID          int64     `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	EventType   string    `json:"event_type"` // subscription_created, subscription_updated, payment_failed, credits_purchased, plan_changed, feature_override
	Detail      string    `json:"detail"`
	CreatedAt   time.Time `json:"created_at"`
}
