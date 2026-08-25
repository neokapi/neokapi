package server

import (
	"context"
	"log/slog"
	"sort"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/store"
)

// Custody coverage: which of the workspace's points have someone who actually
// holds them, and which have nobody.
//
// The list is one derivation with three readers. It is the operator's queue
// (who do we still need), the buyer's exposure (here is the content nobody
// governs), and the unit the pricing envelope counts. Deriving it twice would
// let the three disagree, so it hangs off the same points the profile list
// already computes rather than off a query of its own.
//
// It reports and never blocks. An ungoverned point is an org-chart gap, not a
// content defect, and the standing rule is that a build must not hard-fail
// because the people side has not caught up. There is deliberately no gate here
// to turn on.

// ContextProfileCustodian is one person holding a point.
type ContextProfileCustodian struct {
	UserID string `json:"user_id"`
	Name   string `json:"name,omitempty"`
	Email  string `json:"email,omitempty"`
	// Scope renders the region their membership names — empty for someone whose
	// authority is not bounded to a region.
	Scope string `json:"scope,omitempty"`
	// ProjectID names the membership that grants the custody, since a person can
	// hold the same point through more than one project.
	ProjectID string `json:"project_id"`
}

// ContextProfileCustody is who governs one point.
type ContextProfileCustody struct {
	// Covered is true when at least one person holds this point specifically.
	// Blanket authority does not make a point covered: the question the report
	// answers is which content has nobody who knows it, and an owner who can
	// approve everything is the fallback rather than the answer.
	Covered bool `json:"covered"`
	// Custodians are the people whose bounded region reaches this point and who
	// may author what governs it.
	Custodians []ContextProfileCustodian `json:"custodians"`
	// Fallback are the people who can act here only because their authority is
	// unbounded. Reported so an uncovered point reads as "falls back to the
	// owner" rather than as "nobody can act".
	Fallback []ContextProfileCustodian `json:"fallback,omitempty"`
}

// custodyIndex accumulates who may govern what across a workspace's projects.
type custodyIndex struct {
	// bounded holds custodians by the region they govern.
	bounded []boundCustodian
	// unbounded holds people with custodial authority over the whole space.
	unbounded []ContextProfileCustodian
	roles     map[string]platauth.Permission // workspace|roleID -> permissions
	users     map[string]*platauth.User
}

type boundCustodian struct {
	who   ContextProfileCustodian
	reach platauth.CoordinateReach
}

// buildCustodyIndex reads the memberships of every project in the workspace and
// sorts them into bounded custodians and blanket authority.
//
// Store failures are skipped rather than fatal, matching the rest of this
// surface: a project whose members cannot be read leaves the rest of the board
// standing. The consequence is that a read failure makes a point look *less*
// covered than it is, which is the safe direction for a report — it overstates
// the gap rather than hiding it.
func (s *Server) buildCustodyIndex(ctx context.Context, wsID string, projects []*store.Project) *custodyIndex {
	idx := &custodyIndex{
		roles: map[string]platauth.Permission{},
		users: map[string]*platauth.User{},
	}
	if s.AuthStore == nil {
		return idx
	}

	seen := map[string]bool{} // userID|projectID|scope
	for _, p := range projects {
		if p == nil || p.WorkspaceID != wsID || p.Archived {
			continue
		}
		members, err := s.AuthStore.ListProjectMembers(ctx, p.ID)
		if err != nil {
			slog.InfoContext(ctx, "custody coverage: cannot list project members; the point will read as less covered",
				"project", p.ID, "error", err)
			continue
		}
		for _, m := range members {
			if m == nil || m.UserID == "" {
				continue
			}
			perms, ok := idx.permissionsFor(ctx, s, m.WorkspaceID, m.RoleID)
			if !ok || perms&platauth.CustodialPermissions == 0 {
				continue
			}
			reach := platauth.CoordinateReach{}.Add(m.Coordinates)
			who := ContextProfileCustodian{
				UserID:    m.UserID,
				Scope:     m.Coordinates.String(),
				ProjectID: p.ID,
			}
			key := m.UserID + "|" + p.ID + "|" + who.Scope
			if seen[key] {
				continue
			}
			seen[key] = true
			idx.fill(ctx, s, &who)

			if platauth.IsCustodian(perms, reach) {
				idx.bounded = append(idx.bounded, boundCustodian{who: who, reach: reach})
				continue
			}
			idx.unbounded = append(idx.unbounded, who)
		}
	}
	return idx
}

// permissionsFor resolves a role template's permissions, memoized per workspace
// and role so a workspace with many members reads each template once.
func (i *custodyIndex) permissionsFor(ctx context.Context, s *Server, wsID, roleID string) (platauth.Permission, bool) {
	key := wsID + "|" + roleID
	if perms, ok := i.roles[key]; ok {
		return perms, true
	}
	rt, err := s.AuthStore.GetRoleTemplate(ctx, wsID, roleID)
	if err != nil || rt == nil {
		return 0, false
	}
	i.roles[key] = rt.Permissions
	return rt.Permissions, true
}

// fill attaches the display identity, memoized per user. A lookup failure
// leaves the id, which still names the person unambiguously.
func (i *custodyIndex) fill(ctx context.Context, s *Server, who *ContextProfileCustodian) {
	u, ok := i.users[who.UserID]
	if !ok {
		u, _ = s.AuthStore.GetUser(ctx, who.UserID)
		i.users[who.UserID] = u
	}
	if u != nil {
		who.Name, who.Email = u.Name, u.Email
	}
}

// custodyAt answers who governs one point.
func (i *custodyIndex) custodyAt(point map[string]string) *ContextProfileCustody {
	out := &ContextProfileCustody{Custodians: []ContextProfileCustodian{}}
	for _, b := range i.bounded {
		if b.reach.Reaches(point) {
			out.Custodians = append(out.Custodians, b.who)
		}
	}
	out.Fallback = append(out.Fallback, i.unbounded...)
	out.Covered = len(out.Custodians) > 0

	sortCustodians(out.Custodians)
	sortCustodians(out.Fallback)
	return out
}

func sortCustodians(list []ContextProfileCustodian) {
	sort.Slice(list, func(a, b int) bool {
		if list[a].UserID != list[b].UserID {
			return list[a].UserID < list[b].UserID
		}
		return list[a].ProjectID < list[b].ProjectID
	})
}

// attachCustody fills in each profile's custody. An unbound voice — a voice the
// hub holds that no collection binds — has no point, so it gets no custody
// rather than an empty one that would read as an uncovered place.
func (s *Server) attachCustody(ctx context.Context, wsID string, projects []*store.Project, profiles []ContextProfile) {
	if s.AuthStore == nil {
		return
	}
	idx := s.buildCustodyIndex(ctx, wsID, projects)
	for i := range profiles {
		if profiles[i].Slug != defaultProfileSlug && !profiles[i].Declared {
			continue
		}
		profiles[i].Custody = idx.custodyAt(profiles[i].Coordinates)
	}
}
