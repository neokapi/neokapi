package server

import (
	"context"
	"fmt"
	"hash/fnv"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"
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
//
// Failures are told by cause, not by job. A run that fans out thousands of jobs
// into a workspace that has reached its AI usage limit fails every one of them
// for the same reason, and a summons per job is thousands of notifications and
// emails about one condition. So failures in one workspace, for the same
// audience, of the same kind and with the same normalised reason, form a group
// for jobFailureWindow. Each recipient holds one notification for the group,
// rewritten with the running count as more jobs join it, and gets at most one
// email about it. A single job that fails on its own is a group of one, and
// reads and mails exactly as a per-job summons.
//
// The notification store is the shared state, which is what makes this hold on
// more than one server instance:
//
//   - The group's notification carries the group key as its source_event_id,
//     and the store's unique (user_id, source_event_id) index makes creating it
//     an atomic claim. The instance whose insert lands opens the group for that
//     recipient and owns its email; every other instance folds into it.
//   - notification_group_members records each job once per group, so the count
//     is exact and a redelivered event neither grows it nor mails again.
//   - The email ceiling counts the recipient's job-failure groups in the
//     workspace over the last hour, read from the same table.

const (
	// jobFailureWindow is how long a group stays open. A failure whose cause
	// matches a group the recipient received within the window joins it; the
	// first failure after the window opens a new group and a new email.
	jobFailureWindow = time.Hour

	// jobFailureMailDelay is how long the email for a new group waits before it
	// is written, so that it can say how many jobs a burst took down rather
	// than announcing the first of them. The in-app notification is immediate.
	jobFailureMailDelay = 2 * time.Minute

	// jobFailureMailCeiling is the most job-failure emails one recipient gets
	// about one workspace in any hour, whatever the causes. Failures past it
	// still reach the in-app notification list.
	jobFailureMailCeiling = 3

	// jobFailureMemberRetention is how long a group's member rows are kept.
	// They matter only while the group is open.
	jobFailureMemberRetention = 24 * time.Hour
)

// jobFailureWorkspacePrefix is the group-key prefix shared by every job-failure
// group in one workspace. The email ceiling counts groups under it.
func jobFailureWorkspacePrefix(wsKey string) string {
	return "job-failures:" + wsKey + ":"
}

// jobFailureCausePrefix is the group-key prefix of one audience's failures of
// one cause in a workspace. The audience is part of it because the count is
// per group: an initiator's notification counts the jobs they asked for, and
// never a colleague's.
func jobFailureCausePrefix(wsKey, audience, cause string) string {
	return jobFailureWorkspacePrefix(wsKey) + audience + ":" + cause + ":"
}

// jobFailureGroupKey names the group a cause opens at a moment: the cause
// prefix and the start of the window the moment falls in. Two instances that
// open a group for the same cause in the same window compute the same key,
// which is what lets the store's unique index settle the race.
func jobFailureGroupKey(causePrefix string, at time.Time) string {
	return causePrefix + strconv.FormatInt(at.UTC().Truncate(jobFailureWindow).Unix(), 10)
}

// jobFailureAudience names who a failure is told to, for the group key.
func jobFailureAudience(ev platev.Event) string {
	if initiator := ev.Data["initiator"]; initiator != "" {
		return "user-" + initiator
	}
	return "owners"
}

var (
	failureDoubleQuoted = regexp.MustCompile("\"[^\"]*\"|`[^`]*`")
	failureSingleQuoted = regexp.MustCompile(`(^|[\s:(=])'[^']*'`)
	failureIDLike       = regexp.MustCompile(`[\p{L}\p{N}_.:/-]{8,}`)
	failureLongNumber   = regexp.MustCompile(`\d{4,}`)
)

// jobFailureCause reduces a failure to the key that says whether two failures
// happened for the same reason: the job kind, and the error with everything
// particular to one job taken out. That is the job's own identifiers, item and
// locale; quoted values; and long tokens with digits in them, which are
// request ids, timestamps and byte counts. "workspace AI quota exceeded" is
// the same cause on every job it stops; "openai: API error 401" and
// "openai: API error 429" stay two causes.
func jobFailureCause(ev platev.Event) string {
	reason := strings.ToLower(ev.Data["error"])
	for _, v := range []string{
		ev.Data["job_id"], ev.Data["item"], ev.Data["target_locale"],
		ev.Data["push_id"], ev.Data["step_id"], ev.ProjectID,
	} {
		if len(v) < 2 {
			continue
		}
		re := regexp.MustCompile(`(^|[^\p{L}\p{N}_])` + regexp.QuoteMeta(strings.ToLower(v)) + `($|[^\p{L}\p{N}_])`)
		reason = re.ReplaceAllString(reason, "${1}…${2}")
	}
	reason = failureDoubleQuoted.ReplaceAllString(reason, `"…"`)
	reason = failureSingleQuoted.ReplaceAllString(reason, `$1'…'`)
	reason = failureIDLike.ReplaceAllStringFunc(reason, func(tok string) string {
		if strings.ContainsAny(tok, "0123456789") {
			return "#"
		}
		return tok
	})
	reason = failureLongNumber.ReplaceAllString(reason, "#")
	reason = strings.Join(strings.Fields(reason), " ")

	h := fnv.New64a()
	_, _ = h.Write([]byte(ev.Data["job_kind"]))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(reason))
	return strconv.FormatUint(h.Sum64(), 16)
}

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

