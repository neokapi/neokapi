// Package schema declares the terminology (terms) table layouts once, as
// data, and renders them to SQLite or PostgreSQL DDL through the dialect seam
// in core/storage/schema. The two terms backends — framework SQLite
// (terms) and bowrain Postgres — both build their migration SQL from these
// descriptors, so the schemas cannot drift.
//
// SQLite output is byte-identical to the historical hand-written migrations
// (golden-tested); Postgres output is semantically identical to the historical
// v1-v4 migrations (statement-set tested). Fuzzy-match infrastructure is
// inherently dialect-specific (an FTS5 trigram virtual table + triggers vs a
// pg_trgm GIN index + a tsvector column and trigger) and lives here as literal
// per-dialect blocks, not in the seam.
//
// This mirrors the content memory (memory/schema) dialect seam; see that package for the
// content memory half of the same refactor.
package schema

import (
	"strings"

	sq "github.com/neokapi/neokapi/core/storage/schema"
)

// Terms table descriptors. Column specs are written side by side per dialect
// so type drift is visible at the definition site. Tenant tables gain a leading
// workspace_id column and workspace_id-prefixed PK/FK on Postgres only. The
// SQLite-only project_id column (absent on Postgres, which scopes by
// workspace_id) carries an empty Postgres spec so the seam omits it there.
var (
	tbConcepts = sq.Table{
		Name:        "tb_concepts",
		Tenant:      true,
		SQLiteWidth: 12,
		Columns: []sq.Column{
			{Name: "id", SQLite: "TEXT PRIMARY KEY", PG: "TEXT NOT NULL"},
			// project_id scopes local (per-file) terms stores; Postgres scopes by
			// workspace_id instead and has no project_id column.
			{Name: "project_id", SQLite: "TEXT NOT NULL DEFAULT ''"},
			{Name: "stream", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "TEXT NOT NULL DEFAULT ''"},
			{Name: "domain", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "TEXT NOT NULL DEFAULT ''"},
			{Name: "definition", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "TEXT NOT NULL DEFAULT ''"},
			{Name: "properties", SQLite: "TEXT", PG: "TEXT"},
			{Name: "created_at", SQLite: "TEXT NOT NULL", PG: "TIMESTAMPTZ NOT NULL DEFAULT NOW()"},
			{Name: "updated_at", SQLite: "TEXT NOT NULL", PG: "TIMESTAMPTZ NOT NULL DEFAULT NOW()"},
			// source is added by a later migration (v2 SQLite / v4 PG).
			{Name: "source", SQLite: "TEXT NOT NULL DEFAULT 'terminology'", PG: "TEXT NOT NULL DEFAULT 'terminology'"},
		},
		PGPK: []string{"id"},
		Indexes: []sq.Index{
			{Name: "idx_tb_concepts_stream", PGName: "idx_tb_concepts_ws_stream", Cols: []string{"stream"}},
		},
	}

	tbTerms = sq.Table{
		Name:        "tb_terms",
		Tenant:      true,
		SQLiteWidth: 14,
		Columns: []sq.Column{
			{Name: "id", SQLite: "INTEGER PRIMARY KEY AUTOINCREMENT", PG: "SERIAL PRIMARY KEY"},
			{Name: "concept_id", SQLite: "TEXT NOT NULL REFERENCES tb_concepts(id) ON DELETE CASCADE", PG: "TEXT NOT NULL"},
			{Name: "text", SQLite: "TEXT NOT NULL", PG: "TEXT NOT NULL"},
			{Name: "text_lower", SQLite: "TEXT NOT NULL", PG: "TEXT NOT NULL"},
			{Name: "locale", SQLite: "TEXT NOT NULL", PG: "TEXT NOT NULL"},
			{Name: "status", SQLite: "TEXT NOT NULL DEFAULT 'approved'", PG: "TEXT NOT NULL DEFAULT 'approved'"},
			{Name: "part_of_speech", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "TEXT NOT NULL DEFAULT ''"},
			{Name: "gender", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "TEXT NOT NULL DEFAULT ''"},
			{Name: "note", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "TEXT NOT NULL DEFAULT ''"},
			// competitor_term is added by a later migration (v2 SQLite / v4 PG).
			{Name: "competitor_term", SQLite: "INTEGER NOT NULL DEFAULT 0", PG: "BOOLEAN NOT NULL DEFAULT FALSE"},
			// validity columns are added by a later migration (v3 SQLite / v4 PG).
			{Name: "valid_from", SQLite: "TEXT", PG: "TIMESTAMPTZ"},
			{Name: "valid_to", SQLite: "TEXT", PG: "TIMESTAMPTZ"},
			{Name: "tags", SQLite: "TEXT NOT NULL DEFAULT '{}'", PG: "JSONB NOT NULL DEFAULT '{}'"},
		},
		FKs: []sq.FK{{Cols: []string{"concept_id"}, RefTable: "tb_concepts", RefCols: []string{"id"}, SQLiteInline: true}},
		Indexes: []sq.Index{
			{Name: "idx_tb_terms_concept", PGName: "idx_tb_terms_ws_concept", Cols: []string{"concept_id"}},
			{Name: "idx_tb_terms_locale", PGName: "idx_tb_terms_ws_locale", Cols: []string{"locale"}},
			{Name: "idx_tb_terms_text", PGName: "idx_tb_terms_ws_text", Cols: []string{"text_lower", "locale"}},
		},
	}

	tbRelations = sq.Table{
		Name:        "tb_relations",
		Tenant:      true,
		SQLiteWidth: 12,
		Columns: []sq.Column{
			{Name: "id", SQLite: "TEXT PRIMARY KEY", PG: "TEXT NOT NULL"},
			{Name: "source_id", SQLite: "TEXT NOT NULL REFERENCES tb_concepts(id) ON DELETE CASCADE", PG: "TEXT NOT NULL"},
			{Name: "target_id", SQLite: "TEXT NOT NULL REFERENCES tb_concepts(id) ON DELETE CASCADE", PG: "TEXT NOT NULL"},
			{Name: "relation", SQLite: "TEXT NOT NULL", PG: "TEXT NOT NULL"},
			{Name: "note", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "TEXT NOT NULL DEFAULT ''"},
			{Name: "stream", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "TEXT NOT NULL DEFAULT ''"},
			{Name: "valid_from", SQLite: "TEXT", PG: "TIMESTAMPTZ", Comment: "            -- RFC3339, NULL = unbounded"},
			{Name: "valid_to", SQLite: "TEXT", PG: "TIMESTAMPTZ"},
			{Name: "tags", SQLite: "TEXT NOT NULL DEFAULT '{}'", PG: "JSONB NOT NULL DEFAULT '{}'", Comment: "  -- JSON object"},
			{Name: "created_at", SQLite: "TEXT NOT NULL", PG: "TIMESTAMPTZ NOT NULL DEFAULT NOW()"},
		},
		PGPK: []string{"id"},
		FKs: []sq.FK{
			{Cols: []string{"source_id"}, RefTable: "tb_concepts", RefCols: []string{"id"}, SQLiteInline: true},
			{Cols: []string{"target_id"}, RefTable: "tb_concepts", RefCols: []string{"id"}, SQLiteInline: true},
		},
		Indexes: []sq.Index{
			{Name: "idx_tb_relations_source", PGName: "idx_tb_relations_ws_source", Cols: []string{"source_id"}},
			{Name: "idx_tb_relations_target", PGName: "idx_tb_relations_ws_target", Cols: []string{"target_id"}},
			{Name: "idx_tb_relations_stream", PGName: "idx_tb_relations_ws_stream", Cols: []string{"stream"}},
		},
	}
)

