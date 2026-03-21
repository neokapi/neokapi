package auth

import "github.com/neokapi/neokapi/bowrain/storage"

// authMigrationsPg is a single clean PostgreSQL schema that represents the
// final state of all 8 incremental SQLite auth migrations consolidated into one.
var authMigrationsPg = []storage.Migration{
	{
		Version:     1,
		Description: "create auth schema",
		SQL: `
			CREATE TABLE users (
				id         TEXT PRIMARY KEY,
				email      TEXT UNIQUE NOT NULL,
				name       TEXT NOT NULL,
				avatar_url TEXT NOT NULL DEFAULT '',
				oidc_sub   TEXT NOT NULL DEFAULT '',
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX idx_users_oidc_sub ON users(oidc_sub);

			CREATE TABLE workspaces (
				id          TEXT PRIMARY KEY,
				name        TEXT NOT NULL,
				slug        TEXT UNIQUE NOT NULL,
				description TEXT NOT NULL DEFAULT '',
				logo_url    TEXT NOT NULL DEFAULT '',
				type        TEXT NOT NULL DEFAULT 'team',
				created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);

			CREATE TABLE workspace_members (
				workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
				user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				role         TEXT NOT NULL DEFAULT 'member',
				joined_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (workspace_id, user_id)
			);

			CREATE TABLE unclaimed_projects (
				project_id     TEXT PRIMARY KEY,
				claim_token    TEXT UNIQUE NOT NULL,
				name           TEXT NOT NULL,
				source_locale  TEXT NOT NULL,
				target_locales TEXT NOT NULL,
				created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				expires_at     TIMESTAMPTZ NOT NULL
			);
			CREATE INDEX idx_unclaimed_expires ON unclaimed_projects(expires_at);

			CREATE TABLE workspace_invites (
				id           TEXT PRIMARY KEY,
				workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
				code         TEXT UNIQUE NOT NULL,
				email        TEXT,
				role         TEXT NOT NULL DEFAULT 'member',
				max_uses     INTEGER NOT NULL DEFAULT 1,
				use_count    INTEGER NOT NULL DEFAULT 0,
				created_by   TEXT NOT NULL REFERENCES users(id),
				expires_at   TIMESTAMPTZ NOT NULL,
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);

			CREATE TABLE refresh_tokens (
				id         TEXT PRIMARY KEY,
				user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				token_hash TEXT NOT NULL UNIQUE,
				expires_at TIMESTAMPTZ NOT NULL,
				created_at TIMESTAMPTZ NOT NULL
			);
			CREATE INDEX idx_refresh_tokens_user ON refresh_tokens(user_id);
		`,
	},
	{
		Version:     2,
		Description: "create api_tokens table",
		SQL: `
			CREATE TABLE api_tokens (
				id           TEXT PRIMARY KEY,
				user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
				name         TEXT NOT NULL,
				token_hash   TEXT UNIQUE NOT NULL,
				token_prefix TEXT NOT NULL,
				scopes       TEXT NOT NULL DEFAULT '["*"]',
				last_used_at TIMESTAMPTZ,
				expires_at   TIMESTAMPTZ,
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX idx_api_tokens_workspace ON api_tokens(workspace_id);
			CREATE INDEX idx_api_tokens_user ON api_tokens(user_id);
		`,
	},
	{
		Version:     3,
		Description: "add languages to workspaces and rename locale columns in unclaimed_projects",
		SQL: `
			ALTER TABLE workspaces ADD COLUMN languages TEXT NOT NULL DEFAULT '[]';
			ALTER TABLE unclaimed_projects RENAME COLUMN source_locale TO default_source_language;
			ALTER TABLE unclaimed_projects RENAME COLUMN target_locales TO target_languages;
		`,
	},
	{
		Version:     4,
		Description: "add plan and stripe_customer_id to workspaces",
		SQL: `
			ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS plan TEXT NOT NULL DEFAULT 'free';
			ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS stripe_customer_id TEXT;
		`,
	},
	{
		Version:     5,
		Description: "create role_templates and project_members tables",
		SQL: `
			CREATE TABLE role_templates (
				id           TEXT NOT NULL,
				workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
				name         TEXT NOT NULL,
				display_name TEXT NOT NULL DEFAULT '',
				description  TEXT NOT NULL DEFAULT '',
				permissions  BIGINT NOT NULL DEFAULT 0,
				is_builtin   BOOLEAN NOT NULL DEFAULT FALSE,
				position     INTEGER NOT NULL DEFAULT 0,
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (workspace_id, id),
				UNIQUE (workspace_id, name)
			);

			CREATE TABLE project_members (
				project_id   TEXT NOT NULL,
				user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				role_id      TEXT NOT NULL,
				workspace_id TEXT NOT NULL,
				languages    TEXT NOT NULL DEFAULT '[]',
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (project_id, user_id)
			);
			CREATE INDEX idx_project_members_user ON project_members(user_id, workspace_id);
			CREATE INDEX idx_project_members_role ON project_members(workspace_id, role_id);
		`,
	},
}
