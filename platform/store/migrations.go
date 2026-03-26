package store

import "github.com/neokapi/neokapi/bowrain/storage"

var storeMigrations = []storage.Migration{
	{
		Version:     1,
		Description: "create projects table",
		SQL: `
			CREATE TABLE projects (
				id             TEXT PRIMARY KEY,
				name           TEXT NOT NULL,
				source_locale  TEXT NOT NULL,
				target_locales TEXT NOT NULL DEFAULT '',
				properties     TEXT NOT NULL DEFAULT '{}',
				created_at     TEXT NOT NULL DEFAULT (datetime('now')),
				updated_at     TEXT NOT NULL DEFAULT (datetime('now'))
			);
		`,
	},
	{
		Version:     2,
		Description: "create blocks table",
		SQL: `
			CREATE TABLE blocks (
				id           TEXT NOT NULL,
				project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				name         TEXT NOT NULL DEFAULT '',
				type         TEXT NOT NULL DEFAULT '',
				mime_type    TEXT NOT NULL DEFAULT '',
				translatable INTEGER NOT NULL DEFAULT 1,
				content_hash TEXT NOT NULL DEFAULT '',
				context_hash TEXT NOT NULL DEFAULT '',
				source_json  TEXT NOT NULL DEFAULT '[]',
				targets_json TEXT NOT NULL DEFAULT '{}',
				properties   TEXT NOT NULL DEFAULT '{}',
				annotations  TEXT NOT NULL DEFAULT '{}',
				stored_at    TEXT NOT NULL DEFAULT (datetime('now')),
				updated_at   TEXT NOT NULL DEFAULT (datetime('now')),
				PRIMARY KEY (project_id, id)
			);
			CREATE INDEX idx_blocks_content_hash ON blocks(content_hash);
			CREATE INDEX idx_blocks_project ON blocks(project_id);
		`,
	},
	{
		Version:     3,
		Description: "create versions table",
		SQL: `
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
		`,
	},
	{
		Version:     4,
		Description: "add workspace_id to projects",
		SQL: `
			ALTER TABLE projects ADD COLUMN workspace_id TEXT NOT NULL DEFAULT '';
			CREATE INDEX idx_projects_workspace ON projects(workspace_id);
		`,
	},
	{
		Version:     5,
		Description: "create change_log table for incremental sync",
		SQL: `
			CREATE TABLE change_log (
				seq          INTEGER PRIMARY KEY AUTOINCREMENT,
				project_id   TEXT NOT NULL,
				block_id     TEXT NOT NULL,
				change_type  TEXT NOT NULL,
				locale       TEXT,
				content_hash TEXT,
				logged_at    TEXT NOT NULL
			);
			CREATE INDEX idx_changelog_project_seq ON change_log(project_id, seq);
			CREATE INDEX idx_changelog_project_locale ON change_log(project_id, locale, seq);
		`,
	},
	{
		Version:     6,
		Description: "create items table",
		SQL: `
			CREATE TABLE items (
				project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				name         TEXT NOT NULL,
				format       TEXT NOT NULL DEFAULT '',
				item_type    TEXT NOT NULL DEFAULT 'file',
				source_bytes BLOB,
				block_index  TEXT NOT NULL DEFAULT '{}',
				properties   TEXT NOT NULL DEFAULT '{}',
				created_at   TEXT NOT NULL DEFAULT (datetime('now')),
				updated_at   TEXT NOT NULL DEFAULT (datetime('now')),
				PRIMARY KEY (project_id, name)
			);
			CREATE INDEX idx_items_project ON items(project_id);
		`,
	},
	{
		Version:     7,
		Description: "add item_name column to blocks",
		SQL: `
			ALTER TABLE blocks ADD COLUMN item_name TEXT NOT NULL DEFAULT '';
			CREATE INDEX idx_blocks_item ON blocks(project_id, item_name);
		`,
	},
	{
		Version:     8,
		Description: "change blocks primary key to include item_name",
		SQL: `
			-- Recreate blocks table with (project_id, item_name, id) primary key.
			-- This allows different files to have blocks with the same ID.
			CREATE TABLE blocks_new (
				id           TEXT NOT NULL,
				project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				item_name    TEXT NOT NULL DEFAULT '',
				name         TEXT NOT NULL DEFAULT '',
				type         TEXT NOT NULL DEFAULT '',
				mime_type    TEXT NOT NULL DEFAULT '',
				translatable INTEGER NOT NULL DEFAULT 1,
				content_hash TEXT NOT NULL DEFAULT '',
				context_hash TEXT NOT NULL DEFAULT '',
				source_json  TEXT NOT NULL DEFAULT '[]',
				targets_json TEXT NOT NULL DEFAULT '{}',
				properties   TEXT NOT NULL DEFAULT '{}',
				annotations  TEXT NOT NULL DEFAULT '{}',
				stored_at    TEXT NOT NULL DEFAULT (datetime('now')),
				updated_at   TEXT NOT NULL DEFAULT (datetime('now')),
				PRIMARY KEY (project_id, item_name, id)
			);
			INSERT INTO blocks_new SELECT id, project_id, item_name, name, type, mime_type,
				translatable, content_hash, context_hash, source_json, targets_json,
				properties, annotations, stored_at, updated_at FROM blocks;
			DROP TABLE blocks;
			ALTER TABLE blocks_new RENAME TO blocks;
			CREATE INDEX idx_blocks_content_hash ON blocks(content_hash);
			CREATE INDEX idx_blocks_project ON blocks(project_id);
			CREATE INDEX idx_blocks_item ON blocks(project_id, item_name);
		`,
	},
	{
		Version:     9,
		Description: "create block_history table",
		SQL: `
			CREATE TABLE IF NOT EXISTS block_history (
				id          INTEGER PRIMARY KEY AUTOINCREMENT,
				project_id  TEXT NOT NULL,
				block_id    TEXT NOT NULL,
				locale      TEXT NOT NULL,
				change_type TEXT NOT NULL,
				text        TEXT NOT NULL DEFAULT '',
				coded_text  TEXT NOT NULL DEFAULT '',
				origin      TEXT NOT NULL DEFAULT '',
				author      TEXT NOT NULL DEFAULT '',
				created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
			);
			CREATE INDEX IF NOT EXISTS idx_block_history_lookup ON block_history(project_id, block_id, locale);
		`,
	},
	{
		Version:     10,
		Description: "create block_notes table",
		SQL: `
			CREATE TABLE IF NOT EXISTS block_notes (
				id         TEXT PRIMARY KEY,
				project_id TEXT NOT NULL,
				block_id   TEXT NOT NULL,
				author     TEXT NOT NULL DEFAULT '',
				text       TEXT NOT NULL,
				created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
			);
			CREATE INDEX IF NOT EXISTS idx_block_notes_lookup ON block_notes(project_id, block_id);
		`,
	},
	{
		Version:     11,
		Description: "create automation_rules table",
		SQL: `
			CREATE TABLE automation_rules (
				id         TEXT PRIMARY KEY,
				project_id TEXT NOT NULL,
				name       TEXT NOT NULL,
				trigger    TEXT NOT NULL,
				conditions TEXT NOT NULL DEFAULT '[]',
				actions    TEXT NOT NULL DEFAULT '[]',
				enabled    INTEGER NOT NULL DEFAULT 1,
				builtin    INTEGER NOT NULL DEFAULT 0,
				created_at TEXT NOT NULL DEFAULT (datetime('now')),
				updated_at TEXT NOT NULL DEFAULT (datetime('now')),
				FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
			);
			CREATE INDEX idx_automation_rules_project ON automation_rules(project_id);
		`,
	},
	{
		Version:     12,
		Description: "create automation_history table",
		SQL: `
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
		`,
	},
	{
		Version:     13,
		Description: "create review queue tables (AD-022)",
		SQL: `
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
				decided_at    TEXT NOT NULL DEFAULT '',
				comment       TEXT NOT NULL DEFAULT '',
				edits         TEXT NOT NULL DEFAULT '{}',
				confidence    REAL NOT NULL DEFAULT 0,
				locale        TEXT NOT NULL DEFAULT '',
				created_at    TEXT NOT NULL DEFAULT (datetime('now')),
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
		`,
	},
	{
		Version:     14,
		Description: "create notifications table",
		SQL: `
			CREATE TABLE notifications (
				id          TEXT PRIMARY KEY,
				user_id     TEXT NOT NULL,
				type        TEXT NOT NULL DEFAULT 'general',
				title       TEXT NOT NULL,
				body        TEXT NOT NULL DEFAULT '',
				project_id  TEXT NOT NULL DEFAULT '',
				link_url    TEXT NOT NULL DEFAULT '',
				read        INTEGER NOT NULL DEFAULT 0,
				created_at  TEXT NOT NULL DEFAULT (datetime('now'))
			);
			CREATE INDEX idx_notifications_user ON notifications(user_id, read, created_at DESC);
		`,
	},
	{
		Version:     15,
		Description: "add source_id to blocks and change PK to (project_id, id)",
		SQL: `
			CREATE TABLE blocks_new (
				id           TEXT NOT NULL,
				project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				item_name    TEXT NOT NULL DEFAULT '',
				source_id    TEXT NOT NULL DEFAULT '',
				name         TEXT NOT NULL DEFAULT '',
				type         TEXT NOT NULL DEFAULT '',
				mime_type    TEXT NOT NULL DEFAULT '',
				translatable INTEGER NOT NULL DEFAULT 1,
				content_hash TEXT NOT NULL DEFAULT '',
				context_hash TEXT NOT NULL DEFAULT '',
				source_json  TEXT NOT NULL DEFAULT '[]',
				targets_json TEXT NOT NULL DEFAULT '{}',
				properties   TEXT NOT NULL DEFAULT '{}',
				annotations  TEXT NOT NULL DEFAULT '{}',
				stored_at    TEXT NOT NULL DEFAULT (datetime('now')),
				updated_at   TEXT NOT NULL DEFAULT (datetime('now')),
				PRIMARY KEY (project_id, id)
			);
			INSERT INTO blocks_new (id, project_id, item_name, source_id, name, type, mime_type,
				translatable, content_hash, context_hash, source_json, targets_json,
				properties, annotations, stored_at, updated_at)
			SELECT id, project_id, item_name, id, name, type, mime_type,
				translatable, content_hash, context_hash, source_json, targets_json,
				properties, annotations, stored_at, updated_at FROM blocks;
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
		Version:     16,
		Description: "add streams support",
		SQL: `
			-- Streams table
			CREATE TABLE streams (
				project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				name        TEXT NOT NULL,
				parent      TEXT NOT NULL DEFAULT '',
				base_cursor INTEGER NOT NULL DEFAULT 0,
				archived    INTEGER NOT NULL DEFAULT 0,
				visibility  TEXT NOT NULL DEFAULT 'public',
				description TEXT NOT NULL DEFAULT '',
				created_at  TEXT NOT NULL DEFAULT (datetime('now')),
				created_by  TEXT NOT NULL DEFAULT '',
				PRIMARY KEY (project_id, name)
			);

			-- Stream members table (for shared visibility)
			CREATE TABLE stream_members (
				project_id TEXT NOT NULL,
				stream     TEXT NOT NULL,
				user_id    TEXT NOT NULL,
				added_at   TEXT NOT NULL DEFAULT (datetime('now')),
				PRIMARY KEY (project_id, stream, user_id),
				FOREIGN KEY (project_id, stream) REFERENCES streams(project_id, name) ON DELETE CASCADE
			);

			-- Add stream column to items table (recreate with stream in PK)
			CREATE TABLE items_new (
				project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				stream       TEXT NOT NULL DEFAULT 'main',
				name         TEXT NOT NULL,
				format       TEXT NOT NULL DEFAULT '',
				item_type    TEXT NOT NULL DEFAULT 'file',
				source_bytes BLOB,
				block_index  TEXT NOT NULL DEFAULT '{}',
				properties   TEXT NOT NULL DEFAULT '{}',
				created_at   TEXT NOT NULL DEFAULT (datetime('now')),
				updated_at   TEXT NOT NULL DEFAULT (datetime('now')),
				PRIMARY KEY (project_id, stream, name)
			);
			INSERT INTO items_new (project_id, stream, name, format, item_type, source_bytes, block_index, properties, created_at, updated_at)
				SELECT project_id, 'main', name, format, item_type, source_bytes, block_index, properties, created_at, updated_at FROM items;
			DROP TABLE items;
			ALTER TABLE items_new RENAME TO items;
			CREATE INDEX idx_items_project ON items(project_id);
			CREATE INDEX idx_items_project_stream ON items(project_id, stream);

			-- Add stream column to change_log table
			ALTER TABLE change_log ADD COLUMN stream TEXT NOT NULL DEFAULT 'main';
			DROP INDEX IF EXISTS idx_changelog_project_seq;
			DROP INDEX IF EXISTS idx_changelog_project_locale;
			CREATE INDEX idx_changelog_project_stream_seq ON change_log(project_id, stream, seq);
			CREATE INDEX idx_changelog_project_stream_locale ON change_log(project_id, stream, locale, seq);

			-- Add stream column to block_history table
			ALTER TABLE block_history ADD COLUMN stream TEXT NOT NULL DEFAULT 'main';
			DROP INDEX IF EXISTS idx_block_history_lookup;
			CREATE INDEX idx_block_history_lookup ON block_history(project_id, stream, block_id, locale);

			-- Add stream column to block_notes table
			ALTER TABLE block_notes ADD COLUMN stream TEXT NOT NULL DEFAULT 'main';
			DROP INDEX IF EXISTS idx_block_notes_lookup;
			CREATE INDEX idx_block_notes_lookup ON block_notes(project_id, stream, block_id);
		`,
	},
	{
		Version:     17,
		Description: "add id column to items",
		SQL: `
			ALTER TABLE items ADD COLUMN id TEXT NOT NULL DEFAULT '';
			UPDATE items SET id = lower(hex(randomblob(4))) WHERE id = '';
			CREATE UNIQUE INDEX idx_items_id ON items(project_id, stream, id);
		`,
	},
	{
		Version:     18,
		Description: "add collections table and collection_id to items",
		SQL: `
			CREATE TABLE collections (
				id               TEXT NOT NULL,
				project_id       TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				name             TEXT NOT NULL,
				kind             TEXT NOT NULL DEFAULT 'uploaded',
				item_label       TEXT NOT NULL DEFAULT 'item',
				is_default       INTEGER NOT NULL DEFAULT 0,
				stream           TEXT NOT NULL DEFAULT '',
				connector_config TEXT NOT NULL DEFAULT '{}',
				created_at       TEXT NOT NULL DEFAULT (datetime('now')),
				updated_at       TEXT NOT NULL DEFAULT (datetime('now')),
				PRIMARY KEY (project_id, id)
			);
			CREATE INDEX idx_collections_project ON collections(project_id);
			CREATE UNIQUE INDEX idx_collections_default ON collections(project_id)
				WHERE is_default = 1;

			-- Add collection_id to items.
			ALTER TABLE items ADD COLUMN collection_id TEXT NOT NULL DEFAULT '';
			CREATE INDEX idx_items_collection ON items(project_id, collection_id);

			-- Backfill: create a default collection for every existing project.
			INSERT INTO collections (id, project_id, name, kind, item_label, is_default, created_at, updated_at)
				SELECT lower(hex(randomblob(4))), id, 'default', 'uploaded', 'item', 1,
					datetime('now'), datetime('now')
				FROM projects;

			-- Assign existing items to their project's default collection.
			UPDATE items SET collection_id = (
				SELECT c.id FROM collections c WHERE c.project_id = items.project_id AND c.is_default = 1
			) WHERE collection_id = '';

			-- Backfill: create a "main" stream for every existing project that doesn't have one.
			INSERT OR IGNORE INTO streams (project_id, name, parent, base_cursor, archived, visibility, description, created_at, created_by)
				SELECT id, 'main', '', 0, 0, 'public', '', datetime('now'), ''
				FROM projects
				WHERE id NOT IN (SELECT project_id FROM streams WHERE name = 'main');
		`,
	},
	{
		Version:     19,
		Description: "add archived columns to projects",
		SQL: `
			ALTER TABLE projects ADD COLUMN archived INTEGER NOT NULL DEFAULT 0;
			ALTER TABLE projects ADD COLUMN archived_at TEXT;
		`,
	},
	{
		Version:     20,
		Description: "create audit_log table",
		SQL: `
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
		`,
	},
	{
		Version:     21,
		Description: "rename locale columns to language columns",
		SQL: `
			ALTER TABLE projects RENAME COLUMN source_locale TO default_source_language;
			ALTER TABLE projects RENAME COLUMN target_locales TO target_languages;
			ALTER TABLE projects ADD COLUMN target_language_mode TEXT NOT NULL DEFAULT 'defined';
		`,
	},
	{
		Version:     22,
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
				data         TEXT NOT NULL DEFAULT '{}',
				created_at   TEXT NOT NULL DEFAULT (datetime('now'))
			);
			CREATE INDEX idx_activities_workspace ON activities(workspace_id, created_at DESC);
			CREATE INDEX idx_activities_project ON activities(workspace_id, project_id, created_at DESC);
			CREATE INDEX idx_activities_actor ON activities(workspace_id, actor_id, created_at DESC);
		`,
	},
	{
		Version:     23,
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
				data         TEXT NOT NULL DEFAULT '{}',
				due_at       TEXT,
				created_at   TEXT NOT NULL DEFAULT (datetime('now')),
				updated_at   TEXT NOT NULL DEFAULT (datetime('now')),
				completed_at TEXT
			);
			CREATE INDEX idx_tasks_workspace ON tasks(workspace_id, status, created_at DESC);
			CREATE INDEX idx_tasks_project ON tasks(workspace_id, project_id, status);
			CREATE INDEX idx_tasks_assignee ON tasks(workspace_id, assignee_id, status);
		`,
	},
	{
		Version:     24,
		Description: "create notification_preferences table and extend notifications",
		SQL: `
			CREATE TABLE notification_preferences (
				user_id      TEXT NOT NULL,
				workspace_id TEXT NOT NULL,
				category     TEXT NOT NULL,
				channel_web     INTEGER NOT NULL DEFAULT 1,
				channel_email   INTEGER NOT NULL DEFAULT 0,
				channel_push    INTEGER NOT NULL DEFAULT 0,
				channel_desktop INTEGER NOT NULL DEFAULT 0,
				updated_at   TEXT NOT NULL DEFAULT (datetime('now')),
				UNIQUE(user_id, workspace_id, category)
			);
			CREATE INDEX idx_notif_pref_user ON notification_preferences(user_id, workspace_id);

			ALTER TABLE notifications ADD COLUMN category  TEXT NOT NULL DEFAULT '';
			ALTER TABLE notifications ADD COLUMN group_key TEXT NOT NULL DEFAULT '';
			ALTER TABLE notifications ADD COLUMN actor_id  TEXT NOT NULL DEFAULT '';
			ALTER TABLE notifications ADD COLUMN actor_name TEXT NOT NULL DEFAULT '';
			ALTER TABLE notifications ADD COLUMN task_id   TEXT NOT NULL DEFAULT '';
			ALTER TABLE notifications ADD COLUMN priority  TEXT NOT NULL DEFAULT 'normal';
		`,
	},
	{
		Version:     25,
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
				asset_id    TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
				locale      TEXT NOT NULL,
				blob_key    TEXT NOT NULL,
				status      TEXT NOT NULL DEFAULT 'pending',
				mime_type   TEXT NOT NULL DEFAULT '',
				size_bytes  INTEGER NOT NULL DEFAULT 0,
				properties  TEXT NOT NULL DEFAULT '{}',
				created_at  TEXT NOT NULL DEFAULT (datetime('now')),
				updated_at  TEXT NOT NULL DEFAULT (datetime('now')),
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
		Version:     26,
		Description: "replace source_bytes with preview_html in items",
		SQL: `
			CREATE TABLE items_new (
				id           TEXT NOT NULL DEFAULT '',
				project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				stream       TEXT NOT NULL DEFAULT 'main',
				name         TEXT NOT NULL,
				format       TEXT NOT NULL DEFAULT '',
				item_type    TEXT NOT NULL DEFAULT 'file',
				block_index  TEXT NOT NULL DEFAULT '{}',
				preview_html TEXT NOT NULL DEFAULT '',
				properties   TEXT NOT NULL DEFAULT '{}',
				collection_id TEXT NOT NULL DEFAULT '',
				created_at   TEXT NOT NULL DEFAULT (datetime('now')),
				updated_at   TEXT NOT NULL DEFAULT (datetime('now')),
				PRIMARY KEY (project_id, stream, name)
			);
			INSERT INTO items_new (id, project_id, stream, name, format, item_type, block_index, properties, collection_id, created_at, updated_at)
				SELECT id, project_id, stream, name, format, item_type, block_index, properties, collection_id, created_at, updated_at FROM items;
			DROP TABLE items;
			ALTER TABLE items_new RENAME TO items;
			CREATE INDEX idx_items_project ON items(project_id);
			CREATE INDEX idx_items_project_stream ON items(project_id, stream);
			CREATE UNIQUE INDEX idx_items_id ON items(project_id, stream, id);
			CREATE INDEX idx_items_collection ON items(project_id, collection_id);
		`,
	},
	{
		Version:     27,
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
		Version:     28,
		Description: "add dashboard_visibility to projects",
		SQL: `
			ALTER TABLE projects ADD COLUMN dashboard_visibility TEXT NOT NULL DEFAULT 'private';
		`,
	},
	{
		Version:     29,
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
				last_sent_at TEXT NOT NULL,
				PRIMARY KEY (user_id, workspace_id, frequency)
			);
		`,
	},
	{
		Version:     30,
		Description: "add stream tags and stream lock support",
		SQL: `
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

			ALTER TABLE streams ADD COLUMN locked INTEGER NOT NULL DEFAULT 0;
			ALTER TABLE streams ADD COLUMN locked_by TEXT NOT NULL DEFAULT '';
			ALTER TABLE streams ADD COLUMN locked_at TEXT;
		`,
	},
}
