package agent

import "github.com/neokapi/neokapi/bowrain/storage"

// Migrations is the agent-store schema as a single consolidated baseline.
//
// LEDGER — every version this subsystem has ever issued, now folded in:
//
//	1  agent schema (baseline)
//
// Baseline is version 2 — above every number issued, so an existing database
// applies it once and any drift between its schema and its bookkeeping is
// repaired. Retired numbers are never reused; the next migration is version 3.
var Migrations = []storage.Migration{
	{
		Version:     2,
		Description: "agent baseline (folds 1)",
		SQL: `
			CREATE TABLE IF NOT EXISTS agent_conversations (
				id           TEXT PRIMARY KEY,
				workspace_id TEXT NOT NULL,
				user_id      TEXT NOT NULL,
				project_id   TEXT NOT NULL DEFAULT '',
				title        TEXT NOT NULL DEFAULT '',
				status       TEXT NOT NULL DEFAULT 'active',
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_agent_conv_workspace_user ON agent_conversations(workspace_id, user_id);

			CREATE TABLE IF NOT EXISTS agent_messages (
				id              TEXT PRIMARY KEY,
				conversation_id TEXT NOT NULL REFERENCES agent_conversations(id) ON DELETE CASCADE,
				role            TEXT NOT NULL,
				content         TEXT NOT NULL DEFAULT '',
				input_tokens    INTEGER NOT NULL DEFAULT 0,
				output_tokens   INTEGER NOT NULL DEFAULT 0,
				created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_agent_msg_conv ON agent_messages(conversation_id, created_at);

			CREATE TABLE IF NOT EXISTS agent_tool_calls (
				id         TEXT PRIMARY KEY,
				message_id TEXT NOT NULL REFERENCES agent_messages(id) ON DELETE CASCADE,
				tool_name  TEXT NOT NULL,
				input      JSONB NOT NULL DEFAULT '{}',
				output     JSONB NOT NULL DEFAULT '{}',
				status     TEXT NOT NULL DEFAULT 'pending',
				duration   BIGINT NOT NULL DEFAULT 0,
				error      TEXT NOT NULL DEFAULT ''
			);
			CREATE INDEX IF NOT EXISTS idx_agent_tc_msg ON agent_tool_calls(message_id);

			CREATE TABLE IF NOT EXISTS agent_config (
				workspace_id      TEXT PRIMARY KEY,
				enabled           BOOLEAN NOT NULL DEFAULT FALSE,
				allowed_tools     JSONB NOT NULL DEFAULT '[]',
				denied_tools      JSONB NOT NULL DEFAULT '[]',
				require_approval  JSONB NOT NULL DEFAULT '[]',
				code_exec_enabled BOOLEAN NOT NULL DEFAULT FALSE,
				max_concurrent    INTEGER NOT NULL DEFAULT 3
			);

			CREATE TABLE IF NOT EXISTS agent_usage (
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
			CREATE INDEX IF NOT EXISTS idx_agent_usage_ws_created ON agent_usage(workspace_id, created_at);
		`,
	},
}
