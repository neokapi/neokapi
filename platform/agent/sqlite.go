package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/core/id"
	platagent "github.com/neokapi/neokapi/platform/agent"
)

// SQLiteStore implements AgentStore using SQLite.
type SQLiteStore struct {
	db *storage.DB
}

// NewSQLiteStore opens (or creates) a SQLite-backed AgentStore.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := storage.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open agent database: %w", err)
	}
	if err := storage.Migrate(db, sqliteMigrations); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate agent schema: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

// NewSQLiteStoreFromDB wraps an existing DB.
func NewSQLiteStoreFromDB(db *storage.DB) (*SQLiteStore, error) {
	if err := storage.Migrate(db, sqliteMigrations); err != nil {
		return nil, fmt.Errorf("migrate agent schema: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Close() error { return s.db.Close() }

// ---------------------------------------------------------------------------
// Conversations
// ---------------------------------------------------------------------------

func (s *SQLiteStore) CreateConversation(ctx context.Context, conv *platagent.Conversation) error {
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
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		conv.ID, conv.WorkspaceID, conv.UserID, conv.ProjectID, conv.Title, string(conv.Status),
		now.Format(time.RFC3339), now.Format(time.RFC3339))
	return err
}

func (s *SQLiteStore) GetConversation(ctx context.Context, convID string) (*platagent.Conversation, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, workspace_id, user_id, project_id, title, status, created_at, updated_at
		 FROM agent_conversations WHERE id = ?`, convID)
	return scanConversationSQLite(row)
}

func (s *SQLiteStore) ListConversations(ctx context.Context, workspaceID, userID string, limit, offset int) ([]*platagent.Conversation, int, error) {
	if limit <= 0 {
		limit = 20
	}

	var total int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM agent_conversations WHERE workspace_id = ? AND user_id = ?`,
		workspaceID, userID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, workspace_id, user_id, project_id, title, status, created_at, updated_at
		 FROM agent_conversations
		 WHERE workspace_id = ? AND user_id = ?
		 ORDER BY updated_at DESC LIMIT ? OFFSET ?`,
		workspaceID, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var convs []*platagent.Conversation
	for rows.Next() {
		c, err := scanConversationSQLiteRows(rows)
		if err != nil {
			return nil, 0, err
		}
		convs = append(convs, c)
	}
	return convs, total, rows.Err()
}

func (s *SQLiteStore) UpdateConversation(ctx context.Context, conv *platagent.Conversation) error {
	conv.UpdatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`UPDATE agent_conversations SET title = ?, status = ?, project_id = ?, updated_at = ? WHERE id = ?`,
		conv.Title, string(conv.Status), conv.ProjectID, conv.UpdatedAt.Format(time.RFC3339), conv.ID)
	return err
}

func (s *SQLiteStore) DeleteConversation(ctx context.Context, convID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM agent_conversations WHERE id = ?`, convID)
	return err
}

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

func (s *SQLiteStore) AddMessage(ctx context.Context, msg *platagent.Message) error {
	if msg.ID == "" {
		msg.ID = id.New()
	}
	msg.CreatedAt = time.Now().UTC()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_messages (id, conversation_id, role, content, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		msg.ID, msg.ConversationID, string(msg.Role), msg.Content, msg.CreatedAt.Format(time.RFC3339))
	return err
}

func (s *SQLiteStore) ListMessages(ctx context.Context, conversationID string, limit, offset int) ([]*platagent.Message, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, conversation_id, role, content, created_at
		 FROM agent_messages
		 WHERE conversation_id = ?
		 ORDER BY created_at ASC LIMIT ? OFFSET ?`,
		conversationID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []*platagent.Message
	for rows.Next() {
		var m platagent.Message
		var createdAt string
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &createdAt); err != nil {
			return nil, err
		}
		m.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		msgs = append(msgs, &m)
	}

	// Attach tool calls to each message.
	for _, m := range msgs {
		tcs, err := s.listToolCalls(ctx, m.ID)
		if err != nil {
			return nil, err
		}
		m.ToolCalls = tcs
	}

	return msgs, rows.Err()
}

func (s *SQLiteStore) listToolCalls(ctx context.Context, messageID string) ([]platagent.ToolCall, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, message_id, tool_name, input, output, status, duration, error
		 FROM agent_tool_calls WHERE message_id = ?`, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tcs []platagent.ToolCall
	for rows.Next() {
		var tc platagent.ToolCall
		var input, output string
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

func (s *SQLiteStore) AddToolCall(ctx context.Context, tc *platagent.ToolCall) error {
	if tc.ID == "" {
		tc.ID = id.New()
	}
	input := string(tc.Input)
	if input == "" {
		input = "{}"
	}
	output := string(tc.Output)
	if output == "" {
		output = "{}"
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_tool_calls (id, message_id, tool_name, input, output, status, duration, error)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		tc.ID, tc.MessageID, tc.ToolName, input, output, string(tc.Status), int64(tc.Duration), tc.Error)
	return err
}

func (s *SQLiteStore) UpdateToolCall(ctx context.Context, tc *platagent.ToolCall) error {
	output := string(tc.Output)
	if output == "" {
		output = "{}"
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE agent_tool_calls SET output = ?, status = ?, duration = ?, error = ? WHERE id = ?`,
		output, string(tc.Status), int64(tc.Duration), tc.Error, tc.ID)
	return err
}

// ---------------------------------------------------------------------------
// Config
// ---------------------------------------------------------------------------

func (s *SQLiteStore) GetAgentConfig(ctx context.Context, workspaceID string) (*platagent.AgentConfig, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT workspace_id, enabled, allowed_tools, denied_tools, require_approval, code_exec_enabled, max_concurrent
		 FROM agent_config WHERE workspace_id = ?`, workspaceID)

	var cfg platagent.AgentConfig
	var enabled, codeExec int
	var allowedJSON, deniedJSON, approvalJSON string
	err := row.Scan(&cfg.WorkspaceID, &enabled, &allowedJSON, &deniedJSON, &approvalJSON, &codeExec, &cfg.MaxConcurrent)
	if err == sql.ErrNoRows {
		return defaultConfig(workspaceID), nil
	}
	if err != nil {
		return nil, err
	}
	cfg.Enabled = enabled != 0
	cfg.CodeExecEnabled = codeExec != 0
	_ = json.Unmarshal([]byte(allowedJSON), &cfg.AllowedTools)
	_ = json.Unmarshal([]byte(deniedJSON), &cfg.DeniedTools)
	_ = json.Unmarshal([]byte(approvalJSON), &cfg.RequireApproval)
	return &cfg, nil
}

func (s *SQLiteStore) SaveAgentConfig(ctx context.Context, cfg *platagent.AgentConfig) error {
	allowedJSON, _ := json.Marshal(cfg.AllowedTools)
	deniedJSON, _ := json.Marshal(cfg.DeniedTools)
	approvalJSON, _ := json.Marshal(cfg.RequireApproval)

	enabled := 0
	if cfg.Enabled {
		enabled = 1
	}
	codeExec := 0
	if cfg.CodeExecEnabled {
		codeExec = 1
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_config (workspace_id, enabled, allowed_tools, denied_tools, require_approval, code_exec_enabled, max_concurrent)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(workspace_id) DO UPDATE SET
			enabled = excluded.enabled,
			allowed_tools = excluded.allowed_tools,
			denied_tools = excluded.denied_tools,
			require_approval = excluded.require_approval,
			code_exec_enabled = excluded.code_exec_enabled,
			max_concurrent = excluded.max_concurrent`,
		cfg.WorkspaceID, enabled, string(allowedJSON), string(deniedJSON), string(approvalJSON), codeExec, cfg.MaxConcurrent)
	return err
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func defaultConfig(workspaceID string) *platagent.AgentConfig {
	return &platagent.AgentConfig{
		WorkspaceID:   workspaceID,
		Enabled:       false,
		MaxConcurrent: 3,
	}
}

func scanConversationSQLite(row *sql.Row) (*platagent.Conversation, error) {
	var c platagent.Conversation
	var createdAt, updatedAt string
	err := row.Scan(&c.ID, &c.WorkspaceID, &c.UserID, &c.ProjectID, &c.Title, &c.Status, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	c.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &c, nil
}

func scanConversationSQLiteRows(rows *sql.Rows) (*platagent.Conversation, error) {
	var c platagent.Conversation
	var createdAt, updatedAt string
	err := rows.Scan(&c.ID, &c.WorkspaceID, &c.UserID, &c.ProjectID, &c.Title, &c.Status, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	c.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &c, nil
}
