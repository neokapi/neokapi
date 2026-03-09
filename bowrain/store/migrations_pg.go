package store

import "github.com/gokapi/gokapi/bowrain/storage"

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
			ALTER TABLE blocks ADD COLUMN source_id TEXT NOT NULL DEFAULT '';

			-- Copy existing id to source_id for backward compatibility.
			UPDATE blocks SET source_id = id;

			-- Drop old PK and recreate with (project_id, id).
			-- Postgres cannot ALTER PK, so recreate the table.
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
			INSERT INTO blocks_new SELECT id, project_id, item_name, source_id, name, type, mime_type,
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
		Version:     3,
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
}
