package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/core/id"
)

// NotificationType classifies notifications.
type NotificationType string

const (
	NotificationReviewAssigned  NotificationType = "review.assigned"
	NotificationReviewCompleted NotificationType = "review.completed"
	NotificationExtractionDone  NotificationType = "extraction.completed"
	NotificationGeneral         NotificationType = "general"

	// Task notifications
	NotificationTaskAssigned  NotificationType = "task.assigned"
	NotificationTaskDueSoon   NotificationType = "task.due_soon"
	NotificationTaskOverdue   NotificationType = "task.overdue"
	NotificationTaskCompleted NotificationType = "task.completed"

	// Quality notifications
	NotificationGateFailed NotificationType = "quality.gate.failed"
	NotificationBrandDrift NotificationType = "brand.drift"

	// Social notifications
	NotificationMention NotificationType = "mention"
	NotificationComment NotificationType = "comment"

	// Automation notifications
	NotificationFlowFailed     NotificationType = "flow.failed"
	NotificationConnectorError NotificationType = "connector.error"

	// System notifications
	NotificationQuotaWarning NotificationType = "quota.warning"

	// Content availability
	NotificationContentAvailable    NotificationType = "content.available"
	NotificationContentReadyForWork NotificationType = "content.ready"

	// Progress milestones
	NotificationProgressMilestone NotificationType = "progress.milestone"

	// Stream operations
	NotificationStreamMerged NotificationType = "stream.merged"

	// Release readiness
	NotificationVersionReady NotificationType = "version.ready"

	// Team changes
	NotificationMemberJoined NotificationType = "member.joined"

	// Deadline awareness
	NotificationDeadlineApproaching NotificationType = "deadline.approaching"
)

// Notification is a user-targeted notification.
type Notification struct {
	ID        string           `json:"id"`
	UserID    string           `json:"user_id"`
	Type      NotificationType `json:"type"`
	Title     string           `json:"title"`
	Body      string           `json:"body"`
	ProjectID string           `json:"project_id,omitempty"`
	LinkURL   string           `json:"link_url,omitempty"` // deep link target
	Read      bool             `json:"read"`
	CreatedAt time.Time        `json:"created_at"`

	// Extended fields (Bowrain AD-014)
	Category  string `json:"category,omitempty"`   // preference category for routing
	GroupKey  string `json:"group_key,omitempty"`  // for grouping related notifications
	ActorID   string `json:"actor_id,omitempty"`   // who triggered the notification
	ActorName string `json:"actor_name,omitempty"` // display name of actor
	TaskID    string `json:"task_id,omitempty"`    // linked task
	Priority  string `json:"priority,omitempty"`   // "normal" or "high"

	// SourceEventID is the bus event this notification came from, when it came
	// from one. One user hears about one event once, however many times the
	// event is delivered. '' for the notifications raised directly (a mention,
	// a deadline, a task assignment), which have no event behind them.
	SourceEventID string `json:"source_event_id,omitempty"`
}

// NotificationStore persists user notifications.
type NotificationStore struct {
	db *sql.DB
}

// NewNotificationStore creates a notification store backed by PostgreSQL.
func NewNotificationStore(db *sql.DB) *NotificationStore {
	return &NotificationStore{db: db}
}

