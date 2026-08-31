package billing

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingBillingStore embeds mockBillingStore and records DeductCredits calls.
type recordingBillingStore struct {
	mockBillingStore

	deductedAmount int64
	deductedOp     string
	deductedRefID  string
	deductCalls    int
}

func (r *recordingBillingStore) DeductCredits(_ context.Context, _ string, amount int64, op string, refID string) error {
	r.deductCalls++
	r.deductedAmount = amount
	r.deductedOp = op
	r.deductedRefID = refID
	return r.err
}

// mockEmailSender records Send calls.
type mockEmailSender struct {
	sendCalls int
	lastTo    string
	lastSubj  string
	lastBody  string
}

func (m *mockEmailSender) Send(_ context.Context, to, subject, htmlBody string) error {
	m.sendCalls++
	m.lastTo = to
	m.lastSubj = subject
	m.lastBody = htmlBody
	return nil
}

func TestUsageHooks_DeductTokens(t *testing.T) {
	store := &recordingBillingStore{
		remaining: 50000,
	}
	h := &UsageHooks{
		Store: store,
	}

	ctx := t.Context()
	h.DeductTokens(ctx, "ws-1", 500, "ai_translation", "job-42")

	assert.Equal(t, 1, store.deductCalls)
	assert.Equal(t, TokensToCredits(500), store.deductedAmount)
	assert.Equal(t, "ai_translation", store.deductedOp)
	assert.Equal(t, "job-42", store.deductedRefID)
}

func TestUsageHooks_DeductTokens_NilReceiver(t *testing.T) {
	var h *UsageHooks
	require.NotPanics(t, func() {
		h.DeductTokens(t.Context(), "ws-1", 100, "op", "ref")
	})
}

func TestUsageHooks_DeductTokens_NilStore(t *testing.T) {
	h := &UsageHooks{Store: nil}
	require.NotPanics(t, func() {
		h.DeductTokens(t.Context(), "ws-1", 100, "op", "ref")
	})
}

func TestUsageHooks_DeductContainerTime(t *testing.T) {
	store := &recordingBillingStore{
		remaining: 50000,
	}
	h := &UsageHooks{
		Store: store,
	}

	dur := 30 * time.Second
	ctx := t.Context()
	h.DeductContainerTime(ctx, "ws-2", dur, "run-99")

	assert.Equal(t, 1, store.deductCalls)
	assert.Equal(t, ContainerTimeCredits(dur), store.deductedAmount)
	assert.Equal(t, "bravo_container", store.deductedOp)
	assert.Equal(t, "run-99", store.deductedRefID)
}

func TestUsageHooks_DeductContainerTime_NilReceiver(t *testing.T) {
	var h *UsageHooks
	require.NotPanics(t, func() {
		h.DeductContainerTime(t.Context(), "ws-1", 5*time.Second, "ref")
	})
}

func TestUsageHooks_CheckCreditThresholds_Warning(t *testing.T) {
	sender := &mockEmailSender{}
	// remaining=1000 out of total=1000+1000=2000 means 50% used -- not enough.
	// We need 80%+ usage. With mockBillingStore: total = remaining + 1000, used = total - remaining.
	// For 80%: used/total >= 0.8 => (total - remaining)/total >= 0.8
	// => remaining/total <= 0.2 => remaining <= 0.2 * (remaining + 1000)
	// => 0.8*remaining <= 200 => remaining <= 250.
	// Use remaining=100: total=1100, used=1000, pct=1000/1100=0.909 > 0.8
	store := &recordingBillingStore{
		remaining: 100,
	}
	notifier := &BillingNotifier{Sender: sender, Store: store}
	h := &UsageHooks{
		Store:    store,
		Notifier: notifier,
		GetOwnerEmail: func(_ context.Context, _ string) string {
			return "owner@example.com"
		},
	}

	h.DeductTokens(t.Context(), "ws-warn", 10, "ai_translation", "ref-1")

	assert.Equal(t, 1, sender.sendCalls)
	assert.Equal(t, "owner@example.com", sender.lastTo)
	assert.Contains(t, sender.lastSubj, "running low")
}

