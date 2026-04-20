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
}

// NotificationStore persists user notifications.
type NotificationStore struct {
	db *sql.DB
}

// NewNotificationStore creates a notification store backed by PostgreSQL.
func NewNotificationStore(db *sql.DB) *NotificationStore {
	return &NotificationStore{db: db}
}

// Create inserts a new notification.
func (s *NotificationStore) Create(ctx context.Context, n *Notification) error {
	if n.ID == "" {
		n.ID = id.New()
	}
	if n.CreatedAt.IsZero() {
		n.CreatedAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO notifications (id, user_id, type, title, body, project_id, link_url, read, created_at, category, group_key, actor_id, actor_name, task_id, priority)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
		n.ID, n.UserID, string(n.Type), n.Title, n.Body,
		n.ProjectID, n.LinkURL, false, n.CreatedAt.UTC().Format(time.RFC3339Nano),
		n.Category, n.GroupKey, n.ActorID, n.ActorName, n.TaskID, n.Priority)
	return err
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
		if err := rows.Scan(&n.ID, &n.UserID, &typ, &n.Title, &n.Body, &n.ProjectID, &n.LinkURL, &n.Read, &n.CreatedAt, &n.Category, &n.GroupKey, &n.ActorID, &n.ActorName, &n.TaskID, &n.Priority); err != nil {
			return nil, err
		}
		n.Type = NotificationType(typ)
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
		if err := rows.Scan(&n.ID, &n.UserID, &typ, &n.Title, &n.Body, &n.ProjectID, &n.LinkURL, &n.Read, &n.CreatedAt, &n.Category, &n.GroupKey, &n.ActorID, &n.ActorName, &n.TaskID, &n.Priority); err != nil {
			return nil, err
		}
		n.Type = NotificationType(typ)
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
