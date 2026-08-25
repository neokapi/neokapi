package server

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/billing"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
)

// Custodian seats: the one thing a plan meters about people.
//
// A custodian is derived, never declared — someone holding a permission that
// authors what governs content (voice, terms) over a bounded region. That is
// what makes the billable role and the authorization model the same fact, so a
// seat count cannot drift from what people can actually do.
//
// Two enforcements, deliberately different in kind:
//
//   - At grant time, a membership that would exceed the plan's allowance is
//     refused, with the count and the limit, so the answer is actionable.
//   - When a plan carries no custodian seats — the state a lapsed trial leaves
//     behind — existing custodial authority stops resolving. Nothing is deleted:
//     the voice, the terms, the rules and the coordinates stay exactly as they
//     are, and the authority returns the moment a plan does. Suspension and
//     deletion are different code paths, and only one of them is reachable here.

// custodianAllowance returns the plan's custodian limit and whether it binds.
// A limit of -1 is unlimited.
func custodianAllowance(plan string) (limit int, bounded bool) {
	if plan == "" {
		return -1, false
	}
	limit = billing.GetLimit(billing.Plan(plan), billing.LimitMaxCustodians)
	return limit, limit >= 0
}

// custodialAuthoritySuspended reports whether the workspace's plan carries no
// custodian seats at all, which is what a lapsed trial leaves behind.
//
// Only bounded custody lapses. Blanket authority — the workspace owner — is
// untouched, so an expired workspace can still approve its own work rather than
// being bricked by its billing state.
func custodialAuthoritySuspended(plan string) bool {
	limit, bounded := custodianAllowance(plan)
	return bounded && limit == 0
}

// countCustodiansExcept counts the distinct people holding bounded custody
// anywhere in the workspace, excluding one — the person the grant is for, whose
// seat the caller adds back once.
//
// The count is of PEOPLE, not memberships: one custodian holding three regions
// across two projects is one seat.
//
// A store failure returns ok=false and the caller allows the grant: refusing to
// add a custodian because a member list could not be read would turn a database
// blip into a billing decision.
func (s *Server) countCustodiansExcept(ctx context.Context, wsID, exceptUserID string) (int, bool) {
	if s.AuthStore == nil || s.Services == nil || s.Services.Project == nil {
		return 0, false
	}
	projects, err := s.Services.Project.ListProjects(ctx)
	if err != nil {
		slog.InfoContext(ctx, "custodian seats: cannot list projects; allowing the grant", "error", err)
		return 0, false
	}
	holders := map[string]bool{}
	for _, p := range projects {
		if p == nil || p.WorkspaceID != wsID || p.Archived {
			continue
		}
		members, err := s.AuthStore.ListProjectMembers(ctx, p.ID)
		if err != nil {
			slog.InfoContext(ctx, "custodian seats: cannot list members; allowing the grant",
				"project", p.ID, "error", err)
			return 0, false
		}
		for _, m := range members {
			if m == nil || m.UserID == "" || m.UserID == exceptUserID {
				continue
			}
			rt, err := s.AuthStore.GetRoleTemplate(ctx, m.WorkspaceID, m.RoleID)
			if err != nil || rt == nil {
				continue
			}
			if platauth.IsCustodian(rt.Permissions, platauth.CoordinateReach{}.Add(m.Coordinates)) {
				holders[m.UserID] = true
			}
		}
	}
	return len(holders), true
}

// wouldBeCustodian reports whether a proposed membership is custody: a role that
// authors what governs content, held over a bounded region.
func (s *Server) wouldBeCustodian(ctx context.Context, wsID, roleID string, coords platauth.CoordinateFilter) bool {
	if s.AuthStore == nil || coords.Unconstrained() {
		return false
	}
	rt, err := s.AuthStore.GetRoleTemplate(ctx, wsID, roleID)
	if err != nil || rt == nil {
		return false
	}
	return platauth.IsCustodian(rt.Permissions, platauth.CoordinateReach{}.Add(coords))
}

// guardCustodianSeat refuses a membership that would put the workspace over its
// custodian allowance. Returns nil when the grant is not custody, when the plan
// does not bound it, or when the count cannot be established.
//
// targetUserID is the person the membership is for, on both the add and the
// update path. They are excluded from the count and then added back once, so the
// seat is a *person*: someone who already holds custody elsewhere gaining a
// second region is still one custodian, and re-saving an existing custodian does
// not count their own seat against them.
func (s *Server) guardCustodianSeat(c echo.Context, projectID, roleID string, coords platauth.CoordinateFilter, targetUserID string) error {
	ctx := c.Request().Context()
	wsID, _ := c.Get("workspace_id").(string)
	plan, _ := c.Get("workspace_plan").(string)
	if wsID == "" {
		return nil
	}
	limit, bounded := custodianAllowance(plan)
	if !bounded {
		return nil
	}
	if !s.wouldBeCustodian(ctx, wsID, roleID, coords) {
		return nil
	}
	others, ok := s.countCustodiansExcept(ctx, wsID, targetUserID)
	if !ok {
		return nil
	}
	if others+1 <= limit {
		return nil
	}
	// apiErr writes the response and returns c.JSON's error, which is nil on
	// success — so returning it directly would let the caller's `err != nil`
	// check fall through and grant the membership anyway, behind a 403 the client
	// had already been sent. The refusal is the sentinel, exactly as deny() does
	// it (see handlers_forge_setup.go).
	_ = apiErr(c, http.StatusForbidden, "custodian_limit_reached", map[string]any{
		"current": others + 1,
		"limit":   limit,
	})
	return errAccessDenied
}

// recordCustodyLapse notes, once per request, that a caller's bounded custody
// did not resolve because the workspace's plan carries no custodian seats.
//
// It is an audit line rather than a denial: the caller is not doing anything
// wrong, and the surface they are on will simply not offer the governing action.
// The line is what makes a lapsed trial legible afterwards — otherwise the only
// evidence would be a person reporting that a button disappeared.
func (s *Server) recordCustodyLapse(c echo.Context, reach platauth.CoordinateReach) {
	s.emitAudit(c, auditEvent{
		Type:   platev.EventAuthzDenied,
		Effect: "suspend",
		Data: map[string]string{
			"reason":      "custodian_seats_unavailable",
			"coordinates": reach.String(),
			"path":        c.Path(),
		},
	})
}
