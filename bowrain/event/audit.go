package event

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
)

// AuditLogger subscribes to all events and persists them to the append-only,
// hash-chained audit_log table. PostgreSQL-only — the server always uses
// PostgreSQL for the content store.
type AuditLogger struct {
	db  *sql.DB
	bus platev.EventBus
	sub *platev.Subscription
}

// AuditEntry represents a single audit log record.
type AuditEntry struct {
	ID           int64     `json:"id"`
	ChainKey     string    `json:"chain_key"`
	ProjectID    string    `json:"project_id"`
	WorkspaceID  string    `json:"workspace_id"`
	EventType    string    `json:"event_type"`
	Actor        string    `json:"actor"`
	Source       string    `json:"source"`
	ResourceType string    `json:"resource_type,omitempty"`
	ResourceID   string    `json:"resource_id,omitempty"`
	Effect       string    `json:"effect,omitempty"`
	Data         string    `json:"data"` // JSON string
	Before       string    `json:"before,omitempty"`
	After        string    `json:"after,omitempty"`
	RequestID    string    `json:"request_id,omitempty"`
	IP           string    `json:"ip,omitempty"`
	UserAgent    string    `json:"user_agent,omitempty"`
	CausationID  string    `json:"causation_id,omitempty"`
	PrevHash     string    `json:"prev_hash,omitempty"`
	Hash         string    `json:"hash,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// NewAuditLogger creates and starts an audit logger that persists all events.
func NewAuditLogger(db *sql.DB, bus platev.EventBus) *AuditLogger {
	a := &AuditLogger{db: db, bus: bus}
	a.sub = bus.SubscribeGroup("audit-logger", a.handleEvent)
	return a
}

// chainKeyFor partitions the hash chain. Workspace-scoped events chain per
// workspace; project-only (content) events chain per project; everything else
// shares the "system" chain.
func chainKeyFor(ev platev.Event) string {
	if ev.WorkspaceID != "" {
		return ev.WorkspaceID
	}
	if ev.ProjectID != "" {
		return ev.ProjectID
	}
	return "system"
}

// canonicalPayload builds a deterministic serialization of the auditable fields
// for hashing. map[string]string marshals with sorted keys, so the output is
// stable for a given event.
func canonicalPayload(ev platev.Event, dataJSON, beforeJSON, afterJSON string) string {
	var b strings.Builder
	fields := []string{
		// Truncated to microseconds to match PostgreSQL TIMESTAMPTZ precision,
		// so a row's hash can be reproduced from its stored created_at.
		ev.Timestamp.UTC().Truncate(time.Microsecond).Format(time.RFC3339Nano),
		string(ev.Type),
		ev.Actor,
		ev.Source,
		ev.ProjectID,
		ev.WorkspaceID,
		ev.ResourceType,
		ev.ResourceID,
		ev.Effect,
		ev.RequestID,
		ev.IP,
		ev.UserAgent,
		ev.CausationID,
		dataJSON,
		beforeJSON,
		afterJSON,
	}
	for i, f := range fields {
		if i > 0 {
			b.WriteByte('\x1f') // unit separator
		}
		b.WriteString(f)
	}
	return b.String()
}

func marshalOrEmpty(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	b, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(b)
}

func (a *AuditLogger) handleEvent(ev platev.Event) {
	dataJSON, err := json.Marshal(ev.Data)
	if err != nil || len(ev.Data) == 0 {
		dataJSON = []byte("{}")
	}
	beforeJSON := marshalOrEmpty(ev.Before)
	afterJSON := marshalOrEmpty(ev.After)

	chainKey := chainKeyFor(ev)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := a.insertChained(ctx, ev, chainKey, string(dataJSON), beforeJSON, afterJSON); err != nil {
		slog.Warn("audit logger failed to persist event", "event_id", ev.ID, "event_type", ev.Type, "error", err)
	}
}

// insertChained writes one audit row, linking it to the previous row in the same
// chain via a SHA-256 hash chain. A per-chain advisory lock serializes writers
// so the chain stays consistent under concurrency.
func (a *AuditLogger) insertChained(ctx context.Context, ev platev.Event, chainKey, dataJSON, beforeJSON, afterJSON string) error {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin audit tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Serialize concurrent appends to this chain.
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('bowrain_audit'), hashtext($1))`, chainKey); err != nil {
		return fmt.Errorf("acquire audit chain lock: %w", err)
	}

	var prevHash string
	err = tx.QueryRowContext(ctx,
		`SELECT hash FROM audit_log WHERE chain_key = $1 ORDER BY id DESC LIMIT 1`, chainKey).Scan(&prevHash)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("read prev hash: %w", err)
	}

	sum := sha256.Sum256([]byte(prevHash + "\x1e" + canonicalPayload(ev, dataJSON, beforeJSON, afterJSON)))
	hash := hex.EncodeToString(sum[:])

	_, err = tx.ExecContext(ctx,
		`INSERT INTO audit_log
			(chain_key, project_id, workspace_id, event_type, actor, source,
			 resource_type, resource_id, effect, data, before_state, after_state,
			 request_id, ip, user_agent, causation_id, prev_hash, hash, created_at)
		 VALUES ($1, NULLIF($2,''), $3, $4, $5, $6, $7, $8, $9, $10,
			 NULLIF($11,'')::jsonb, NULLIF($12,'')::jsonb, $13, $14, $15, $16, $17, $18, $19)`,
		chainKey, ev.ProjectID, ev.WorkspaceID, string(ev.Type), ev.Actor, ev.Source,
		ev.ResourceType, ev.ResourceID, ev.Effect, dataJSON, beforeJSON, afterJSON,
		ev.RequestID, ev.IP, ev.UserAgent, ev.CausationID, prevHash, hash, ev.Timestamp)
	if err != nil {
		return fmt.Errorf("insert audit row: %w", err)
	}

	return tx.Commit()
}