// summonOnJobFailure tells the people who can act on one failed job, by adding
// the job to the group its cause belongs to for each recipient.
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
	if s.NotificationStore == nil {
		// Without the store there is nothing to group against and nothing to
		// bound the email by, so nothing is sent.
		slog.WarnContext(ctx, "job failed with no notification store; nobody will be told", "job_id", jobID)
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

	wsKey := wsID
	if wsKey == "" {
		wsKey = wsSlug
	}
	now := time.Now().UTC()
	causePrefix := jobFailureCausePrefix(wsKey, jobFailureAudience(ev), jobFailureCause(ev))
	link := jobFailureLink(wsSlug, ev)

	type membership struct {
		added bool
		size  int
	}
	groups := map[string]membership{}

	for _, userID := range recipients {
		groupKey := s.jobFailureGroupFor(ctx, userID, causePrefix, now)
		m, seen := groups[groupKey]
		if !seen {
			added, size, err := s.NotificationStore.AddGroupMember(ctx, groupKey, jobID, now)
			if err != nil {
				slog.WarnContext(ctx, "job failure summons: cannot record the job in its group; counting it once",
					"job_id", jobID, "group_key", groupKey, "error", err)
				added, size = true, 1
			}
			if added && size == 1 {
				// A group just opened, which is rare enough to tidy on.
				if err := s.NotificationStore.PruneGroupMembers(ctx, now.Add(-jobFailureMemberRetention)); err != nil {
					slog.DebugContext(ctx, "job failure summons: prune failed", "error", err)
				}
			}
			m = membership{added: added, size: size}
			groups[groupKey] = m
		}

		title, body := jobFailureText(ev, m.size)
		created, err := s.NotificationDispatcher.DispatchOnce(ctx, bstore.Notification{
			UserID:        userID,
			Type:          bstore.NotificationFlowFailed,
			Title:         title,
			Body:          body,
			ProjectID:     ev.ProjectID,
			LinkURL:       link,
			Category:      string(bstore.CategoryAutomation),
			GroupKey:      groupKey,
			SourceEventID: groupKey,
			ActorID:       "system",
			// High, and meant literally: work stopped and will not resume on
			// its own. Quiet hours suppress everything below this, and a
			// failure that waited until morning to be shown is a failure
			// discovered by its consequences.
			Priority: "high",
		})
		if err != nil {
			slog.WarnContext(ctx, "job failure summons: cannot store the notification",
				"job_id", jobID, "user_id", userID, "group_key", groupKey, "error", err)
			continue
		}
		if created {
			s.queueJobFailureMail(ctx, userID, groupKey, wsKey, wsID, wsSlug, link, ev, now)
			continue
		}
		// The recipient already holds this group's notification. A job that
		// joined the group rewrites it with the new count; a redelivery of a
		// job already counted changes nothing.
		if m.added {
			if err := s.NotificationStore.RewriteGroup(ctx, userID, groupKey, title, body, link); err != nil {
				slog.WarnContext(ctx, "job failure summons: cannot update the grouped notification",
					"job_id", jobID, "user_id", userID, "group_key", groupKey, "error", err)
			}
		}
	}
}