// Create inserts a new notification, reporting whether it is new.
//
// created=false means this user already has this notification: the source event
// was delivered again, which the queues do on every deploy rollover and on
// every reclaim of a stranded entry. The caller uses it to skip the push and
// the email that go with a first telling.
func (s *NotificationStore) Create(ctx context.Context, n *Notification) (created bool, err error) {
	if n.ID == "" {
		n.ID = id.New()
	}
	if n.CreatedAt.IsZero() {
		n.CreatedAt = time.Now().UTC()
	}

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO notifications (id, user_id, type, title, body, project_id, link_url, read, created_at, category, group_key, actor_id, actor_name, task_id, priority, source_event_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		 ON CONFLICT DO NOTHING`,
		n.ID, n.UserID, string(n.Type), n.Title, n.Body,
		n.ProjectID, n.LinkURL, false, n.CreatedAt.UTC().Format(time.RFC3339Nano),
		n.Category, n.GroupKey, n.ActorID, n.ActorName, n.TaskID, n.Priority, n.SourceEventID)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

// List returns notifications for a user, newest first.
func (s *NotificationStore) List(ctx context.Context, userID string, limit int, unreadOnly bool) ([]Notification, error) {
	if limit <= 0 {
		limit = 50
	}

	where := "user_id = $1"
	args := []any{userID}
	if unreadOnly {
		where += " AND read = false"
	}

	query := fmt.Sprintf(
		`SELECT id, user_id, type, title, body, project_id, link_url, read, created_at, category, group_key, actor_id, actor_name, task_id, priority
		 FROM notifications WHERE %s ORDER BY created_at DESC LIMIT $2`, where)
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notifications []Notification
	for rows.Next() {
		var n Notification
		var typ string
		// created_at is written as RFC3339 text. PostgreSQL parses that into its
		// TIMESTAMPTZ column; SQLite keeps it a string, so a *time.Time scan fails
		// against the schema the tests use — notifications could be written happily
		// and never read back. scanTime accepts both.
		var createdAt scanTime
		if err := rows.Scan(&n.ID, &n.UserID, &typ, &n.Title, &n.Body, &n.ProjectID, &n.LinkURL, &n.Read, &createdAt, &n.Category, &n.GroupKey, &n.ActorID, &n.ActorName, &n.TaskID, &n.Priority); err != nil {
			return nil, err
		}
		n.Type = NotificationType(typ)
		n.CreatedAt = createdAt.Time
		notifications = append(notifications, n)
	}
	return notifications, rows.Err()
}

// UnreadCount returns the number of unread notifications for a user.
func (s *NotificationStore) UnreadCount(ctx context.Context, userID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND read = false`, userID).Scan(&count)
	return count, err
}

// MarkRead marks a single notification as read.
func (s *NotificationStore) MarkRead(ctx context.Context, notificationID, userID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE notifications SET read = true WHERE id = $1 AND user_id = $2`,
		notificationID, userID)
	return err
}

// MarkAllRead marks all notifications as read for a user.
func (s *NotificationStore) MarkAllRead(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE notifications SET read = true WHERE user_id = $1 AND read = false`, userID)
	return err
}

// MarkReadByGroupKey marks all notifications with the given group key as read.
func (s *NotificationStore) MarkReadByGroupKey(ctx context.Context, groupKey string) error {
	if groupKey == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE notifications SET read = true WHERE group_key = $1 AND read = false`,
		groupKey)
	return err
}

// ExistsByGroupKey reports whether the user already holds a notification under
// the given group key, read or not.
//
// This is what makes "tell them once" survive a restart and a second instance.
// A summons that deduplicated in memory would mail again after every deploy,
// and twice over on a two-instance deployment; the group key is already the
// column that ties a notification to the thing it is about, so asking the table
// is both durable and cluster-wide. It is a check-then-insert rather than a
// unique constraint, so two events racing inside the same millisecond can still
// produce two rows — a far cheaper failure than a schema that refuses the
// second notification anyone ever groups.
func (s *NotificationStore) ExistsByGroupKey(ctx context.Context, userID, groupKey string) (bool, error) {
	if userID == "" || groupKey == "" {
		return false, nil
	}
	var exists bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM notifications WHERE user_id = $1 AND group_key = $2)`,
		userID, groupKey).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// ListUnreadSince returns unread notifications for a user created after the given time.
func (s *NotificationStore) ListUnreadSince(ctx context.Context, userID string, since time.Time) ([]Notification, error) {
	query := `SELECT id, user_id, type, title, body, project_id, link_url, read, created_at, category, group_key, actor_id, actor_name, task_id, priority
		 FROM notifications WHERE user_id = $1 AND read = false AND created_at > $2 ORDER BY created_at DESC`
	rows, err := s.db.QueryContext(ctx, query, userID, since.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notifications []Notification
	for rows.Next() {
		var n Notification
		var typ string
		// created_at is written as RFC3339 text. PostgreSQL parses that into its
		// TIMESTAMPTZ column; SQLite keeps it a string, so a *time.Time scan fails
		// against the schema the tests use — notifications could be written happily
		// and never read back. scanTime accepts both.
		var createdAt scanTime
		if err := rows.Scan(&n.ID, &n.UserID, &typ, &n.Title, &n.Body, &n.ProjectID, &n.LinkURL, &n.Read, &createdAt, &n.Category, &n.GroupKey, &n.ActorID, &n.ActorName, &n.TaskID, &n.Priority); err != nil {
			return nil, err
		}
		n.Type = NotificationType(typ)
		n.CreatedAt = createdAt.Time
		notifications = append(notifications, n)
	}
	return notifications, rows.Err()
}

// Delete removes a notification.
func (s *NotificationStore) Delete(ctx context.Context, notificationID, userID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM notifications WHERE id = $1 AND user_id = $2`,
		notificationID, userID)
	return err
}
