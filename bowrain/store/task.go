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
	TaskSourceReview   TaskType = "source_review"
	TaskCustom         TaskType = "custom"
)

// IsVolume reports whether this task type's count grows with the amount of
// content pushed, rather than with the number of anomalies found.
//
// The distinction decides who may hold the task. Volume work is a contributor's
// — free, uncapped, assigned by language. Everything else is an escalation: what
// a check flagged and no rule resolves, or a proposal to change what governs
// content. A custodian seat is priced against the review function it replaces,
// so a custodian sitting on a volume queue is the failure that would invalidate
// the price — see TaskClass and routeTaskClass.
func (t TaskType) IsVolume() bool {
	switch t {
	case TaskTranslate, TaskReview, TaskSourceReview:
		return true
	default:
		return false
	}
}

// TaskClass names which of the two queues a task belongs to. It is written onto
// every task Data map under TaskDataClass so the ratio between them is
// queryable: the standing invariant is that tasks per custodian tracks anomaly
// count, not content pushed, and an invariant nobody can measure is a wish.
type TaskClass string

const (
	// TaskClassVolume grows with content. Contributors hold it.
	TaskClassVolume TaskClass = "volume"
	// TaskClassEscalation grows with anomalies and governance proposals.
	// Custodians hold it.
	TaskClassEscalation TaskClass = "escalation"
)

// TaskDataClass is the Data key carrying a task's TaskClass.
const TaskDataClass = "class"

// ClassOf returns the class a task type belongs to.
func ClassOf(t TaskType) TaskClass {
	if t.IsVolume() {
		return TaskClassVolume
	}
	return TaskClassEscalation
}

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
	Status      string     // empty = all; use Statuses for multi-status filter
	Statuses    []string   // if set, matches any of these statuses (overrides Status)
	Type        string     // empty = all; use Types for a multi-type filter
	Types       []string   // if set, matches any of these types (overrides Type)
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
	db *sql.DB
}

// NewTaskStore creates a PostgreSQL-backed task store.
func NewTaskStore(db *sql.DB) *TaskStore {
	return &TaskStore{db: db}
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
	// The class is stamped here rather than at each call site so no future task
	// can be created without one. It is derived from the type, so a caller
	// cannot disagree with it either: the ratio between the two queues is the
	// standing invariant, and an invariant that depends on every caller
	// remembering is not one.
	t.Data[TaskDataClass] = string(ClassOf(t.Type))

	dataJSON, _ := json.Marshal(t.Data)
	var dueAt any
	if t.DueAt != nil {
		dueAt = t.DueAt.UTC().Format(time.RFC3339)
	}
	var completedAt any
	if t.CompletedAt != nil {
		completedAt = t.CompletedAt.UTC().Format(time.RFC3339)
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO tasks (id, workspace_id, project_id, stream, type, status, priority,
		 title, description, assignee_id, created_by, completed_by, data, due_at,
		 created_at, updated_at, completed_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`,
		t.ID, t.WorkspaceID, t.ProjectID, t.Stream,
		string(t.Type), string(t.Status), string(t.Priority),
		t.Title, t.Description, t.AssigneeID, t.CreatedBy, t.CompletedBy,
		string(dataJSON), dueAt,
		t.CreatedAt.UTC().Format(time.RFC3339Nano),
		t.UpdatedAt.UTC().Format(time.RFC3339Nano),
		completedAt)
	return err
}

// Get retrieves a task by ID.
func (s *TaskStore) Get(ctx context.Context, taskID string) (*Task, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, workspace_id, project_id, stream, type, status, priority,
		 title, description, assignee_id, created_by, completed_by, data, due_at,
		 created_at, updated_at, completed_at
		 FROM tasks WHERE id = $1`, taskID)
	return scanTask(row)
}

// taskWhere renders the query's filters as a WHERE clause of $N placeholder
// tokens plus the values they bind. Every value travels through args; the
// clause text is assembled from constants only.
//
// Counts and the list share it, so the kanban column totals and the rows a
// column pages through always describe the same set.
func taskWhere(q TaskQuery) (clause string, args []any) {
	var where []string
	next := func() string { return fmt.Sprintf("$%d", len(args)+1) }

	if q.WorkspaceID != "" {
		where = append(where, "workspace_id = "+next())
		args = append(args, q.WorkspaceID)
	}
	if q.ProjectID != "" {
		where = append(where, "project_id = "+next())
		args = append(args, q.ProjectID)
	}
	if q.AssigneeID != "" {
		where = append(where, "assignee_id = "+next())
		args = append(args, q.AssigneeID)
	}
	if len(q.Statuses) > 0 {
		placeholders := make([]string, len(q.Statuses))
		for i, st := range q.Statuses {
			placeholders[i] = next()
			args = append(args, st)
		}
		where = append(where, "status IN ("+strings.Join(placeholders, ",")+")")
	} else if q.Status != "" {
		where = append(where, "status = "+next())
		args = append(args, q.Status)
	}
	if len(q.Types) > 0 {
		placeholders := make([]string, len(q.Types))
		for i, ty := range q.Types {
			placeholders[i] = next()
			args = append(args, ty)
		}
		where = append(where, "type IN ("+strings.Join(placeholders, ",")+")")
	} else if q.Type != "" {
		where = append(where, "type = "+next())
		args = append(args, q.Type)
	}
	if q.Priority != "" {
		where = append(where, "priority = "+next())
		args = append(args, q.Priority)
	}
	if q.DueBefore != nil {
		where = append(where, "due_at IS NOT NULL AND due_at <= "+next())
		args = append(args, q.DueBefore.UTC().Format(time.RFC3339))
	}
	if q.Cursor != "" {
		where = append(where, "created_at < "+next())
		args = append(args, q.Cursor)
	}

	if len(where) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(where, " AND "), args
}

// List returns tasks matching the query.
func (s *TaskStore) List(ctx context.Context, q TaskQuery) (*TaskResult, error) {
	if q.Limit <= 0 {
		q.Limit = 50
	}

	whereClause, args := taskWhere(q)

	query := fmt.Sprintf(
		`SELECT id, workspace_id, project_id, stream, type, status, priority,
		 title, description, assignee_id, created_by, completed_by, data, due_at,
		 created_at, updated_at, completed_at
		 FROM tasks %s ORDER BY created_at DESC LIMIT $%d`, whereClause, len(args)+1)
	args = append(args, q.Limit+1)

	rows, err := s.db.QueryContext(ctx, query, args...)
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

// TaskCounts is the status rollup for a task query: one total per status plus
// the sum across all of them.
type TaskCounts struct {
	ByStatus map[string]int `json:"by_status"`
	Total    int            `json:"total"`
}

// Counts returns how many tasks match the query, grouped by status. Limit and
// Cursor are ignored — a column header counts the whole set, not the page. The
// map carries every known status, zero-filled, so a board renders an empty
// column rather than omitting it.
func (s *TaskStore) Counts(ctx context.Context, q TaskQuery) (*TaskCounts, error) {
	q.Limit, q.Cursor = 0, ""
	whereClause, args := taskWhere(q)

	query := fmt.Sprintf(`SELECT status, COUNT(*) FROM tasks %s GROUP BY status`, whereClause)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := &TaskCounts{ByStatus: map[string]int{
		string(TaskStatusOpen):       0,
		string(TaskStatusInProgress): 0,
		string(TaskStatusCompleted):  0,
		string(TaskStatusCancelled):  0,
	}}
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return nil, err
		}
		counts.ByStatus[status] = n
		counts.Total += n
	}
	return counts, rows.Err()
}

