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

// TaskType classifies tasks.
type TaskType string

const (
	TaskTranslate      TaskType = "translate"
	TaskReview         TaskType = "review"
	TaskReviewTerms    TaskType = "review_terms"
	TaskFixQuality     TaskType = "fix_quality"
	TaskFixBrandVoice  TaskType = "fix_brand_voice"
	TaskFixTerminology TaskType = "fix_terminology"
	TaskConnectorSetup TaskType = "connector_setup"
	TaskCustom         TaskType = "custom"
)

// TaskStatus tracks task lifecycle.
type TaskStatus string

const (
	TaskStatusOpen       TaskStatus = "open"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusCancelled  TaskStatus = "cancelled"
)

// TaskPriority defines task urgency.
type TaskPriority string

const (
	TaskPriorityLow    TaskPriority = "low"
	TaskPriorityNormal TaskPriority = "normal"
	TaskPriorityHigh   TaskPriority = "high"
	TaskPriorityUrgent TaskPriority = "urgent"
)

// Task is an actionable work item assigned to a person.
type Task struct {
	ID          string            `json:"id"`
	WorkspaceID string            `json:"workspace_id"`
	ProjectID   string            `json:"project_id"`
	Stream      string            `json:"stream,omitempty"`
	Type        TaskType          `json:"type"`
	Status      TaskStatus        `json:"status"`
	Priority    TaskPriority      `json:"priority"`
	Title       string            `json:"title"`
	Description string            `json:"description,omitempty"`
	AssigneeID  string            `json:"assignee_id,omitempty"`
	CreatedBy   string            `json:"created_by"`
	CompletedBy string            `json:"completed_by,omitempty"`
	Data        map[string]string `json:"data,omitempty"`
	DueAt       *time.Time        `json:"due_at,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	CompletedAt *time.Time        `json:"completed_at,omitempty"`
}

// TaskQuery defines filters for listing tasks.
type TaskQuery struct {
	WorkspaceID string
	ProjectID   string
	AssigneeID  string
	Status      string     // empty = all
	Type        string     // empty = all
	Priority    string     // empty = all
	DueBefore   *time.Time // if set, only tasks with due_at <= this time
	Limit       int
	Cursor      string // created_at cursor
}

// TaskResult is a paginated result set.
type TaskResult struct {
	Tasks      []Task `json:"tasks"`
	NextCursor string `json:"next_cursor,omitempty"`
}

// TaskStore persists tasks.
type TaskStore struct {
	db      *sql.DB
	dialect Dialect
}

// NewTaskStore creates a SQLite-backed task store.
func NewTaskStore(db *sql.DB) *TaskStore {
	return &TaskStore{db: db, dialect: DialectSQLite}
}

// NewPostgresTaskStore creates a PostgreSQL-backed task store.
func NewPostgresTaskStore(db *sql.DB) *TaskStore {
	return &TaskStore{db: db, dialect: DialectPostgres}
}

func (s *TaskStore) q(query string) string {
	return Rebind(s.dialect, query)
}

// Create inserts a new task.
func (s *TaskStore) Create(ctx context.Context, t *Task) error {
	if t.ID == "" {
		t.ID = id.New()
	}
	now := time.Now().UTC()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	t.UpdatedAt = now
	if t.Status == "" {
		t.Status = TaskStatusOpen
	}
	if t.Priority == "" {
		t.Priority = TaskPriorityNormal
	}
	if t.Data == nil {
		t.Data = map[string]string{}
	}

	dataJSON, _ := json.Marshal(t.Data)
	var dueAt interface{}
	if t.DueAt != nil {
		dueAt = t.DueAt.UTC().Format(time.RFC3339)
	}
	var completedAt interface{}
	if t.CompletedAt != nil {
		completedAt = t.CompletedAt.UTC().Format(time.RFC3339)
	}

	_, err := s.db.ExecContext(ctx, s.q(
		`INSERT INTO tasks (id, workspace_id, project_id, stream, type, status, priority,
		 title, description, assignee_id, created_by, completed_by, data, due_at,
		 created_at, updated_at, completed_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		t.ID, t.WorkspaceID, t.ProjectID, t.Stream,
		string(t.Type), string(t.Status), string(t.Priority),
		t.Title, t.Description, t.AssigneeID, t.CreatedBy, t.CompletedBy,
		string(dataJSON), dueAt,
		t.CreatedAt.UTC().Format(time.RFC3339),
		t.UpdatedAt.UTC().Format(time.RFC3339),
		completedAt)
	return err
}

// Get retrieves a task by ID.
func (s *TaskStore) Get(ctx context.Context, taskID string) (*Task, error) {
	row := s.db.QueryRowContext(ctx, s.q(
		`SELECT id, workspace_id, project_id, stream, type, status, priority,
		 title, description, assignee_id, created_by, completed_by, data, due_at,
		 created_at, updated_at, completed_at
		 FROM tasks WHERE id = ?`), taskID)
	return scanTask(row)
}

