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

// NotificationDispatcher bridges events to user-targeted notifications
// with preference-aware routing.
type NotificationDispatcher struct {
	bus       platev.EventBus
	store     *bstore.NotificationStore
	prefStore *bstore.PreferenceStore
	sender    NotificationSender
	targetFn  NotificationTarget
	sub       *platev.Subscription
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
	d.sub = bus.SubscribeAll(d.handleEvent)
	return d
}

// Close unsubscribes from the event bus.
func (d *NotificationDispatcher) Close() {
	if d.sub != nil {
		d.bus.Unsubscribe(d.sub)
	}
}

func (d *NotificationDispatcher) handleEvent(ev platev.Event) {
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

		// Send real-time via WebSocket.
		if d.sender != nil {
			d.sender.NotifyUser(userID, &notification)
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

	default:
		return nil
	}

	return n
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
