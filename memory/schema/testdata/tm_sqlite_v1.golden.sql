
		CREATE TABLE IF NOT EXISTS tm_entries (
			id              TEXT PRIMARY KEY,
			project_id      TEXT NOT NULL DEFAULT '',
			stream          TEXT NOT NULL DEFAULT '',
			hint_src_lang   TEXT NOT NULL DEFAULT '',
			properties      TEXT NOT NULL DEFAULT '',
			note            TEXT NOT NULL DEFAULT '',
			created_at      TEXT NOT NULL,
			updated_at      TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_tm_project ON tm_entries(project_id);
		CREATE INDEX IF NOT EXISTS idx_tm_updated ON tm_entries(updated_at DESC);
		CREATE INDEX IF NOT EXISTS idx_tm_stream  ON tm_entries(stream);

		CREATE TABLE IF NOT EXISTS tm_variants (
			entry_id    TEXT NOT NULL REFERENCES tm_entries(id) ON DELETE CASCADE,
			locale      TEXT NOT NULL,
			coded       TEXT NOT NULL,
			plain       TEXT NOT NULL,
			struct_key  TEXT NOT NULL,
			general_key TEXT NOT NULL,
			PRIMARY KEY (entry_id, locale)
		);
		CREATE INDEX IF NOT EXISTS idx_tm_var_locale      ON tm_variants(locale);
		CREATE INDEX IF NOT EXISTS idx_tm_var_plain_loc   ON tm_variants(plain, locale);
		CREATE INDEX IF NOT EXISTS idx_tm_var_struct_loc  ON tm_variants(struct_key, locale);
		CREATE INDEX IF NOT EXISTS idx_tm_var_general_loc ON tm_variants(general_key, locale);

		CREATE VIRTUAL TABLE IF NOT EXISTS tm_variant_search USING fts5(
			text,
			locale UNINDEXED,
			entry_id UNINDEXED,
			tokenize='icu'
		);

		CREATE VIRTUAL TABLE IF NOT EXISTS tm_variant_trigram USING fts5(
			plain, struct_key, general_key,
			locale UNINDEXED,
			entry_id UNINDEXED,
			tokenize='trigram'
		);

		CREATE TABLE IF NOT EXISTS tm_entry_entities (
			entry_id       TEXT NOT NULL REFERENCES tm_entries(id) ON DELETE CASCADE,
			placeholder_id TEXT NOT NULL,
			entity_type    TEXT NOT NULL,
			PRIMARY KEY (entry_id, placeholder_id)
		);
		CREATE INDEX IF NOT EXISTS idx_entities_type ON tm_entry_entities(entity_type);

		CREATE TABLE IF NOT EXISTS tm_entry_entity_values (
			entry_id       TEXT NOT NULL,
			placeholder_id TEXT NOT NULL,
			locale         TEXT NOT NULL,
			text_value     TEXT NOT NULL DEFAULT '',
			start_pos      INTEGER NOT NULL DEFAULT 0,
			end_pos        INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (entry_id, placeholder_id, locale),
			FOREIGN KEY (entry_id, placeholder_id)
				REFERENCES tm_entry_entities(entry_id, placeholder_id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_entity_values_text ON tm_entry_entity_values(text_value, locale);

		CREATE TABLE IF NOT EXISTS tm_import_sessions (
			id                    TEXT PRIMARY KEY,
			file_key              TEXT NOT NULL,
			file_hash             TEXT NOT NULL DEFAULT '',
			file_size_bytes       INTEGER NOT NULL DEFAULT 0,
			imported_at           TEXT NOT NULL,
			imported_by           TEXT NOT NULL DEFAULT '',
			tool_name             TEXT NOT NULL DEFAULT '',
			tool_version          TEXT NOT NULL DEFAULT '',
			seg_type              TEXT NOT NULL DEFAULT '',
			admin_lang            TEXT NOT NULL DEFAULT '',
			src_lang              TEXT NOT NULL DEFAULT '',
			data_type             TEXT NOT NULL DEFAULT '',
			original_format       TEXT NOT NULL DEFAULT '',
			original_encoding     TEXT NOT NULL DEFAULT '',
			entry_count           INTEGER NOT NULL DEFAULT 0,
			properties_json       TEXT NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_sessions_file_hash   ON tm_import_sessions(file_hash);
		CREATE INDEX IF NOT EXISTS idx_sessions_imported_at ON tm_import_sessions(imported_at DESC);

		CREATE TABLE IF NOT EXISTS tm_entry_origins (
			entry_id   TEXT NOT NULL REFERENCES tm_entries(id) ON DELETE CASCADE,
			ordinal    INTEGER NOT NULL,
			source     TEXT NOT NULL,
			key        TEXT NOT NULL DEFAULT '',
			reference  TEXT NOT NULL DEFAULT '',
			added_at   TEXT NOT NULL,
			added_by   TEXT NOT NULL DEFAULT '',
			session_id TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (entry_id, ordinal)
		);
		CREATE INDEX IF NOT EXISTS idx_origins_source  ON tm_entry_origins(source);
		CREATE INDEX IF NOT EXISTS idx_origins_key     ON tm_entry_origins(key);
		CREATE INDEX IF NOT EXISTS idx_origins_session ON tm_entry_origins(session_id);
		