// Close unsubscribes from the event bus.
func (a *AuditLogger) Close() {
	if a.sub != nil {
		a.bus.Unsubscribe(a.sub)
	}
}

// AuditQuery specifies filters for querying audit log entries.
type AuditQuery struct {
	ProjectID    string // Filter by project ID
	WorkspaceID  string // Filter by workspace (matches workspace_id or the project's workspace)
	EventType    string // Filter by event type prefix (e.g. "member" matches "member.added")
	Actor        string // Filter by actor
	ResourceType string // Filter by resource type
	Effect       string // Filter by effect ("deny" to see denials)
	Search       string // Free-text search in event data
	Limit        int
	Offset       int
}

const auditSelectCols = `id, chain_key, COALESCE(project_id, ''), workspace_id, event_type, actor, source,
	resource_type, resource_id, effect, data,
	COALESCE(before_state::text, ''), COALESCE(after_state::text, ''),
	request_id, ip, user_agent, causation_id, prev_hash, hash, created_at`

// ListAuditLog returns audit entries for a project, optionally filtered by event type.
func (a *AuditLogger) ListAuditLog(ctx context.Context, projectID, eventType string, limit, offset int) ([]AuditEntry, error) {
	return a.QueryAuditLog(ctx, AuditQuery{ProjectID: projectID, EventType: eventType, Limit: limit, Offset: offset})
}

// QueryAuditLog returns audit entries matching the given query filters.
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

	if q.WorkspaceID != "" {
		// Match workspace-scoped events directly, and project events whose
		// project belongs to the workspace.
		p1 := ph()
		args = append(args, q.WorkspaceID)
		p2 := ph()
		args = append(args, q.WorkspaceID)
		where = append(where, fmt.Sprintf(
			"(a.workspace_id = %s OR EXISTS (SELECT 1 FROM projects p WHERE p.id = a.project_id AND p.workspace_id = %s))", p1, p2))
	}
	if q.ProjectID != "" {
		where = append(where, "a.project_id = "+ph())
		args = append(args, q.ProjectID)
	}
	if q.EventType != "" {
		where = append(where, "a.event_type LIKE "+ph())
		args = append(args, q.EventType+"%")
	}
	if q.Actor != "" {
		where = append(where, "a.actor = "+ph())
		args = append(args, q.Actor)
	}
	if q.ResourceType != "" {
		where = append(where, "a.resource_type = "+ph())
		args = append(args, q.ResourceType)
	}
	if q.Effect != "" {
		where = append(where, "a.effect = "+ph())
		args = append(args, q.Effect)
	}
	if q.Search != "" {
		where = append(where, "a.data::text ILIKE "+ph())
		args = append(args, "%"+q.Search+"%")
	}

	var qb strings.Builder
	qb.WriteString("SELECT " + auditSelectCols + " FROM audit_log a")
	if len(where) > 0 {
		qb.WriteString(" WHERE ")
		qb.WriteString(strings.Join(where, " AND "))
	}
	fmt.Fprintf(&qb, " ORDER BY a.created_at DESC, a.id DESC LIMIT %s OFFSET %s", ph(), ph())
	args = append(args, q.Limit, q.Offset)

	return a.scanEntries(ctx, qb.String(), args)
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
		if err := rows.Scan(&e.ID, &e.ChainKey, &e.ProjectID, &e.WorkspaceID, &e.EventType, &e.Actor, &e.Source,
			&e.ResourceType, &e.ResourceID, &e.Effect, &e.Data, &e.Before, &e.After,
			&e.RequestID, &e.IP, &e.UserAgent, &e.CausationID, &e.PrevHash, &e.Hash, &e.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
