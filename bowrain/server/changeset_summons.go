package server

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/labstack/echo/v4"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	"github.com/neokapi/neokapi/bowrain/mailer"
	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// The summons: how a submitted change-set reaches the people who must review it.
//
// Submitting a governed change-set flips a status and publishes
// changeset.submitted on the event bus, which reaches the audit chain and the
// SSE relay and stops there: the notification dispatcher resolves recipients
// from ev.ProjectID, which a workspace-scoped change-set does not carry.
//
// So the summons is computed here. On submit a change-set opens one review task
// in the workspace queue, notifies every eligible reviewer, and mails them; on
// approve, reject, abandon, or merge the task closes again.
//
// Separation of duties is the reason the reach is computed rather than assumed:
// HandleApproveChangeSet refuses the author, so a summons that reached only the
// author would be a summons to nobody. changeSetReviewers therefore returns the
// members who hold manage_voice — the approve/reject gate — minus the author.

// changeSetReviewPath is the web hub's route for one change-set. The sub-nav
// calls the surface "Changes", and the URL names the destination the sub-nav
// names, so this is /:ws/context/changes/:id — not /:ws/changesets/:id.
func changeSetReviewPath(wsSlug, changesetID string) string {
	return "/" + wsSlug + "/context/changes/" + changesetID
}

// changeSetReviewers lists the workspace members eligible to review a governed
// change-set: those whose effective workspace permissions include manage_voice
// (the gate on approve and reject), excluding the change-set's author, who
// separation of duties bars from reviewing their own work.
//
// Permissions resolve exactly as ProjectAccessMiddleware's workspace-role
// fallback does — a per-workspace role override when one is set, the role
// default otherwise — so the reach can never disagree with the gate.
//
// This is deliberately not narrowed by region, unlike the review queue. A
// change-set governs the terms every project in the workspace resolves against,
// so it sits at no point: there is nothing to narrow by, and inventing a point
// for it would summon a subset of the people its merge actually affects. When a
// change-set gains a declared region, this is where that narrowing belongs — the
// coordinate-scoped reach is already resolved per member, so the change here is
// a filter, not a mechanism.
func (s *Server) changeSetReviewers(ctx context.Context, wsID, authorID string) []string {
	if s.AuthStore == nil || wsID == "" {
		return nil
	}
	members, err := s.AuthStore.ListMembers(ctx, wsID)
	if err != nil {
		slog.WarnContext(ctx, "change-set summons: cannot list workspace members; no reviewer will be told",
			"workspace_id", wsID, "error", err)
		return nil
	}
	var out []string
	for _, m := range members {
		if m == nil || m.UserID == "" || m.UserID == authorID {
			continue
		}
		if s.workspaceRolePermissions(ctx, wsID, m.Role).Has(platauth.PermManageVoice) {
			out = append(out, m.UserID)
		}
	}
	return out
}

// workspaceRolePermissions resolves one workspace role's effective permissions,
// honouring a per-workspace override of the role's defaults.
func (s *Server) workspaceRolePermissions(ctx context.Context, wsID string, role platauth.Role) platauth.Permission {
	if role == "" {
		return 0
	}
	if perms, ok, err := s.AuthStore.GetWorkspaceRoleOverride(ctx, wsID, role); err == nil && ok {
		return perms
	}
	return platauth.DefaultPermissionsForRole(role).Permissions
}

// summonChangeSetReviewers opens the review task for a submitted change-set and
// tells its eligible reviewers, over every channel the workspace has: the task
// queue, the notification store (and the live web socket behind it), and email.
//
// It is best-effort by design. The change-set is already in review by the time
// this runs; a task store that refuses the insert or a mailer that is not
// configured must not turn a successful submit into a failed request. Every
// failure is logged against the change-set so a missing summons is diagnosable.
func (s *Server) summonChangeSetReviewers(c echo.Context, cs *knowledge.ChangeSet, opCount int, governed bool) {
	if cs == nil {
		return
	}
	ctx := context.WithoutCancel(c.Request().Context())
	wsSlug := c.Param("ws")
	reviewers := s.changeSetReviewers(ctx, cs.WorkspaceID, cs.CreatedBy)

	task := s.openChangeSetReviewTask(ctx, cs, opCount, governed, reviewers)
	link := changeSetReviewPath(wsSlug, cs.ID)
	title := changeSetReviewTitle(cs, opCount)
	body := changeSetReviewBody(cs, opCount, governed)

	for _, userID := range reviewers {
		s.notifyChangeSetReviewer(ctx, userID, cs, task, title, body, link)
	}
	if len(reviewers) == 0 {
		// Not an error: a solo workspace has nobody but the author, and the task
		// still lands unassigned in the queue. Worth a line, because it is also
		// what a workspace with no manage_voice member other than the author
		// looks like, and that one is a configuration problem.
		slog.InfoContext(ctx, "change-set submitted with no eligible reviewer; the review task is unassigned",
			"workspace_id", cs.WorkspaceID, "change_set_id", cs.ID, "author", cs.CreatedBy)
		return
	}
	s.mailChangeSetReviewers(ctx, reviewers, cs, wsSlug, opCount, requestBaseURL(c)+link)
}

