package store

import "github.com/neokapi/neokapi/bowrain/storage"

// storeMigrationsPg is a single clean PostgreSQL schema that represents the
// final state of all 8 incremental SQLite migrations consolidated into one.
var storeMigrationsPg = []storage.Migration{
	{
		Version:     1,
		Description: "create content store schema",
		SQL: `
			CREATE TABLE projects (
				id             TEXT PRIMARY KEY,
				name           TEXT NOT NULL,
				source_locale  TEXT NOT NULL,
				target_locales TEXT NOT NULL DEFAULT '',
				properties     TEXT NOT NULL DEFAULT '{}',
				workspace_id   TEXT NOT NULL DEFAULT '',
				created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX idx_projects_workspace ON projects(workspace_id);

			CREATE TABLE items (
				project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				name         TEXT NOT NULL,
				format       TEXT NOT NULL DEFAULT '',
				item_type    TEXT NOT NULL DEFAULT 'file',
				source_bytes BYTEA,
				block_index  TEXT NOT NULL DEFAULT '{}',
				properties   TEXT NOT NULL DEFAULT '{}',
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (project_id, name)
			);
			CREATE INDEX idx_items_project ON items(project_id);

			CREATE TABLE blocks (
				id           TEXT NOT NULL,
				project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				item_name    TEXT NOT NULL DEFAULT '',
				name         TEXT NOT NULL DEFAULT '',
				type         TEXT NOT NULL DEFAULT '',
				mime_type    TEXT NOT NULL DEFAULT '',
				translatable BOOLEAN NOT NULL DEFAULT TRUE,
				content_hash TEXT NOT NULL DEFAULT '',
				context_hash TEXT NOT NULL DEFAULT '',
				source_json  TEXT NOT NULL DEFAULT '[]',
				targets_json TEXT NOT NULL DEFAULT '{}',
				properties   TEXT NOT NULL DEFAULT '{}',
				annotations  TEXT NOT NULL DEFAULT '{}',
				stored_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (project_id, item_name, id)
			);
			CREATE INDEX idx_blocks_content_hash ON blocks(content_hash);
			CREATE INDEX idx_blocks_project ON blocks(project_id);
			CREATE INDEX idx_blocks_item ON blocks(project_id, item_name);

			CREATE TABLE versions (
				id          TEXT PRIMARY KEY,
				project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				label       TEXT NOT NULL,
				description TEXT NOT NULL DEFAULT '',
				block_count INTEGER NOT NULL DEFAULT 0,
				created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX idx_versions_project ON versions(project_id);

			CREATE TABLE version_blocks (
				version_id   TEXT NOT NULL REFERENCES versions(id) ON DELETE CASCADE,
				block_id     TEXT NOT NULL,
				content_hash TEXT NOT NULL,
				PRIMARY KEY (version_id, block_id)
			);

			CREATE TABLE change_log (
				seq          BIGSERIAL PRIMARY KEY,
				project_id   TEXT NOT NULL,
				block_id     TEXT NOT NULL,
				change_type  TEXT NOT NULL,
				locale       TEXT,
				content_hash TEXT,
				logged_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX idx_changelog_project_seq ON change_log(project_id, seq);
			CREATE INDEX idx_changelog_project_locale ON change_log(project_id, locale, seq);

			CREATE TABLE block_history (
				id          BIGSERIAL PRIMARY KEY,
				project_id  TEXT NOT NULL,
				block_id    TEXT NOT NULL,
				locale      TEXT NOT NULL,
				change_type TEXT NOT NULL,
				text        TEXT NOT NULL DEFAULT '',
				coded_text  TEXT NOT NULL DEFAULT '',
				origin      TEXT NOT NULL DEFAULT '',
				author      TEXT NOT NULL DEFAULT '',
				created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX idx_block_history_lookup ON block_history(project_id, block_id, locale);

			CREATE TABLE block_notes (
				id         TEXT PRIMARY KEY,
				project_id TEXT NOT NULL,
				block_id   TEXT NOT NULL,
				author     TEXT NOT NULL DEFAULT '',
				text       TEXT NOT NULL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX idx_block_notes_lookup ON block_notes(project_id, block_id);
		`,
	},
	{
		Version:     2,
		Description: "add source_id to blocks and change PK to (project_id, id)",
		SQL: `
			ALTER TABLE blocks ADD COLUMN IF NOT EXISTS source_id TEXT NOT NULL DEFAULT '';

			-- Copy existing id to source_id for backward compatibility.
			UPDATE blocks SET source_id = id WHERE source_id = '';

			-- Drop old PK and recreate with (project_id, id).
			-- Postgres cannot ALTER PK, so recreate the table.
			DROP TABLE IF EXISTS blocks_new;
			CREATE TABLE blocks_new (
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
				targets_json TEXT NOT NULL DEFAULT '{}',
				properties   TEXT NOT NULL DEFAULT '{}',
				annotations  TEXT NOT NULL DEFAULT '{}',
				stored_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (project_id, id)
			);
			INSERT INTO blocks_new
				SELECT DISTINCT ON (project_id, id)
					id, project_id, item_name, source_id, name, type, mime_type,
					translatable, content_hash, context_hash, source_json, targets_json,
					properties, annotations, stored_at, updated_at
				FROM blocks
				ORDER BY project_id, id, updated_at DESC;
			DROP TABLE blocks;
			ALTER TABLE blocks_new RENAME TO blocks;
			CREATE INDEX idx_blocks_content_hash ON blocks(content_hash);
			CREATE INDEX idx_blocks_project ON blocks(project_id);
			CREATE INDEX idx_blocks_item ON blocks(project_id, item_name);
			CREATE UNIQUE INDEX idx_blocks_source_id ON blocks(project_id, item_name, source_id)
				WHERE source_id != '';
		`,
	},
	{
		Version:     3,
		Description: "add streams support",
		SQL: `
			CREATE TABLE streams (
				project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				name        TEXT NOT NULL,
				parent      TEXT NOT NULL DEFAULT '',
				base_cursor BIGINT NOT NULL DEFAULT 0,
				archived    BOOLEAN NOT NULL DEFAULT FALSE,
				visibility  TEXT NOT NULL DEFAULT 'public',
				description TEXT NOT NULL DEFAULT '',
				created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				created_by  TEXT NOT NULL DEFAULT '',
				PRIMARY KEY (project_id, name)
			);

			CREATE TABLE stream_members (
				project_id  TEXT NOT NULL,
				stream      TEXT NOT NULL,
				user_id     TEXT NOT NULL,
				added_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (project_id, stream, user_id),
				FOREIGN KEY (project_id, stream) REFERENCES streams(project_id, name) ON DELETE CASCADE
			);

			ALTER TABLE items ADD COLUMN stream TEXT NOT NULL DEFAULT 'main';
			ALTER TABLE change_log ADD COLUMN stream TEXT NOT NULL DEFAULT 'main';
			ALTER TABLE block_history ADD COLUMN stream TEXT NOT NULL DEFAULT 'main';
			ALTER TABLE block_notes ADD COLUMN stream TEXT NOT NULL DEFAULT 'main';

			-- Update items primary key to include stream.
			DROP TABLE IF EXISTS items_new;
			CREATE TABLE items_new (
				project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				name         TEXT NOT NULL,
				stream       TEXT NOT NULL DEFAULT 'main',
				format       TEXT NOT NULL DEFAULT '',
				item_type    TEXT NOT NULL DEFAULT 'file',
				source_bytes BYTEA,
				block_index  TEXT NOT NULL DEFAULT '{}',
				properties   TEXT NOT NULL DEFAULT '{}',
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (project_id, stream, name)
			);
			INSERT INTO items_new
				SELECT project_id, name, stream, format, item_type, source_bytes, block_index, properties, created_at, updated_at
				FROM items;
			DROP TABLE items;
			ALTER TABLE items_new RENAME TO items;
			CREATE INDEX idx_items_project ON items(project_id);
			CREATE INDEX idx_items_project_stream ON items(project_id, stream);

			CREATE INDEX idx_changelog_stream ON change_log(project_id, stream, seq);
			CREATE INDEX idx_block_history_stream ON block_history(project_id, stream, block_id, locale);
			CREATE INDEX idx_block_notes_stream ON block_notes(project_id, stream, block_id);
		`,
	},
	{
		Version:     4,
		Description: "automation, review queue, and notification tables",
		SQL: `
			CREATE TABLE automation_rules (
				id         TEXT PRIMARY KEY,
				project_id TEXT NOT NULL,
				name       TEXT NOT NULL,
				trigger    TEXT NOT NULL,
				conditions TEXT NOT NULL DEFAULT '[]',
				actions    TEXT NOT NULL DEFAULT '[]',
				enabled    BOOLEAN NOT NULL DEFAULT TRUE,
				builtin    BOOLEAN NOT NULL DEFAULT FALSE,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
			);
			CREATE INDEX idx_automation_rules_project ON automation_rules(project_id);

			CREATE TABLE automation_history (
				id         TEXT PRIMARY KEY,
				rule_id    TEXT NOT NULL,
				project_id TEXT NOT NULL,
				event_id   TEXT NOT NULL DEFAULT '',
				status     TEXT NOT NULL,
				error      TEXT NOT NULL DEFAULT '',
				started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				ended_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX idx_automation_history_project ON automation_history(project_id);
			CREATE INDEX idx_automation_history_rule ON automation_history(rule_id);

			CREATE TABLE review_items (
				id            TEXT PRIMARY KEY,
				project_id    TEXT NOT NULL,
				type          TEXT NOT NULL,
				status        TEXT NOT NULL DEFAULT 'pending',
				push_id       TEXT NOT NULL DEFAULT '',
				data          TEXT NOT NULL,
				occurrences   TEXT NOT NULL DEFAULT '[]',
				assigned_to   TEXT NOT NULL DEFAULT '',
				decided_by    TEXT NOT NULL DEFAULT '',
				decided_at    TIMESTAMPTZ,
				comment       TEXT NOT NULL DEFAULT '',
				edits         TEXT NOT NULL DEFAULT '{}',
				confidence    DOUBLE PRECISION NOT NULL DEFAULT 0,
				locale        TEXT NOT NULL DEFAULT '',
				created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
			);
			CREATE INDEX idx_review_items_project_status ON review_items(project_id, status);
			CREATE INDEX idx_review_items_project_type ON review_items(project_id, type);
			CREATE INDEX idx_review_items_assigned ON review_items(project_id, assigned_to);
			CREATE INDEX idx_review_items_confidence ON review_items(project_id, confidence);

			CREATE TABLE rejected_terms (
				project_id  TEXT NOT NULL,
				term_text   TEXT NOT NULL,
				locale      TEXT NOT NULL,
				rejected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (project_id, term_text, locale)
			);

			CREATE TABLE dnt_entries (
				project_id  TEXT NOT NULL,
				text        TEXT NOT NULL,
				entity_type TEXT NOT NULL DEFAULT '',
				locale      TEXT NOT NULL,
				source      TEXT NOT NULL DEFAULT '',
				created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (project_id, text, locale)
			);

			CREATE TABLE notifications (
				id          TEXT PRIMARY KEY,
				user_id     TEXT NOT NULL,
				type        TEXT NOT NULL DEFAULT 'general',
				title       TEXT NOT NULL,
				body        TEXT NOT NULL DEFAULT '',
				project_id  TEXT NOT NULL DEFAULT '',
				link_url    TEXT NOT NULL DEFAULT '',
				read        BOOLEAN NOT NULL DEFAULT FALSE,
				created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX idx_notifications_user ON notifications(user_id, read, created_at DESC);
		`,
	},
	{
		Version:     5,
		Description: "add id column to items",
		SQL: `
			ALTER TABLE items ADD COLUMN id TEXT NOT NULL DEFAULT '';
			UPDATE items SET id = substr(md5(random()::text), 1, 8) WHERE id = '';
			CREATE UNIQUE INDEX idx_items_id ON items(project_id, stream, id);
		`,
	},
	{
		Version:     6,
		Description: "add collections table and collection_id to items",
		SQL: `
			CREATE TABLE collections (
				id               TEXT NOT NULL,
				project_id       TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				name             TEXT NOT NULL,
				kind             TEXT NOT NULL DEFAULT 'uploaded',
				item_label       TEXT NOT NULL DEFAULT 'item',
				is_default       BOOLEAN NOT NULL DEFAULT FALSE,
				stream           TEXT NOT NULL DEFAULT '',
				connector_config TEXT NOT NULL DEFAULT '{}',
				created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (project_id, id)
			);
			CREATE INDEX idx_collections_project ON collections(project_id);
			CREATE UNIQUE INDEX idx_collections_default ON collections(project_id)
				WHERE is_default = TRUE;

			ALTER TABLE items ADD COLUMN collection_id TEXT NOT NULL DEFAULT '';
			CREATE INDEX idx_items_collection ON items(project_id, collection_id);

			-- Backfill default collections for existing projects.
			INSERT INTO collections (id, project_id, name, kind, item_label, is_default, created_at, updated_at)
				SELECT substr(md5(random()::text), 1, 8), id, 'default', 'uploaded', 'item', TRUE,
					NOW(), NOW()
				FROM projects;

			-- Assign existing items to their project's default collection.
			UPDATE items SET collection_id = (
				SELECT c.id FROM collections c WHERE c.project_id = items.project_id AND c.is_default = TRUE
			) WHERE collection_id = '';

			-- Backfill: create a "main" stream for every existing project that doesn't have one.
			INSERT INTO streams (project_id, name, parent, base_cursor, archived, visibility, description, created_at, created_by)
				SELECT id, 'main', '', 0, FALSE, 'public', '', NOW(), ''
				FROM projects
				WHERE id NOT IN (SELECT project_id FROM streams WHERE name = 'main')
				ON CONFLICT DO NOTHING;
		`,
	},
	{
		Version:     7,
		Description: "add archived columns to projects",
		SQL: `
			ALTER TABLE projects ADD COLUMN archived BOOLEAN NOT NULL DEFAULT FALSE;
			ALTER TABLE projects ADD COLUMN archived_at TIMESTAMPTZ;
		`,
	},
	{
		Version:     8,
		Description: "create audit_log table",
		SQL: `
			CREATE TABLE audit_log (
				id         BIGSERIAL PRIMARY KEY,
				project_id TEXT NOT NULL,
				event_type TEXT NOT NULL,
				actor      TEXT NOT NULL DEFAULT '',
				source     TEXT NOT NULL DEFAULT '',
				data       JSONB NOT NULL DEFAULT '{}',
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX idx_audit_log_project ON audit_log(project_id, created_at DESC);
			CREATE INDEX idx_audit_log_type ON audit_log(project_id, event_type, created_at DESC);
		`,
	},
	{
		Version:     9,
		Description: "rename locale columns to language columns",
		SQL: `
			ALTER TABLE projects RENAME COLUMN source_locale TO default_source_language;
			ALTER TABLE projects RENAME COLUMN target_locales TO target_languages;
			ALTER TABLE projects ADD COLUMN target_language_mode TEXT NOT NULL DEFAULT 'defined';
		`,
	},
	{
		Version:     10,
		Description: "create activities table",
		SQL: `
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
				data         JSONB NOT NULL DEFAULT '{}',
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX idx_activities_workspace ON activities(workspace_id, created_at DESC);
			CREATE INDEX idx_activities_project ON activities(workspace_id, project_id, created_at DESC);
			CREATE INDEX idx_activities_actor ON activities(workspace_id, actor_id, created_at DESC);
		`,
	},
	{
		Version:     11,
		Description: "create tasks table",
		SQL: `
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
				data         JSONB NOT NULL DEFAULT '{}',
				due_at       TIMESTAMPTZ,
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				completed_at TIMESTAMPTZ
			);
			CREATE INDEX idx_tasks_workspace ON tasks(workspace_id, status, created_at DESC);
			CREATE INDEX idx_tasks_project ON tasks(workspace_id, project_id, status);
			CREATE INDEX idx_tasks_assignee ON tasks(workspace_id, assignee_id, status);
		`,
	},
	{
		Version:     12,
		Description: "create notification_preferences table and extend notifications",
		SQL: `
			CREATE TABLE notification_preferences (
				user_id      TEXT NOT NULL,
				workspace_id TEXT NOT NULL,
				category     TEXT NOT NULL,
				channel_web     BOOLEAN NOT NULL DEFAULT TRUE,
				channel_email   BOOLEAN NOT NULL DEFAULT FALSE,
				channel_push    BOOLEAN NOT NULL DEFAULT FALSE,
				channel_desktop BOOLEAN NOT NULL DEFAULT FALSE,
				updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				UNIQUE(user_id, workspace_id, category)
			);
			CREATE INDEX idx_notif_pref_user ON notification_preferences(user_id, workspace_id);

			ALTER TABLE notifications ADD COLUMN category   TEXT NOT NULL DEFAULT '';
			ALTER TABLE notifications ADD COLUMN group_key  TEXT NOT NULL DEFAULT '';
			ALTER TABLE notifications ADD COLUMN actor_id   TEXT NOT NULL DEFAULT '';
			ALTER TABLE notifications ADD COLUMN actor_name TEXT NOT NULL DEFAULT '';
			ALTER TABLE notifications ADD COLUMN task_id    TEXT NOT NULL DEFAULT '';
			ALTER TABLE notifications ADD COLUMN priority   TEXT NOT NULL DEFAULT 'normal';
		`,
	},
	{
		Version:     13,
		Description: "create asset tables (AD-029)",
		SQL: `
			CREATE TABLE assets (
				id                TEXT PRIMARY KEY,
				project_id        TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				item_name         TEXT NOT NULL DEFAULT '',
				source_id         TEXT NOT NULL DEFAULT '',
				blob_key          TEXT NOT NULL,
				mime_type         TEXT NOT NULL,
				filename          TEXT NOT NULL DEFAULT '',
				size_bytes        BIGINT NOT NULL DEFAULT 0,
				alt_text          TEXT NOT NULL DEFAULT '',
				properties        TEXT NOT NULL DEFAULT '{}',
				processing_status TEXT NOT NULL DEFAULT 'none',
				processing_hint   TEXT NOT NULL DEFAULT '',
				stream            TEXT NOT NULL DEFAULT 'main',
				created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX idx_assets_project_item ON assets(project_id, item_name);
			CREATE UNIQUE INDEX idx_assets_blob ON assets(project_id, blob_key)
				WHERE stream = 'main';

			CREATE TABLE asset_variants (
				asset_id    TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
				locale      TEXT NOT NULL,
				blob_key    TEXT NOT NULL,
				status      TEXT NOT NULL DEFAULT 'pending',
				mime_type   TEXT NOT NULL DEFAULT '',
				size_bytes  BIGINT NOT NULL DEFAULT 0,
				properties  TEXT NOT NULL DEFAULT '{}',
				created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
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
		`,
	},
	{
		Version:     14,
		Description: "replace source_bytes with preview_html in items",
		SQL: `
			ALTER TABLE items DROP COLUMN IF EXISTS source_bytes;
			ALTER TABLE items ADD COLUMN IF NOT EXISTS preview_html TEXT NOT NULL DEFAULT '';
		`,
	},
	{
		Version:     15,
		Description: "add default_stream column to projects",
		SQL: `
			ALTER TABLE projects ADD COLUMN default_stream TEXT NOT NULL DEFAULT '';

			-- Backfill: set default_stream for projects that have items on 'main'.
			UPDATE projects SET default_stream = 'main'
			WHERE id IN (SELECT DISTINCT project_id FROM items WHERE stream = 'main');

			-- For projects with no 'main' items but items on exactly one stream,
			-- set that stream as default.
			UPDATE projects SET default_stream = (
				SELECT stream FROM items WHERE items.project_id = projects.id
				GROUP BY stream
				ORDER BY COUNT(*) DESC LIMIT 1
			)
			WHERE default_stream = ''
			  AND id IN (SELECT DISTINCT project_id FROM items);
		`,
	},
	{
		Version:     16,
		Description: "add dashboard_visibility to projects",
		SQL: `
			ALTER TABLE projects ADD COLUMN IF NOT EXISTS dashboard_visibility TEXT NOT NULL DEFAULT 'private';
		`,
	},
	{
		Version:     17,
		Description: "create digest settings and state tables",
		SQL: `
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
				last_sent_at TIMESTAMPTZ NOT NULL,
				PRIMARY KEY (user_id, workspace_id, frequency)
			);
		`,
	},
	{
		Version:     18,
		Description: "add stream tags and stream lock support",
		SQL: `
			CREATE TABLE stream_tags (
				id         TEXT PRIMARY KEY,
				project_id TEXT NOT NULL,
				stream     TEXT NOT NULL,
				name       TEXT NOT NULL,
				kind       TEXT NOT NULL DEFAULT 'custom',
				cursor     BIGINT NOT NULL DEFAULT 0,
				metadata   TEXT NOT NULL DEFAULT '{}',
				created_by TEXT NOT NULL DEFAULT '',
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				FOREIGN KEY (project_id, stream) REFERENCES streams(project_id, name) ON DELETE CASCADE
			);
			CREATE UNIQUE INDEX idx_stream_tags_unique ON stream_tags(project_id, stream, name);
			CREATE INDEX idx_stream_tags_stream ON stream_tags(project_id, stream);
			CREATE INDEX idx_stream_tags_project_kind ON stream_tags(project_id, kind);

			ALTER TABLE streams ADD COLUMN locked BOOLEAN NOT NULL DEFAULT FALSE;
			ALTER TABLE streams ADD COLUMN locked_by TEXT NOT NULL DEFAULT '';
			ALTER TABLE streams ADD COLUMN locked_at TIMESTAMPTZ;
		`,
	},
	{
		Version:     19,
		Description: "create automation_runs, automation_steps, automation_logs, leader_leases tables (AD-035, #169)",
		SQL: `
			CREATE TABLE automation_runs (
				id           TEXT PRIMARY KEY,
				project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				trigger_type TEXT NOT NULL,
				trigger_id   TEXT NOT NULL DEFAULT '',
				trigger_data JSONB NOT NULL DEFAULT '{}',
				status       TEXT NOT NULL DEFAULT 'pending',
				step_count   INT NOT NULL DEFAULT 0,
				done_count   INT NOT NULL DEFAULT 0,
				error        TEXT NOT NULL DEFAULT '',
				started_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				ended_at     TIMESTAMPTZ
			);
			CREATE INDEX idx_automation_runs_project ON automation_runs(project_id, started_at DESC);

			CREATE TABLE automation_steps (
				id          TEXT PRIMARY KEY,
				run_id      TEXT NOT NULL REFERENCES automation_runs(id) ON DELETE CASCADE,
				rule_name   TEXT NOT NULL DEFAULT '',
				action_type TEXT NOT NULL,
				status      TEXT NOT NULL DEFAULT 'pending',
				config      JSONB NOT NULL DEFAULT '{}',
				job_ids     JSONB NOT NULL DEFAULT '[]',
				task_ids    JSONB NOT NULL DEFAULT '[]',
				total_jobs  INT NOT NULL DEFAULT 0,
				done_jobs   INT NOT NULL DEFAULT 0,
				error       TEXT NOT NULL DEFAULT '',
				started_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				ended_at    TIMESTAMPTZ
			);
			CREATE INDEX idx_automation_steps_run ON automation_steps(run_id);

			CREATE TABLE automation_logs (
				id        TEXT PRIMARY KEY,
				step_id   TEXT NOT NULL,
				run_id    TEXT NOT NULL,
				level     TEXT NOT NULL DEFAULT 'info',
				message   TEXT NOT NULL,
				data      JSONB NOT NULL DEFAULT '{}',
				timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX idx_automation_logs_step ON automation_logs(step_id, timestamp);
			CREATE INDEX idx_automation_logs_run ON automation_logs(run_id, timestamp);

			CREATE TABLE leader_leases (
				name       TEXT PRIMARY KEY,
				holder_id  TEXT NOT NULL,
				expires_at TIMESTAMPTZ NOT NULL
			);

			CREATE TABLE pending_pushes (
				push_id    TEXT PRIMARY KEY,
				project_id TEXT NOT NULL,
				items      TEXT NOT NULL DEFAULT '',
				ws_slug    TEXT NOT NULL DEFAULT '',
				actor      TEXT NOT NULL DEFAULT '',
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);

			DO $$ BEGIN
				IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'translation_jobs') THEN
					ALTER TABLE translation_jobs ADD COLUMN IF NOT EXISTS step_id TEXT NOT NULL DEFAULT '';
				END IF;
				IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'extraction_jobs') THEN
					ALTER TABLE extraction_jobs ADD COLUMN IF NOT EXISTS step_id TEXT NOT NULL DEFAULT '';
				END IF;
			END $$;
		`,
	},
}
