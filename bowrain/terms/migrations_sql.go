package terms

// The SQL of each shipped terms migration, frozen as it shipped. A shipped
// migration means the same statements for every database that runs it, so
// these do not follow the schema descriptors (terms/schema): a test holds each
// to its renderer, and a change to the descriptors is a new numbered
// migration.
const (
	termsPostgresBaseline = `		CREATE TABLE IF NOT EXISTS tb_concepts (
			workspace_id     TEXT NOT NULL,
			id               TEXT NOT NULL,
			stream           TEXT NOT NULL DEFAULT '',
			domain           TEXT NOT NULL DEFAULT '',
			definition       TEXT NOT NULL DEFAULT '',
			properties       TEXT,
			created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			source           TEXT NOT NULL DEFAULT 'terminology',
			do_not_translate BOOLEAN NOT NULL DEFAULT FALSE,
			advisory         BOOLEAN NOT NULL DEFAULT FALSE,
			PRIMARY KEY (workspace_id, id)
		);
		CREATE INDEX IF NOT EXISTS idx_tb_concepts_ws_stream ON tb_concepts(workspace_id, stream);
		CREATE TABLE IF NOT EXISTS tb_terms (
			workspace_id    TEXT NOT NULL,
			id              SERIAL PRIMARY KEY,
			concept_id      TEXT NOT NULL,
			text            TEXT NOT NULL,
			text_lower      TEXT NOT NULL,
			locale          TEXT NOT NULL,
			status          TEXT NOT NULL DEFAULT 'approved',
			part_of_speech  TEXT NOT NULL DEFAULT '',
			gender          TEXT NOT NULL DEFAULT '',
			note            TEXT NOT NULL DEFAULT '',
			competitor_term BOOLEAN NOT NULL DEFAULT FALSE,
			valid_from      TIMESTAMPTZ,
			valid_to        TIMESTAMPTZ,
			tags            JSONB NOT NULL DEFAULT '{}',
			forms           JSONB NOT NULL DEFAULT '[]',
			FOREIGN KEY (workspace_id, concept_id) REFERENCES tb_concepts(workspace_id, id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_tb_terms_ws_concept ON tb_terms(workspace_id, concept_id);
		CREATE INDEX IF NOT EXISTS idx_tb_terms_ws_locale ON tb_terms(workspace_id, locale);
		CREATE INDEX IF NOT EXISTS idx_tb_terms_ws_text ON tb_terms(workspace_id, text_lower, locale);
		CREATE TABLE IF NOT EXISTS tb_relations (
			workspace_id TEXT NOT NULL,
			id           TEXT NOT NULL,
			source_id    TEXT NOT NULL,
			target_id    TEXT NOT NULL,
			relation     TEXT NOT NULL,
			note         TEXT NOT NULL DEFAULT '',
			stream       TEXT NOT NULL DEFAULT '',
			valid_from   TIMESTAMPTZ,
			valid_to     TIMESTAMPTZ,
			tags         JSONB NOT NULL DEFAULT '{}',
			created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (workspace_id, id),
			FOREIGN KEY (workspace_id, source_id) REFERENCES tb_concepts(workspace_id, id) ON DELETE CASCADE,
			FOREIGN KEY (workspace_id, target_id) REFERENCES tb_concepts(workspace_id, id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_tb_relations_ws_source ON tb_relations(workspace_id, source_id);
		CREATE INDEX IF NOT EXISTS idx_tb_relations_ws_target ON tb_relations(workspace_id, target_id);
		CREATE INDEX IF NOT EXISTS idx_tb_relations_ws_stream ON tb_relations(workspace_id, stream);

		CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;

		CREATE INDEX IF NOT EXISTS idx_tb_terms_trgm ON tb_terms USING gin (text_lower gin_trgm_ops);

		ALTER TABLE tb_terms ADD COLUMN IF NOT EXISTS search_tsv tsvector;
		UPDATE tb_terms SET search_tsv = to_tsvector('simple', text_lower)
			WHERE search_tsv IS NULL;
		CREATE INDEX IF NOT EXISTS idx_tb_terms_search_tsv ON tb_terms USING gin (search_tsv);

		CREATE OR REPLACE FUNCTION tb_terms_search_tsv_update() RETURNS trigger AS $$
		BEGIN
			NEW.search_tsv := to_tsvector('simple', NEW.text_lower);
			RETURN NEW;
		END $$ LANGUAGE plpgsql;

		DROP TRIGGER IF EXISTS tb_terms_search_tsv_trigger ON tb_terms;
		CREATE TRIGGER tb_terms_search_tsv_trigger BEFORE INSERT OR UPDATE ON tb_terms
			FOR EACH ROW EXECUTE FUNCTION tb_terms_search_tsv_update();
		`

	termsPostgresV6 = `		ALTER TABLE tb_terms ADD COLUMN IF NOT EXISTS forms JSONB NOT NULL DEFAULT '[]';
`

	termsPostgresV7 = `		ALTER TABLE tb_concepts ADD COLUMN IF NOT EXISTS do_not_translate BOOLEAN NOT NULL DEFAULT FALSE;
`

	termsPostgresV8 = `		ALTER TABLE tb_concepts ADD COLUMN IF NOT EXISTS advisory BOOLEAN NOT NULL DEFAULT FALSE;
`

	termsPostgresV10 = `		ALTER TABLE tb_terms ADD COLUMN IF NOT EXISTS text_normalized TEXT NOT NULL DEFAULT '';
		CREATE INDEX IF NOT EXISTS idx_tb_terms_ws_normalized ON tb_terms(workspace_id, locale, text_normalized);
`
)

// The SQL of the terms migrations that give tb_concepts its revision column,
// frozen as they shipped. They do not follow storage.RevisionColumn: a test
// holds them to it, and a change to either is a new numbered migration.
const (
	termsPostgresRevision1 = `
				ALTER TABLE tb_concepts ADD COLUMN IF NOT EXISTS revision TEXT;
				ALTER TABLE tb_concepts ALTER COLUMN revision SET DEFAULT gen_random_uuid()::text;
			`
	termsPostgresRevision2 = `UPDATE tb_concepts SET revision = gen_random_uuid()::text WHERE revision IS NULL;`
	termsPostgresRevision3 = `ALTER TABLE tb_concepts ADD CONSTRAINT tb_concepts_revision_not_null CHECK (revision IS NOT NULL) NOT VALID;`
	termsPostgresRevision4 = `ALTER TABLE tb_concepts VALIDATE CONSTRAINT tb_concepts_revision_not_null;`
	termsPostgresRevision5 = `
				ALTER TABLE tb_concepts ALTER COLUMN revision SET NOT NULL;
				ALTER TABLE tb_concepts DROP CONSTRAINT tb_concepts_revision_not_null;
			`
)

// termsPostgresImports is the record of each terms import (ImportLedger).
const termsPostgresImports = `
CREATE TABLE IF NOT EXISTS tb_imports (
    workspace_id TEXT NOT NULL,
    id           TEXT NOT NULL,
    name         TEXT NOT NULL DEFAULT '',
    created_by   TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL,
    updated_by   TEXT NOT NULL DEFAULT '',
    updated_at   TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (workspace_id, id)
);
CREATE INDEX IF NOT EXISTS tb_imports_updated ON tb_imports (workspace_id, updated_at DESC);`
