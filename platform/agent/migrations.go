package agent

import "github.com/neokapi/neokapi/bowrain/storage"

// SQLite migrations for the agent store.
var sqliteMigrations = []storage.Migration{
	{
		Version:     1,
		Description: "create agent conversations table",
		SQL: `
			CREATE TABLE agent_conversations (
				id           TEXT PRIMARY KEY,
				workspace_id TEXT NOT NULL,
				user_id      TEXT NOT NULL,
				project_id   TEXT NOT NULL DEFAULT '',
				title        TEXT NOT NULL DEFAULT '',
				status       TEXT NOT NULL DEFAULT 'active',
				created_at   TEXT NOT NULL DEFAULT (datetime('now')),
				updated_at   TEXT NOT NULL DEFAULT (datetime('now'))
			);
			CREATE INDEX idx_agent_conv_workspace_user ON agent_conversations(workspace_id, user_id);
		`,
	},
	{
		Version:     2,
		Description: "create agent messages table",
		SQL: `
			CREATE TABLE agent_messages (
				id              TEXT PRIMARY KEY,
				conversation_id TEXT NOT NULL REFERENCES agent_conversations(id) ON DELETE CASCADE,
				role            TEXT NOT NULL,
				content         TEXT NOT NULL DEFAULT '',
				created_at      TEXT NOT NULL DEFAULT (datetime('now'))
			);
			CREATE INDEX idx_agent_msg_conv ON agent_messages(conversation_id, created_at);
		`,
	},
	{
		Version:     3,
		Description: "create agent tool calls table",
		SQL: `
			CREATE TABLE agent_tool_calls (
				id         TEXT PRIMARY KEY,
				message_id TEXT NOT NULL REFERENCES agent_messages(id) ON DELETE CASCADE,
				tool_name  TEXT NOT NULL,
				input      TEXT NOT NULL DEFAULT '{}',
				output     TEXT NOT NULL DEFAULT '{}',
				status     TEXT NOT NULL DEFAULT 'pending',
				duration   INTEGER NOT NULL DEFAULT 0,
				error      TEXT NOT NULL DEFAULT ''
			);
			CREATE INDEX idx_agent_tc_msg ON agent_tool_calls(message_id);
		`,
	},
	{
		Version:     4,
		Description: "create agent config table",
		SQL: `
			CREATE TABLE agent_config (
				workspace_id     TEXT PRIMARY KEY,
				enabled          INTEGER NOT NULL DEFAULT 0,
				allowed_tools    TEXT NOT NULL DEFAULT '[]',
				denied_tools     TEXT NOT NULL DEFAULT '[]',
				require_approval TEXT NOT NULL DEFAULT '[]',
				code_exec_enabled INTEGER NOT NULL DEFAULT 0,
				max_concurrent   INTEGER NOT NULL DEFAULT 3
			);
		`,
	},
}

// PostgreSQL migrations for the agent store.
var postgresMigrations = []storage.Migration{
	{
		Version:     1,
		Description: "create agent conversations table",
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
		`,
	},
	{
		Version:     2,
		Description: "create agent messages table",
		SQL: `
			CREATE TABLE agent_messages (
				id              TEXT PRIMARY KEY,
				conversation_id TEXT NOT NULL REFERENCES agent_conversations(id) ON DELETE CASCADE,
				role            TEXT NOT NULL,
				content         TEXT NOT NULL DEFAULT '',
				created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX idx_agent_msg_conv ON agent_messages(conversation_id, created_at);
		`,
	},
	{
		Version:     3,
		Description: "create agent tool calls table",
		SQL: `
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
		`,
	},
	{
		Version:     4,
		Description: "create agent config table",
		SQL: `
			CREATE TABLE agent_config (
				workspace_id      TEXT PRIMARY KEY,
				enabled           BOOLEAN NOT NULL DEFAULT FALSE,
				allowed_tools     JSONB NOT NULL DEFAULT '[]',
				denied_tools      JSONB NOT NULL DEFAULT '[]',
				require_approval  JSONB NOT NULL DEFAULT '[]',
				code_exec_enabled BOOLEAN NOT NULL DEFAULT FALSE,
				max_concurrent    INTEGER NOT NULL DEFAULT 3
			);
		`,
	},
}
