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
				id            TEXT PRIMARY KEY,
				email         TEXT UNIQUE NOT NULL,
				name          TEXT NOT NULL,
				avatar_url    TEXT NOT NULL DEFAULT '',
				oidc_sub      TEXT NOT NULL DEFAULT '',
				onboarded_at  TIMESTAMPTZ,
				created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
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

			-- Slug rename history: when a workspace is renamed, the old slug is
			-- reserved for a grace period (default 30d) so it cannot be reused
			-- for impersonation. Reservations are GC'd after expiry.
			CREATE TABLE workspace_slug_reservations (
				slug           TEXT PRIMARY KEY,
				workspace_id   TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
				reserved_until TIMESTAMPTZ NOT NULL,
				created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX idx_slug_reservations_until ON workspace_slug_reservations(reserved_until);

			-- Email-change requests: a verification token is sent to the new
			-- address. Confirmation writes the new email through to Keycloak
			-- via the admin API and updates users.email.
			CREATE TABLE email_change_requests (
				id         TEXT PRIMARY KEY,
				user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				new_email  TEXT NOT NULL,
				token_hash TEXT UNIQUE NOT NULL,
				expires_at TIMESTAMPTZ NOT NULL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX idx_email_change_user ON email_change_requests(user_id);
			CREATE INDEX idx_email_change_expires ON email_change_requests(expires_at);
		`,
	},
	// PR #428 added onboarded_at, workspace_slug_reservations, and
	// email_change_requests in the v1 baseline. Existing dev DBs already
	// have v1 applied without these, so roll them forward idempotently.
	// Issue #430.
	{
		Version:     2,
		Description: "add onboarded_at + slug reservations + email-change requests (#428 catch-up)",
		SQL: `
			ALTER TABLE users
				ADD COLUMN IF NOT EXISTS onboarded_at TIMESTAMPTZ;

			CREATE TABLE IF NOT EXISTS workspace_slug_reservations (
				slug           TEXT PRIMARY KEY,
				workspace_id   TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
				reserved_until TIMESTAMPTZ NOT NULL,
				created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_slug_reservations_until
				ON workspace_slug_reservations(reserved_until);

			CREATE TABLE IF NOT EXISTS email_change_requests (
				id         TEXT PRIMARY KEY,
				user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				new_email  TEXT NOT NULL,
				token_hash TEXT UNIQUE NOT NULL,
				expires_at TIMESTAMPTZ NOT NULL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_email_change_user
				ON email_change_requests(user_id);
			CREATE INDEX IF NOT EXISTS idx_email_change_expires
				ON email_change_requests(expires_at);
		`,
	},
	{
		Version:     3,
		Description: "groups, deny rules, workspace role overrides, separation-of-duties settings",
		SQL: `
			-- Groups (teams): bind a set of users to project roles in bulk.
			CREATE TABLE IF NOT EXISTS groups (
				id           TEXT PRIMARY KEY,
				workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
				name         TEXT NOT NULL,
				description  TEXT NOT NULL DEFAULT '',
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				UNIQUE (workspace_id, name)
			);
			CREATE INDEX IF NOT EXISTS idx_groups_workspace ON groups(workspace_id);

			CREATE TABLE IF NOT EXISTS group_members (
				group_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
				user_id  TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				PRIMARY KEY (group_id, user_id)
			);
			CREATE INDEX IF NOT EXISTS idx_group_members_user ON group_members(user_id);

			-- Group → project role bindings. Languages scope is JSON (empty = all).
			CREATE TABLE IF NOT EXISTS group_role_bindings (
				id           TEXT PRIMARY KEY,
				group_id     TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
				workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
				project_id   TEXT NOT NULL,
				role_id      TEXT NOT NULL,
				languages    TEXT NOT NULL DEFAULT '[]',
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_group_bindings_project ON group_role_bindings(project_id);
			CREATE INDEX IF NOT EXISTS idx_group_bindings_group ON group_role_bindings(group_id);

			-- Deny rules: negative permissions that always win over grants.
			-- subject_type is 'user' | 'role' | 'group'; project_id empty = workspace-wide.
			CREATE TABLE IF NOT EXISTS deny_rules (
				id           TEXT PRIMARY KEY,
				workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
				subject_type TEXT NOT NULL,
				subject_id   TEXT NOT NULL,
				project_id   TEXT NOT NULL DEFAULT '',
				denied_perms BIGINT NOT NULL DEFAULT 0,
				reason       TEXT NOT NULL DEFAULT '',
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_deny_rules_workspace ON deny_rules(workspace_id);

			-- Per-workspace overrides for the default permissions of the four
			-- built-in workspace roles (owner/admin/member/viewer), so the
			-- workspace-role fallback is tunable without code changes.
			CREATE TABLE IF NOT EXISTS workspace_role_overrides (
				workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
				role         TEXT NOT NULL,
				permissions  BIGINT NOT NULL DEFAULT 0,
				PRIMARY KEY (workspace_id, role)
			);

			-- Separation-of-duties policy per workspace. mode is 'off' | 'warn' | 'block'.
			CREATE TABLE IF NOT EXISTS sod_settings (
				workspace_id TEXT PRIMARY KEY REFERENCES workspaces(id) ON DELETE CASCADE,
				mode         TEXT NOT NULL DEFAULT 'warn',
				updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
		`,
	},
}
