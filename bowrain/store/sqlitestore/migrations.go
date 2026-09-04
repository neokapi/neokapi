package sqlitestore

import "github.com/neokapi/neokapi/bowrain/storage"

// storeMigrations defines the SQLite content store schema. Mirrors
// bowrain/store/migrations.go (the Postgres schema) with the dialect
// differences translated (TIMESTAMPTZ → TEXT, JSONB → TEXT, BIGSERIAL →
// INTEGER PRIMARY KEY AUTOINCREMENT). Version 1 is the launch baseline;
// desktops carry long-lived local databases, so every schema change after it
// MUST be an incremental migration.
//
// RETIRED VERSION NUMBERS — never reuse: 2 ("Live Preview", AD-023/AD-036)
// and 3 ("block occurrences", AD-036) ran on live databases before being
// folded into the v1 baseline. New migrations start at 4.
var storeMigrations = []storage.Migration{
	{
		Version:     1,
		Description: "content store schema (baseline)",
		SQL: `
			-- Projects
			CREATE TABLE projects (
				id                      TEXT PRIMARY KEY,
				name                    TEXT NOT NULL,
				default_source_language TEXT NOT NULL DEFAULT '',
				target_languages        TEXT NOT NULL DEFAULT '',
				target_language_mode    TEXT NOT NULL DEFAULT 'defined',
				default_stream          TEXT NOT NULL DEFAULT '',
				dashboard_visibility    TEXT NOT NULL DEFAULT 'private',
				properties              TEXT NOT NULL DEFAULT '{}',
				workspace_id            TEXT NOT NULL DEFAULT '',
				converge_policy         TEXT NOT NULL DEFAULT 'on-push',
				archived                BOOLEAN NOT NULL DEFAULT FALSE,
				archived_at             TEXT,
				created_at              TEXT NOT NULL DEFAULT (datetime('now')),
				updated_at              TEXT NOT NULL DEFAULT (datetime('now'))
			);
			CREATE INDEX idx_projects_workspace ON projects(workspace_id);

			-- Streams
			CREATE TABLE streams (
				project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				name        TEXT NOT NULL,
				parent      TEXT NOT NULL DEFAULT '',
				base_cursor INTEGER NOT NULL DEFAULT 0,
				archived    BOOLEAN NOT NULL DEFAULT FALSE,
				visibility  TEXT NOT NULL DEFAULT 'public',
				description TEXT NOT NULL DEFAULT '',
				locked      BOOLEAN NOT NULL DEFAULT FALSE,
				locked_by   TEXT NOT NULL DEFAULT '',
				locked_at   TEXT,
				created_at  TEXT NOT NULL DEFAULT (datetime('now')),
				created_by  TEXT NOT NULL DEFAULT '',
				PRIMARY KEY (project_id, name)
			);

			CREATE TABLE stream_members (
				project_id TEXT NOT NULL,
				stream     TEXT NOT NULL,
				user_id    TEXT NOT NULL,
				added_at   TEXT NOT NULL DEFAULT (datetime('now')),
				PRIMARY KEY (project_id, stream, user_id),
				FOREIGN KEY (project_id, stream) REFERENCES streams(project_id, name) ON DELETE CASCADE
			);

			CREATE TABLE stream_tags (
				id         TEXT PRIMARY KEY,
				project_id TEXT NOT NULL,
				stream     TEXT NOT NULL,
				name       TEXT NOT NULL,
				kind       TEXT NOT NULL DEFAULT 'custom',
				cursor     INTEGER NOT NULL DEFAULT 0,
				metadata   TEXT NOT NULL DEFAULT '{}',
				created_by TEXT NOT NULL DEFAULT '',
				created_at TEXT NOT NULL DEFAULT (datetime('now')),
				FOREIGN KEY (project_id, stream) REFERENCES streams(project_id, name) ON DELETE CASCADE
			);
			CREATE UNIQUE INDEX idx_stream_tags_unique ON stream_tags(project_id, stream, name);
			CREATE INDEX idx_stream_tags_stream ON stream_tags(project_id, stream);
			CREATE INDEX idx_stream_tags_project_kind ON stream_tags(project_id, kind);

			-- Collections
			CREATE TABLE collections (
				id               TEXT NOT NULL,
				project_id       TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				name             TEXT NOT NULL,
				kind             TEXT NOT NULL DEFAULT 'uploaded',
				item_label       TEXT NOT NULL DEFAULT 'item',
				is_default       BOOLEAN NOT NULL DEFAULT FALSE,
				stream           TEXT NOT NULL DEFAULT '',
				connector_config TEXT NOT NULL DEFAULT '{}',
				created_at       TEXT NOT NULL DEFAULT (datetime('now')),
				updated_at       TEXT NOT NULL DEFAULT (datetime('now')),
				PRIMARY KEY (project_id, id)
			);
			CREATE INDEX idx_collections_project ON collections(project_id);
			CREATE UNIQUE INDEX idx_collections_default ON collections(project_id)
				WHERE is_default = TRUE;

			-- Items
			CREATE TABLE items (
				project_id    TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				name          TEXT NOT NULL,
				id            TEXT NOT NULL DEFAULT '',
				stream        TEXT NOT NULL DEFAULT 'main',
				format        TEXT NOT NULL DEFAULT '',
				item_type     TEXT NOT NULL DEFAULT 'file',
				block_index   TEXT NOT NULL DEFAULT '{}',
				preview_html  TEXT NOT NULL DEFAULT '',
				properties    TEXT NOT NULL DEFAULT '{}',
				collection_id TEXT NOT NULL DEFAULT '',
				created_at    TEXT NOT NULL DEFAULT (datetime('now')),
				updated_at    TEXT NOT NULL DEFAULT (datetime('now')),
				PRIMARY KEY (project_id, stream, name)
			);
			CREATE INDEX idx_items_project ON items(project_id);
			CREATE INDEX idx_items_project_stream ON items(project_id, stream);
			CREATE INDEX idx_items_collection ON items(project_id, collection_id);
			CREATE UNIQUE INDEX idx_items_id ON items(project_id, stream, id);

			-- Blocks hold source content + project metadata only.
			-- Targets and annotations live in their own kind-specific
			-- tables (#403/#405). Mirrors Postgres baseline.
			CREATE TABLE blocks (
				id           TEXT NOT NULL,
				project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				item_name    TEXT NOT NULL DEFAULT '',
				source_id    TEXT NOT NULL DEFAULT '',
				name         TEXT NOT NULL DEFAULT '',
				type         TEXT NOT NULL DEFAULT '',
				mime_type    TEXT NOT NULL DEFAULT '',
				translatable BOOLEAN NOT NULL DEFAULT TRUE,
				content_hash TEXT NOT NULL DEFAULT '',
				context_hash TEXT NOT NULL DEFAULT '',
				source_json  TEXT NOT NULL DEFAULT '[]',
				properties   TEXT NOT NULL DEFAULT '{}',
				stored_at    TEXT NOT NULL DEFAULT (datetime('now')),
				updated_at   TEXT NOT NULL DEFAULT (datetime('now')),
				PRIMARY KEY (project_id, id)
			);
			CREATE INDEX idx_blocks_content_hash ON blocks(content_hash);
			CREATE INDEX idx_blocks_project ON blocks(project_id);
			CREATE INDEX idx_blocks_item ON blocks(project_id, item_name);
			CREATE UNIQUE INDEX idx_blocks_source_id ON blocks(project_id, item_name, source_id)
				WHERE source_id != '';

			-- Change log
			CREATE TABLE change_log (
				seq         INTEGER PRIMARY KEY AUTOINCREMENT,
				project_id  TEXT NOT NULL,
				block_id    TEXT NOT NULL,
				change_type TEXT NOT NULL,
				locale      TEXT,
				content_hash TEXT,
				stream      TEXT NOT NULL DEFAULT 'main',
				logged_at   TEXT NOT NULL DEFAULT (datetime('now'))
			);
			CREATE INDEX idx_changelog_project_seq ON change_log(project_id, seq);
			CREATE INDEX idx_changelog_project_locale ON change_log(project_id, locale, seq);
			CREATE INDEX idx_changelog_stream ON change_log(project_id, stream, seq);

			-- Block history
			CREATE TABLE block_history (
				id          INTEGER PRIMARY KEY AUTOINCREMENT,
				project_id  TEXT NOT NULL,
				block_id    TEXT NOT NULL,
				locale      TEXT NOT NULL,
				change_type TEXT NOT NULL,
				text        TEXT NOT NULL DEFAULT '',
				coded_text  TEXT NOT NULL DEFAULT '',
				origin      TEXT NOT NULL DEFAULT '',
				author      TEXT NOT NULL DEFAULT '',
				stream      TEXT NOT NULL DEFAULT 'main',
				created_at  TEXT NOT NULL DEFAULT (datetime('now'))
			);
			CREATE INDEX idx_block_history_lookup ON block_history(project_id, block_id, locale);
			CREATE INDEX idx_block_history_stream ON block_history(project_id, stream, block_id, locale);

			-- Block notes
			CREATE TABLE block_notes (
				id         TEXT PRIMARY KEY,
				project_id TEXT NOT NULL,
				block_id   TEXT NOT NULL,
				author     TEXT NOT NULL DEFAULT '',
				text       TEXT NOT NULL,
				stream     TEXT NOT NULL DEFAULT 'main',
				created_at TEXT NOT NULL DEFAULT (datetime('now'))
			);
			CREATE INDEX idx_block_notes_lookup ON block_notes(project_id, block_id);
			CREATE INDEX idx_block_notes_stream ON block_notes(project_id, stream, block_id);

			-- Versions
			CREATE TABLE versions (
				id          TEXT PRIMARY KEY,
				project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				label       TEXT NOT NULL,
				description TEXT NOT NULL DEFAULT '',
				block_count INTEGER NOT NULL DEFAULT 0,
				created_at  TEXT NOT NULL DEFAULT (datetime('now'))
			);
			CREATE INDEX idx_versions_project ON versions(project_id);

			CREATE TABLE version_blocks (
				version_id   TEXT NOT NULL REFERENCES versions(id) ON DELETE CASCADE,
				block_id     TEXT NOT NULL,
				content_hash TEXT NOT NULL,
				PRIMARY KEY (version_id, block_id)
			);

			-- Assets (Bowrain AD-007)
			CREATE TABLE assets (
				id                TEXT PRIMARY KEY,
				project_id        TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				item_name         TEXT NOT NULL DEFAULT '',
				source_id         TEXT NOT NULL DEFAULT '',
				blob_key          TEXT NOT NULL,
				mime_type         TEXT NOT NULL,
				filename          TEXT NOT NULL DEFAULT '',
				size_bytes        INTEGER NOT NULL DEFAULT 0,
				alt_text          TEXT NOT NULL DEFAULT '',
				properties        TEXT NOT NULL DEFAULT '{}',
				processing_status TEXT NOT NULL DEFAULT 'none',
				processing_hint   TEXT NOT NULL DEFAULT '',
				stream            TEXT NOT NULL DEFAULT 'main',
				created_at        TEXT NOT NULL DEFAULT (datetime('now')),
				updated_at        TEXT NOT NULL DEFAULT (datetime('now'))
			);
			CREATE INDEX idx_assets_project_item ON assets(project_id, item_name);
			CREATE UNIQUE INDEX idx_assets_blob ON assets(project_id, blob_key)
				WHERE stream = 'main';

			CREATE TABLE asset_variants (
				asset_id   TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
				locale     TEXT NOT NULL,
				blob_key   TEXT NOT NULL,
				status     TEXT NOT NULL DEFAULT 'pending',
				mime_type  TEXT NOT NULL DEFAULT '',
				size_bytes INTEGER NOT NULL DEFAULT 0,
				properties TEXT NOT NULL DEFAULT '{}',
				created_at TEXT NOT NULL DEFAULT (datetime('now')),
				updated_at TEXT NOT NULL DEFAULT (datetime('now')),
				PRIMARY KEY (asset_id, locale)
			);

			CREATE TABLE block_asset_refs (
				project_id TEXT NOT NULL,
				block_id   TEXT NOT NULL,
				asset_id   TEXT NOT NULL,
				ref_type   TEXT NOT NULL DEFAULT 'embedded',
				stream     TEXT NOT NULL DEFAULT 'main',
				PRIMARY KEY (project_id, block_id, asset_id)
			);
			CREATE INDEX idx_block_asset_refs_asset ON block_asset_refs(project_id, asset_id);

			-- Automations
			CREATE TABLE automation_rules (
				id         TEXT PRIMARY KEY,
				project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				name       TEXT NOT NULL,
				trigger    TEXT NOT NULL,
				conditions TEXT NOT NULL DEFAULT '[]',
				actions    TEXT NOT NULL DEFAULT '[]',
				enabled    BOOLEAN NOT NULL DEFAULT TRUE,
				builtin    BOOLEAN NOT NULL DEFAULT FALSE,
				created_at TEXT NOT NULL DEFAULT (datetime('now')),
				updated_at TEXT NOT NULL DEFAULT (datetime('now'))
			);
			CREATE INDEX idx_automation_rules_project ON automation_rules(project_id);

			CREATE TABLE automation_history (
				id         TEXT PRIMARY KEY,
				rule_id    TEXT NOT NULL,
				project_id TEXT NOT NULL,
				event_id   TEXT NOT NULL DEFAULT '',
				status     TEXT NOT NULL,
				error      TEXT NOT NULL DEFAULT '',
				started_at TEXT NOT NULL DEFAULT (datetime('now')),
				ended_at   TEXT NOT NULL DEFAULT (datetime('now'))
			);
			CREATE INDEX idx_automation_history_project ON automation_history(project_id);
			CREATE INDEX idx_automation_history_rule ON automation_history(rule_id);

			-- Automation runs (Bowrain AD-013)
			CREATE TABLE automation_runs (
				id           TEXT PRIMARY KEY,
				project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				trigger_type TEXT NOT NULL,
				trigger_id   TEXT NOT NULL DEFAULT '',
				trigger_data TEXT NOT NULL DEFAULT '{}',
				status       TEXT NOT NULL DEFAULT 'pending',
				step_count   INTEGER NOT NULL DEFAULT 0,
				done_count   INTEGER NOT NULL DEFAULT 0,
				error        TEXT NOT NULL DEFAULT '',
				started_at   TEXT NOT NULL DEFAULT (datetime('now')),
				ended_at     TEXT
			);
			CREATE INDEX idx_automation_runs_project ON automation_runs(project_id, started_at DESC);

			CREATE TABLE automation_steps (
				id          TEXT PRIMARY KEY,
				run_id      TEXT NOT NULL REFERENCES automation_runs(id) ON DELETE CASCADE,
				rule_name   TEXT NOT NULL DEFAULT '',
				action_type TEXT NOT NULL,
				status      TEXT NOT NULL DEFAULT 'pending',
				config      TEXT NOT NULL DEFAULT '{}',
				job_ids     TEXT NOT NULL DEFAULT '[]',
				task_ids    TEXT NOT NULL DEFAULT '[]',
				total_jobs  INTEGER NOT NULL DEFAULT 0,
				done_jobs   INTEGER NOT NULL DEFAULT 0,
				error       TEXT NOT NULL DEFAULT '',
				started_at  TEXT NOT NULL DEFAULT (datetime('now')),
				ended_at    TEXT
			);
			CREATE INDEX idx_automation_steps_run ON automation_steps(run_id);

			CREATE TABLE automation_logs (
				id        TEXT PRIMARY KEY,
				step_id   TEXT NOT NULL,
				run_id    TEXT NOT NULL,
				level     TEXT NOT NULL DEFAULT 'info',
				message   TEXT NOT NULL,
				data      TEXT NOT NULL DEFAULT '{}',
				timestamp TEXT NOT NULL DEFAULT (datetime('now'))
			);
			CREATE INDEX idx_automation_logs_step ON automation_logs(step_id, timestamp);
			CREATE INDEX idx_automation_logs_run ON automation_logs(run_id, timestamp);

			-- Convergence runs (strategy 2026-07-kapi-up doc 03): one
			-- goal-seeking reconciliation of a project toward its ship gates,
			-- driving the venue-neutral core/convergence.Loop server-side and
			-- persisting every emitted convergence.Event for SSE replay.
			CREATE TABLE convergence_runs (
				id             TEXT PRIMARY KEY,
				project_id     TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				trigger        TEXT NOT NULL DEFAULT 'manual',
				state          TEXT NOT NULL DEFAULT 'running',
				passes         INTEGER NOT NULL DEFAULT 0,
				standing       TEXT NOT NULL DEFAULT '{}',
				failing_checks INTEGER NOT NULL DEFAULT 0,
				error          TEXT NOT NULL DEFAULT '',
				created_at     TEXT NOT NULL DEFAULT (datetime('now')),
				finished_at    TEXT
			);
			CREATE INDEX idx_convergence_runs_project ON convergence_runs(project_id, created_at DESC);
			-- At most one running run per project: a DB-level guard so the
			-- one-run-per-project constraint is atomic (F8), not a racy SELECT.
			CREATE UNIQUE INDEX idx_convergence_runs_one_active ON convergence_runs(project_id) WHERE state = 'running';

			CREATE TABLE convergence_run_events (
				run_id  TEXT NOT NULL REFERENCES convergence_runs(id) ON DELETE CASCADE,
				seq     INTEGER NOT NULL,
				payload TEXT NOT NULL DEFAULT '{}',
				PRIMARY KEY (run_id, seq)
			);

			-- Review queue
			CREATE TABLE review_items (
				id          TEXT PRIMARY KEY,
				project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				type        TEXT NOT NULL,
				status      TEXT NOT NULL DEFAULT 'pending',
				push_id     TEXT NOT NULL DEFAULT '',
				data        TEXT NOT NULL,
				occurrences TEXT NOT NULL DEFAULT '[]',
				assigned_to TEXT NOT NULL DEFAULT '',
				decided_by  TEXT NOT NULL DEFAULT '',
				decided_at  TEXT,
				comment     TEXT NOT NULL DEFAULT '',
				edits       TEXT NOT NULL DEFAULT '{}',
				confidence  REAL NOT NULL DEFAULT 0,
				locale      TEXT NOT NULL DEFAULT '',
				created_at  TEXT NOT NULL DEFAULT (datetime('now'))
			);
			CREATE INDEX idx_review_items_project_status ON review_items(project_id, status);
			CREATE INDEX idx_review_items_project_type ON review_items(project_id, type);
			CREATE INDEX idx_review_items_assigned ON review_items(project_id, assigned_to);
			CREATE INDEX idx_review_items_confidence ON review_items(project_id, confidence);

			CREATE TABLE rejected_terms (
				project_id  TEXT NOT NULL,
				term_text   TEXT NOT NULL,
				locale      TEXT NOT NULL,
				rejected_at TEXT NOT NULL DEFAULT (datetime('now')),
				PRIMARY KEY (project_id, term_text, locale)
			);

			CREATE TABLE dnt_entries (
				project_id  TEXT NOT NULL,
				text        TEXT NOT NULL,
				entity_type TEXT NOT NULL DEFAULT '',
				locale      TEXT NOT NULL,
				source      TEXT NOT NULL DEFAULT '',
				created_at  TEXT NOT NULL DEFAULT (datetime('now')),
				PRIMARY KEY (project_id, text, locale)
			);

			-- Notifications
			CREATE TABLE notifications (
				id         TEXT PRIMARY KEY,
				user_id    TEXT NOT NULL,
				type       TEXT NOT NULL DEFAULT 'general',
				title      TEXT NOT NULL,
				body       TEXT NOT NULL DEFAULT '',
				project_id TEXT NOT NULL DEFAULT '',
				link_url   TEXT NOT NULL DEFAULT '',
				read       BOOLEAN NOT NULL DEFAULT FALSE,
				category   TEXT NOT NULL DEFAULT '',
				group_key  TEXT NOT NULL DEFAULT '',
				actor_id   TEXT NOT NULL DEFAULT '',
				actor_name TEXT NOT NULL DEFAULT '',
				task_id    TEXT NOT NULL DEFAULT '',
				priority   TEXT NOT NULL DEFAULT 'normal',
				created_at TEXT NOT NULL DEFAULT (datetime('now'))
			);
			CREATE INDEX idx_notifications_user ON notifications(user_id, read, created_at DESC);

			CREATE TABLE notification_preferences (
				user_id         TEXT NOT NULL,
				workspace_id    TEXT NOT NULL,
				category        TEXT NOT NULL,
				channel_web     BOOLEAN NOT NULL DEFAULT TRUE,
				channel_email   BOOLEAN NOT NULL DEFAULT FALSE,
				channel_push    BOOLEAN NOT NULL DEFAULT FALSE,
				channel_desktop BOOLEAN NOT NULL DEFAULT FALSE,
				updated_at      TEXT NOT NULL DEFAULT (datetime('now')),
				UNIQUE(user_id, workspace_id, category)
			);
			CREATE INDEX idx_notif_pref_user ON notification_preferences(user_id, workspace_id);

			-- Activities
			CREATE TABLE activities (
				id           TEXT PRIMARY KEY,
				workspace_id TEXT NOT NULL,
				project_id   TEXT NOT NULL DEFAULT '',
				stream       TEXT NOT NULL DEFAULT '',
				actor_id     TEXT NOT NULL,
				actor_name   TEXT NOT NULL DEFAULT '',
				type         TEXT NOT NULL,
				entity_type  TEXT NOT NULL DEFAULT '',
				entity_id    TEXT NOT NULL DEFAULT '',
				summary      TEXT NOT NULL DEFAULT '',
				data         TEXT NOT NULL DEFAULT '{}',
				created_at   TEXT NOT NULL DEFAULT (datetime('now'))
			);
			CREATE INDEX idx_activities_workspace ON activities(workspace_id, created_at DESC);
			CREATE INDEX idx_activities_project ON activities(workspace_id, project_id, created_at DESC);
			CREATE INDEX idx_activities_actor ON activities(workspace_id, actor_id, created_at DESC);

			-- Tasks
			CREATE TABLE tasks (
				id           TEXT PRIMARY KEY,
				workspace_id TEXT NOT NULL,
				project_id   TEXT NOT NULL,
				stream       TEXT NOT NULL DEFAULT '',
				type         TEXT NOT NULL DEFAULT 'custom',
				status       TEXT NOT NULL DEFAULT 'open',
				priority     TEXT NOT NULL DEFAULT 'normal',
				title        TEXT NOT NULL,
				description  TEXT NOT NULL DEFAULT '',
				assignee_id  TEXT NOT NULL DEFAULT '',
				created_by   TEXT NOT NULL,
				completed_by TEXT NOT NULL DEFAULT '',
				data         TEXT NOT NULL DEFAULT '{}',
				due_at       TEXT,
				created_at   TEXT NOT NULL DEFAULT (datetime('now')),
				updated_at   TEXT NOT NULL DEFAULT (datetime('now')),
				completed_at TEXT
			);
			CREATE INDEX idx_tasks_workspace ON tasks(workspace_id, status, created_at DESC);
			CREATE INDEX idx_tasks_project ON tasks(workspace_id, project_id, status);
			CREATE INDEX idx_tasks_assignee ON tasks(workspace_id, assignee_id, status);

			-- Digest
			CREATE TABLE digest_settings (
				user_id      TEXT NOT NULL,
				workspace_id TEXT NOT NULL,
				frequency    TEXT NOT NULL DEFAULT 'daily',
				quiet_start  TEXT NOT NULL DEFAULT '',
				quiet_end    TEXT NOT NULL DEFAULT '',
				timezone     TEXT NOT NULL DEFAULT 'UTC',
				PRIMARY KEY (user_id, workspace_id)
			);

			CREATE TABLE digest_state (
				user_id      TEXT NOT NULL,
				workspace_id TEXT NOT NULL,
				frequency    TEXT NOT NULL DEFAULT 'daily',
				last_sent_at TEXT NOT NULL,
				PRIMARY KEY (user_id, workspace_id, frequency)
			);

			-- Audit log
			CREATE TABLE audit_log (
				id         INTEGER PRIMARY KEY AUTOINCREMENT,
				project_id TEXT NOT NULL,
				event_type TEXT NOT NULL,
				actor      TEXT NOT NULL DEFAULT '',
				source     TEXT NOT NULL DEFAULT '',
				data       TEXT NOT NULL DEFAULT '{}',
				created_at TEXT NOT NULL DEFAULT (datetime('now'))
			);
			CREATE INDEX idx_audit_log_project ON audit_log(project_id, created_at DESC);
			CREATE INDEX idx_audit_log_type ON audit_log(project_id, event_type, created_at DESC);

			-- Leader leases (distributed coordination)
			CREATE TABLE leader_leases (
				name       TEXT PRIMARY KEY,
				holder_id  TEXT NOT NULL,
				expires_at TEXT NOT NULL
			);

			-- Pending pushes (push completion tracking)
			CREATE TABLE pending_pushes (
				push_id    TEXT PRIMARY KEY,
				project_id TEXT NOT NULL,
				items      TEXT NOT NULL DEFAULT '',
				ws_slug    TEXT NOT NULL DEFAULT '',
				actor      TEXT NOT NULL DEFAULT '',
				created_at TEXT NOT NULL DEFAULT (datetime('now'))
			);

			-- Activity read state (cross-device sync)
			CREATE TABLE activity_state (
				user_id      TEXT NOT NULL,
				workspace_id TEXT NOT NULL,
				last_seen_at TEXT NOT NULL DEFAULT (datetime('now')),
				PRIMARY KEY (user_id, workspace_id)
			);

-- Overlay storage, split by kind for access-pattern-specific
			-- indexes (#403). Mirrors the Postgres baseline with TEXT/JSON
			-- → TEXT dialect translation.
			CREATE TABLE translations (
				project_id    TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				stream        TEXT NOT NULL DEFAULT 'main',
				block_id      TEXT NOT NULL,
				locale        TEXT NOT NULL,
				text          TEXT NOT NULL DEFAULT '',
				target_json   TEXT NOT NULL DEFAULT '{}',
				provider      TEXT NOT NULL DEFAULT '',
				metadata      TEXT NOT NULL DEFAULT '{}',
				updated_at    TEXT NOT NULL DEFAULT (datetime('now')),
				PRIMARY KEY (project_id, stream, block_id, locale)
			);
			CREATE INDEX idx_translations_project_locale
				ON translations(project_id, stream, locale, updated_at DESC);
			CREATE INDEX idx_translations_project_block
				ON translations(project_id, stream, block_id);

			CREATE TABLE annotations (
				project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				stream     TEXT NOT NULL DEFAULT 'main',
				block_id   TEXT NOT NULL,
				kind       TEXT NOT NULL,
				payload    TEXT NOT NULL DEFAULT '{}',
				updated_at TEXT NOT NULL DEFAULT (datetime('now')),
				PRIMARY KEY (project_id, stream, block_id, kind)
			);
			CREATE INDEX idx_annotations_project_kind
				ON annotations(project_id, stream, kind, updated_at DESC);
			CREATE INDEX idx_annotations_project_block
				ON annotations(project_id, stream, block_id);

			CREATE TABLE overlays_ext (
				project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				stream     TEXT NOT NULL DEFAULT 'main',
				block_id   TEXT NOT NULL,
				kind       TEXT NOT NULL,
				payload    TEXT NOT NULL DEFAULT '{}',
				updated_at TEXT NOT NULL DEFAULT (datetime('now')),
				PRIMARY KEY (project_id, stream, block_id, kind)
			);
			CREATE INDEX idx_overlays_ext_project_kind
				ON overlays_ext(project_id, stream, kind);
			CREATE INDEX idx_overlays_ext_project_block
				ON overlays_ext(project_id, stream, block_id);
		`,
	},
	{
		Version:     4,
		Description: "workspace-scoped connector configs (durable connectors)",
		SQL: `
			-- Mirrors connector_configs in bowrain/store/migrations.go (Version 5).
			-- config is a JSON map (TEXT) whose secret values are sealed with
			-- crypto.Cipher; timestamps are TEXT RFC3339 so a single
			-- ConnectorConfigStore(*sql.DB) scans identically on both drivers.
			CREATE TABLE IF NOT EXISTS connector_configs (
				id           TEXT NOT NULL,
				workspace_id TEXT NOT NULL,
				type         TEXT NOT NULL,
				name         TEXT NOT NULL DEFAULT '',
				config       TEXT NOT NULL DEFAULT '{}',
				created_at   TEXT NOT NULL DEFAULT '',
				updated_at   TEXT NOT NULL DEFAULT '',
				PRIMARY KEY (workspace_id, id)
			);
			CREATE INDEX IF NOT EXISTS idx_connector_configs_ws ON connector_configs(workspace_id);
		`,
	},
	{
		Version:     5,
		Description: "stream properties (extensible metadata, incl. voice binding)",
		SQL: `
			-- Mirrors streams.properties in bowrain/store/migrations.go (Version 6):
			-- a JSON TEXT map carrying the stream-level voice binding
			-- (voice_profile_id) and other extensible metadata.
			ALTER TABLE streams ADD COLUMN properties TEXT NOT NULL DEFAULT '{}';
		`,
	},
	{
		Version:     6,
		Description: "convergence run loop-observability columns (stall_reason, stage, activity)",
		SQL: `
			-- Mirrors convergence_runs observability columns in
			-- bowrain/store/migrations.go (Version 7): labeled stalls +
			-- loop-position context (strategy 2026-07-dogfood doc 06, themes C/D).
			-- SQLite ALTER TABLE ADD COLUMN adds one column per statement.
			ALTER TABLE convergence_runs ADD COLUMN stall_reason   TEXT NOT NULL DEFAULT '';
			ALTER TABLE convergence_runs ADD COLUMN current_stage  TEXT NOT NULL DEFAULT '';
			ALTER TABLE convergence_runs ADD COLUMN current_locale TEXT NOT NULL DEFAULT '';
			ALTER TABLE convergence_runs ADD COLUMN last_activity  TEXT NOT NULL DEFAULT '';
		`,
	},
	{
		Version:     7,
		Description: "source-first convergence: blocked-on-source count on a run",
		SQL: `
			-- Mirrors bowrain/store/migrations.go (Version 8): the source-first
			-- settle phase holds source blocks below the gate; the run row carries
			-- how many, so the UI can render "N segments need source review"
			-- (strategy 2026-07-dogfood doc 07 / roadmap epic 019).
			ALTER TABLE convergence_runs ADD COLUMN blocked_on_source INTEGER NOT NULL DEFAULT 0;
		`,
	},
	{
		Version:     8,
		Description: "connector last-sync timestamp (real status, not fabricated now)",
		SQL: `
			-- Mirrors connector_configs.last_sync_at in
			-- bowrain/store/migrations.go (Version 9): the last successful
			-- fetch/publish time per connector, so status reports a real value.
			ALTER TABLE connector_configs ADD COLUMN last_sync_at TEXT NOT NULL DEFAULT '';
		`,
	},
	{
		Version:     9,
		Description: "connector last-error (observable background ingest failures)",
		SQL: `
			-- Mirrors connector_configs.last_error in
			-- bowrain/store/migrations.go (Version 10): the most recent sync
			-- failure per connector, so a failed background ingest surfaces on
			-- status instead of leaving a silently empty project. Cleared on the
			-- next successful sync.
			ALTER TABLE connector_configs ADD COLUMN last_error TEXT NOT NULL DEFAULT '';
		`,
	},
	{
		Version:     10,
		Description: "proposed source changes (back-to-source review, RV-F)",
		SQL: `
			-- Mirrors proposed_source_changes in bowrain/store/migrations.go
			-- (Version 11): a reviewer's proposed source-text fix, approved by a
			-- source owner (which re-drafts every locale) or rejected.
			CREATE TABLE IF NOT EXISTS proposed_source_changes (
				id              TEXT PRIMARY KEY,
				workspace_id    TEXT NOT NULL,
				project_id      TEXT NOT NULL,
				stream          TEXT NOT NULL DEFAULT 'main',
				item_name       TEXT NOT NULL DEFAULT '',
				block_id        TEXT NOT NULL,
				kind            TEXT NOT NULL DEFAULT 'text-fix',
				original_source TEXT NOT NULL DEFAULT '',
				proposed_source TEXT NOT NULL DEFAULT '',
				rationale       TEXT NOT NULL DEFAULT '',
				found_in_locale TEXT NOT NULL DEFAULT '',
				finder_user     TEXT NOT NULL DEFAULT '',
				status          TEXT NOT NULL DEFAULT 'open',
				decided_by      TEXT NOT NULL DEFAULT '',
				decision_reason TEXT NOT NULL DEFAULT '',
				created_at      TEXT NOT NULL DEFAULT '',
				updated_at      TEXT NOT NULL DEFAULT '',
				decided_at      TEXT
			);
			CREATE INDEX IF NOT EXISTS idx_source_proposals_project_status
				ON proposed_source_changes(project_id, status);
		`,
	},
	{
		Version:     11,
		Description: "block stand-off overlays column (persist term/entity/segmentation/qa overlays across store round-trip)",
		SQL: `
			-- Mirrors blocks.overlays in bowrain/store/migrations.go (Version 12):
			-- the positional, run-anchored stand-off layers (segmentation, term,
			-- entity, term-candidate, qa, alignment) persist alongside source_json
			-- so they survive a store round-trip instead of being dropped on the
			-- next GetBlocks (which broke the entity→concept promote path). Stored
			-- as a JSON array (TEXT); empty overlays default to '[]', like
			-- source_json.
			ALTER TABLE blocks ADD COLUMN overlays TEXT NOT NULL DEFAULT '[]';
		`,
	},
	{
		Version:     12,
		Description: "GitHub App installation ownership (an installation belongs to one workspace)",
		SQL: `
			-- Mirrors forge_installations in bowrain/store/migrations.go
			-- (Version 13): which workspace a GitHub App installation belongs to,
			-- the sole authority the post-install setup endpoints consult before
			-- acting on an installation id. workspace_id is empty until the signed
			-- setup state claims the row; the app-level webhook only records and
			-- removes. INTEGER PRIMARY KEY holds the same 64-bit id as the
			-- Postgres BIGINT.
			CREATE TABLE IF NOT EXISTS forge_installations (
				installation_id INTEGER PRIMARY KEY,
				workspace_id    TEXT NOT NULL DEFAULT '',
				account         TEXT NOT NULL DEFAULT '',
				created_at      TEXT NOT NULL DEFAULT '',
				updated_at      TEXT NOT NULL DEFAULT ''
			);
			CREATE INDEX IF NOT EXISTS idx_forge_installations_ws
				ON forge_installations(workspace_id);
		`,
	},
	{
		Version:     13,
		Description: "collection context: coordinates, ownership, and the entry hash the context content type reconciles on",
		SQL: `
			-- Mirrors the collections columns in bowrain/store/migrations.go
			-- (Version 14): the point the collection sits at in the project's
			-- context space, which side owns the row ('recipe' or 'workspace',
			-- backfilled to 'workspace' for everything created before the context
			-- content type existed), and the hash of the context entry it was
			-- reconciled from: the value that makes a re-push idempotent.
			-- SQLite ALTER TABLE ADD COLUMN adds one column per statement.
			ALTER TABLE collections ADD COLUMN context      TEXT NOT NULL DEFAULT '{}';
			ALTER TABLE collections ADD COLUMN owner        TEXT NOT NULL DEFAULT 'workspace';
			ALTER TABLE collections ADD COLUMN context_hash TEXT NOT NULL DEFAULT '';
		`,
	},
	{
		Version:     14,
		Description: "unit decisions ledger (decisions travel the sync protocol)",
		SQL: `
			-- Mirrors unit_decisions in bowrain/store/migrations.go (Version
			-- 16): the latest workflow decision per (item, unit, variant), the
			-- server-side fold of core/state. One contract, two backends.
			CREATE TABLE IF NOT EXISTS unit_decisions (
				project_id   TEXT NOT NULL,
				stream       TEXT NOT NULL DEFAULT 'main',
				item_name    TEXT NOT NULL DEFAULT '',
				unit         TEXT NOT NULL,
				variant      TEXT NOT NULL,
				status       TEXT NOT NULL DEFAULT '',
				target_hash  TEXT NOT NULL DEFAULT '',
				review_state TEXT NOT NULL DEFAULT '',
				decided_by   TEXT NOT NULL DEFAULT '',
				decided_at   TEXT NOT NULL DEFAULT '',
				note         TEXT NOT NULL DEFAULT '',
				parked       INTEGER NOT NULL DEFAULT 0,
				assignee     TEXT NOT NULL DEFAULT '',
				updated      TEXT NOT NULL DEFAULT '',
				updated_at   TEXT NOT NULL DEFAULT (datetime('now')),
				PRIMARY KEY (project_id, stream, item_name, unit, variant)
			);
			CREATE INDEX IF NOT EXISTS idx_unit_decisions_project ON unit_decisions(project_id, stream);
		`,
	},
	{
		Version:     15,
		Description: "stream scope for convergence runs",
		SQL: `
			-- Mirrors migration 17 in bowrain/store/migrations.go: a run's
			-- derivation and produce are scoped to one stream; '' means "main".
			ALTER TABLE convergence_runs ADD COLUMN stream TEXT NOT NULL DEFAULT '';
		`,
	},
	{
		Version:     16,
		Description: "source word count computed at write, not derived per read",
		SQL: `
			-- Mirrors migration 18 in bowrain/store/migrations.go: NULL marks a
			-- row that predates the column; readers decode its source_json once
			-- and every rewrite fills the column.
			ALTER TABLE blocks ADD COLUMN word_count INTEGER;
		`,
	},
	{
		Version:     17,
		Description: "a redelivered event files one line and tells one person once",
		SQL: `
			-- Mirrors the Postgres baseline: the event a row came from is what
			-- makes a second delivery of it a no-op rather than a duplicate.
			ALTER TABLE notifications ADD COLUMN source_event_id TEXT NOT NULL DEFAULT '';
			CREATE UNIQUE INDEX notifications_user_source_event
				ON notifications(user_id, source_event_id) WHERE source_event_id <> '';
			ALTER TABLE activities ADD COLUMN event_id TEXT NOT NULL DEFAULT '';
			CREATE UNIQUE INDEX activities_event_id
				ON activities(event_id) WHERE event_id <> '';
		`,
	},
	{
		Version:     18,
		Description: "a decision records the source it blessed, not only the translation",
		SQL: `
			-- Mirrors the Postgres baseline's unit_decisions.content_hash: the
			-- BASIS a decision blessed: the hash of the SOURCE it approved a
			-- translation FOR. Empty means the record predates the column, which
			-- a reader must not read as a source that has moved.
			ALTER TABLE unit_decisions ADD COLUMN content_hash TEXT NOT NULL DEFAULT '';
		`,
	},
	{
		Version:     19,
		Description: "where a collection's strings can be read in place",
		SQL: `
			-- Mirrors the collections columns in bowrain/store/migrations.go
			-- (Version 25): the preview host a collection declares, and the
			-- kind that says how a view is found within it. One contract, two
			-- backends. SQLite adds one column per statement.
			ALTER TABLE collections ADD COLUMN preview_kind TEXT NOT NULL DEFAULT '';
			ALTER TABLE collections ADD COLUMN preview_url  TEXT NOT NULL DEFAULT '';
		`,
	},
	{
		Version:     20,
		Description: "a stream owns its content, and a file is identified by what it is",
		SQL: `
			-- Mirrors bowrain/store/migrations.go version 27. One contract, two
			-- backends, and unlike the server's, this database belongs to
			-- whoever installed the desktop app, so it is migrated rather than
			-- reset.
			--
			-- SQLite cannot alter a primary key, so the three tables whose key
			-- moves are rebuilt beside themselves and renamed into place. The
			-- columns are spelled out because a rebuild is only as complete as
			-- its column list.

			-- ---- items: the id becomes the key, the path becomes an address ----
			UPDATE items SET id = lower(hex(randomblob(6))) WHERE id = '';

			CREATE TABLE items_rekeyed (
				project_id    TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				name          TEXT NOT NULL,
				id            TEXT NOT NULL,
				stream        TEXT NOT NULL DEFAULT 'main',
				format        TEXT NOT NULL DEFAULT '',
				item_type     TEXT NOT NULL DEFAULT 'file',
				block_index   TEXT NOT NULL DEFAULT '{}',
				preview_html  TEXT NOT NULL DEFAULT '',
				properties    TEXT NOT NULL DEFAULT '{}',
				collection_id TEXT NOT NULL DEFAULT '',
				created_at    TEXT NOT NULL DEFAULT (datetime('now')),
				updated_at    TEXT NOT NULL DEFAULT (datetime('now')),
				PRIMARY KEY (project_id, stream, id)
			);
			INSERT INTO items_rekeyed
				(project_id, name, id, stream, format, item_type, block_index, preview_html, properties, collection_id, created_at, updated_at)
				SELECT project_id, name, id, stream, format, item_type, block_index, preview_html, properties, collection_id, created_at, updated_at
				FROM items;
			DROP TABLE items;
			ALTER TABLE items_rekeyed RENAME TO items;
			CREATE INDEX idx_items_project ON items(project_id);
			CREATE INDEX idx_items_project_stream ON items(project_id, stream);
			CREATE INDEX idx_items_collection ON items(project_id, collection_id);
			-- A path still addresses at most one item within a stream. It is a
			-- constraint on the address, no longer the identity.
			CREATE UNIQUE INDEX idx_items_stream_name ON items(project_id, stream, name);

			-- ---- blocks: per stream, and pointing at the item's id ----
			CREATE TABLE blocks_rekeyed (
				id           TEXT NOT NULL,
				project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				stream       TEXT NOT NULL DEFAULT 'main',
				item_name    TEXT NOT NULL DEFAULT '',
				item_id      TEXT NOT NULL DEFAULT '',
				source_id    TEXT NOT NULL DEFAULT '',
				name         TEXT NOT NULL DEFAULT '',
				type         TEXT NOT NULL DEFAULT '',
				mime_type    TEXT NOT NULL DEFAULT '',
				translatable BOOLEAN NOT NULL DEFAULT TRUE,
				content_hash TEXT NOT NULL DEFAULT '',
				context_hash TEXT NOT NULL DEFAULT '',
				source_json  TEXT NOT NULL DEFAULT '[]',
				overlays     TEXT NOT NULL DEFAULT '[]',
				word_count   INTEGER,
				properties   TEXT NOT NULL DEFAULT '{}',
				stored_at    TEXT NOT NULL DEFAULT (datetime('now')),
				updated_at   TEXT NOT NULL DEFAULT (datetime('now')),
				PRIMARY KEY (project_id, stream, id)
			);
			-- Every block written before this migration belongs to main: the
			-- stream column did not exist, and neither did a way for a branch to
			-- hold content of its own.
			INSERT INTO blocks_rekeyed
				(id, project_id, stream, item_name, item_id, source_id, name, type, mime_type, translatable,
				 content_hash, context_hash, source_json, overlays, word_count, properties, stored_at, updated_at)
				SELECT b.id, b.project_id, 'main', b.item_name,
				       COALESCE((SELECT i.id FROM items i
				                 WHERE i.project_id = b.project_id AND i.stream = 'main' AND i.name = b.item_name), ''),
				       b.source_id, b.name, b.type, b.mime_type, b.translatable,
				       b.content_hash, b.context_hash, b.source_json, b.overlays, b.word_count, b.properties,
				       b.stored_at, b.updated_at
				FROM blocks b;
			DROP TABLE blocks;
			ALTER TABLE blocks_rekeyed RENAME TO blocks;
			CREATE INDEX idx_blocks_content_hash ON blocks(content_hash);
			CREATE INDEX idx_blocks_project ON blocks(project_id);
			CREATE INDEX idx_blocks_item ON blocks(project_id, stream, item_name);
			CREATE INDEX idx_blocks_item_id ON blocks(project_id, stream, item_id);
			CREATE UNIQUE INDEX idx_blocks_source_id ON blocks(project_id, stream, item_name, source_id)
				WHERE source_id != '';

			-- ---- governance follows the file, not its address ----
			CREATE TABLE unit_decisions_rekeyed (
				project_id   TEXT NOT NULL,
				stream       TEXT NOT NULL DEFAULT 'main',
				item_id      TEXT NOT NULL DEFAULT '',
				item_name    TEXT NOT NULL DEFAULT '',
				unit         TEXT NOT NULL,
				variant      TEXT NOT NULL,
				status       TEXT NOT NULL DEFAULT '',
				target_hash  TEXT NOT NULL DEFAULT '',
				content_hash TEXT NOT NULL DEFAULT '',
				review_state TEXT NOT NULL DEFAULT '',
				decided_by   TEXT NOT NULL DEFAULT '',
				decided_at   TEXT NOT NULL DEFAULT '',
				note         TEXT NOT NULL DEFAULT '',
				parked       INTEGER NOT NULL DEFAULT 0,
				assignee     TEXT NOT NULL DEFAULT '',
				updated      TEXT NOT NULL DEFAULT '',
				updated_at   TEXT NOT NULL DEFAULT (datetime('now')),
				PRIMARY KEY (project_id, stream, item_id, unit, variant)
			);
			INSERT OR IGNORE INTO unit_decisions_rekeyed
				(project_id, stream, item_id, item_name, unit, variant, status, target_hash, content_hash,
				 review_state, decided_by, decided_at, note, parked, assignee, updated, updated_at)
				SELECT d.project_id, d.stream,
				       COALESCE((SELECT i.id FROM items i
				                 WHERE i.project_id = d.project_id AND i.stream = d.stream AND i.name = d.item_name), ''),
				       d.item_name, d.unit, d.variant, d.status, d.target_hash, d.content_hash,
				       d.review_state, d.decided_by, d.decided_at, d.note, d.parked, d.assignee, d.updated, d.updated_at
				FROM unit_decisions d;
			DROP TABLE unit_decisions;
			ALTER TABLE unit_decisions_rekeyed RENAME TO unit_decisions;
			CREATE INDEX idx_unit_decisions_project ON unit_decisions(project_id, stream);
			CREATE INDEX idx_unit_decisions_item ON unit_decisions(project_id, stream, item_id);

			ALTER TABLE assets                  ADD COLUMN item_id TEXT NOT NULL DEFAULT '';
			ALTER TABLE proposed_source_changes ADD COLUMN item_id TEXT NOT NULL DEFAULT '';
			UPDATE assets SET item_id = COALESCE((SELECT i.id FROM items i
				WHERE i.project_id = assets.project_id AND i.stream = assets.stream AND i.name = assets.item_name), '');
			UPDATE proposed_source_changes SET item_id = COALESCE((SELECT i.id FROM items i
				WHERE i.project_id = proposed_source_changes.project_id
				  AND i.stream = proposed_source_changes.stream
				  AND i.name = proposed_source_changes.item_name), '');
		`,
	},
	{
		Version:     21,
		Description: "the ledger records the source the platform last drafted a unit against",
		SQL: `
			-- Mirrors bowrain/store/migrations.go version 30: the source hash
			-- the platform's latest draft of the unit was made against, kept
			-- beside the decision on the same row and never written over it.
			-- Empty means the platform has recorded no draft for the unit.
			ALTER TABLE unit_decisions ADD COLUMN draft_basis TEXT NOT NULL DEFAULT '';
		`,
	},
}
