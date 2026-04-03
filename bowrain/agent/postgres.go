package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/core/id"
	platagent "github.com/neokapi/neokapi/bowrain/core/agent"
)

// PostgresStore implements AgentStore using PostgreSQL.
type PostgresStore struct {
	db *sql.DB
}

// NewPostgresStore creates a PostgreSQL-backed AgentStore from a shared PgDB.
func NewPostgresStore(pgDB *storage.PgDB) (*PostgresStore, error) {
	if err := storage.MigratePostgresNS(pgDB, "agent_schema_migrations", postgresMigrations); err != nil {
		return nil, fmt.Errorf("migrate agent schema: %w", err)
	}
	return &PostgresStore{db: pgDB.DB}, nil
}

func (s *PostgresStore) Close() error { return nil } // shared DB; don't close

// ---------------------------------------------------------------------------
// Conversations
// ---------------------------------------------------------------------------

func (s *PostgresStore) CreateConversation(ctx context.Context, conv *platagent.Conversation) error {
	if conv.ID == "" {
		conv.ID = id.New()
	}
	now := time.Now().UTC()
	conv.CreatedAt = now
	conv.UpdatedAt = now
	if conv.Status == "" {
		conv.Status = platagent.ConversationActive
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_conversations (id, workspace_id, user_id, project_id, title, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		conv.ID, conv.WorkspaceID, conv.UserID, conv.ProjectID, conv.Title, string(conv.Status), now, now)
	return err
}

func (s *PostgresStore) GetConversation(ctx context.Context, convID string) (*platagent.Conversation, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, workspace_id, user_id, project_id, title, status, created_at, updated_at
		 FROM agent_conversations WHERE id = $1`, convID)
	return scanConversationPg(row)
}

func (s *PostgresStore) ListConversations(ctx context.Context, workspaceID, userID string, limit, offset int) ([]*platagent.Conversation, int, error) {
	if limit <= 0 {
		limit = 20
	}

	var total int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM agent_conversations WHERE workspace_id = $1 AND user_id = $2`,
		workspaceID, userID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, workspace_id, user_id, project_id, title, status, created_at, updated_at
		 FROM agent_conversations
		 WHERE workspace_id = $1 AND user_id = $2
		 ORDER BY updated_at DESC LIMIT $3 OFFSET $4`,
		workspaceID, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var convs []*platagent.Conversation
	for rows.Next() {
		c, err := scanConversationPgRows(rows)
		if err != nil {
			return nil, 0, err
		}
		convs = append(convs, c)
	}
	return convs, total, rows.Err()
}

func (s *PostgresStore) UpdateConversation(ctx context.Context, conv *platagent.Conversation) error {
	conv.UpdatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`UPDATE agent_conversations SET title = $1, status = $2, project_id = $3, updated_at = $4 WHERE id = $5`,
		conv.Title, string(conv.Status), conv.ProjectID, conv.UpdatedAt, conv.ID)
	return err
}

func (s *PostgresStore) DeleteConversation(ctx context.Context, convID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM agent_conversations WHERE id = $1`, convID)
	return err
}

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

func (s *PostgresStore) AddMessage(ctx context.Context, msg *platagent.Message) error {
	if msg.ID == "" {
		msg.ID = id.New()
	}
	msg.CreatedAt = time.Now().UTC()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_messages (id, conversation_id, role, content, input_tokens, output_tokens, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		msg.ID, msg.ConversationID, string(msg.Role), msg.Content,
		msg.InputTokens, msg.OutputTokens, msg.CreatedAt)
	return err
}

func (s *PostgresStore) ListMessages(ctx context.Context, conversationID string, limit, offset int) ([]*platagent.Message, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, conversation_id, role, content, input_tokens, output_tokens, created_at
		 FROM agent_messages
		 WHERE conversation_id = $1
		 ORDER BY created_at ASC LIMIT $2 OFFSET $3`,
		conversationID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []*platagent.Message
	for rows.Next() {
		var m platagent.Message
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.InputTokens, &m.OutputTokens, &m.CreatedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, &m)
	}

	for _, m := range msgs {
		tcs, err := s.listToolCalls(ctx, m.ID)
		if err != nil {
			return nil, err
		}
		m.ToolCalls = tcs
	}

	return msgs, rows.Err()
}

func (s *PostgresStore) listToolCalls(ctx context.Context, messageID string) ([]platagent.ToolCall, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, message_id, tool_name, input, output, status, duration, error
		 FROM agent_tool_calls WHERE message_id = $1`, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tcs []platagent.ToolCall
	for rows.Next() {
		var tc platagent.ToolCall
		var input, output []byte
		var durationNs int64
		if err := rows.Scan(&tc.ID, &tc.MessageID, &tc.ToolName, &input, &output, &tc.Status, &durationNs, &tc.Error); err != nil {
			return nil, err
		}
		tc.Input = json.RawMessage(input)
		tc.Output = json.RawMessage(output)
		tc.Duration = time.Duration(durationNs)
		tcs = append(tcs, tc)
	}
	return tcs, rows.Err()
}

// ---------------------------------------------------------------------------
// Tool Calls
// ---------------------------------------------------------------------------

