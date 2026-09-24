package auth

import (
	"context"
	"fmt"
)

// PlanSyncer updates the workspace's cached plan and Stripe customer ID from
// billing subscription state. It implements billing.WorkspacePlanSyncer without
// requiring billing to import auth.
//
// Authorization reads the cache on each request. Subscription writers use this
// adapter to update it, except the trial sweeper, which updates subscription and
// workspace rows atomically in the billing store.
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
