package agent

import "github.com/neokapi/neokapi/bowrain/storage"

// migrations defines the complete PostgreSQL agent-store schema.
// Bowrain is not yet in production; there is no migration history to
// preserve, so we keep a single baseline migration that represents
// the current design.
var migrations = []storage.Migration{
	{
		Version:     1,
		Description: "agent schema (baseline)",
		SQL: `
			CREATE TABLE agent_conversations (
				id           TEXT PRIMARY KEY,
				workspace_id TEXT NOT NULL,
				user_id      TEXT NOT NULL,
				project_id   TEXT NOT NULL DEFAULT '',
				title        TEXT NOT NULL DEFAULT '',
				status       TEXT NOT NULL DEFAULT 'active',
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX idx_agent_conv_workspace_user ON agent_conversations(workspace_id, user_id);

			CREATE TABLE agent_messages (
				id              TEXT PRIMARY KEY,
				conversation_id TEXT NOT NULL REFERENCES agent_conversations(id) ON DELETE CASCADE,
				role            TEXT NOT NULL,
				content         TEXT NOT NULL DEFAULT '',
				input_tokens    INTEGER NOT NULL DEFAULT 0,
				output_tokens   INTEGER NOT NULL DEFAULT 0,
				created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX idx_agent_msg_conv ON agent_messages(conversation_id, created_at);

			CREATE TABLE agent_tool_calls (
				id         TEXT PRIMARY KEY,
				message_id TEXT NOT NULL REFERENCES agent_messages(id) ON DELETE CASCADE,
				tool_name  TEXT NOT NULL,
				input      JSONB NOT NULL DEFAULT '{}',
				output     JSONB NOT NULL DEFAULT '{}',
				status     TEXT NOT NULL DEFAULT 'pending',
				duration   BIGINT NOT NULL DEFAULT 0,
				error      TEXT NOT NULL DEFAULT ''
			);
			CREATE INDEX idx_agent_tc_msg ON agent_tool_calls(message_id);

			CREATE TABLE agent_config (
				workspace_id      TEXT PRIMARY KEY,
				enabled           BOOLEAN NOT NULL DEFAULT FALSE,
				allowed_tools     JSONB NOT NULL DEFAULT '[]',
				denied_tools      JSONB NOT NULL DEFAULT '[]',
				require_approval  JSONB NOT NULL DEFAULT '[]',
				code_exec_enabled BOOLEAN NOT NULL DEFAULT FALSE,
				max_concurrent    INTEGER NOT NULL DEFAULT 3
			);

			CREATE TABLE agent_usage (
				id              TEXT PRIMARY KEY,
				workspace_id    TEXT NOT NULL,
				user_id         TEXT NOT NULL,
				conversation_id TEXT NOT NULL,
				message_id      TEXT NOT NULL DEFAULT '',
				kind            TEXT NOT NULL,
				input_tokens    INTEGER NOT NULL DEFAULT 0,
				output_tokens   INTEGER NOT NULL DEFAULT 0,
				duration_sec    DOUBLE PRECISION NOT NULL DEFAULT 0,
				created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX idx_agent_usage_ws_created ON agent_usage(workspace_id, created_at);
		`,
	},
}
