package server

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/bowrain/auth"
	"github.com/neokapi/neokapi/bowrain/billing"
	platauth "github.com/neokapi/neokapi/platform/auth"
)

// planSyncAdapter implements billing.WorkspacePlanSyncer by updating the
// workspace's cached Plan and StripeCustomerID via the AuthStore.
type planSyncAdapter struct {
	authStore auth.AuthStore
}

func (a *planSyncAdapter) SyncWorkspacePlan(ctx context.Context, workspaceID, plan, stripeCustomerID string) error {
	w, err := a.authStore.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("get workspace %s: %w", workspaceID, err)
	}
	w.Plan = plan
	if stripeCustomerID != "" {
		w.StripeCustomerID = stripeCustomerID
	}
	return a.authStore.UpdateWorkspace(ctx, w)
}

// ownerEmailResolver resolves the workspace owner's email for billing notifications.
type ownerEmailResolver struct {
	authStore auth.AuthStore
}

func (r *ownerEmailResolver) GetOwnerEmail(ctx context.Context, workspaceID string) string {
	members, err := r.authStore.ListMembers(ctx, workspaceID)
	if err != nil {
		return ""
	}
	for _, m := range members {
		if m.Role == platauth.RoleOwner {
			u, err := r.authStore.GetUser(ctx, m.UserID)
			if err == nil {
				return u.Email
			}
		}
	}
	return ""
}

// billingGuardEvent returns a GuardEventFunc that fires PostHog events.
// Returns nil (no-op) when PostHog is not configured.
func (s *Server) billingGuardEvent() billing.GuardEventFunc {
	if s.PostHogClient == nil {
		return nil
	}
	return func(event string, workspaceID string, props map[string]any) {
		s.PostHogClient.CaptureEvent(workspaceID, event, props)
	}
}
