package billing

import (
	"context"
	"log"
)

// DefaultTrialDays is the default Pro trial period for new workspaces.
const DefaultTrialDays = 14

// SetupTrial sets up a 14-day Pro trial for a new workspace.
// It creates the subscription record locally (Stripe trial is activated
// at checkout time via TrialDays). This gives the workspace Pro features
// and credits immediately.
func SetupTrial(ctx context.Context, store BillingStore, workspaceID string) {
	if store == nil {
		return
	}

	sub := &Subscription{
		WorkspaceID: workspaceID,
		Plan:        PlanPro,
		Status:      "trialing",
		SeatCount:   1,
	}
	if err := store.UpsertSubscription(ctx, sub); err != nil {
		log.Printf("billing: failed to set up trial for workspace %s: %v", workspaceID, err)
		return
	}

	// Grant Pro-level weekly credits for the trial.
	if _, err := EnsureWeeklyAllocation(ctx, store, workspaceID, PlanPro); err != nil {
		log.Printf("billing: failed to allocate trial credits for workspace %s: %v", workspaceID, err)
	}

	_ = store.RecordBillingEvent(ctx, &BillingEvent{
		WorkspaceID: workspaceID,
		EventType:   "trial_started",
		Detail:      "14-day Pro trial activated",
	})
}
