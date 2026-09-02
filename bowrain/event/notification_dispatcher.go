package event

import (
	"context"
	"log/slog"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// NotificationTarget resolves which user IDs should be notified for a project event.
// This is provided by the server, typically using workspace membership data.
type NotificationTarget func(ctx context.Context, projectID string, excludeActorID string) ([]string, error)

// NotificationSender sends real-time notifications to connected clients.
type NotificationSender interface {
	NotifyUser(userID string, notification *bstore.Notification)
}

// DigestEmailer sends email notifications.
type DigestEmailer interface {
	SendImmediate(ctx context.Context, userID string, notification *bstore.Notification) error
}

// TaskAssignmentEmailer sends the dedicated task-assignment email.
//
// Separate from DigestEmailer because it is a different message, not a
// different priority of the same one: SendImmediate renders the generic
// notification template from a title and a body, while an assignment has a task
// behind it — a description, an assigner, a queue to open. The dispatcher
// decides whether to send; the implementation resolves the recipient, the
// workspace, and the link.
type TaskAssignmentEmailer interface {
	SendTaskAssigned(ctx context.Context, userID string, task *bstore.Task, workspaceSlug string) error
}

// WorkspaceSlugResolver maps a workspace id to its slug.
//
// The two are not interchangeable and the split is load-bearing: tasks are keyed
// by workspace id, notification preferences by slug (that is what the
// preferences API writes). A preference check that passed a task's WorkspaceID
// straight through would match no stored row and silently read as "default".
type WorkspaceSlugResolver func(ctx context.Context, workspaceID string) (string, error)

// NotificationDispatcher bridges events to user-targeted notifications
// with preference-aware routing.
type NotificationDispatcher struct {
	bus         platev.EventBus
	store       *bstore.NotificationStore
	prefStore   *bstore.PreferenceStore
	digestStore *bstore.DigestStore
	sender      NotificationSender
	targetFn    NotificationTarget
	mailer      DigestEmailer
	taskMailer  TaskAssignmentEmailer
	slugFn      WorkspaceSlugResolver
	sub         *platev.Subscription
}

// NewNotificationDispatcher creates a dispatcher that listens to events and creates notifications.
func NewNotificationDispatcher(
	bus platev.EventBus,
	store *bstore.NotificationStore,
	prefStore *bstore.PreferenceStore,
	sender NotificationSender,
	targetFn NotificationTarget,
) *NotificationDispatcher {
	d := &NotificationDispatcher{
		bus:       bus,
		store:     store,
		prefStore: prefStore,
		sender:    sender,
		targetFn:  targetFn,
	}
	d.sub = bus.SubscribeGroup("notifications", d.handleEvent)
	return d
}

// Close unsubscribes from the event bus.
func (d *NotificationDispatcher) Close() {
	if d.sub != nil {
		d.bus.Unsubscribe(d.sub)
	}
}

// SetMailer sets the mailer for immediate email delivery of high-priority
// notifications. A mailer that can also send the dedicated task-assignment
// template is picked up here, so the two travel together and a test that
// supplies a bare DigestEmailer keeps working.
func (d *NotificationDispatcher) SetMailer(m DigestEmailer) {
	d.mailer = m
	if tm, ok := m.(TaskAssignmentEmailer); ok {
		d.taskMailer = tm
	}
}

// SetWorkspaceSlugResolver wires the id→slug lookup the task-assignment email
// needs to read a preference and build a link. Without it, assignment email is
// off: mailing on a preference that could not be read would take the choice
// away from the person it belongs to.
func (d *NotificationDispatcher) SetWorkspaceSlugResolver(fn WorkspaceSlugResolver) {
	d.slugFn = fn
}

// SetDigestStore sets the digest store for quiet hours lookups.
func (d *NotificationDispatcher) SetDigestStore(ds *bstore.DigestStore) {
	d.digestStore = ds
}

// isQuietHours checks whether the given user is currently in their quiet hours.
func (d *NotificationDispatcher) isQuietHours(ctx context.Context, userID, workspaceSlug string) bool {
	if d.digestStore == nil || workspaceSlug == "" {
		return false
	}
	ds, err := d.digestStore.GetSettings(ctx, userID, workspaceSlug)
	if err != nil {
		return false
	}
	return d.digestStore.IsInQuietHours(ds, time.Now().UTC())
}

// handleEvent turns one event into a notification per target user, and reports
// whether the inbox rows landed.
//
// Every delivery is keyed on the event, so the redelivery this error return
// enables — and the ones a deploy rollover produces anyway — writes one inbox
// row per user and sends one email. A row that was already there suppresses
// both the push and the mail: the person has been told, and being told twice
// about one thing is how an inbox stops being read.
func (d *NotificationDispatcher) handleEvent(ev platev.Event) error {
	// Auto-mute resolved issues.
	d.handleAutoMute(ev)

	n := d.mapEventToNotification(ev)
	if n == nil {
		return nil
	}
	n.SourceEventID = ev.ID

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Determine who to notify.
	var userIDs []string
	if d.targetFn != nil && ev.ProjectID != "" {
		var err error
		userIDs, err = d.targetFn(ctx, ev.ProjectID, ev.Actor)
		if err != nil {
			slog.Warn("notification dispatcher failed to resolve targets for project", "id", ev.ProjectID, "error", err)
			return err
		}
	}

	// Create a notification for each target user.
	var failed error
	for _, userID := range userIDs {
		notification := *n
		notification.UserID = userID

		// Check preferences if preference store is available.
		if d.prefStore != nil && notification.Category != "" {
			pref, err := d.prefStore.Get(ctx, userID, ev.Data["workspace_slug"], bstore.NotificationCategory(notification.Category))
			if err == nil && !pref.Web {
				continue // User opted out of web notifications for this category.
			}
		}

		created, err := d.store.Create(ctx, &notification)
		if err != nil {
			slog.Warn("notification dispatcher failed to persist notification for user", "id", userID, "error", err)
			failed = err
			continue
		}
		if !created {
			continue // This user has already been told about this event.
		}

		// During quiet hours, suppress push and email for non-urgent notifications.
		// High-priority notifications always deliver immediately.
		quiet := notification.Priority != "high" && d.isQuietHours(ctx, userID, ev.Data["workspace_slug"])

		// Send real-time via WebSocket.
		if d.sender != nil && !quiet {
			d.sender.NotifyUser(userID, &notification)
		}

		// Send immediate email for high-priority notifications.
		if d.mailer != nil && notification.Priority == "high" {
			if err := d.mailer.SendImmediate(ctx, userID, &notification); err != nil {
				slog.Warn("failed to send immediate email for user", "id", userID, "error", err)
			}
		}
	}
	return failed
}

func (d *NotificationDispatcher) mapEventToNotification(ev platev.Event) *bstore.Notification {
	n := &bstore.Notification{
		ProjectID: ev.ProjectID,
		ActorID:   ev.Actor,
		ActorName: ev.Data["actor_name"],
		Priority:  "normal",
	}

	switch ev.Type {
	// flow.failed is deliberately absent. It has a dedicated consumer —
	// server.subscribeJobFailures — because the recipients are not the ones this
	// function's caller resolves: it takes every member of ev.ProjectID, and a
	// failed job goes to the person who asked for the work, falling back to the
	// workspace's owners. Handling it here as well would summon the whole
	// project alongside them, twice.
	case platev.EventQualityGateFail:
		n.Type = bstore.NotificationGateFailed
		n.Title = "Quality gate failed"
		n.Body = "A quality gate check failed"
		n.Category = string(bstore.CategoryQuality)
		n.Priority = "high"

	case platev.EventVoiceDrift:
		n.Type = bstore.NotificationVoiceDrift
		n.Title = "Voice drift detected"
		n.Body = "Content has drifted from the voice guidelines"
		n.Category = string(bstore.CategoryQuality)

	case platev.EventExtractionCompleted:
		n.Type = bstore.NotificationExtractionDone
		n.Title = "Extraction completed"
		n.Body = "Entity and term extraction has completed"
		n.Category = string(bstore.CategoryAutomation)

	case platev.EventStreamMerged:
		n.Type = bstore.NotificationStreamMerged
		n.Title = "Stream merged"
		n.Body = "Stream " + ev.Data["stream"] + " was merged"
		n.Category = string(bstore.CategoryProject)

	case platev.EventPushCompleted:
		n.Type = bstore.NotificationContentAvailable
		n.Title = "New content available"
		n.Body = "New content has been pushed and is ready for translation"
		n.Category = string(bstore.CategoryTask)

	case platev.EventPushAutomationsCompleted:
		n.Type = bstore.NotificationContentReadyForWork
		n.Title = "Content ready for review"
		n.Body = "AI translation and extraction completed, and content is ready for human review"
		n.Category = string(bstore.CategoryTask)

	case platev.EventVersionCreated:
		n.Type = bstore.NotificationVersionReady
		n.Title = "New version created"
		n.Body = "Version " + ev.Data["label"] + " has been created"
		n.Category = string(bstore.CategoryProject)

	default:
		return nil
	}

	return n
}

// handleAutoMute automatically marks related notifications as read when issues are resolved.
func (d *NotificationDispatcher) handleAutoMute(ev platev.Event) {
	if d.store == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	switch ev.Type {
	case platev.EventQualityGatePass:
		// When a gate passes, mute related gate-failed notifications.
		groupKey := ev.Data["gate_name"] + ":" + ev.Data["locale"]
		if groupKey != ":" {
			if err := d.store.MarkReadByGroupKey(ctx, groupKey); err != nil {
				slog.Warn("auto-mute failed for group key", "id", groupKey, "error", err)
			}
		}
	}
}

// DispatchMention creates and sends a mention notification.
func (d *NotificationDispatcher) DispatchMention(ctx context.Context, mentionedUserID, actorID, actorName, body, projectID, linkURL string) {
	if d.store == nil || mentionedUserID == "" || mentionedUserID == actorID {
		return
	}

	n := &bstore.Notification{
		UserID:    mentionedUserID,
		Type:      bstore.NotificationMention,
		Title:     actorName + " mentioned you",
		Body:      body,
		ProjectID: projectID,
		LinkURL:   linkURL,
		Category:  string(bstore.CategoryMention),
		ActorID:   actorID,
		ActorName: actorName,
		Priority:  "normal",
	}

	if _, err := d.store.Create(ctx, n); err != nil {
		slog.Warn("failed to create mention notification for user", "id", mentionedUserID, "error", err)
		return
	}

	if d.sender != nil {
		d.sender.NotifyUser(mentionedUserID, n)
	}
}

// DispatchDeadlineApproaching creates notifications for tasks approaching their deadline.
func (d *NotificationDispatcher) DispatchDeadlineApproaching(ctx context.Context, task *bstore.Task) {
	if d.store == nil || task.AssigneeID == "" {
		return
	}

	n := &bstore.Notification{
		UserID:    task.AssigneeID,
		Type:      bstore.NotificationDeadlineApproaching,
		Title:     "Deadline approaching",
		Body:      "Task \"" + task.Title + "\" is due soon",
		ProjectID: task.ProjectID,
		Category:  string(bstore.CategoryTask),
		TaskID:    task.ID,
		ActorID:   "system",
		Priority:  "high",
	}

	if _, err := d.store.Create(ctx, n); err != nil {
		slog.Warn("failed to create deadline notification for user", "id", task.AssigneeID, "error", err)
		return
	}

	if d.sender != nil {
		d.sender.NotifyUser(task.AssigneeID, n)
	}

	// Deadline notifications are always high-priority — send immediate email.
	if d.mailer != nil {
		if err := d.mailer.SendImmediate(ctx, task.AssigneeID, n); err != nil {
			slog.Warn("failed to send deadline email for user", "id", task.AssigneeID, "error", err)
		}
	}
}

// DispatchTaskNotification creates and sends a notification for a task event.
func (d *NotificationDispatcher) DispatchTaskNotification(ctx context.Context, task *bstore.Task, notifType bstore.NotificationType, title, body string) {
	if d.store == nil || task.AssigneeID == "" {
		return
	}

	n := &bstore.Notification{
		UserID:    task.AssigneeID,
		Type:      notifType,
		Title:     title,
		Body:      body,
		ProjectID: task.ProjectID,
		Category:  string(bstore.CategoryTask),
		TaskID:    task.ID,
		ActorID:   task.CreatedBy,
		Priority:  string(task.Priority),
	}

	if _, err := d.store.Create(ctx, n); err != nil {
		slog.Warn("failed to create task notification for user", "id", task.AssigneeID, "error", err)
		return
	}

	if d.sender != nil {
		d.sender.NotifyUser(task.AssigneeID, n)
	}

	d.mailTaskAssignment(ctx, task)
}

// mailTaskAssignment sends the assignment email — but only for a task marked
// high or urgent.
//
// Routine assignments stay in-app and on the badge. A queue that mails on every
// item teaches its readers to ignore it, and then the urgent one arrives looking
// like all the others. Mail is for the ones that cannot wait for someone to
// open the queue.
func (d *NotificationDispatcher) mailTaskAssignment(ctx context.Context, task *bstore.Task) {
	if d.taskMailer == nil || task == nil || task.AssigneeID == "" {
		return
	}
	if !taskWarrantsEmail(task) {
		return
	}
	if d.slugFn == nil {
		return
	}
	slug, err := d.slugFn(ctx, task.WorkspaceID)
	if err != nil || slug == "" {
		slog.Warn("task assignment email skipped: cannot resolve the workspace slug",
			"task", task.ID, "workspace_id", task.WorkspaceID, "error", err)
		return
	}
	if d.prefStore != nil {
		pref, err := d.prefStore.Get(ctx, task.AssigneeID, slug, bstore.CategoryTask)
		if err == nil && pref != nil && !pref.Email {
			return
		}
	}
	if err := d.taskMailer.SendTaskAssigned(ctx, task.AssigneeID, task, slug); err != nil {
		slog.Warn("task assignment email failed", "task", task.ID, "user_id", task.AssigneeID, "error", err)
	}
}

// taskWarrantsEmail decides whether one task assignment is worth a mail.
//
// High and urgent qualify. A change-set review task never does, whatever its
// priority: the change-set summons already sends that reviewer the dedicated
// review-request email, which says more about the change than a generic
// assignment can, and two messages about one change-set is one too many. The
// guard reads the change-set id the summons writes onto the task rather than the
// task's type, so a terms-review task opened by any other route still mails.
func taskWarrantsEmail(task *bstore.Task) bool {
	if task.Data["changeset_id"] != "" {
		return false
	}
	switch task.Priority {
	case bstore.TaskPriorityHigh, bstore.TaskPriorityUrgent:
		return true
	default:
		return false
	}
}

// DispatchToProject creates notifications for all target users of a project.
// This is the public API for components (like ProgressTracker) that need to
// send project-scoped notifications without accessing dispatcher internals.
func (d *NotificationDispatcher) DispatchToProject(ctx context.Context, projectID, excludeActorID string, prototype bstore.Notification) {
	if d.store == nil {
		return
	}

	var userIDs []string
	if d.targetFn != nil && projectID != "" {
		var err error
		userIDs, err = d.targetFn(ctx, projectID, excludeActorID)
		if err != nil {
			slog.Warn("failed to resolve targets for project", "id", projectID, "error", err)
			return
		}
	}

	d.DispatchToUsers(ctx, userIDs, prototype)
}

// DispatchToUsers creates and pushes one copy of a prototype notification per
// named user. It is the project-free half of DispatchToProject: a workspace-scoped
// summons (a governed change-set waiting on its reviewers) has an explicit
// recipient list and no project to resolve members from, so it names the users
// directly rather than going through the project target function.
func (d *NotificationDispatcher) DispatchToUsers(ctx context.Context, userIDs []string, prototype bstore.Notification) {
	if d.store == nil {
		return
	}
	for _, userID := range userIDs {
		if userID == "" {
			continue
		}
		n := prototype
		n.UserID = userID
		if _, err := d.store.Create(ctx, &n); err != nil {
			slog.Warn("failed to create notification for user", "id", userID, "error", err)
			continue
		}
		if d.sender != nil {
			d.sender.NotifyUser(userID, &n)
		}
	}
}
