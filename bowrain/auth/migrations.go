package auth

import "github.com/neokapi/neokapi/bowrain/storage"

// Migrations is the complete PostgreSQL auth schema as a single consolidated
// baseline.
//
// LEDGER — every version this subsystem has ever issued, now folded in:
//
//	1  auth schema (baseline)
//	2  add onboarded_at + slug reservations + email-change requests (#428 catch-up)
//	3  groups, deny rules, workspace role overrides, separation-of-duties settings
//	4  per-workspace preferred AI model (customer model choice)
//	5  refresh-token reuse detection (family_id + consumed_at)
//	6  workspace default voice profile (hierarchical resolver base)
//	7  user locale preference (locale-aware transactional emails)
//	8  auth baseline (folded 1-7)
//	9  auth baseline (folds 1-8) + api token machine identity
//	10 auth baseline (folds 1-9) + workspace voice profile column rename
//
// Versions 2, 4 and 6 were catch-ups: PR #428 had already added their columns
// and tables to the v1 baseline, so they existed only to roll forward
// databases that applied v1 before it was amended. A baseline creates
// everything at once, so they leave no trace here beyond this ledger.
//
// The v5 backfill (UPDATE refresh_tokens SET family_id = id WHERE family_id =
// ”) is not carried. It gave each pre-existing token its own singleton family
// so a legacy token could never trigger a cross-token family wipe; a database
// built from this baseline has no such tokens, and every token minted since
// carries a family_id from the code.
//
// Baseline is version 11 — above every number issued, so an existing database
// applies it once and any drift between its schema and its bookkeeping is
// repaired. Retired numbers are never reused; the next migration is version 12.
//
// The subsystem carries exactly one baseline (migrations/schema_test.go
// enforces it), so a schema change is made by editing the baseline in place and
// bumping its version. Version 11 adds the coordinate columns that scope a
// membership to a region of the context space, plus the workspace a custodian
// seat is billed to. They reach an existing database through the ALTERs beside
// each CREATE, and an existing database reads the baseline again only because
// the version moved — which is the lesson version 10 paid for: its column
// rename reached the CREATE and stopped there, so it moved for databases built
// after it and stayed put for every database built before, and every workspace
// query in production failed until the bump landed.
var Migrations = []storage.Migration{
	{
		Version:     11,
		Description: "auth baseline (folds 1-10) + membership coordinates and custodian seat billing",
		SQL: `
			CREATE TABLE IF NOT EXISTS users (
				id            TEXT PRIMARY KEY,
				email         TEXT UNIQUE NOT NULL,
				name          TEXT NOT NULL,
				avatar_url    TEXT NOT NULL DEFAULT '',
				oidc_sub      TEXT NOT NULL DEFAULT '',
				onboarded_at  TIMESTAMPTZ,
				-- BCP-47 primary subtag captured from Accept-Language at first
				-- OIDC sign-in (empty = unknown → English). Drives the
				-- locale-aware transactional-email send path.
				locale        TEXT NOT NULL DEFAULT '',
				created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_users_oidc_sub ON users(oidc_sub);

			CREATE TABLE IF NOT EXISTS workspaces (
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
				preferred_model      TEXT NOT NULL DEFAULT '',
				voice_profile_id     TEXT NOT NULL DEFAULT '',
				created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			-- The CREATE above serves an empty database; this serves one that
			-- already has workspaces, where CREATE ... IF NOT EXISTS is a no-op
			-- and would leave the column under its former name while every
			-- query in postgres.go asks for the new one.
			--
			-- RENAME COLUMN takes no IF EXISTS, so the guard is explicit: rename
			-- only where the old name is still there and the new one is not.
			DO $$
			BEGIN
				IF EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_schema = current_schema() AND table_name = 'workspaces' AND column_name = 'brand_voice_profile_id'
				) AND NOT EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_schema = current_schema() AND table_name = 'workspaces' AND column_name = 'voice_profile_id'
				) THEN
					ALTER TABLE workspaces RENAME COLUMN brand_voice_profile_id TO voice_profile_id;
				END IF;
			END $$;

			CREATE TABLE IF NOT EXISTS workspace_members (
				workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
				user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				role         TEXT NOT NULL DEFAULT 'member',
				joined_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (workspace_id, user_id)
			);

			CREATE TABLE IF NOT EXISTS unclaimed_projects (
				project_id               TEXT PRIMARY KEY,
				claim_token              TEXT UNIQUE NOT NULL,
				name                     TEXT NOT NULL,
				default_source_language  TEXT NOT NULL,
				target_languages         TEXT NOT NULL,
				created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				expires_at               TIMESTAMPTZ NOT NULL
			);
			CREATE INDEX IF NOT EXISTS idx_unclaimed_expires ON unclaimed_projects(expires_at);

			CREATE TABLE IF NOT EXISTS workspace_invites (
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

			-- Rotating refresh tokens with reuse detection. family_id groups a
			-- rotation chain: initial login opens a family, each rotation mints
			-- the successor in the same family. consumed_at marks a token as
			-- already rotated; presenting a consumed token again is the
			-- signature of a stolen, replayed token and revokes the whole family.
			CREATE TABLE IF NOT EXISTS refresh_tokens (
				id          TEXT PRIMARY KEY,
				user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				token_hash  TEXT NOT NULL UNIQUE,
				family_id   TEXT NOT NULL DEFAULT '',
				consumed_at TIMESTAMPTZ,
				expires_at  TIMESTAMPTZ NOT NULL,
				created_at  TIMESTAMPTZ NOT NULL
			);
			CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user ON refresh_tokens(user_id);
			CREATE INDEX IF NOT EXISTS idx_refresh_tokens_family ON refresh_tokens(family_id);

			CREATE TABLE IF NOT EXISTS api_tokens (
				id           TEXT PRIMARY KEY,
				user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
				name         TEXT NOT NULL,
				-- The machine that holds this token, when it is a machine token:
				-- a CI runner, a kapi-action step, an agent-driven kapi. Work
				-- authored under it is attributed to "agent/<agent_name>" rather
				-- than to user_id, so the person whose token it is stays an
				-- eligible reviewer of what the machine proposes. '' = an
				-- ordinary personal token.
				agent_name   TEXT NOT NULL DEFAULT '',
				token_hash   TEXT UNIQUE NOT NULL,
				token_prefix TEXT NOT NULL,
				scopes       TEXT NOT NULL DEFAULT '["*"]',
				last_used_at TIMESTAMPTZ,
				expires_at   TIMESTAMPTZ,
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			-- The CREATE above serves an empty database; this serves one that
			-- already has api_tokens, where CREATE ... IF NOT EXISTS is a no-op
			-- and would leave the new column missing. Both are idempotent, both
			-- land the same column, and neither is a second baseline.
			ALTER TABLE api_tokens ADD COLUMN IF NOT EXISTS agent_name TEXT NOT NULL DEFAULT '';
			CREATE INDEX IF NOT EXISTS idx_api_tokens_workspace ON api_tokens(workspace_id);
			CREATE INDEX IF NOT EXISTS idx_api_tokens_user ON api_tokens(user_id);

			CREATE TABLE IF NOT EXISTS role_templates (
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

			CREATE TABLE IF NOT EXISTS project_members (
				project_id   TEXT NOT NULL,
				user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				role_id      TEXT NOT NULL,
				workspace_id TEXT NOT NULL,
				languages    TEXT NOT NULL DEFAULT '[]',
				-- The region of the project's context space this membership
				-- governs, as a JSON object of axis → value. '{}' is the whole
				-- space, which is what every membership written before regions
				-- existed carries.
				coordinates  TEXT NOT NULL DEFAULT '{}',
				-- The workspace that pays for this membership when it is a
				-- custodian seat. Always workspace_id today; it exists so the
				-- seat count a plan is priced on does not have to be retrofitted
				-- when an external custodian is billed to their own workspace.
				billed_to_workspace_id TEXT NOT NULL DEFAULT '',
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (project_id, user_id)
			);
			-- The CREATE above serves an empty database; these serve one that
			-- already has project_members, where CREATE ... IF NOT EXISTS is a
			-- no-op and would leave the new columns missing.
			ALTER TABLE project_members ADD COLUMN IF NOT EXISTS coordinates TEXT NOT NULL DEFAULT '{}';
			ALTER TABLE project_members ADD COLUMN IF NOT EXISTS billed_to_workspace_id TEXT NOT NULL DEFAULT '';
			CREATE INDEX IF NOT EXISTS idx_project_members_user ON project_members(user_id, workspace_id);
			CREATE INDEX IF NOT EXISTS idx_project_members_role ON project_members(workspace_id, role_id);

			-- Slug rename history: when a workspace is renamed, the old slug is
			-- reserved for a grace period (default 30d) so it cannot be reused
			-- for impersonation. Reservations are GC'd after expiry.
			CREATE TABLE IF NOT EXISTS workspace_slug_reservations (
				slug           TEXT PRIMARY KEY,
				workspace_id   TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
				reserved_until TIMESTAMPTZ NOT NULL,
				created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_slug_reservations_until ON workspace_slug_reservations(reserved_until);

			-- Email-change requests: a verification token is sent to the new
			-- address. Confirmation writes the new email through to Keycloak
			-- via the admin API and updates users.email.
			CREATE TABLE IF NOT EXISTS email_change_requests (
				id         TEXT PRIMARY KEY,
				user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				new_email  TEXT NOT NULL,
				token_hash TEXT UNIQUE NOT NULL,
				expires_at TIMESTAMPTZ NOT NULL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_email_change_user ON email_change_requests(user_id);
			CREATE INDEX IF NOT EXISTS idx_email_change_expires ON email_change_requests(expires_at);

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
			-- Regions on group bindings, for the same reason as on
			-- project_members: permission resolution unions both sources, so a
			-- binding without one would hand out unconstrained custody through a
			-- door the direct grant had closed.
			CREATE TABLE IF NOT EXISTS group_role_bindings (
				id           TEXT PRIMARY KEY,
				group_id     TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
				workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
				project_id   TEXT NOT NULL,
				role_id      TEXT NOT NULL,
				languages    TEXT NOT NULL DEFAULT '[]',
				coordinates  TEXT NOT NULL DEFAULT '{}',
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			ALTER TABLE group_role_bindings ADD COLUMN IF NOT EXISTS coordinates TEXT NOT NULL DEFAULT '{}';
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
