package terms

// The SQL of each shipped terms migration, frozen as it shipped. A shipped
// migration means the same statements for every database that runs it, so
// these do not follow the schema descriptors (terms/schema): a test holds each
// to its renderer, and a change to the descriptors is a new numbered
// migration.
const (
	termsSQLiteV1 = `
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
		`

	termsSQLiteV2 = `
		ALTER TABLE tb_concepts ADD COLUMN source TEXT NOT NULL DEFAULT 'terminology';
		ALTER TABLE tb_terms ADD COLUMN competitor_term INTEGER NOT NULL DEFAULT 0;
		`

	termsSQLiteV3 = `
		CREATE TABLE IF NOT EXISTS tb_relations (
			id          TEXT PRIMARY KEY,
			source_id   TEXT NOT NULL REFERENCES tb_concepts(id) ON DELETE CASCADE,
			target_id   TEXT NOT NULL REFERENCES tb_concepts(id) ON DELETE CASCADE,
			relation    TEXT NOT NULL,
			note        TEXT NOT NULL DEFAULT '',
			stream      TEXT NOT NULL DEFAULT '',
			valid_from  TEXT,            -- RFC3339, NULL = unbounded
			valid_to    TEXT,
			tags        TEXT NOT NULL DEFAULT '{}',  -- JSON object
			created_at  TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_tb_relations_source ON tb_relations(source_id);
		CREATE INDEX IF NOT EXISTS idx_tb_relations_target ON tb_relations(target_id);
		CREATE INDEX IF NOT EXISTS idx_tb_relations_stream ON tb_relations(stream);

		ALTER TABLE tb_terms ADD COLUMN valid_from TEXT;
		ALTER TABLE tb_terms ADD COLUMN valid_to TEXT;
		ALTER TABLE tb_terms ADD COLUMN tags TEXT NOT NULL DEFAULT '{}';
		`

	termsSQLiteV4 = `
		ALTER TABLE tb_terms ADD COLUMN forms TEXT NOT NULL DEFAULT '[]';
		`

	termsSQLiteV5 = `
		ALTER TABLE tb_concepts ADD COLUMN do_not_translate INTEGER NOT NULL DEFAULT 0;
		`

	termsSQLiteV6 = `
		ALTER TABLE tb_concepts ADD COLUMN advisory INTEGER NOT NULL DEFAULT 0;
		`

	termsSQLiteV7 = `		ALTER TABLE tb_terms ADD COLUMN text_normalized TEXT NOT NULL DEFAULT '';
		CREATE INDEX IF NOT EXISTS idx_tb_terms_normalized ON tb_terms(locale, text_normalized);
`
)
