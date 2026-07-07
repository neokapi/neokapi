
		CREATE TABLE IF NOT EXISTS tb_concepts (
			id          TEXT PRIMARY KEY,
			project_id  TEXT NOT NULL DEFAULT '',
			stream      TEXT NOT NULL DEFAULT '',
			domain      TEXT NOT NULL DEFAULT '',
			definition  TEXT NOT NULL DEFAULT '',
			properties  TEXT,
			created_at  TEXT NOT NULL,
			updated_at  TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_tb_concepts_stream ON tb_concepts(stream);

		CREATE TABLE IF NOT EXISTS tb_terms (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			concept_id    TEXT NOT NULL REFERENCES tb_concepts(id) ON DELETE CASCADE,
			text          TEXT NOT NULL,
			text_lower    TEXT NOT NULL,
			locale        TEXT NOT NULL,
			status        TEXT NOT NULL DEFAULT 'approved',
			part_of_speech TEXT NOT NULL DEFAULT '',
			gender        TEXT NOT NULL DEFAULT '',
			note          TEXT NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_tb_terms_concept ON tb_terms(concept_id);
		CREATE INDEX IF NOT EXISTS idx_tb_terms_locale ON tb_terms(locale);
		CREATE INDEX IF NOT EXISTS idx_tb_terms_text ON tb_terms(text_lower, locale);

		-- FTS5 trigram index for fuzzy term matching.
		CREATE VIRTUAL TABLE IF NOT EXISTS tb_terms_trigram USING fts5(
			text_lower,
			content='tb_terms', content_rowid='id',
			tokenize='trigram'
		);

		CREATE TRIGGER tb_terms_trigram_ai AFTER INSERT ON tb_terms BEGIN
			INSERT INTO tb_terms_trigram(rowid, text_lower) VALUES (new.id, new.text_lower);
		END;
		CREATE TRIGGER tb_terms_trigram_ad AFTER DELETE ON tb_terms BEGIN
			INSERT INTO tb_terms_trigram(tb_terms_trigram, rowid, text_lower)
			VALUES ('delete', old.id, old.text_lower);
		END;
		CREATE TRIGGER tb_terms_trigram_au AFTER UPDATE ON tb_terms BEGIN
			INSERT INTO tb_terms_trigram(tb_terms_trigram, rowid, text_lower)
			VALUES ('delete', old.id, old.text_lower);
			INSERT INTO tb_terms_trigram(rowid, text_lower) VALUES (new.id, new.text_lower);
		END;
		