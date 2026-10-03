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
// repaired. Retired numbers are never reused; later versions follow it.
//
//	3  tool policies name apply_edits where they named update_block
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
	{
		Version:     3,
		Description: "tool policies name apply_edits where they named update_block",
		// The agent writes content with apply_edits, after reading it with
		// read_blocks. A policy matches tools by name, so a denial or an
		// approval that named update_block moves to apply_edits, and an allow
		// list that admitted it admits the edit tools.
		SQL: `
			UPDATE agent_config SET denied_tools = (
				SELECT COALESCE(jsonb_agg(DISTINCT CASE WHEN t = 'update_block' THEN 'apply_edits' ELSE t END), '[]'::jsonb)
				FROM jsonb_array_elements_text(denied_tools) AS t)
			WHERE denied_tools @> '["update_block"]'::jsonb;

			UPDATE agent_config SET require_approval = (
				SELECT COALESCE(jsonb_agg(DISTINCT CASE WHEN t = 'update_block' THEN 'apply_edits' ELSE t END), '[]'::jsonb)
				FROM jsonb_array_elements_text(require_approval) AS t)
			WHERE require_approval @> '["update_block"]'::jsonb;

			UPDATE agent_config SET allowed_tools = (
				SELECT COALESCE(jsonb_agg(DISTINCT t), '[]'::jsonb) FROM (
					SELECT CASE WHEN e = 'update_block' THEN 'apply_edits' ELSE e END AS t
					FROM jsonb_array_elements_text(allowed_tools) AS e
					UNION ALL SELECT 'read_blocks'
					UNION ALL SELECT 'describe_format') AS tools)
			WHERE allowed_tools @> '["update_block"]'::jsonb;
		`,
	},
}
