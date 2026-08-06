package server

import (
	"context"

	"github.com/labstack/echo/v4"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/knowledge"
)

// This file holds the two rules that decide who may author and who may review a
// governed change-set. Both exist because the common one-person story is that
// an AI authors and pushes through CI, and the human whose workspace it is has
// to be able to review that — while a change-set a person wrote themselves must
// still find a second pair of eyes wherever a second pair exists.
//
//   - authorIdentity separates the author of a change from the account that
//     carried it. A push mediated by the loop is authored by the machine.
//   - soloReviewOverride is the narrow exception to separation of duties: when
//     nobody else in the workspace may review, the owner may review their own
//     work, and the record says so.

// ---------------------------------------------------------------------------
// Authorship
// ---------------------------------------------------------------------------

// authorIdentity is the identity that work created on this request is recorded
// against.
//
// For an ordinary session or a personal API token this is the user ID, exactly
// as before. For a machine API token (one minted with an agent name) it is
// "agent/<name>" — the machine, not the person whose token it is.
//
// Authorization is deliberately not routed through here: every permission check
// still reads user_id. A machine can do no more than its token's owner can. The
// only thing that moves is the name on the work, and the consequence that
// matters is that the owner is not the author, so the owner can review it.
func authorIdentity(c echo.Context) string {
	if machine, _ := c.Get("author_identity").(string); machine != "" {
		return machine
	}
	actor, _ := c.Get("user_id").(string)
	return actor
}

// onBehalfOf names the accountable person when the request's author is a
// machine, and is empty otherwise. It is what keeps a machine identity from
// being an anonymous one: every machine-authored record can be traced back to
// the human who minted the token.
func onBehalfOf(c echo.Context) string {
	if machine, _ := c.Get("author_identity").(string); machine == "" {
		return ""
	}
	actor, _ := c.Get("user_id").(string)
	return actor
}

// ---------------------------------------------------------------------------
// The solo-reviewer override
// ---------------------------------------------------------------------------

// reviewPermission is the permission a workspace member must hold to review a
// governed change-set. It is the same gate HandleApproveChangeSet applies to
// the caller, so "eligible reviewer" means the same thing on both sides of the
// question.
const reviewPermission = platauth.PermManageBrand

// soloReviewOverride reports whether actor may review their own change-set
// because there is nobody else in the workspace who could.
//
// Both conditions must hold:
//
//   - actor is the workspace owner. A member who happens to be alone on a
//     permission list does not get to self-approve; the override is the owner's
//     to exercise, because the owner is who the workspace ultimately answers
//     for.
//   - no other member of the workspace holds the review permission. The moment
//     a second eligible reviewer exists the override is gone — not softened,
//     not warned about — and it is checked live on every attempt rather than
//     cached.
//
// It fails closed. No auth store, an unreadable membership list, an unknown
// workspace: no override, and the ordinary separation-of-duties refusal stands.
// Deny rules are likewise not consulted, which can only overcount eligible
// reviewers — and overcounting withholds the override, which is the safe
// direction to be wrong in.
func (s *Server) soloReviewOverride(ctx context.Context, wsID, actor string, cs *knowledge.ChangeSet) bool {
	if s.AuthStore == nil || wsID == "" || actor == "" || cs == nil {
		return false
	}
	// Only ever relevant to a self-review. A machine-authored change-set never
	// reaches here: its author is "agent/<name>", which no user ID equals.
	if actor != cs.CreatedBy {
		return false
	}

	membership, err := s.AuthStore.GetMembership(ctx, wsID, actor)
	if err != nil || membership == nil || membership.Role != platauth.RoleOwner {
		return false
	}

	members, err := s.AuthStore.ListMembers(ctx, wsID)
	if err != nil {
		return false
	}
	for _, m := range members {
		if m == nil || m.UserID == actor {
			continue
		}
		if s.memberCanReview(ctx, wsID, m) {
			return false
		}
	}
	return true
}

// memberCanReview reports whether a workspace member holds the review
// permission, honoring a per-workspace override of their role's defaults — the
// same resolution order resolveWorkspacePermissions applies to a live request,
// so a member who could review cannot be missed here.
func (s *Server) memberCanReview(ctx context.Context, wsID string, m *platauth.Membership) bool {
	if perms, ok, err := s.AuthStore.GetWorkspaceRoleOverride(ctx, wsID, m.Role); err == nil && ok {
		return perms.Has(reviewPermission)
	}
	resolved := platauth.DefaultPermissionsForRole(m.Role)
	return resolved != nil && resolved.Permissions.Has(reviewPermission)
}