// tbTermsTrigramBlock is the SQLite word-search infrastructure: a contentless
// FTS5 trigram virtual table over tb_terms.text_lower. The trigram tokenizer is
// identical across cgo and no-cgo builds, so a .db created by one binary stays
// queryable by the other.
const tbTermsTrigramBlock = "\t\t-- FTS5 trigram index for fuzzy term matching.\n" +
	"\t\tCREATE VIRTUAL TABLE IF NOT EXISTS tb_terms_trigram USING fts5(\n" +
	"\t\t\ttext_lower,\n" +
	"\t\t\tcontent='tb_terms', content_rowid='id',\n" +
	"\t\t\ttokenize='trigram'\n" +
	"\t\t);\n"

// tbTermsTrigramTriggers keeps the contentless trigram index in sync with
// tb_terms on insert/delete/update.
const tbTermsTrigramTriggers = "\t\tCREATE TRIGGER tb_terms_trigram_ai AFTER INSERT ON tb_terms BEGIN\n" +
	"\t\t\tINSERT INTO tb_terms_trigram(rowid, text_lower) VALUES (new.id, new.text_lower);\n" +
	"\t\tEND;\n" +
	"\t\tCREATE TRIGGER tb_terms_trigram_ad AFTER DELETE ON tb_terms BEGIN\n" +
	"\t\t\tINSERT INTO tb_terms_trigram(tb_terms_trigram, rowid, text_lower)\n" +
	"\t\t\tVALUES ('delete', old.id, old.text_lower);\n" +
	"\t\tEND;\n" +
	"\t\tCREATE TRIGGER tb_terms_trigram_au AFTER UPDATE ON tb_terms BEGIN\n" +
	"\t\t\tINSERT INTO tb_terms_trigram(tb_terms_trigram, rowid, text_lower)\n" +
	"\t\t\tVALUES ('delete', old.id, old.text_lower);\n" +
	"\t\t\tINSERT INTO tb_terms_trigram(rowid, text_lower) VALUES (new.id, new.text_lower);\n" +
	"\t\tEND;\n"

