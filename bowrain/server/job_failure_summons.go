package server

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/mailer"
	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// The other summons: how a failed job reaches the person waiting on it.
//
// A job that fails is the one outcome nobody discovers on their own. A failed
// review is visible in a queue; a failed job looks exactly like a job that is
// still running, and the difference only shows up as content that never arrived.
// So the rule is full summons by default — in-app, activity, and mail — with
// the per-category preference the way out for anyone who wants the badge and
// nothing else.
//
// This runs off flow.failed, which the workers publish (see
// bowrain/jobs/job_failed_event.go). It deliberately does not run through the
// notification dispatcher's generic event map: that map resolves recipients from
// ev.ProjectID and fans out to every member of the project, which for a failure
// is exactly wrong. A failure goes to the person who asked for the work, and
// only falls back to the people responsible for the workspace when the platform
// started the work itself.

// jobFailureGroupKey ties every notification about one job together. It is what
// makes the summons idempotent: the store is asked whether the recipient already
// holds a notification under this key, so a redelivered event, a second server
// instance, or a restart cannot produce a second summons for the same job.
func jobFailureGroupKey(jobID string) string { return "job-failed:" + jobID }

// subscribeJobFailures wires the summons to the bus.
//
// SubscribeGroup, not Subscribe: on a multi-instance deployment exactly one
// instance should act on each failure. The group name is its own, separate from
// the dispatcher's, so both consumers see every event.
//
// The handler always acknowledges. The summons is best-effort by design — the
// job has already failed, and a store that refuses an insert must not become a
// second failure — and every miss is logged against the job.
func (s *Server) subscribeJobFailures() {
	if s.EventBus == nil {
		return
	}
	s.EventBus.SubscribeGroup("job-failure-summons", func(ev platev.Event) error {
		if ev.Type != platev.EventFlowFailed {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		s.summonOnJobFailure(ctx, ev)
		return nil
	})
}

// summonOnJobFailure tells the people who can act on one failed job, over every
// channel the workspace has: the notification store (and the live socket behind
// it), and email.
//
// Best-effort throughout. The job has already failed and been recorded by the
// time this runs; a store that refuses an insert or a mailer that is not
// configured must not turn into a second failure. Every miss is logged against
// the job so a summons that did not arrive is diagnosable.
func (s *Server) summonOnJobFailure(ctx context.Context, ev platev.Event) {
	jobID := ev.Data["job_id"]
	if jobID == "" || s.NotificationDispatcher == nil {
		return
	}
	wsSlug := ev.Data["workspace_slug"]
	wsID := s.jobFailureWorkspaceID(ctx, ev)

	recipients := s.jobFailureRecipients(ctx, ev, wsID)
	if len(recipients) == 0 {
		slog.WarnContext(ctx, "job failed with nobody to tell",
			"job_id", jobID, "workspace_slug", wsSlug, "workspace_id", wsID)
		return
	}

	title := jobFailureTitle(ev)
	body := jobFailureBody(ev)
	link := jobFailureLink(wsSlug, ev)
	groupKey := jobFailureGroupKey(jobID)

	// Dedupe per recipient rather than per job: a job whose summons half
	// succeeded should complete on the redelivery, not be skipped wholesale.
	fresh := make([]string, 0, len(recipients))
	for _, userID := range recipients {
		if s.alreadyToldAboutJob(ctx, userID, groupKey) {
			continue
		}
		fresh = append(fresh, userID)
	}
	if len(fresh) == 0 {
		return
	}

	s.NotificationDispatcher.DispatchToUsers(ctx, fresh, bstore.Notification{
		Type:      bstore.NotificationFlowFailed,
		Title:     title,
		Body:      body,
		ProjectID: ev.ProjectID,
		LinkURL:   link,
		Category:  string(bstore.CategoryAutomation),
		GroupKey:  groupKey,
		ActorID:   "system",
		// High, and meant literally: work stopped and will not resume on its
		// own. Quiet hours suppress everything below this, and a failure that
		// waited until morning to be shown is a failure discovered by its
		// consequences.
		Priority: "high",
	})

	s.mailJobFailure(ctx, fresh, ev, wsID, wsSlug, link)
}

// jobFailureWorkspaceID resolves the workspace id for a failure event. The
// translation queue carries it on the row; the extraction queue's table never
// had the column, so its events carry the slug alone and the id is resolved
// from it here.
func (s *Server) jobFailureWorkspaceID(ctx context.Context, ev platev.Event) string {
	if id := ev.Data["workspace_id"]; id != "" {
		return id
	}
	if ev.WorkspaceID != "" {
		return ev.WorkspaceID
	}
	slug := ev.Data["workspace_slug"]
	if slug == "" || s.AuthStore == nil {
		return ""
	}
	ws, err := s.AuthStore.GetWorkspaceBySlug(ctx, slug)
	if err != nil || ws == nil {
		return ""
	}
	return ws.ID
}

// jobFailureRecipients answers who hears about this failure.
//
// The initiator, when the job records one — the person whose push or translate
// is waiting on it, and the only person who can tell whether it mattered. When
// the platform started the work itself (automation fan-out, forge ingest, model
// sweep, or any job enqueued before created_by existed) there is no such person,
// and the failure belongs to whoever is responsible for the workspace: its
// owners and admins.
//
// Deliberately not "every member of the project": a translation failure is not
// news for a translator who never asked for it, and a workspace that mails
// everyone about everything is a workspace whose mail is filtered away.
func (s *Server) jobFailureRecipients(ctx context.Context, ev platev.Event, wsID string) []string {
	if initiator := ev.Data["initiator"]; initiator != "" {
		return []string{initiator}
	}
	if s.AuthStore == nil || wsID == "" {
		return nil
	}
	members, err := s.AuthStore.ListMembers(ctx, wsID)
	if err != nil {
		slog.WarnContext(ctx, "job failure summons: cannot list workspace members; nobody will be told",
			"workspace_id", wsID, "error", err)
		return nil
	}
	var out []string
	for _, m := range members {
		if m == nil || m.UserID == "" {
			continue
		}
		if m.Role == platauth.RoleOwner || m.Role == platauth.RoleAdmin {
			out = append(out, m.UserID)
		}
	}
	return out
}

// alreadyToldAboutJob reports whether this user has already been summoned for
// this job. With no store to ask, it answers false — a duplicate summons is a
// smaller harm than a summons that never happens.
func (s *Server) alreadyToldAboutJob(ctx context.Context, userID, groupKey string) bool {
	if s.NotificationStore == nil {
		return false
	}
	exists, err := s.NotificationStore.ExistsByGroupKey(ctx, userID, groupKey)
	if err != nil {
		slog.WarnContext(ctx, "job failure summons: cannot check for an earlier notification; sending anyway",
			"user_id", userID, "group_key", groupKey, "error", err)
		return false
	}
	return exists
}

// mailJobFailure sends the job-failure email to each recipient who has not
// turned automation email off for this workspace.
//
// Mail-on-failure is the default, which is why this does not consult the
// notification dispatcher's high-priority email path: that one
// sends the generic notification template and ignores the preference entirely.
// Here the dedicated template goes out, and a recipient who wants the badge
// without the mail turns the automation category's email channel off.
func (s *Server) mailJobFailure(ctx context.Context, recipients []string, ev platev.Event, wsID, wsSlug, path string) {
	if s.Mailer == nil || s.AuthStore == nil {
		return
	}
	jobURL := s.absoluteAppURL(path)
	if jobURL == "" {
		slog.WarnContext(ctx, "job failure summons: no app origin configured, so no mail was sent; set BOWRAIN_APP_PUBLIC_URL",
			"job_id", ev.Data["job_id"])
		return
	}
	wsName := wsSlug
	if s.AuthStore != nil && wsID != "" {
		if ws, err := s.AuthStore.GetWorkspace(ctx, wsID); err == nil && ws != nil && ws.Name != "" {
			wsName = ws.Name
		}
	}
	data := mailer.JobFailedData{
		WorkspaceName: wsName,
		JobKind:       jobKindProse(ev.Data["job_kind"]),
		Subject:       jobFailureSubject(ev),
		Reason:        ev.Data["error"],
		JobURL:        jobURL,
	}
	if data.Reason == "" {
		data.Reason = "No reason was recorded."
	}

	for _, userID := range recipients {
		if !s.wantsAutomationEmail(ctx, userID, wsSlug) {
			continue
		}
		u, err := s.AuthStore.GetUser(ctx, userID)
		if err != nil || u == nil || u.Email == "" {
			continue
		}
		if err := s.Mailer.SendJobFailed(ctx, u.Email, u.Locale, data); err != nil {
			slog.WarnContext(ctx, "job failure summons: email failed",
				"job_id", ev.Data["job_id"], "user_id", userID, "error", err)
		}
	}
}

// wantsAutomationEmail reports whether a user still has email on for automation
// notifications in this workspace.
//
// Failures ship with it on. DefaultPreferences has the automation category's
// email channel off, which was written when the category's only members were
// completions — nothing there is worth a mail. A failure is, so the default is
// overridden here rather than in DefaultPreferences: flipping it there would
// also mail every extraction that finished. A stored preference always wins, so
// turning it off in notification preferences still means notify-only.
func (s *Server) wantsAutomationEmail(ctx context.Context, userID, wsSlug string) bool {
	if s.PreferenceStore == nil {
		return true
	}
	pref, stored, err := s.PreferenceStore.Lookup(ctx, userID, wsSlug, bstore.CategoryAutomation)
	if err != nil || !stored || pref == nil {
		return true
	}
	return pref.Email
}

// absoluteAppURL turns an in-app path into the link an email can carry, or ""
// when this deployment has not been told its own origin. A transactional mail
// whose only action is a broken link is worse than no mail, so the caller skips
// sending rather than guessing a host.
func (s *Server) absoluteAppURL(path string) string {
	origin := strings.TrimSuffix(s.Config.AppPublicURL, "/")
	if origin == "" {
		return ""
	}
	return origin + path
}

// jobFailureLink points at the surface where the failure can be read and the
// work re-run: the project's run history. A job with no project — nothing
// enqueues one today, but the event shape permits it — lands on the workspace.
func jobFailureLink(wsSlug string, ev platev.Event) string {
	if wsSlug == "" {
		return "/"
	}
	if ev.ProjectID == "" {
		return "/" + wsSlug
	}
	stream := ev.Data["stream"]
	if stream == "" {
		stream = "main"
	}
	return "/" + wsSlug + "/p/" + ev.ProjectID + "/s/" + stream + "/runs"
}

// jobKindProse renders a job kind as the noun a reader would use. The wire
// values are the queue's own vocabulary; "sync" in particular is what the
// protocol calls it and "push" is what the user ran.
func jobKindProse(kind string) string {
	switch kind {
	case "sync":
		return "push"
	case "model-sweep":
		return "model evaluation"
	case "":
		return "background"
	default:
		return kind
	}
}

// jobFailureSubject names what the job was working on, as specifically as the
// event allows: the item and its target locale when there is one, the item
// alone otherwise, and the project when the job named no item.
func jobFailureSubject(ev platev.Event) string {
	item := ev.Data["item"]
	locale := ev.Data["target_locale"]
	switch {
	case item != "" && locale != "":
		return item + " → " + locale
	case item != "":
		return item
	case ev.ProjectID != "":
		return "project " + ev.ProjectID
	default:
		return "your workspace"
	}
}

// jobFailureTitle is the one line that has to carry the failure — in the
// notification list, and in the activity feed behind it.
func jobFailureTitle(ev platev.Event) string {
	return fmt.Sprintf("A %s job did not finish", jobKindProse(ev.Data["job_kind"]))
}

// jobFailureBody says what stopped and why, in that order.
func jobFailureBody(ev platev.Event) string {
	subject := jobFailureSubject(ev)
	if reason := ev.Data["error"]; reason != "" {
		return subject + ": " + reason
	}
	return subject + " stopped before it finished, with no reason recorded."
}
