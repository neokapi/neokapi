package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/id"
)

// ActivityType classifies activities in the feed.
type ActivityType string

const (
	// Content lifecycle
	ActivityItemPushed      ActivityType = "item.pushed"
	ActivityItemPulled      ActivityType = "item.pulled"
	ActivityBlockTranslated ActivityType = "block.translated"
	ActivityBlockReviewed   ActivityType = "block.reviewed"
	ActivityBlockCommented  ActivityType = "block.commented"

	// Project management
	ActivityProjectCreated ActivityType = "project.created"
	ActivityProjectUpdated ActivityType = "project.updated"
	ActivityLocaleAdded    ActivityType = "locale.added"
	ActivityMemberAdded    ActivityType = "member.added"
	ActivityMemberRemoved  ActivityType = "member.removed"

	// Stream operations
	ActivityStreamCreated  ActivityType = "stream.created"
	ActivityStreamMerged   ActivityType = "stream.merged"
	ActivityStreamLocked   ActivityType = "stream.locked"
	ActivityStreamUnlocked ActivityType = "stream.unlocked"
	ActivityStreamTagged   ActivityType = "stream.tagged"

	// Automation & AI
	ActivityFlowCompleted  ActivityType = "flow.completed"
	ActivityFlowFailed     ActivityType = "flow.failed"
	ActivityJobCompleted   ActivityType = "job.completed"
	ActivityJobFailed      ActivityType = "job.failed"
	ActivityExtractionDone ActivityType = "extraction.completed"

	// Quality
	ActivityGatePassed ActivityType = "gate.passed"
	ActivityGateFailed ActivityType = "gate.failed"
	ActivityBrandDrift ActivityType = "brand.drift"

	// Review queue
	ActivityReviewAssigned ActivityType = "review.assigned"
	ActivityReviewDecided  ActivityType = "review.decided"

	// Connectors
	ActivityConnectorSynced ActivityType = "connector.synced"
	ActivityConnectorFailed ActivityType = "connector.failed"

	// Tasks
	ActivityTaskCreated    ActivityType = "task.created"
	ActivityTaskCompleted  ActivityType = "task.completed"
	ActivityTaskReassigned ActivityType = "task.reassigned"

	// Versions
	ActivityVersionCreated ActivityType = "version.created"

	// Progress
	ActivityProgressMilestone ActivityType = "progress.milestone"
)

// Activity is an immutable record of something that happened.
type Activity struct {
	ID          string            `json:"id"`
	WorkspaceID string            `json:"workspace_id"`
	ProjectID   string            `json:"project_id,omitempty"`
	Stream      string            `json:"stream,omitempty"`
	ActorID     string            `json:"actor_id"`
	ActorName   string            `json:"actor_name"`
	Type        ActivityType      `json:"type"`
	EntityType  string            `json:"entity_type,omitempty"`
	EntityID    string            `json:"entity_id,omitempty"`
	Summary     string            `json:"summary"`
	Data        map[string]string `json:"data"`
	CreatedAt   time.Time         `json:"created_at"`
}

// ActivityQuery defines filters for listing activities.
type ActivityQuery struct {
	WorkspaceID string
	ProjectID   string
	Stream      string
	ActorID     string
	Type        string // prefix match (e.g., "block" matches "block.*")
	Since       time.Time
	Limit       int
	Cursor      string // created_at cursor for pagination
}

// ActivityResult is a paginated result set.
type ActivityResult struct {
	Activities []Activity `json:"activities"`
	NextCursor string     `json:"next_cursor,omitempty"`
}

// ActivityStore persists activities.
type ActivityStore struct {
	db *sql.DB
}

// NewActivityStore creates a PostgreSQL-backed activity store.
func NewActivityStore(db *sql.DB) *ActivityStore {
	return &ActivityStore{db: db}
}