// jobFailureGroupFor returns the group a failure joins for one recipient: the
// newest group of the same cause the recipient received within the window, or
// the key a new group opens under.
func (s *Server) jobFailureGroupFor(ctx context.Context, userID, causePrefix string, now time.Time) string {
	key, err := s.NotificationStore.LatestGroupSince(ctx, userID, causePrefix, now.Add(-jobFailureWindow))
	if err != nil {
		slog.WarnContext(ctx, "job failure summons: cannot look up an open group; using the window's own",
			"user_id", userID, "error", err)
	}
	if key != "" {
		return key
	}
	return jobFailureGroupKey(causePrefix, now)
}

// queueJobFailureMail schedules the email for a group this instance just
// opened for a recipient, unless the recipient has reached the hourly ceiling
// for the workspace.
func (s *Server) queueJobFailureMail(ctx context.Context, userID, groupKey, wsKey, wsID, wsSlug, link string, ev platev.Event, now time.Time) {
	opened, err := s.NotificationStore.CountGroupsSince(ctx, userID, jobFailureWorkspacePrefix(wsKey), now.Add(-time.Hour))
	if err != nil {
		slog.WarnContext(ctx, "job failure summons: cannot count recent groups; mailing anyway",
			"user_id", userID, "error", err)
		opened = 1
	}
	if opened > jobFailureMailCeiling {
		if opened == jobFailureMailCeiling+1 {
			slog.WarnContext(ctx, "job failure summons: email ceiling reached; further failures in this workspace reach the recipient in the app only",
				"user_id", userID, "workspace_slug", wsSlug, "workspace_id", wsID,
				"ceiling", jobFailureMailCeiling, "per", "hour")
		}
		return
	}
	s.failureMail().add(ctx, pendingFailureMail{
		groupKey: groupKey,
		userID:   userID,
		wsID:     wsID,
		wsSlug:   wsSlug,
		link:     link,
		ev:       ev,
	})
}

// pendingFailureMail is one recipient's email about one group, waiting for the
// group to settle.
type pendingFailureMail struct {
	groupKey string
	userID   string
	wsID     string
	wsSlug   string
	link     string
	ev       platev.Event
}

// failureMailQueue holds the emails this instance owes for the groups it
// opened. Each group's mail goes out jobFailureMailDelay after the group
// opened, with the count at that moment. Shutdown flushes whatever is still
// waiting, so a deploy sends the mail early rather than dropping it.
type failureMailQueue struct {
	send  func(ctx context.Context, group []pendingFailureMail)
	delay time.Duration

	mu      sync.Mutex
	pending map[string][]pendingFailureMail
	timers  map[string]*time.Timer
}

func (s *Server) failureMail() *failureMailQueue {
	s.failureMailOnce.Do(func() {
		s.failureMailQ = &failureMailQueue{
			send:    s.sendJobFailureMail,
			delay:   jobFailureMailDelay,
			pending: map[string][]pendingFailureMail{},
			timers:  map[string]*time.Timer{},
		}
	})
	return s.failureMailQ
}

