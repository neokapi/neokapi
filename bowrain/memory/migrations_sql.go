package memory

// The SQL of each shipped content memory migration, frozen as it shipped. A
// shipped migration means the same statements for every database that runs
// it, so these do not follow the schema descriptors (memory/schema) or
// storage.RevisionColumn: a test holds each to its renderer, and a change to
// either is a new numbered migration.
const (
	memoryPostgresBaseline = `		CREATE TABLE IF NOT EXISTS tm_entries (
			workspace_id  TEXT NOT NULL,
			id            TEXT NOT NULL,
			project_id    TEXT NOT NULL DEFAULT '',
			stream        TEXT NOT NULL DEFAULT '',
			hint_src_lang TEXT NOT NULL DEFAULT '',
			properties    JSONB NOT NULL DEFAULT '{}'::jsonb,
			note          TEXT NOT NULL DEFAULT '',
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (workspace_id, id)
		);
		CREATE INDEX IF NOT EXISTS idx_tm_ws_project ON tm_entries(workspace_id, project_id);
		CREATE INDEX IF NOT EXISTS idx_tm_ws_updated ON tm_entries(workspace_id, updated_at DESC);
		CREATE INDEX IF NOT EXISTS idx_tm_ws_stream  ON tm_entries(workspace_id, stream);

		CREATE TABLE IF NOT EXISTS tm_variants (
			workspace_id TEXT NOT NULL,
			entry_id     TEXT NOT NULL,
			locale       TEXT NOT NULL,
			coded        TEXT NOT NULL,
			plain        TEXT NOT NULL,
			struct_key   TEXT NOT NULL,
			general_key  TEXT NOT NULL,
			PRIMARY KEY (workspace_id, entry_id, locale),
			FOREIGN KEY (workspace_id, entry_id) REFERENCES tm_entries(workspace_id, id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_tm_var_ws_locale   ON tm_variants(workspace_id, locale);
		CREATE INDEX IF NOT EXISTS idx_tm_var_plain_loc   ON tm_variants(workspace_id, plain, locale);
		CREATE INDEX IF NOT EXISTS idx_tm_var_struct_loc  ON tm_variants(workspace_id, struct_key, locale);
		CREATE INDEX IF NOT EXISTS idx_tm_var_general_loc ON tm_variants(workspace_id, general_key, locale);

		CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;
		CREATE INDEX IF NOT EXISTS idx_tm_var_trgm_plain   ON tm_variants USING gin (plain gin_trgm_ops);
		CREATE INDEX IF NOT EXISTS idx_tm_var_trgm_struct  ON tm_variants USING gin (struct_key gin_trgm_ops);
		CREATE INDEX IF NOT EXISTS idx_tm_var_trgm_general ON tm_variants USING gin (general_key gin_trgm_ops);

		ALTER TABLE tm_variants ADD COLUMN IF NOT EXISTS search_tsv tsvector
			GENERATED ALWAYS AS (to_tsvector('simple', plain)) STORED;
		CREATE INDEX IF NOT EXISTS idx_tm_var_search_tsv ON tm_variants USING gin (search_tsv);

		CREATE TABLE IF NOT EXISTS tm_entry_entities (
			workspace_id   TEXT NOT NULL,
			entry_id       TEXT NOT NULL,
			placeholder_id TEXT NOT NULL,
			entity_type    TEXT NOT NULL,
			concept_id     TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (workspace_id, entry_id, placeholder_id),
			FOREIGN KEY (workspace_id, entry_id) REFERENCES tm_entries(workspace_id, id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_tm_entities_type    ON tm_entry_entities(workspace_id, entity_type);
		CREATE INDEX IF NOT EXISTS idx_tm_entities_concept ON tm_entry_entities(workspace_id, concept_id);

		CREATE TABLE IF NOT EXISTS tm_entry_entity_values (
			workspace_id   TEXT NOT NULL,
			entry_id       TEXT NOT NULL,
			placeholder_id TEXT NOT NULL,
			locale         TEXT NOT NULL,
			text_value     TEXT NOT NULL DEFAULT '',
			start_pos      INTEGER NOT NULL DEFAULT 0,
			end_pos        INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (workspace_id, entry_id, placeholder_id, locale),
			FOREIGN KEY (workspace_id, entry_id, placeholder_id) REFERENCES tm_entry_entities(workspace_id, entry_id, placeholder_id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_tm_entity_values_text ON tm_entry_entity_values(workspace_id, text_value, locale);

		CREATE TABLE IF NOT EXISTS tm_import_sessions (
			workspace_id      TEXT NOT NULL,
			id                TEXT NOT NULL,
			file_key          TEXT NOT NULL,
			file_hash         TEXT NOT NULL DEFAULT '',
			file_size_bytes   BIGINT NOT NULL DEFAULT 0,
			imported_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			imported_by       TEXT NOT NULL DEFAULT '',
			tool_name         TEXT NOT NULL DEFAULT '',
			tool_version      TEXT NOT NULL DEFAULT '',
			seg_type          TEXT NOT NULL DEFAULT '',
			admin_lang        TEXT NOT NULL DEFAULT '',
			src_lang          TEXT NOT NULL DEFAULT '',
			data_type         TEXT NOT NULL DEFAULT '',
			original_format   TEXT NOT NULL DEFAULT '',
			original_encoding TEXT NOT NULL DEFAULT '',
			entry_count       INTEGER NOT NULL DEFAULT 0,
			properties        JSONB NOT NULL DEFAULT '{}'::jsonb,
			PRIMARY KEY (workspace_id, id)
		);
		CREATE INDEX IF NOT EXISTS idx_tm_sessions_hash ON tm_import_sessions(workspace_id, file_hash);
		CREATE INDEX IF NOT EXISTS idx_tm_sessions_time ON tm_import_sessions(workspace_id, imported_at DESC);

		CREATE TABLE IF NOT EXISTS tm_entry_origins (
			workspace_id TEXT NOT NULL,
			entry_id     TEXT NOT NULL,
			ordinal      INTEGER NOT NULL,
			source       TEXT NOT NULL,
			key          TEXT NOT NULL DEFAULT '',
			reference    TEXT NOT NULL DEFAULT '',
			added_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			added_by     TEXT NOT NULL DEFAULT '',
			session_id   TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (workspace_id, entry_id, ordinal),
			FOREIGN KEY (workspace_id, entry_id) REFERENCES tm_entries(workspace_id, id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_tm_origin_source  ON tm_entry_origins(workspace_id, source);
		CREATE INDEX IF NOT EXISTS idx_tm_origin_key     ON tm_entry_origins(workspace_id, key);
		CREATE INDEX IF NOT EXISTS idx_tm_origin_session ON tm_entry_origins(workspace_id, session_id);
`

	memoryPostgresVersionChain = `		ALTER TABLE tm_entries ADD COLUMN IF NOT EXISTS point TEXT NOT NULL DEFAULT '';
		ALTER TABLE tm_entries ADD COLUMN IF NOT EXISTS unit TEXT NOT NULL DEFAULT '';
		ALTER TABLE tm_entry_origins ADD COLUMN IF NOT EXISTS context_fp TEXT NOT NULL DEFAULT '';
		CREATE INDEX IF NOT EXISTS idx_tm_ws_unit ON tm_entries(workspace_id, unit, point);
`

	memoryPostgresRevision1 = `
				ALTER TABLE tm_entries ADD COLUMN IF NOT EXISTS revision TEXT;
				ALTER TABLE tm_entries ALTER COLUMN revision SET DEFAULT gen_random_uuid()::text;
			`

	memoryPostgresRevision2 = `UPDATE tm_entries SET revision = gen_random_uuid()::text WHERE revision IS NULL;`

	memoryPostgresRevision3 = `ALTER TABLE tm_entries ADD CONSTRAINT tm_entries_revision_not_null CHECK (revision IS NOT NULL) NOT VALID;`

	memoryPostgresRevision4 = `ALTER TABLE tm_entries VALIDATE CONSTRAINT tm_entries_revision_not_null;`

	memoryPostgresRevision5 = `
				ALTER TABLE tm_entries ALTER COLUMN revision SET NOT NULL;
				ALTER TABLE tm_entries DROP CONSTRAINT tm_entries_revision_not_null;
			`
)