// openChangeSetReviewTask creates the workspace review task for a submitted
// change-set, assigned to the first eligible reviewer (or left unassigned when
// there is none, so the work is still visible in the queue). Data carries the
// change-set id so the decision path can find and close it.
func (s *Server) openChangeSetReviewTask(ctx context.Context, cs *knowledge.ChangeSet, opCount int, governed bool, reviewers []string) *bstore.Task {
	if s.TaskStore == nil {
		return nil
	}
	var assignee string
	if len(reviewers) > 0 {
		assignee = reviewers[0]
	}
	priority := bstore.TaskPriorityNormal
	if governed {
		priority = bstore.TaskPriorityHigh
	}
	task := &bstore.Task{
		WorkspaceID: cs.WorkspaceID,
		// A change-set is workspace-scoped: it governs the terms every project
		// resolves against, so it belongs to no single project.
		ProjectID:   "",
		Type:        bstore.TaskReviewTerms,
		Status:      bstore.TaskStatusOpen,
		Priority:    priority,
		Title:       changeSetReviewTitle(cs, opCount),
		Description: changeSetReviewBody(cs, opCount, governed),
		AssigneeID:  assignee,
		CreatedBy:   cs.CreatedBy,
		Data: map[string]string{
			"changeset_id": cs.ID,
			"op_count":     strconv.Itoa(opCount),
			"governed":     strconv.FormatBool(governed),
		},
	}
	if err := s.TaskStore.Create(ctx, task); err != nil {
		slog.ErrorContext(ctx, "change-set summons: failed to open the review task; the change-set is in review with nothing in the queue",
			"workspace_id", cs.WorkspaceID, "change_set_id", cs.ID, "error", err)
		return nil
	}
	return task
}

// notifyChangeSetReviewer persists and pushes one reviewer's notification.
func (s *Server) notifyChangeSetReviewer(ctx context.Context, userID string, cs *knowledge.ChangeSet, task *bstore.Task, title, body, link string) {
	if s.NotificationDispatcher == nil {
		return
	}
	n := bstore.Notification{
		UserID:   userID,
		Type:     bstore.NotificationReviewAssigned,
		Title:    title,
		Body:     body,
		LinkURL:  link,
		Category: string(bstore.CategoryTask),
		GroupKey: "changeset:" + cs.ID,
		ActorID:  cs.CreatedBy,
		Priority: "normal",
	}
	if task != nil {
		n.TaskID = task.ID
	}
	s.NotificationDispatcher.DispatchToUsers(ctx, []string{userID}, n)
}

// mailChangeSetReviewers sends the review-request email to each reviewer who has
// not turned task email off for this workspace.
//
// Send-on-submit is the default: the task category ships with email enabled
// (store.DefaultPreferences), and a reviewer who wants no mail turns it off in
// notification preferences. Preferences key on the workspace slug, matching what
// the preferences API writes.
func (s *Server) mailChangeSetReviewers(ctx context.Context, reviewers []string, cs *knowledge.ChangeSet, wsSlug string, opCount int, reviewURL string) {
	if s.Mailer == nil || s.AuthStore == nil {
		return
	}
	wsName := wsSlug
	if ws, err := s.AuthStore.GetWorkspace(ctx, cs.WorkspaceID); err == nil && ws != nil && ws.Name != "" {
		wsName = ws.Name
	}
	authorName := s.displayName(ctx, cs.CreatedBy)

	for _, userID := range reviewers {
		if !s.wantsTaskEmail(ctx, userID, wsSlug) {
			continue
		}
		u, err := s.AuthStore.GetUser(ctx, userID)
		if err != nil || u == nil || u.Email == "" {
			continue
		}
		data := mailer.ReviewRequestData{
			WorkspaceName: wsName,
			ChangeSetName: cs.Name,
			AuthorName:    authorName,
			ChangeCount:   pluralChanges(opCount),
			ReviewURL:     reviewURL,
		}
		if err := s.Mailer.SendReviewRequest(ctx, u.Email, u.Locale, data); err != nil {
			slog.WarnContext(ctx, "change-set summons: review-request email failed",
				"change_set_id", cs.ID, "user_id", userID, "error", err)
		}
	}
}

