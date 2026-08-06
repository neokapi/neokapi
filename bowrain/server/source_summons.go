package server

import (
	"context"
	"log/slog"
	"strings"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// The source-review summons: how a source change that needs a decision reaches
// the people entitled to make it.
//
// Both venues that open a source-review task — a reviewer's proposed source
// change (handlers_source_proposals.go) and a run held below the source gate
// (createSourceReviewTask, driven by the convergence orchestrator) — used to
// resolve their recipient by walking the project's members and taking the FIRST
// whose role carried PermEditSource. One person was told. Everyone else equally
// entitled to settle the source never heard, and if that one person was away
// the source sat unsettled with the run parked behind it and no second name to
// escalate to.
//
// sourceOwners is the source-side counterpart of changeSetReviewers: EVERY
// member whose effective permissions for the project include PermEditSource,
// resolved exactly as ProjectAccessMiddleware resolves them — explicit project
// membership (or a group binding) first, the workspace role as the fallback,
// honouring a per-workspace role override — so the reach can never disagree
// with the rights that gate the decision.
//
// Task-assignment semantics follow the change-set summons: ONE task, assigned
// to the first eligible owner so it has a name on it, left unassigned but
// visible in the queue when there is none, and a notification to every eligible
// owner so the work is not invisible to the rest of them.

// sourceOwners lists the users who may edit a project's source content, in a
// stable order (workspace roster first). Returns nil when nobody qualifies —
// which is a real answer, not a failure: the task still lands in the queue.
func (s *Server) sourceOwners(ctx context.Context, proj *platstore.Project) []string {
	if s.AuthStore == nil || proj == nil || proj.ID == "" {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	add := func(userID string, perms platauth.Permission) {
		if userID == "" || seen[userID] || !perms.Has(platauth.PermEditSource) {
			return
		}
		seen[userID] = true
		out = append(out, userID)
	}

	if proj.WorkspaceID != "" {
		members, err := s.AuthStore.ListMembers(ctx, proj.WorkspaceID)
		if err != nil {
			slog.WarnContext(ctx, "source summons: cannot list workspace members; the reach falls back to project membership",
				"workspace_id", proj.WorkspaceID, "project", proj.ID, "error", err)
		}
		for _, m := range members {
			if m == nil {
				continue
			}
			add(m.UserID, s.projectPermissionsFor(ctx, proj, m.UserID, m.Role))
		}
	}

	// A project membership can name someone the workspace roster does not (a
	// guest bound to a single project), so the reach is the union of both
	// membership tables rather than whichever one happens to be populated.
	pms, err := s.AuthStore.ListProjectMembers(ctx, proj.ID)
	if err != nil {
		slog.WarnContext(ctx, "source summons: cannot list project members",
			"project", proj.ID, "error", err)
		return out
	}
	for _, pm := range pms {
		if pm == nil {
			continue
		}
		add(pm.UserID, s.projectPermissionsFor(ctx, proj, pm.UserID, ""))
	}
	return out
}

// projectPermissionsFor resolves one user's effective permissions on a project
// the way ProjectAccessMiddleware does: an explicit project membership (or
// group role binding) decides, and only when the user has neither does the
// workspace role — with its per-workspace override — stand in.
func (s *Server) projectPermissionsFor(ctx context.Context, proj *platstore.Project, userID string, wsRole platauth.Role) platauth.Permission {
	if userID == "" {
		return 0
	}
	if resolved, err := s.AuthStore.ResolveProjectPermissions(ctx, proj.ID, userID); err == nil && resolved != nil {
		return resolved.Permissions
	}
	return s.workspaceRolePermissions(ctx, proj.WorkspaceID, wsRole)
}

// notifySourceOwners persists and pushes one notification per eligible source
// owner for a freshly opened source-review task. It is the fan-out half of the
// summons: the task carries a single assignee, the notification carries the
// whole reach.
func (s *Server) notifySourceOwners(ctx context.Context, task *bstore.Task, owners []string, title, body string) {
	if s.NotificationDispatcher == nil || task == nil {
		return
	}
	if len(owners) == 0 {
		// Not an error: a project whose members hold no source-editing right
		// still gets the task in the queue, where an admin can pick it up. Worth
		// a line, because it is also what a mis-configured role set looks like.
		slog.InfoContext(ctx, "source review opened with no eligible source owner; the task is unassigned",
			"project", task.ProjectID, "task", task.ID)
		return
	}
	s.NotificationDispatcher.DispatchToUsers(ctx, owners, bstore.Notification{
		Type:      bstore.NotificationTaskAssigned,
		Title:     title,
		Body:      body,
		ProjectID: task.ProjectID,
		Category:  string(bstore.CategoryTask),
		TaskID:    task.ID,
		ActorID:   task.CreatedBy,
		Priority:  string(task.Priority),
	})
}

// priorApprovers names the people whose approvals a source change is about to
// undo. A source edit demotes every reviewed/signed-off target on the block
// (the decision.stale demotion in the store): that is somebody's finished work
// being reopened, and they learn about it from this summons or not at all.
//
// The decision ledger is the record of who decided what, so the reviewers are
// read from it rather than inferred from the block. DecidedBy is written as the
// decider's email when the auth store could resolve one and the opaque user id
// otherwise (recordReviewDecision), so both spellings resolve back to a user
// here; machine deciders ("ai/…", "agent/…") are nobody to notify.
//
// Best-effort throughout: a store without the ledger capability, an unreadable
// block, or an unresolvable name yields fewer recipients, never an error.
func (s *Server) priorApprovers(ctx context.Context, projectID, stream, blockID string) []string {
	ds, ok := s.ContentStore.(platstore.DecisionStore)
	if !ok || projectID == "" || blockID == "" {
		return nil
	}
	sb, err := s.ContentStore.GetBlock(ctx, projectID, stream, blockID)
	if err != nil || sb == nil || sb.SourceID == "" {
		return nil
	}
	decisions, err := ds.ListUnitDecisions(ctx, projectID, stream)
	if err != nil {
		slog.WarnContext(ctx, "source summons: cannot read the decision ledger; the demoted approval's reviewer will not be told",
			"project", projectID, "block", blockID, "error", err)
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, d := range decisions {
		if d.Unit != sb.SourceID {
			continue
		}
		if d.ReviewState != "approved" && d.ReviewState != "signed-off" {
			continue
		}
		userID := s.userIDForDecider(ctx, d.DecidedBy)
		if userID == "" || seen[userID] {
			continue
		}
		seen[userID] = true
		out = append(out, userID)
	}
	return out
}

// userIDForDecider maps a decision ledger's DecidedBy back to a user id.
// Returns "" for a machine decider or a name no user answers to.
func (s *Server) userIDForDecider(ctx context.Context, decidedBy string) string {
	if decidedBy == "" || s.AuthStore == nil {
		return ""
	}
	if strings.HasPrefix(decidedBy, "ai/") || strings.HasPrefix(decidedBy, "agent/") {
		return ""
	}
	if strings.Contains(decidedBy, "@") {
		if u, err := s.AuthStore.GetUserByEmail(ctx, decidedBy); err == nil && u != nil {
			return u.ID
		}
		return ""
	}
	if u, err := s.AuthStore.GetUser(ctx, decidedBy); err == nil && u != nil {
		return u.ID
	}
	return ""
}

// notifyDemotedApprovers tells the reviewers whose approvals a proposed source
// change would undo, minus anyone already summoned as a source owner (they have
// the fuller message). The wording is theirs, not the owner's: they are not
// being asked to decide, they are being told their decision is at stake.
func (s *Server) notifyDemotedApprovers(ctx context.Context, task *bstore.Task, owners, approvers []string) {
	if s.NotificationDispatcher == nil || task == nil || len(approvers) == 0 {
		return
	}
	summoned := make(map[string]bool, len(owners))
	for _, o := range owners {
		summoned[o] = true
	}
	var tell []string
	for _, a := range approvers {
		if !summoned[a] {
			tell = append(tell, a)
		}
	}
	if len(tell) == 0 {
		return
	}
	s.NotificationDispatcher.DispatchToUsers(ctx, tell, bstore.Notification{
		Type:      bstore.NotificationTaskAssigned,
		Title:     "A proposed source change affects your approval",
		Body:      "If the proposed source change is approved, the translations you approved for this content are re-drafted.",
		ProjectID: task.ProjectID,
		Category:  string(bstore.CategoryTask),
		TaskID:    task.ID,
		ActorID:   task.CreatedBy,
		Priority:  string(task.Priority),
	})
}