func TestUsageHooks_CheckCreditThresholds_Exhausted(t *testing.T) {
	sender := &mockEmailSender{}
	// remaining=0 triggers the exhausted path in mockBillingStore:
	// total=50000, used=50000, remaining=0.
	store := &recordingBillingStore{
		remaining: 0,
	}
	notifier := &BillingNotifier{Sender: sender, Store: store}
	h := &UsageHooks{
		Store:    store,
		Notifier: notifier,
		GetOwnerEmail: func(_ context.Context, _ string) string {
			return "exhausted@example.com"
		},
	}

	h.DeductTokens(t.Context(), "ws-exhausted", 10, "ai_translation", "ref-2")

	assert.Equal(t, 1, sender.sendCalls)
	assert.Equal(t, "exhausted@example.com", sender.lastTo)
	assert.Contains(t, sender.lastSubj, "exhausted")
}

func TestUsageHooks_CheckCreditThresholds_NoNotifier(t *testing.T) {
	store := &recordingBillingStore{
		remaining: 0,
	}
	h := &UsageHooks{
		Store:    store,
		Notifier: nil, // no notifier
	}

	require.NotPanics(t, func() {
		h.DeductTokens(t.Context(), "ws-1", 10, "op", "ref")
	})
	// DeductCredits should still have been called.
	assert.Equal(t, 1, store.deductCalls)
}

// thresholdFakeStore decouples the plan-only allocation from the full spendable
// balance so the MINOR-7 case can be expressed: a depleted monthly plan bucket
// while purchased credits keep CheckCredits positive. It embeds BillingStore so
// only the two methods checkCreditThresholds calls are provided.
type thresholdFakeStore struct {
	BillingStore
	alloc          *CreditAllocation
	checkRemaining int64
}

func (f *thresholdFakeStore) GetCurrentAllocation(context.Context, string) (*CreditAllocation, error) {
	return f.alloc, nil
}

func (f *thresholdFakeStore) CheckCredits(context.Context, string) (int64, error) {
	return f.checkRemaining, nil
}

// TestUsageHooks_CheckCreditThresholds_PurchasedSuppressesExhaustion is the
// MINOR-7 regression: when the monthly PLAN bucket is fully consumed but
// purchased credits keep the spendable balance positive, the workspace must not
// be told it is out of credits. Exhaustion is judged on CheckCredits (plan +
// trial + purchased), not the plan-only allocation.
func TestUsageHooks_CheckCreditThresholds_PurchasedSuppressesExhaustion(t *testing.T) {
	sender := &mockEmailSender{}
	h := &UsageHooks{
		Store: &thresholdFakeStore{
			alloc:          &CreditAllocation{CreditsTotal: 50_000, CreditsUsed: 50_000}, // plan drained
			checkRemaining: 500_000,                                                      // purchased still spendable
		},
		Notifier:      &BillingNotifier{Sender: sender},
		GetOwnerEmail: func(context.Context, string) string { return "owner@example.com" },
	}

	h.checkCreditThresholds(t.Context(), "ws-1")

	require.Equal(t, 1, sender.sendCalls, "the 80%-plan-used warning still fires")
	assert.Contains(t, sender.lastSubj, "running low")
	assert.NotContains(t, sender.lastSubj, "exhausted",
		"must not send 'exhausted' while purchased credits remain spendable")
}

// TestUsageHooks_CheckCreditThresholds_ExhaustedOnZeroSpendable verifies the
// exhausted notice still fires when the full spendable balance is zero.
func TestUsageHooks_CheckCreditThresholds_ExhaustedOnZeroSpendable(t *testing.T) {
	sender := &mockEmailSender{}
	h := &UsageHooks{
		Store: &thresholdFakeStore{
			alloc:          &CreditAllocation{CreditsTotal: 50_000, CreditsUsed: 50_000},
			checkRemaining: 0,
		},
		Notifier:      &BillingNotifier{Sender: sender},
		GetOwnerEmail: func(context.Context, string) string { return "owner@example.com" },
	}

	h.checkCreditThresholds(t.Context(), "ws-1")

	require.Equal(t, 1, sender.sendCalls)
	assert.Contains(t, sender.lastSubj, "exhausted")
}

func TestUsageHooks_ReportMeter_NilStripe(t *testing.T) {
	store := &recordingBillingStore{
		remaining: 50000,
	}
	h := &UsageHooks{
		Store:  store,
		Stripe: nil, // nil Stripe client
	}

	require.NotPanics(t, func() {
		h.DeductTokens(t.Context(), "ws-1", 100, "ai_translation", "ref")
	})
	require.NotPanics(t, func() {
		h.DeductContainerTime(t.Context(), "ws-1", 10*time.Second, "ref")
	})
}
