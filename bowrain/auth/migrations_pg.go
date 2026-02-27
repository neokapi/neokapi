package auth

import "github.com/gokapi/gokapi/bowrain/storage"

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
}