// pgTermsFuzzyBlock is the Postgres fuzzy + UI-search infrastructure on
// tb_terms: a pg_trgm GIN index for fuzzy candidate retrieval plus a tsvector
// column (maintained by a trigger) and its GIN index for ranked search. The
// SQLite equivalent is the FTS5 trigram virtual table above.
const pgTermsFuzzyBlock = `
		CREATE EXTENSION IF NOT EXISTS pg_trgm;

		CREATE INDEX IF NOT EXISTS idx_tb_terms_trgm ON tb_terms USING gin (text_lower gin_trgm_ops);

		ALTER TABLE tb_terms ADD COLUMN IF NOT EXISTS search_tsv tsvector;
		-- Backfill rows that predate the column. WHERE-guarded so replaying
		-- this block over a populated table is a no-op rather than a full
		-- table rewrite; the trigger below maintains it from here on.
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

// ── SQLite render (byte-identical to the historical migrations) ──────────────

// RenderTermsSQLiteV1 renders the v1 SQLite terms migration body, byte-identical
// to the historical hand-written migration: concepts + terms tables and the
// FTS5 trigram virtual table with its sync triggers. The source column
// (concepts) and competitor/validity columns (terms) are added by later
// migrations.
func RenderTermsSQLiteV1() string {
	conceptsOpt := sq.Opt{IfNotExists: true, Exclude: []string{"source"}}
	termsOpt := sq.Opt{IfNotExists: true, Exclude: []string{"competitor_term", "valid_from", "valid_to", "tags"}}
	sections := []string{
		tbConcepts.Create(sq.SQLite, conceptsOpt) + tbConcepts.CreateIndexes(sq.SQLite, conceptsOpt),
		tbTerms.Create(sq.SQLite, termsOpt) + tbTerms.CreateIndexes(sq.SQLite, termsOpt),
		tbTermsTrigramBlock,
		tbTermsTrigramTriggers,
	}
	return "\n" + strings.Join(sections, "\n") + "\t\t"
}

// RenderTermsSQLiteV2 renders the v2 SQLite migration: source on concepts and the
// competitor_term flag on terms. SQLite ADD COLUMN takes no IF NOT EXISTS.
func RenderTermsSQLiteV2() string {
	return "\n" +
		tbConcepts.AddColumn(sq.SQLite, sq.Opt{}, "source") +
		tbTerms.AddColumn(sq.SQLite, sq.Opt{}, "competitor_term") +
		"\t\t"
}

// RenderTermsSQLiteV3 renders the v3 SQLite migration: the relations table plus the
// term validity columns.
func RenderTermsSQLiteV3() string {
	o := sq.Opt{IfNotExists: true}
	relations := tbRelations.Create(sq.SQLite, o) + tbRelations.CreateIndexes(sq.SQLite, o)
	alters := tbTerms.AddColumn(sq.SQLite, sq.Opt{}, "valid_from") +
		tbTerms.AddColumn(sq.SQLite, sq.Opt{}, "valid_to") +
		tbTerms.AddColumn(sq.SQLite, sq.Opt{}, "tags")
	return "\n" + strings.Join([]string{relations, alters}, "\n") + "\t\t"
}

// ── Postgres render (semantically identical to the historical migrations) ────

// RenderTermsPostgresV1 renders the fresh v1 Postgres schema: concepts (no stream
// or source yet) and terms (no competitor/validity yet), with the base term
// indexes. Column order is canonicalized by the seam (workspace_id first);
// order is cosmetic and the semantic-equivalence test is order-insensitive.
func RenderTermsPostgresV1() string {
	conceptsOpt := sq.Opt{IfNotExists: true, Exclude: []string{"stream", "source"}}
	termsOpt := sq.Opt{IfNotExists: true, Exclude: []string{"competitor_term", "valid_from", "valid_to", "tags"}}
	return tbConcepts.Create(sq.Postgres, conceptsOpt) +
		tbTerms.Create(sq.Postgres, termsOpt) +
		tbTerms.CreateIndexes(sq.Postgres, termsOpt)
}

// RenderTermsPostgresV2 renders the v2 Postgres migration: the stream column on
// concepts and its index.
func RenderTermsPostgresV2() string {
	o := sq.Opt{IfNotExists: true}
	return tbConcepts.AddColumn(sq.Postgres, sq.Opt{}, "stream") +
		tbConcepts.CreateIndexes(sq.Postgres, o, "idx_tb_concepts_stream")
}

// RenderTermsPostgresV3 renders the v3 Postgres migration: the pg_trgm fuzzy index
// and the tsvector search column/trigger.
func RenderTermsPostgresV3() string {
	return pgTermsFuzzyBlock
}

// RenderTermsPostgresBaseline renders the whole current Postgres terms schema
// in one pass: every column present from the start, so nothing is added by a
// later ALTER.
//
// This is what the consolidated tb_schema_migrations baseline applies. The V1–V4
// renderers above stay because they are the record of how the schema was built,
// and the semantic-equivalence test in this package compares what they produce
// against what this renders — a baseline that drifted from the history it folds
// would otherwise be invisible.
//
// It is deliberately assembled from the same descriptors rather than written
// out as literal SQL: a column added to tbConcepts or tbTerms lands in the
// baseline and in the SQLite backend together, which is the property that keeps
// the two backends from drifting.
func RenderTermsPostgresBaseline() string {
	o := sq.Opt{IfNotExists: true}
	return tbConcepts.Create(sq.Postgres, o) +
		tbConcepts.CreateIndexes(sq.Postgres, o) +
		tbTerms.Create(sq.Postgres, o) +
		tbTerms.CreateIndexes(sq.Postgres, o) +
		tbRelations.Create(sq.Postgres, o) +
		tbRelations.CreateIndexes(sq.Postgres, o) +
		pgTermsFuzzyBlock
}

// RenderTermsPostgresV4 renders the v4 Postgres migration: the source column on
// concepts, the competitor/validity columns on terms, and the relations table.
func RenderTermsPostgresV4() string {
	o := sq.Opt{IfNotExists: true}
	var b strings.Builder
	b.WriteString(tbConcepts.AddColumn(sq.Postgres, sq.Opt{}, "source"))
	b.WriteString(tbTerms.AddColumn(sq.Postgres, sq.Opt{}, "competitor_term"))
	b.WriteString(tbTerms.AddColumn(sq.Postgres, sq.Opt{}, "valid_from"))
	b.WriteString(tbTerms.AddColumn(sq.Postgres, sq.Opt{}, "valid_to"))
	b.WriteString(tbTerms.AddColumn(sq.Postgres, sq.Opt{}, "tags"))
	b.WriteString(tbRelations.Create(sq.Postgres, o))
	b.WriteString(tbRelations.CreateIndexes(sq.Postgres, o))
	return b.String()
}
