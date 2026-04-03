package event

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
)

// AuditLogger subscribes to all events and persists them to the audit_log table.
// PostgreSQL-only — the server always uses PostgreSQL for the content store.
type AuditLogger struct {
	db  *sql.DB
	bus platev.EventBus
	sub *platev.Subscription
}

// AuditEntry represents a single audit log record.
type AuditEntry struct {
	ID        int64     `json:"id"`
	ProjectID string    `json:"project_id"`
	EventType string    `json:"event_type"`
	Actor     string    `json:"actor"`
	Source    string    `json:"source"`
	Data      string    `json:"data"` // JSON string
	CreatedAt time.Time `json:"created_at"`
}

// NewAuditLogger creates and starts an audit logger that persists all events.
func NewAuditLogger(db *sql.DB, bus platev.EventBus) *AuditLogger {
	a := &AuditLogger{db: db, bus: bus}
	a.sub = bus.SubscribeGroup("audit-logger", a.handleEvent)
	return a
}

func (a *AuditLogger) handleEvent(ev platev.Event) {
	if ev.ProjectID == "" {
		return
	}

	dataJSON, err := json.Marshal(ev.Data)
	if err != nil {
		dataJSON = []byte("{}")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = a.db.ExecContext(ctx,
		`INSERT INTO audit_log (project_id, event_type, actor, source, data, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		ev.ProjectID, string(ev.Type), ev.Actor, ev.Source, string(dataJSON), ev.Timestamp)
	if err != nil {
		log.Printf("WARNING: audit logger failed to persist event %s (type=%s): %v", ev.ID, ev.Type, err)
	}
}

// Close unsubscribes from the event bus.
func (a *AuditLogger) Close() {
	if a.sub != nil {
		a.bus.Unsubscribe(a.sub)
	}
}

// AuditQuery specifies filters for querying audit log entries.
type AuditQuery struct {
	ProjectID   string // Filter by project ID (empty = all projects in workspace)
	WorkspaceID string // Filter by workspace (requires join with projects table)
	EventType   string // Filter by event type prefix (e.g. "project" matches "project.created")
	Actor       string // Filter by actor
	Search      string // Free-text search in event data (JSONB)
	Limit       int
	Offset      int
}

// ListAuditLog returns audit entries for a project, optionally filtered by event type.
func (a *AuditLogger) ListAuditLog(ctx context.Context, projectID, eventType string, limit, offset int) ([]AuditEntry, error) {
	if limit <= 0 {
		limit = 50
	}

	var query string
	var args []any
	if eventType != "" {
		query = `SELECT id, project_id, event_type, actor, source, data, created_at
			FROM audit_log WHERE project_id = $1 AND event_type = $2
			ORDER BY created_at DESC LIMIT $3 OFFSET $4`
		args = []any{projectID, eventType, limit, offset}
	} else {
		query = `SELECT id, project_id, event_type, actor, source, data, created_at
			FROM audit_log WHERE project_id = $1
			ORDER BY created_at DESC LIMIT $2 OFFSET $3`
		args = []any{projectID, limit, offset}
	}

	return a.scanEntries(ctx, query, args)
}

// QueryAuditLog returns audit entries matching the given query filters.
// Supports workspace-scoped queries by joining with the projects table.
func (a *AuditLogger) QueryAuditLog(ctx context.Context, q AuditQuery) ([]AuditEntry, error) {
	if q.Limit <= 0 {
		q.Limit = 50
	}

	var where []string
	var args []any
	argIdx := 0
	ph := func() string {
		argIdx++
		return fmt.Sprintf("$%d", argIdx)
	}

	from := "audit_log a"

	if q.WorkspaceID != "" {
		from = "audit_log a JOIN projects p ON a.project_id = p.id"
		where = append(where, fmt.Sprintf("p.workspace_id = %s", ph()))
		args = append(args, q.WorkspaceID)
	}
	if q.ProjectID != "" {
		where = append(where, fmt.Sprintf("a.project_id = %s", ph()))
		args = append(args, q.ProjectID)
	}
	if q.EventType != "" {
		where = append(where, fmt.Sprintf("a.event_type LIKE %s", ph()))
		args = append(args, q.EventType+"%")
	}
	if q.Actor != "" {
		where = append(where, fmt.Sprintf("a.actor = %s", ph()))
		args = append(args, q.Actor)
	}
	if q.Search != "" {
		where = append(where, fmt.Sprintf("a.data::text ILIKE %s", ph()))
		args = append(args, "%"+q.Search+"%")
	}

	var qb strings.Builder
	qb.WriteString("SELECT a.id, a.project_id, a.event_type, a.actor, a.source, a.data, a.created_at FROM ")
	qb.WriteString(from)
	if len(where) > 0 {
		qb.WriteString(" WHERE ")
		qb.WriteString(strings.Join(where, " AND "))
	}
	fmt.Fprintf(&qb, " ORDER BY a.created_at DESC LIMIT %s OFFSET %s", ph(), ph())
	query := qb.String()
	args = append(args, q.Limit, q.Offset)

	return a.scanEntries(ctx, query, args)
}

func (a *AuditLogger) scanEntries(ctx context.Context, query string, args []any) ([]AuditEntry, error) {
	rows, err := a.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.EventType, &e.Actor, &e.Source, &e.Data, &e.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