// Create inserts a new activity.
func (s *ActivityStore) Create(ctx context.Context, a *Activity) error {
	if a.ID == "" {
		a.ID = id.New()
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	if a.Data == nil {
		a.Data = map[string]string{}
	}

	dataJSON, err := json.Marshal(a.Data)
	if err != nil {
		dataJSON = []byte("{}")
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO activities (id, workspace_id, project_id, stream, actor_id, actor_name,
		 type, entity_type, entity_id, summary, data, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		a.ID, a.WorkspaceID, a.ProjectID, a.Stream, a.ActorID, a.ActorName,
		string(a.Type), a.EntityType, a.EntityID, a.Summary,
		string(dataJSON), a.CreatedAt.UTC().Format(time.RFC3339))
	return err
}

// List returns activities matching the query, newest first.
func (s *ActivityStore) List(ctx context.Context, q ActivityQuery) (*ActivityResult, error) {
	if q.Limit <= 0 {
		q.Limit = 50
	}

	var where []string
	var args []any
	n := 0
	ph := func() string { n++; return fmt.Sprintf("$%d", n) }

	if q.WorkspaceID != "" {
		where = append(where, "workspace_id = "+ph())
		args = append(args, q.WorkspaceID)
	}
	if q.ProjectID != "" {
		where = append(where, "project_id = "+ph())
		args = append(args, q.ProjectID)
	}
	if q.Stream != "" {
		where = append(where, "stream = "+ph())
		args = append(args, q.Stream)
	}
	if q.ActorID != "" {
		where = append(where, "actor_id = "+ph())
		args = append(args, q.ActorID)
	}
	if q.Type != "" {
		where = append(where, "type LIKE "+ph())
		args = append(args, q.Type+"%")
	}
	if !q.Since.IsZero() {
		where = append(where, "created_at > "+ph())
		args = append(args, q.Since.UTC().Format(time.RFC3339))
	}
	if q.Cursor != "" {
		where = append(where, "created_at < "+ph())
		args = append(args, q.Cursor)
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	query := fmt.Sprintf(
		`SELECT id, workspace_id, project_id, stream, actor_id, actor_name,
		 type, entity_type, entity_id, summary, data, created_at
		 FROM activities %s ORDER BY created_at DESC LIMIT %s`, whereClause, ph())
	args = append(args, q.Limit+1)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var activities []Activity
	for rows.Next() {
		a, err := scanActivity(rows)
		if err != nil {
			return nil, err
		}
		activities = append(activities, *a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := &ActivityResult{}
	if len(activities) > q.Limit {
		result.Activities = activities[:q.Limit]
		result.NextCursor = activities[q.Limit-1].CreatedAt.Format(time.RFC3339Nano)
	} else {
		result.Activities = activities
	}

	return result, nil
}

// DailyCounts returns activity counts per day for a workspace since the given time.
func (s *ActivityStore) DailyCounts(ctx context.Context, workspaceID string, since time.Time) ([]DailyCount, error) {
	query := `SELECT DATE(created_at) AS day, COUNT(*) AS cnt FROM activities
		 WHERE workspace_id = $1 AND created_at > $2
		 GROUP BY day ORDER BY day`

	rows, err := s.db.QueryContext(ctx, query,
		workspaceID, since.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var counts []DailyCount
	for rows.Next() {
		var dc DailyCount
		if err := rows.Scan(&dc.Date, &dc.Count); err != nil {
			return nil, err
		}
		counts = append(counts, dc)
	}
	return counts, rows.Err()
}

// DailyCount is a date + count pair returned by DailyCounts.
type DailyCount struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

func scanActivity(row scanner) (*Activity, error) {
	var a Activity
	var typ, dataJSON, createdAt string

	err := row.Scan(
		&a.ID, &a.WorkspaceID, &a.ProjectID, &a.Stream,
		&a.ActorID, &a.ActorName, &typ, &a.EntityType,
		&a.EntityID, &a.Summary, &dataJSON, &createdAt,
	)
	if err != nil {
		return nil, err
	}

	a.Type = ActivityType(typ)
	a.CreatedAt, _ = parseTime(createdAt)
	if dataJSON != "" {
		_ = json.Unmarshal([]byte(dataJSON), &a.Data)
	}
	if a.Data == nil {
		a.Data = map[string]string{}
	}

	return &a, nil
}

// GetActivitySeenAt returns when the user last viewed the activity feed.
// Returns zero time if never viewed.
func (s *ActivityStore) GetActivitySeenAt(ctx context.Context, userID, workspaceID string) (time.Time, error) {
	var ts string
	err := s.db.QueryRowContext(ctx,
		`SELECT last_seen_at FROM activity_state WHERE user_id = $1 AND workspace_id = $2`,
		userID, workspaceID).Scan(&ts)
	if err != nil {
		return time.Time{}, nil // never seen
	}
	t, _ := parseTime(ts)
	return t, nil
}

// SetActivitySeenAt records when the user last viewed the activity feed.
func (s *ActivityStore) SetActivitySeenAt(ctx context.Context, userID, workspaceID string, seenAt time.Time) error {
	tsVal := seenAt.UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO activity_state (user_id, workspace_id, last_seen_at)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (user_id, workspace_id) DO UPDATE SET last_seen_at = $3`,
		userID, workspaceID, tsVal)
	return err
}

// CountNewActivities returns the number of activities newer than the user's last seen timestamp.
func (s *ActivityStore) CountNewActivities(ctx context.Context, userID, workspaceID string) (int, error) {
	seenAt, _ := s.GetActivitySeenAt(ctx, userID, workspaceID)
	if seenAt.IsZero() {
		return 0, nil // never viewed = no indicator (avoid showing dot on first visit)
	}
	tsVal := seenAt.UTC().Format(time.RFC3339Nano)

	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM activities WHERE workspace_id = $1 AND created_at > $2`,
		workspaceID, tsVal).Scan(&count)
	return count, err
}
