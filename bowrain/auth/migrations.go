package auth

import "github.com/neokapi/neokapi/bowrain/storage"

// authMigrationsPg defines the complete PostgreSQL auth schema.
// Bowrain is not yet in production; there is no migration history to
// preserve, so we keep a single baseline migration that represents
// the current design.
var authMigrationsPg = []storage.Migration{
	{
		Version:     1,
		Description: "auth schema (baseline)",
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
				id                   TEXT PRIMARY KEY,
				name                 TEXT NOT NULL,
				slug                 TEXT UNIQUE NOT NULL,
				description          TEXT NOT NULL DEFAULT '',
				logo_url             TEXT NOT NULL DEFAULT '',
				type                 TEXT NOT NULL DEFAULT 'team',
				languages            TEXT NOT NULL DEFAULT '[]',
				plan                 TEXT NOT NULL DEFAULT 'free',
				stripe_customer_id   TEXT,
				dashboard_visibility TEXT NOT NULL DEFAULT 'private',
				pulse_term_sources   TEXT NOT NULL DEFAULT '{"terminology":true,"brand_vocabulary":false}',
				pulse_access_key     TEXT NOT NULL DEFAULT '',
				created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);

			CREATE TABLE workspace_members (
				workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
				user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				role         TEXT NOT NULL DEFAULT 'member',
				joined_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (workspace_id, user_id)
			);

			CREATE TABLE unclaimed_projects (
				project_id               TEXT PRIMARY KEY,
				claim_token              TEXT UNIQUE NOT NULL,
				name                     TEXT NOT NULL,
				default_source_language  TEXT NOT NULL,
				target_languages         TEXT NOT NULL,
				created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				expires_at               TIMESTAMPTZ NOT NULL
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