// wantsTaskEmail reports whether a user still has email on for task
// notifications in this workspace. With no preference store wired, the default
// (on) applies.
func (s *Server) wantsTaskEmail(ctx context.Context, userID, wsSlug string) bool {
	if s.PreferenceStore == nil {
		return true
	}
	pref, err := s.PreferenceStore.Get(ctx, userID, wsSlug, bstore.CategoryTask)
	if err != nil || pref == nil {
		return true
	}
	return pref.Email
}

// displayName resolves a user's display name for prose, falling back to the id
// so an unresolvable actor still reads as somebody rather than as nothing.
func (s *Server) displayName(ctx context.Context, userID string) string {
	if userID == "" {
		return "someone"
	}
	if s.AuthStore != nil {
		if u, err := s.AuthStore.GetUser(ctx, userID); err == nil && u != nil && u.Name != "" {
			return u.Name
		}
	}
	return userID
}

// closeChangeSetReviewTasks completes the open review task(s) for a change-set
// once it has been decided — approved, rejected, abandoned, or merged — so the
// queue empties when the work is done rather than accumulating decided reviews.
func (s *Server) closeChangeSetReviewTasks(ctx context.Context, wsID, changesetID, actor string) {
	if s.TaskStore == nil || wsID == "" || changesetID == "" {
		return
	}
	res, err := s.TaskStore.List(ctx, bstore.TaskQuery{
		WorkspaceID: wsID,
		Type:        string(bstore.TaskReviewTerms),
		Statuses:    []string{string(bstore.TaskStatusOpen), string(bstore.TaskStatusInProgress)},
		Limit:       500,
	})
	if err != nil {
		slog.WarnContext(ctx, "change-set summons: cannot list review tasks to close",
			"workspace_id", wsID, "change_set_id", changesetID, "error", err)
		return
	}
	completedBy := actor
	if completedBy == "" {
		completedBy = "system"
	}
	for _, t := range res.Tasks {
		if t.Data["changeset_id"] != changesetID {
			continue
		}
		if err := s.TaskStore.Complete(ctx, t.ID, completedBy); err != nil {
			slog.ErrorContext(ctx, "change-set summons: failed to close the review task; it stays open",
				"task", t.ID, "change_set_id", changesetID, "error", err)
		}
	}
}

// changeSetReviewTitle is the one line that has to carry the summons — in the
// task queue, in the notification list, and in the email subject.
func changeSetReviewTitle(cs *knowledge.ChangeSet, opCount int) string {
	return fmt.Sprintf("Review %s (%s)", quoteName(cs.Name), pluralChanges(opCount))
}

// changeSetReviewBody says who proposed the change and what kind it is.
func changeSetReviewBody(cs *knowledge.ChangeSet, opCount int, governed bool) string {
	kind := "changes"
	if governed {
		kind = "governed changes"
	}
	if cs.Description != "" {
		return fmt.Sprintf("%d %s to the workspace terms are waiting for review. %s", opCount, kind, cs.Description)
	}
	return fmt.Sprintf("%d %s to the workspace terms are waiting for review.", opCount, kind)
}

// pluralChanges renders a change count as prose, singular at one.
func pluralChanges(n int) string {
	if n == 1 {
		return "1 change"
	}
	return fmt.Sprintf("%d changes", n)
}

// quoteName wraps a change-set name in quotes, or names it generically when the
// change-set has no name to quote.
func quoteName(name string) string {
	if name == "" {
		return "an unnamed change-set"
	}
	return "“" + name + "”"
}
