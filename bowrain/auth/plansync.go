package auth

import (
	"context"
	"fmt"
)

// PlanSyncer keeps the workspace record's cached plan (and Stripe customer ID) in
// step with the billing subscription that is the source of truth. It satisfies
// billing.WorkspacePlanSyncer structurally, so the billing package needs no
// dependency on auth.
//
// The cache exists because authorization reads the plan on every request
// (WorkspaceAccessMiddleware → PlanGuard/QuotaGuard) and must not call the
// billing store to do it. Every writer of a subscription — the checkout webhook,
// the admin plan override, workspace creation's trial, and the trial sweeper —
// syncs through here, which is why it lives in auth rather than in any one of
// them.
type PlanSyncer struct {
	Store AuthStore
}

// NewPlanSyncer builds a PlanSyncer over the given store.
func NewPlanSyncer(store AuthStore) *PlanSyncer {
	return &PlanSyncer{Store: store}
}

// SyncWorkspacePlan writes the plan onto the workspace record. An empty
// stripeCustomerID leaves the existing one alone: callers that have no customer
// (a local trial, an admin override) must not erase the customer of a workspace
// that has one.
func (p *PlanSyncer) SyncWorkspacePlan(ctx context.Context, workspaceID, plan, stripeCustomerID string) error {
	w, err := p.Store.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("get workspace %s: %w", workspaceID, err)
	}
	w.Plan = plan
	if stripeCustomerID != "" {
		w.StripeCustomerID = stripeCustomerID
	}
	return p.Store.UpdateWorkspace(ctx, w)
}