// CountOpenByType returns how many tasks of the given type are open or
// in_progress for a project — a cheap COUNT(*) the review-loop gate uses to
// detect when the last pending review closed (the whole open review queue is
// empty) so it can hand off to a completing convergence run.
func (s *TaskStore) CountOpenByType(ctx context.Context, workspaceID, projectID, taskType string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tasks
		 WHERE workspace_id = $1 AND project_id = $2 AND type = $3
		   AND status IN ('open', 'in_progress')`,
		workspaceID, projectID, taskType).Scan(&n)
	return n, err
}

// Update updates a task's mutable fields.
func (s *TaskStore) Update(ctx context.Context, t *Task) error {
	t.UpdatedAt = time.Now().UTC()
	dataJSON, _ := json.Marshal(t.Data)
	var dueAt any
	if t.DueAt != nil {
		dueAt = t.DueAt.UTC().Format(time.RFC3339)
	}
	var completedAt any
	if t.CompletedAt != nil {
		completedAt = t.CompletedAt.UTC().Format(time.RFC3339)
	}

	_, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET status = $1, priority = $2, title = $3, description = $4,
		 assignee_id = $5, completed_by = $6, data = $7, due_at = $8,
		 updated_at = $9, completed_at = $10
		 WHERE id = $11`,
		string(t.Status), string(t.Priority), t.Title, t.Description,
		t.AssigneeID, t.CompletedBy, string(dataJSON), dueAt,
		t.UpdatedAt.UTC().Format(time.RFC3339Nano), completedAt,
		t.ID)
	return err
}

// Assign assigns a task to a user and sets status to in_progress.
func (s *TaskStore) Assign(ctx context.Context, taskID, assigneeID string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET assignee_id = $1, status = 'in_progress', updated_at = $2
		 WHERE id = $3 AND status IN ('open', 'in_progress')`,
		assigneeID, now, taskID)
	return err
}

// Complete marks a task as completed.
func (s *TaskStore) Complete(ctx context.Context, taskID, userID string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET status = 'completed', completed_by = $1, completed_at = $2, updated_at = $3
		 WHERE id = $4 AND status IN ('open', 'in_progress')`,
		userID, now, now, taskID)
	return err
}

// Cancel marks a task as cancelled.
func (s *TaskStore) Cancel(ctx context.Context, taskID string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET status = 'cancelled', updated_at = $1
		 WHERE id = $2 AND status IN ('open', 'in_progress')`,
		now, taskID)
	return err
}

// Delete removes a task.
func (s *TaskStore) Delete(ctx context.Context, taskID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM tasks WHERE id = $1`, taskID)
	return err
}

func scanTask(row scanner) (*Task, error) {
	var t Task
	var typ, status, priority, dataJSON string
	var dueAt, completedAt, createdAt, updatedAt scanTime

	err := row.Scan(
		&t.ID, &t.WorkspaceID, &t.ProjectID, &t.Stream,
		&typ, &status, &priority,
		&t.Title, &t.Description, &t.AssigneeID, &t.CreatedBy, &t.CompletedBy,
		&dataJSON, &dueAt, &createdAt, &updatedAt, &completedAt,
	)
	if err != nil {
		return nil, err
	}

	t.Type = TaskType(typ)
	t.Status = TaskStatus(status)
	t.Priority = TaskPriority(priority)
	t.CreatedAt = createdAt.Time
	t.UpdatedAt = updatedAt.Time

	if dueAt.Valid {
		d := dueAt.Time.UTC()
		t.DueAt = &d
	}
	if completedAt.Valid {
		c := completedAt.Time.UTC()
		t.CompletedAt = &c
	}
	if dataJSON != "" {
		if err := json.Unmarshal([]byte(dataJSON), &t.Data); err != nil {
			return nil, fmt.Errorf("task %s: unmarshal data: %w", t.ID, err)
		}
	}

	return &t, nil
}
