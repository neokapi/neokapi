package billing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// trialRecordingStore embeds mockBillingStore and records calls made by SetupTrial.
type trialRecordingStore struct {
	mockBillingStore

	upsertedSubs   []*Subscription
	recordedEvts   []*BillingEvent
	grantedCredits []creditGrant
}

type creditGrant struct {
	workspaceID string
	amount      int64
	source      string
}

func (s *trialRecordingStore) UpsertSubscription(_ context.Context, sub *Subscription) error {
	s.upsertedSubs = append(s.upsertedSubs, sub)
	return nil
}

func (s *trialRecordingStore) RecordBillingEvent(_ context.Context, evt *BillingEvent) error {
	s.recordedEvts = append(s.recordedEvts, evt)
	return nil
}

func (s *trialRecordingStore) GetCurrentAllocation(_ context.Context, _ string) (*CreditAllocation, error) {
	// Simulate no existing allocation, so a credit grant would be observable
	// in grantedCredits if SetupTrial (wrongly) tried to make one.
	return nil, errors.New("no allocation")
}

func (s *trialRecordingStore) GrantCredits(_ context.Context, workspaceID string, amount int64, source string) error {
	s.grantedCredits = append(s.grantedCredits, creditGrant{
		workspaceID: workspaceID,
		amount:      amount,
		source:      source,
	})
	return nil
}

func TestSetupTrial(t *testing.T) {
	store := &trialRecordingStore{}
	ctx := t.Context()

	SetupTrial(ctx, store, "ws-trial")

	require.Len(t, store.upsertedSubs, 1)
	sub := store.upsertedSubs[0]
	assert.Equal(t, "ws-trial", sub.WorkspaceID)
	assert.Equal(t, PlanPro, sub.Plan)
	assert.Equal(t, "trialing", sub.Status)
	assert.Equal(t, 1, sub.SeatCount)
	assert.Empty(t, sub.StripeCustomerID)
	assert.Empty(t, sub.StripeSubscriptionID)
	assert.True(t, sub.CurrentPeriodStart.IsZero())
	assert.True(t, sub.CurrentPeriodEnd.IsZero())
	assert.Nil(t, sub.CancelAt)
}

// SetupTrial grants plan FEATURES only. It must NOT grant any plan credit
// allocation: the trialing workspace's AI credits are its one-time trial grant
// (EnsureTrialGrant at workspace creation), which is what bounds per-signup
// cost exposure to FreeTrialGrantCredits instead of a paid-size allowance.
func TestSetupTrial_GrantsNoPlanCredits(t *testing.T) {
	store := &trialRecordingStore{}
	ctx := t.Context()

	SetupTrial(ctx, store, "ws-credits")

	assert.Empty(t, store.grantedCredits, "a trial must not mint a plan credit allocation")
}

func TestSetupTrial_EventRecorded(t *testing.T) {
	store := &trialRecordingStore{}
	ctx := t.Context()

	SetupTrial(ctx, store, "ws-event")

	require.Len(t, store.recordedEvts, 1)
	evt := store.recordedEvts[0]
	assert.Equal(t, "ws-event", evt.WorkspaceID)
	assert.Equal(t, "trial_started", evt.EventType)
	assert.NotEmpty(t, evt.Detail)
	assert.True(t, evt.CreatedAt.IsZero() || evt.CreatedAt.Before(time.Now().Add(time.Second)))
}

func TestSetupTrial_NilStore(t *testing.T) {
	// Must not panic when store is nil.
	assert.NotPanics(t, func() {
		SetupTrial(t.Context(), nil, "ws-nil")
	})
}
