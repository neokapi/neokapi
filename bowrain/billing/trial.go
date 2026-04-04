package billing

import (
	"context"
	"log/slog"
)

// DefaultTrialDays is the default Pro trial period for new workspaces.
const DefaultTrialDays = 14

// SetupTrial sets up a 14-day Pro trial for a new workspace.
// It creates the subscription record locally (Stripe trial is activated
// at checkout time via TrialDays). This gives the workspace Pro features
// and credits immediately.
//
// If a WorkspacePlanSyncer is provided, the workspace's cached plan field
// is updated to match the trial plan.
func SetupTrial(ctx context.Context, store BillingStore, workspaceID string, syncer ...WorkspacePlanSyncer) {
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
		slog.Info("billing: failed to set up trial for workspace", "id", workspaceID, "error", err)
		return
	}

	// Sync the plan to the workspace record so seat/project limits are correct.
	if len(syncer) > 0 && syncer[0] != nil {
		if err := syncer[0].SyncWorkspacePlan(ctx, workspaceID, string(PlanPro), ""); err != nil {
			slog.Info("billing: failed to sync trial plan for workspace", "id", workspaceID, "error", err)
		}
	}

	// Grant Pro-level weekly credits for the trial.
	if _, err := EnsureWeeklyAllocation(ctx, store, workspaceID, PlanPro); err != nil {
		slog.Info("billing: failed to allocate trial credits for workspace", "id", workspaceID, "error", err)
	}

	_ = store.RecordBillingEvent(ctx, &BillingEvent{
		WorkspaceID: workspaceID,
		EventType:   "trial_started",
		Detail:      "14-day Pro trial activated",
	})
}
