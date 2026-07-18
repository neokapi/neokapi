package billing

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// DefaultTrialDays is the default Pro trial period for new workspaces.
const DefaultTrialDays = 14

// SetupTrial sets up a 14-day Pro trial for a new workspace, using the product
// defaults (PlanPro, DefaultTrialDays). See SetupTrialWith for the config-driven
// variant (ctrl-managed new-workspace plan/trial defaults).
func SetupTrial(ctx context.Context, store BillingStore, workspaceID string, syncer ...WorkspacePlanSyncer) {
	SetupTrialWith(ctx, store, workspaceID, PlanPro, DefaultTrialDays, syncer...)
}

// SetupTrialWith sets up a trial for a new workspace on the given plan and
// duration. This is the config-driven entry point: callers pass the instance's
// ctrl-managed new-workspace defaults (platformconfig DefaultPlan / TrialDays) so
// an admin can change the plan and trial length without a redeploy. An invalid
// plan falls back to PlanPro and a non-positive trialDays to DefaultTrialDays, so
// a misconfigured value never breaks signup.
//
// The trial is local and card-free: no Stripe subscription exists for it, so
// nothing in Stripe ends it. Its whole lifecycle is the trial_ends_at deadline
// written here and the TrialSweeper that enforces it. (A Stripe-side trial via
// CheckoutOptions.TrialDays would mean collecting a card at signup, which is the
// wrong trade for a self-serve launch — see AD-018.)
//
// The trial grants the plan's FEATURES, not its monthly credit allowance: the
// `trialing` status makes EnsureMonthlyAllocation skip the paid plan grant, and
// the workspace's AI credits during (and after) the trial are its one-time
// trial grant (EnsureTrialGrant, made at workspace creation). This is what
// bounds per-signup cost exposure to FreeTrialGrantCredits.
//
// If a WorkspacePlanSyncer is provided, the workspace's cached plan field
// is updated to match the trial plan.
func SetupTrialWith(ctx context.Context, store BillingStore, workspaceID string, plan Plan, trialDays int, syncer ...WorkspacePlanSyncer) {
	if store == nil {
		return
	}
	if !ValidPlans[plan] {
		plan = PlanPro
	}
	if trialDays <= 0 {
		trialDays = DefaultTrialDays
	}

	trialEnds := time.Now().UTC().AddDate(0, 0, trialDays)
	sub := &Subscription{
		WorkspaceID: workspaceID,
		Plan:        plan,
		Status:      "trialing",
		SeatCount:   1,
		TrialEndsAt: &trialEnds,
	}
	if err := store.UpsertSubscription(ctx, sub); err != nil {
		slog.Info("billing: failed to set up trial for workspace", "id", workspaceID, "error", err)
		return
	}

	// Sync the plan to the workspace record so seat/project limits are correct.
	if len(syncer) > 0 && syncer[0] != nil {
		if err := syncer[0].SyncWorkspacePlan(ctx, workspaceID, string(plan), ""); err != nil {
			slog.Info("billing: failed to sync trial plan for workspace", "id", workspaceID, "error", err)
		}
	}

	_ = store.RecordBillingEvent(ctx, &BillingEvent{
		WorkspaceID: workspaceID,
		EventType:   "trial_started",
		Detail:      fmt.Sprintf("%d-day %s trial activated", trialDays, plan),
	})
}
