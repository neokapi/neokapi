package billing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests verify the BillingStore interface contract using the mock store.
// Integration tests against a real database would require a running PostgreSQL instance.

func TestMockStore_SubscriptionRoundtrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping store tests in short mode")
	}

	store := &mockBillingStore{}

	// Verify the interface is satisfied.
	var _ BillingStore = store

	ctx := t.Context()

	// GetSubscription returns nil for mock.
	sub, err := store.GetSubscription(ctx, "ws-1")
	require.NoError(t, err)
	assert.Nil(t, sub)
}

func TestMockStore_CheckCredits(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping store tests in short mode")
	}

	tests := []struct {
		name      string
		remaining int64
		err       error
		wantErr   bool
	}{
		{"positive remaining", 10000, nil, false},
		{"zero remaining", 0, nil, false},
		{"negative remaining", -500, nil, false},
		{"store error", 0, assert.AnError, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockBillingStore{remaining: tt.remaining, err: tt.err}
			remaining, err := store.CheckCredits(t.Context(), "ws-1")
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.remaining, remaining)
			}
		})
	}
}

func TestSubscription_FieldDefaults(t *testing.T) {
	sub := &Subscription{
		WorkspaceID:      "ws-test",
		StripeCustomerID: "cus_test",
		Plan:             PlanFree,
		Status:           "active",
		SeatCount:        1,
	}

	assert.Equal(t, PlanFree, sub.Plan)
	assert.Equal(t, "active", sub.Status)
	assert.Equal(t, 1, sub.SeatCount)
	assert.Nil(t, sub.CancelAt)
}

func TestCreditAllocation_Fields(t *testing.T) {
	now := time.Now().UTC()
	alloc := &CreditAllocation{
		ID:           "alloc-1",
		WorkspaceID:  "ws-1",
		CreditsTotal: 500000,
		CreditsUsed:  123456,
		WeekStart:    WeekStart(now),
		WeekEnd:      WeekEnd(now),
		Source:       "plan",
	}

	assert.Equal(t, int64(500000), alloc.CreditsTotal)
	assert.Equal(t, int64(123456), alloc.CreditsUsed)
	assert.Equal(t, "plan", alloc.Source)
	remaining := alloc.CreditsTotal - alloc.CreditsUsed
	assert.Equal(t, int64(376544), remaining)
}

func TestLedgerEntry_Fields(t *testing.T) {
	entry := LedgerEntry{
		WorkspaceID:  "ws-1",
		Amount:       -1000,
		BalanceAfter: 49000,
		Operation:    "ai_translation",
		ReferenceID:  "job-123",
	}

	assert.Less(t, entry.Amount, 0, "debit should be negative")
	assert.Equal(t, "ai_translation", entry.Operation)
}

func TestFeatureOverride_Expiry(t *testing.T) {
	future := time.Now().Add(24 * time.Hour)
	past := time.Now().Add(-24 * time.Hour)

	active := FeatureOverride{
		Feature:   FeatureConnectorsGit,
		Enabled:   true,
		ExpiresAt: &future,
	}
	assert.True(t, active.ExpiresAt.After(time.Now()), "active override should not be expired")

	expired := FeatureOverride{
		Feature:   FeatureConnectorsGit,
		Enabled:   true,
		ExpiresAt: &past,
	}
	assert.True(t, expired.ExpiresAt.Before(time.Now()), "expired override should be in the past")
}

func TestBillingEvent_Types(t *testing.T) {
	eventTypes := []string{
		"subscription_created",
		"subscription_updated",
		"subscription_deleted",
		"payment_failed",
		"invoice_paid",
		"feature_override",
		"feature_gate_hit",
	}

	for _, et := range eventTypes {
		event := BillingEvent{
			WorkspaceID: "ws-1",
			EventType:   et,
			Detail:      "test event",
		}
		assert.Equal(t, et, event.EventType)
	}
}

func TestPlatformMetrics_Fields(t *testing.T) {
	m := PlatformMetrics{
		MRR:                  12500.00,
		ActiveWorkspaces:     150,
		NewSignups7d:         23,
		NewSignups30d:        87,
		CreditUtilizationPct: 65.5,
		ChurnRate:            2.3,
	}

	assert.Equal(t, 150, m.ActiveWorkspaces)
	assert.InDelta(t, 12500.00, m.MRR, 0.01)
	assert.InDelta(t, 65.5, m.CreditUtilizationPct, 0.01)
}

func TestUpsellOpportunity_Signals(t *testing.T) {
	signals := []string{
		"credit_exhaustion",
		"seat_pressure",
		"feature_gate_hits",
		"high_usage",
		"dormant_paid",
	}

	for _, sig := range signals {
		opp := UpsellOpportunity{
			WorkspaceID:   "ws-1",
			CurrentPlan:   PlanFree,
			Signal:        sig,
			Score:         80,
			SuggestedPlan: PlanPro,
		}
		assert.Equal(t, sig, opp.Signal)
		assert.Greater(t, opp.Score, 0)
	}
}

func TestWorkspaceNote_Fields(t *testing.T) {
	note := WorkspaceNote{
		WorkspaceID: "ws-1",
		AuthorEmail: "admin@bowrain.cloud",
		Content:     "Reached out about upgrade",
		CreatedAt:   time.Now().UTC(),
	}

	require.NotEmpty(t, note.AuthorEmail)
	require.NotEmpty(t, note.Content)
}