// add queues one recipient's mail. The mail goes out after the event handler
// that queued it has returned, so it keeps the handler's context values and
// drops its cancellation.
func (q *failureMailQueue) add(ctx context.Context, p pendingFailureMail) {
	detached := context.WithoutCancel(ctx)
	if q.delay <= 0 {
		q.send(detached, []pendingFailureMail{p})
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.pending[p.groupKey] = append(q.pending[p.groupKey], p)
	if _, ok := q.timers[p.groupKey]; !ok {
		key := p.groupKey
		q.timers[key] = time.AfterFunc(q.delay, func() { q.fire(detached, key) })
	}
}

// fire sends one group's waiting mail.
func (q *failureMailQueue) fire(ctx context.Context, groupKey string) {
	q.mu.Lock()
	group := q.pending[groupKey]
	delete(q.pending, groupKey)
	delete(q.timers, groupKey)
	q.mu.Unlock()
	if len(group) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	q.send(ctx, group)
}

// flush sends every waiting mail now.
func (q *failureMailQueue) flush(ctx context.Context) {
	q.mu.Lock()
	pending := q.pending
	q.pending = map[string][]pendingFailureMail{}
	for _, t := range q.timers {
		t.Stop()
	}
	q.timers = map[string]*time.Timer{}
	q.mu.Unlock()
	for _, group := range pending {
		q.send(ctx, group)
	}
}

// flushJobFailureMail sends every job-failure email this instance is holding.
func (s *Server) flushJobFailureMail(ctx context.Context) {
	if s.failureMailQ == nil {
		return
	}
	s.failureMailQ.flush(ctx)
}

// sendJobFailureMail writes one group's email to each recipient waiting on it.
// A group of one is a single failure and gets the per-job email; a larger group
// gets the grouped one with the count as it stands.
func (s *Server) sendJobFailureMail(ctx context.Context, group []pendingFailureMail) {
	if len(group) == 0 || s.Mailer == nil || s.AuthStore == nil {
		return
	}
	first := group[0]
	size := 1
	if s.NotificationStore != nil {
		if n, err := s.NotificationStore.GroupSize(ctx, first.groupKey); err == nil && n > 0 {
			size = n
		}
	}
	jobURL := s.absoluteAppURL(first.link)
	if jobURL == "" {
		slog.WarnContext(ctx, "job failure summons: no app origin configured, so no mail was sent; set BOWRAIN_APP_PUBLIC_URL",
			"job_id", first.ev.Data["job_id"])
		return
	}
	wsName := first.wsSlug
	if first.wsID != "" {
		if ws, err := s.AuthStore.GetWorkspace(ctx, first.wsID); err == nil && ws != nil && ws.Name != "" {
			wsName = ws.Name
		}
	}
	reason := first.ev.Data["error"]
	if reason == "" {
		reason = "No reason was recorded."
	}

	for _, p := range group {
		if !s.wantsAutomationEmail(ctx, p.userID, p.wsSlug) {
			continue
		}
		u, err := s.AuthStore.GetUser(ctx, p.userID)
		if err != nil || u == nil || u.Email == "" {
			continue
		}
		if size == 1 {
			err = s.Mailer.SendJobFailed(ctx, u.Email, u.Locale, mailer.JobFailedData{
				WorkspaceName: wsName,
				JobKind:       jobKindProse(p.ev.Data["job_kind"]),
				Subject:       jobFailureSubject(p.ev),
				Reason:        reason,
				JobURL:        jobURL,
			})
		} else {
			err = s.Mailer.SendJobFailures(ctx, u.Email, u.Locale, mailer.JobFailuresData{
				WorkspaceName: wsName,
				JobKind:       jobKindProse(p.ev.Data["job_kind"]),
				Count:         strconv.Itoa(size),
				Reason:        reason,
				JobURL:        jobURL,
			})
		}
		if err != nil {
			slog.WarnContext(ctx, "job failure summons: email failed",
				"job_id", p.ev.Data["job_id"], "group_key", p.groupKey, "user_id", p.userID, "error", err)
		}
	}
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

// jobFailureText is the title and body of a group's notification: the per-job
// wording for a group of one, and the count and shared reason for more.
func jobFailureText(ev platev.Event, count int) (title, body string) {
	if count <= 1 {
		return jobFailureTitle(ev), jobFailureBody(ev)
	}
	title = fmt.Sprintf("%d %s jobs did not finish", count, jobKindProse(ev.Data["job_kind"]))
	reason := strings.TrimRight(ev.Data["error"], ". ")
	if reason == "" {
		reason = "no reason was recorded"
	}
	body = "They stopped for the same reason: " + reason + ". The latest was " + jobFailureSubject(ev) + "."
	return title, body
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