func (s *PostgresStore) AddToolCall(ctx context.Context, tc *platagent.ToolCall) error {
	if tc.ID == "" {
		tc.ID = id.New()
	}
	input := tc.Input
	if len(input) == 0 {
		input = json.RawMessage("{}")
	}
	output := tc.Output
	if len(output) == 0 {
		output = json.RawMessage("{}")
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_tool_calls (id, message_id, tool_name, input, output, status, duration, error)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		tc.ID, tc.MessageID, tc.ToolName, string(input), string(output), string(tc.Status), int64(tc.Duration), tc.Error)
	return err
}

func (s *PostgresStore) UpdateToolCall(ctx context.Context, tc *platagent.ToolCall) error {
	output := tc.Output
	if len(output) == 0 {
		output = json.RawMessage("{}")
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE agent_tool_calls SET output = $1, status = $2, duration = $3, error = $4 WHERE id = $5`,
		string(output), string(tc.Status), int64(tc.Duration), tc.Error, tc.ID)
	return err
}

// ---------------------------------------------------------------------------
// Config
// ---------------------------------------------------------------------------

func (s *PostgresStore) GetAgentConfig(ctx context.Context, workspaceID string) (*platagent.AgentConfig, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT workspace_id, enabled, allowed_tools, denied_tools, require_approval, code_exec_enabled, max_concurrent
		 FROM agent_config WHERE workspace_id = $1`, workspaceID)

	var cfg platagent.AgentConfig
	var allowedJSON, deniedJSON, approvalJSON []byte
	err := row.Scan(&cfg.WorkspaceID, &cfg.Enabled, &allowedJSON, &deniedJSON, &approvalJSON, &cfg.CodeExecEnabled, &cfg.MaxConcurrent)
	if err == sql.ErrNoRows {
		return defaultConfig(workspaceID), nil
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(allowedJSON, &cfg.AllowedTools)
	_ = json.Unmarshal(deniedJSON, &cfg.DeniedTools)
	_ = json.Unmarshal(approvalJSON, &cfg.RequireApproval)
	return &cfg, nil
}

func (s *PostgresStore) SaveAgentConfig(ctx context.Context, cfg *platagent.AgentConfig) error {
	allowedJSON, _ := json.Marshal(cfg.AllowedTools)
	deniedJSON, _ := json.Marshal(cfg.DeniedTools)
	approvalJSON, _ := json.Marshal(cfg.RequireApproval)

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_config (workspace_id, enabled, allowed_tools, denied_tools, require_approval, code_exec_enabled, max_concurrent)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT(workspace_id) DO UPDATE SET
			enabled = EXCLUDED.enabled,
			allowed_tools = EXCLUDED.allowed_tools,
			denied_tools = EXCLUDED.denied_tools,
			require_approval = EXCLUDED.require_approval,
			code_exec_enabled = EXCLUDED.code_exec_enabled,
			max_concurrent = EXCLUDED.max_concurrent`,
		cfg.WorkspaceID, cfg.Enabled, string(allowedJSON), string(deniedJSON), string(approvalJSON), cfg.CodeExecEnabled, cfg.MaxConcurrent)
	return err
}

// ---------------------------------------------------------------------------
// Usage metering
// ---------------------------------------------------------------------------

func (s *PostgresStore) RecordUsage(ctx context.Context, rec *platagent.UsageRecord) error {
	if rec.ID == "" {
		rec.ID = id.New()
	}
	rec.CreatedAt = time.Now().UTC()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_usage (id, workspace_id, user_id, conversation_id, message_id, kind, input_tokens, output_tokens, duration_sec, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		rec.ID, rec.WorkspaceID, rec.UserID, rec.ConversationID, rec.MessageID,
		rec.Kind, rec.InputTokens, rec.OutputTokens, rec.DurationSec, rec.CreatedAt)
	return err
}

func (s *PostgresStore) GetUsageSummary(ctx context.Context, workspaceID string, from, to time.Time) (*platagent.UsageSummary, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT
			COALESCE(SUM(input_tokens), 0),
			COALESCE(SUM(output_tokens), 0),
			COALESCE(SUM(duration_sec), 0),
			COUNT(*) FILTER (WHERE kind = 'tokens')
		 FROM agent_usage
		 WHERE workspace_id = $1 AND created_at >= $2 AND created_at <= $3`,
		workspaceID, from, to)

	summary := &platagent.UsageSummary{WorkspaceID: workspaceID}
	err := row.Scan(&summary.TotalInputTokens, &summary.TotalOutputTokens,
		&summary.TotalContainerSec, &summary.MessageCount)
	if err != nil {
		return nil, err
	}
	return summary, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func scanConversationPg(row *sql.Row) (*platagent.Conversation, error) {
	var c platagent.Conversation
	err := row.Scan(&c.ID, &c.WorkspaceID, &c.UserID, &c.ProjectID, &c.Title, &c.Status, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func scanConversationPgRows(rows *sql.Rows) (*platagent.Conversation, error) {
	var c platagent.Conversation
	err := rows.Scan(&c.ID, &c.WorkspaceID, &c.UserID, &c.ProjectID, &c.Title, &c.Status, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}
