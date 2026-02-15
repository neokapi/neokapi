package store

import "github.com/gokapi/gokapi/internal/storage"

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
}