// List returns tasks matching the query.
func (s *TaskStore) List(ctx context.Context, q TaskQuery) (*TaskResult, error) {
	if q.Limit <= 0 {
		q.Limit = 50
	}

	var where []string
	var args []any

	if q.WorkspaceID != "" {
		where = append(where, "workspace_id = ?")
		args = append(args, q.WorkspaceID)
	}
	if q.ProjectID != "" {
		where = append(where, "project_id = ?")
		args = append(args, q.ProjectID)
	}
	if q.AssigneeID != "" {
		where = append(where, "assignee_id = ?")
		args = append(args, q.AssigneeID)
	}
	if q.Status != "" {
		where = append(where, "status = ?")
		args = append(args, q.Status)
	}
	if q.Type != "" {
		where = append(where, "type = ?")
		args = append(args, q.Type)
	}
	if q.Priority != "" {
		where = append(where, "priority = ?")
		args = append(args, q.Priority)
	}
	if q.DueBefore != nil {
		where = append(where, "due_at IS NOT NULL AND due_at <= ?")
		args = append(args, q.DueBefore.UTC().Format(time.RFC3339))
	}
	if q.Cursor != "" {
		where = append(where, "created_at < ?")
		args = append(args, q.Cursor)
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	query := fmt.Sprintf(
		`SELECT id, workspace_id, project_id, stream, type, status, priority,
		 title, description, assignee_id, created_by, completed_by, data, due_at,
		 created_at, updated_at, completed_at
		 FROM tasks %s ORDER BY created_at DESC LIMIT ?`, whereClause)
	args = append(args, q.Limit+1)

	rows, err := s.db.QueryContext(ctx, s.q(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, *t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := &TaskResult{}
	if len(tasks) > q.Limit {
		result.Tasks = tasks[:q.Limit]
		result.NextCursor = tasks[q.Limit-1].CreatedAt.Format(time.RFC3339Nano)
	} else {
		result.Tasks = tasks
	}

	return result, nil
}

// Update updates a task's mutable fields.
func (s *TaskStore) Update(ctx context.Context, t *Task) error {
	t.UpdatedAt = time.Now().UTC()
	dataJSON, _ := json.Marshal(t.Data)
	var dueAt interface{}
	if t.DueAt != nil {
		dueAt = t.DueAt.UTC().Format(time.RFC3339)
	}
	var completedAt interface{}
	if t.CompletedAt != nil {
		completedAt = t.CompletedAt.UTC().Format(time.RFC3339)
	}

	_, err := s.db.ExecContext(ctx, s.q(
		`UPDATE tasks SET status = ?, priority = ?, title = ?, description = ?,
		 assignee_id = ?, completed_by = ?, data = ?, due_at = ?,
		 updated_at = ?, completed_at = ?
		 WHERE id = ?`),
		string(t.Status), string(t.Priority), t.Title, t.Description,
		t.AssigneeID, t.CompletedBy, string(dataJSON), dueAt,
		t.UpdatedAt.UTC().Format(time.RFC3339), completedAt,
		t.ID)
	return err
}

// Assign assigns a task to a user and sets status to in_progress.
func (s *TaskStore) Assign(ctx context.Context, taskID, assigneeID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, s.q(
		`UPDATE tasks SET assignee_id = ?, status = 'in_progress', updated_at = ?
		 WHERE id = ? AND status IN ('open', 'in_progress')`),
		assigneeID, now, taskID)
	return err
}

// Complete marks a task as completed.
func (s *TaskStore) Complete(ctx context.Context, taskID, userID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, s.q(
		`UPDATE tasks SET status = 'completed', completed_by = ?, completed_at = ?, updated_at = ?
		 WHERE id = ? AND status IN ('open', 'in_progress')`),
		userID, now, now, taskID)
	return err
}

// Cancel marks a task as cancelled.
func (s *TaskStore) Cancel(ctx context.Context, taskID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, s.q(
		`UPDATE tasks SET status = 'cancelled', updated_at = ?
		 WHERE id = ? AND status IN ('open', 'in_progress')`),
		now, taskID)
	return err
}

// Delete removes a task.
func (s *TaskStore) Delete(ctx context.Context, taskID string) error {
	_, err := s.db.ExecContext(ctx, s.q(
		`DELETE FROM tasks WHERE id = ?`), taskID)
	return err
}

func scanTask(row scanner) (*Task, error) {
	var t Task
	var typ, status, priority, dataJSON, createdAtStr, updatedAtStr string
	var dueAtStr, completedAtStr sql.NullString

	err := row.Scan(
		&t.ID, &t.WorkspaceID, &t.ProjectID, &t.Stream,
		&typ, &status, &priority,
		&t.Title, &t.Description, &t.AssigneeID, &t.CreatedBy, &t.CompletedBy,
		&dataJSON, &dueAtStr, &createdAtStr, &updatedAtStr, &completedAtStr,
	)
	if err != nil {
		return nil, err
	}

	t.Type = TaskType(typ)
	t.Status = TaskStatus(status)
	t.Priority = TaskPriority(priority)
	t.CreatedAt, _ = parseTime(createdAtStr)
	t.UpdatedAt, _ = parseTime(updatedAtStr)

	if dueAtStr.Valid && dueAtStr.String != "" {
		if d, err := parseTime(dueAtStr.String); err == nil {
			t.DueAt = &d
		}
	}
	if completedAtStr.Valid && completedAtStr.String != "" {
		if c, err := parseTime(completedAtStr.String); err == nil {
			t.CompletedAt = &c
		}
	}
	if dataJSON != "" {
		_ = json.Unmarshal([]byte(dataJSON), &t.Data)
	}

	return &t, nil
}
