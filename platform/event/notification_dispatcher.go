package event

import (
	"context"
	"log"
	"time"

	bstore "github.com/neokapi/neokapi/bowrain/store"
	platev "github.com/neokapi/neokapi/platform/event"
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

// SetMailer sets the mailer for immediate email delivery of high-priority notifications.
func (d *NotificationDispatcher) SetMailer(m DigestEmailer) {
	d.mailer = m
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

func (d *NotificationDispatcher) handleEvent(ev platev.Event) {
	// Auto-mute resolved issues.
	d.handleAutoMute(ev)

	n := d.mapEventToNotification(ev)
	if n == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Determine who to notify.
	var userIDs []string
	if d.targetFn != nil && ev.ProjectID != "" {
		var err error
		userIDs, err = d.targetFn(ctx, ev.ProjectID, ev.Actor)
		if err != nil {
			log.Printf("WARNING: notification dispatcher failed to resolve targets for project %s: %v", ev.ProjectID, err)
			return
		}
	}

	// Create a notification for each target user.
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

		if err := d.store.Create(ctx, &notification); err != nil {
			log.Printf("WARNING: notification dispatcher failed to persist notification for user %s: %v", userID, err)
			continue
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
				log.Printf("WARNING: failed to send immediate email for user %s: %v", userID, err)
			}
		}
	}
}

func (d *NotificationDispatcher) mapEventToNotification(ev platev.Event) *bstore.Notification {
	n := &bstore.Notification{
		ProjectID: ev.ProjectID,
		ActorID:   ev.Actor,
		ActorName: ev.Data["actor_name"],
		Priority:  "normal",
	}

	switch ev.Type {
	case platev.EventFlowFailed:
		n.Type = bstore.NotificationFlowFailed
		n.Title = "Flow failed"
		n.Body = "A processing flow failed in your project"
		n.Category = string(bstore.CategoryAutomation)

	case platev.EventQualityGateFail:
		n.Type = bstore.NotificationGateFailed
		n.Title = "Quality gate failed"
		n.Body = "A quality gate check failed"
		n.Category = string(bstore.CategoryQuality)
		n.Priority = "high"

	case platev.EventBrandVoiceDrift:
		n.Type = bstore.NotificationBrandDrift
		n.Title = "Brand voice drift detected"
		n.Body = "Content has drifted from the brand voice guidelines"
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
		n.Body = "AI translation and extraction completed — content is ready for human review"
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
				log.Printf("WARNING: auto-mute failed for group key %s: %v", groupKey, err)
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

	if err := d.store.Create(ctx, n); err != nil {
		log.Printf("WARNING: failed to create mention notification for user %s: %v", mentionedUserID, err)
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

	if err := d.store.Create(ctx, n); err != nil {
		log.Printf("WARNING: failed to create deadline notification for user %s: %v", task.AssigneeID, err)
		return
	}

	if d.sender != nil {
		d.sender.NotifyUser(task.AssigneeID, n)
	}

	// Deadline notifications are always high-priority — send immediate email.
	if d.mailer != nil {
		if err := d.mailer.SendImmediate(ctx, task.AssigneeID, n); err != nil {
			log.Printf("WARNING: failed to send deadline email for user %s: %v", task.AssigneeID, err)
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

	if err := d.store.Create(ctx, n); err != nil {
		log.Printf("WARNING: failed to create task notification for user %s: %v", task.AssigneeID, err)
		return
	}

	if d.sender != nil {
		d.sender.NotifyUser(task.AssigneeID, n)
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
			log.Printf("WARNING: failed to resolve targets for project %s: %v", projectID, err)
			return
		}
	}

	for _, userID := range userIDs {
		n := prototype
		n.UserID = userID
		if err := d.store.Create(ctx, &n); err != nil {
			log.Printf("WARNING: failed to create notification for user %s: %v", userID, err)
			continue
		}
		if d.sender != nil {
			d.sender.NotifyUser(userID, &n)
		}
	}
}
