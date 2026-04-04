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
		mockBillingStore: mockBillingStore{remaining: 50000},
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
		mockBillingStore: mockBillingStore{remaining: 50000},
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
		mockBillingStore: mockBillingStore{remaining: 100},
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
		mockBillingStore: mockBillingStore{remaining: 0},
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
		mockBillingStore: mockBillingStore{remaining: 0},
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

func TestUsageHooks_ReportMeter_NilStripe(t *testing.T) {
	store := &recordingBillingStore{
		mockBillingStore: mockBillingStore{remaining: 50000},
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
