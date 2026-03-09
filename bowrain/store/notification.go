package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/gokapi/gokapi/core/id"
)

// NotificationType classifies notifications.
type NotificationType string

const (
	NotificationReviewAssigned  NotificationType = "review.assigned"
	NotificationReviewCompleted NotificationType = "review.completed"
	NotificationExtractionDone  NotificationType = "extraction.completed"
	NotificationGeneral         NotificationType = "general"
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
}

// NotificationStore persists user notifications.
type NotificationStore struct {
	db *sql.DB
}

// NewNotificationStore creates a notification store sharing the given database.
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
		`INSERT INTO notifications (id, user_id, type, title, body, project_id, link_url, read, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?)`,
		n.ID, n.UserID, string(n.Type), n.Title, n.Body,
		n.ProjectID, n.LinkURL, n.CreatedAt.Format(time.RFC3339))
	return err
}

// List returns notifications for a user, newest first.
func (s *NotificationStore) List(ctx context.Context, userID string, limit int, unreadOnly bool) ([]Notification, error) {
	if limit <= 0 {
		limit = 50
	}

	where := "user_id = ?"
	args := []any{userID}
	if unreadOnly {
		where += " AND read = 0"
	}

	query := fmt.Sprintf(
		`SELECT id, user_id, type, title, body, project_id, link_url, read, created_at
		 FROM notifications WHERE %s ORDER BY created_at DESC LIMIT ?`, where)
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notifications []Notification
	for rows.Next() {
		var n Notification
		var typ, createdAt, projectID, linkURL string
		var read int
		if err := rows.Scan(&n.ID, &n.UserID, &typ, &n.Title, &n.Body, &projectID, &linkURL, &read, &createdAt); err != nil {
			return nil, err
		}
		n.Type = NotificationType(typ)
		n.ProjectID = projectID
		n.LinkURL = linkURL
		n.Read = read != 0
		n.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		if n.CreatedAt.IsZero() {
			n.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		}
		notifications = append(notifications, n)
	}
	return notifications, rows.Err()
}

// UnreadCount returns the number of unread notifications for a user.
func (s *NotificationStore) UnreadCount(ctx context.Context, userID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notifications WHERE user_id = ? AND read = 0`, userID).Scan(&count)
	return count, err
}

// MarkRead marks a single notification as read.
func (s *NotificationStore) MarkRead(ctx context.Context, notificationID, userID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE notifications SET read = 1 WHERE id = ? AND user_id = ?`,
		notificationID, userID)
	return err
}

// MarkAllRead marks all notifications as read for a user.
func (s *NotificationStore) MarkAllRead(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE notifications SET read = 1 WHERE user_id = ? AND read = 0`, userID)
	return err
}

// Delete removes a notification.
func (s *NotificationStore) Delete(ctx context.Context, notificationID, userID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM notifications WHERE id = ? AND user_id = ?`,
		notificationID, userID)
	return err
}
