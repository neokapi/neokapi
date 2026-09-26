package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
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
	NotificationVoiceDrift NotificationType = "voice.drift"

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

// Grouped notifications.
//
// A group is one notification per recipient that stands for many occurrences
// of the same thing: a burst of jobs that failed for the same reason is told
// once, and that one notification carries the count. The notification's
// source_event_id holds the group key, so the unique (user_id, source_event_id)
// index turns opening a group into an atomic claim. Whichever instance's
// Create reports created=true opened the group, and every other instance
// folds its occurrence into it.

// likePrefix turns a literal prefix into a LIKE pattern that matches strings
// starting with it. Each query spells out ESCAPE '\', which PostgreSQL and
// SQLite both accept.
func likePrefix(prefix string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(prefix) + "%"
}

// LatestGroupSince returns the group key of the user's newest notification
// whose group key starts with prefix and which was created after since, or ""
// when there is none.
func (s *NotificationStore) LatestGroupSince(ctx context.Context, userID, prefix string, since time.Time) (string, error) {
	if userID == "" || prefix == "" {
		return "", nil
	}
	var key string
	err := s.db.QueryRowContext(ctx,
		`SELECT group_key FROM notifications
		 WHERE user_id = $1 AND group_key LIKE $2 ESCAPE '\' AND created_at > $3
		 ORDER BY created_at DESC LIMIT 1`,
		userID, likePrefix(prefix), since.UTC().Format(time.RFC3339Nano)).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return key, err
}

// CountGroupsSince counts the user's notifications whose group key starts with
// prefix and which were created after since.
func (s *NotificationStore) CountGroupsSince(ctx context.Context, userID, prefix string, since time.Time) (int, error) {
	if userID == "" || prefix == "" {
		return 0, nil
	}
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notifications
		 WHERE user_id = $1 AND group_key LIKE $2 ESCAPE '\' AND created_at > $3`,
		userID, likePrefix(prefix), since.UTC().Format(time.RFC3339Nano)).Scan(&n)
	return n, err
}

// RewriteGroup replaces the text and link of the user's notification in a
// group and marks it unread, so a group that grew after it was read shows in
// the badge again. The creation time stays: it records when the group opened,
// and the window a group stays open for is measured from it.
func (s *NotificationStore) RewriteGroup(ctx context.Context, userID, groupKey, title, body, linkURL string) error {
	if userID == "" || groupKey == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE notifications SET title = $3, body = $4, link_url = $5, read = false
		 WHERE user_id = $1 AND group_key = $2`,
		userID, groupKey, title, body, linkURL)
	return err
}

// AddGroupMember records that memberID belongs to the group and returns the
// group's size afterwards. added is false when the member was already
// recorded, which is how a redelivered event is told apart from a new
// occurrence.
func (s *NotificationStore) AddGroupMember(ctx context.Context, groupKey, memberID string, at time.Time) (added bool, size int, err error) {
	if groupKey == "" || memberID == "" {
		return false, 0, nil
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO notification_group_members (group_key, member_id, created_at)
		 VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
		groupKey, memberID, at.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return false, 0, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, 0, err
	}
	size, err = s.GroupSize(ctx, groupKey)
	return rows > 0, size, err
}

// GroupSize returns how many members a group has recorded.
func (s *NotificationStore) GroupSize(ctx context.Context, groupKey string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notification_group_members WHERE group_key = $1`, groupKey).Scan(&n)
	return n, err
}

// PruneGroupMembers deletes member rows recorded before the cutoff. A group's
// members matter only while the group is open, and no group stays open for
// more than a few hours.
func (s *NotificationStore) PruneGroupMembers(ctx context.Context, before time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM notification_group_members WHERE created_at < $1`,
		before.UTC().Format(time.RFC3339Nano))
	return err
}
