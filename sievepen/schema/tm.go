// Package schema declares the translation-memory and (via tb.go) terminology
// table layouts once, as data, and renders them to SQLite or PostgreSQL DDL
// through the dialect seam in core/storage/schema. The two TM backends —
// framework SQLite (sievepen) and bowrain Postgres — both build their migration
// SQL from these descriptors, so the schemas cannot drift.
//
// SQLite output is byte-identical to the historical hand-written migrations
// (golden-tested); Postgres output is semantically identical to the historical
// v4/v5 migrations (statement-set tested). Fuzzy-match infrastructure is
// inherently dialect-specific (FTS5 virtual tables vs pg_trgm GIN + tsvector)
// and lives here as literal per-dialect blocks, not in the seam.
package schema

import (
	"strings"

	sq "github.com/neokapi/neokapi/core/storage/schema"
)

// TM table descriptors. Column specs are written side by side per dialect so
// type drift is visible at the definition site. Tenant tables gain a leading
// workspace_id column and workspace_id-prefixed PK/FK on Postgres only.
var (
	tmEntries = sq.Table{
		Name:        "tm_entries",
		Tenant:      true,
		SQLiteWidth: 16,
		Columns: []sq.Column{
			{Name: "id", SQLite: "TEXT PRIMARY KEY", PG: "TEXT NOT NULL"},
			{Name: "project_id", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "TEXT NOT NULL DEFAULT ''"},
			{Name: "stream", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "TEXT NOT NULL DEFAULT ''"},
			{Name: "hint_src_lang", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "TEXT NOT NULL DEFAULT ''"},
			{Name: "properties", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "JSONB NOT NULL DEFAULT '{}'::jsonb"},
			{Name: "note", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "TEXT NOT NULL DEFAULT ''"},
			{Name: "created_at", SQLite: "TEXT NOT NULL", PG: "TIMESTAMPTZ NOT NULL DEFAULT NOW()"},
			{Name: "updated_at", SQLite: "TEXT NOT NULL", PG: "TIMESTAMPTZ NOT NULL DEFAULT NOW()"},
			// has_codes is added by a later migration (v3 SQLite); Postgres
			// derives the facet at query time and never stores it.
			{Name: "has_codes", SQLite: "INTEGER NOT NULL DEFAULT 0"},
		},
		PGPK: []string{"id"},
		Indexes: []sq.Index{
			{Name: "idx_tm_project", PGName: "idx_tm_ws_project", Cols: []string{"project_id"}},
			{Name: "idx_tm_updated", PGName: "idx_tm_ws_updated", Cols: []string{"updated_at DESC"}},
			{Name: "idx_tm_stream", PGName: "idx_tm_ws_stream", Cols: []string{"stream"}},
		},
	}

	tmVariants = sq.Table{
		Name:   "tm_variants",
		Tenant: true,
		Columns: []sq.Column{
			{Name: "entry_id", SQLite: "TEXT NOT NULL REFERENCES tm_entries(id) ON DELETE CASCADE", PG: "TEXT NOT NULL"},
			{Name: "locale", SQLite: "TEXT NOT NULL", PG: "TEXT NOT NULL"},
			{Name: "coded", SQLite: "TEXT NOT NULL", PG: "TEXT NOT NULL"},
			{Name: "plain", SQLite: "TEXT NOT NULL", PG: "TEXT NOT NULL"},
			{Name: "struct_key", SQLite: "TEXT NOT NULL", PG: "TEXT NOT NULL"},
			{Name: "general_key", SQLite: "TEXT NOT NULL", PG: "TEXT NOT NULL"},
		},
		PK:  []string{"entry_id", "locale"},
		FKs: []sq.FK{{Cols: []string{"entry_id"}, RefTable: "tm_entries", RefCols: []string{"id"}, SQLiteInline: true}},
		Indexes: []sq.Index{
			{Name: "idx_tm_var_locale", PGName: "idx_tm_var_ws_locale", Cols: []string{"locale"}},
			{Name: "idx_tm_var_plain_loc", Cols: []string{"plain", "locale"}},
			{Name: "idx_tm_var_struct_loc", Cols: []string{"struct_key", "locale"}},
			{Name: "idx_tm_var_general_loc", Cols: []string{"general_key", "locale"}},
		},
	}

	tmEntryEntities = sq.Table{
		Name:   "tm_entry_entities",
		Tenant: true,
		Columns: []sq.Column{
			{Name: "entry_id", SQLite: "TEXT NOT NULL REFERENCES tm_entries(id) ON DELETE CASCADE", PG: "TEXT NOT NULL"},
			{Name: "placeholder_id", SQLite: "TEXT NOT NULL", PG: "TEXT NOT NULL"},
			{Name: "entity_type", SQLite: "TEXT NOT NULL", PG: "TEXT NOT NULL"},
			// concept_id is added by a later migration (v2 SQLite / v5 PG).
			{Name: "concept_id", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "TEXT NOT NULL DEFAULT ''"},
		},
		PK:  []string{"entry_id", "placeholder_id"},
		FKs: []sq.FK{{Cols: []string{"entry_id"}, RefTable: "tm_entries", RefCols: []string{"id"}, SQLiteInline: true}},
		Indexes: []sq.Index{
			{Name: "idx_entities_type", PGName: "idx_tm_entities_type", Cols: []string{"entity_type"}},
			{Name: "idx_entities_concept", PGName: "idx_tm_entities_concept", Cols: []string{"concept_id"}},
		},
	}

	tmEntryEntityValues = sq.Table{
		Name:   "tm_entry_entity_values",
		Tenant: true,
		Columns: []sq.Column{
			{Name: "entry_id", SQLite: "TEXT NOT NULL", PG: "TEXT NOT NULL"},
			{Name: "placeholder_id", SQLite: "TEXT NOT NULL", PG: "TEXT NOT NULL"},
			{Name: "locale", SQLite: "TEXT NOT NULL", PG: "TEXT NOT NULL"},
			{Name: "text_value", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "TEXT NOT NULL DEFAULT ''"},
			{Name: "start_pos", SQLite: "INTEGER NOT NULL DEFAULT 0", PG: "INTEGER NOT NULL DEFAULT 0"},
			{Name: "end_pos", SQLite: "INTEGER NOT NULL DEFAULT 0", PG: "INTEGER NOT NULL DEFAULT 0"},
		},
		PK: []string{"entry_id", "placeholder_id", "locale"},
		FKs: []sq.FK{{
			Cols: []string{"entry_id", "placeholder_id"}, RefTable: "tm_entry_entities",
			RefCols: []string{"entry_id", "placeholder_id"},
		}},
		Indexes: []sq.Index{
			{Name: "idx_entity_values_text", PGName: "idx_tm_entity_values_text", Cols: []string{"text_value", "locale"}},
		},
	}

	tmImportSessions = sq.Table{
		Name:        "tm_import_sessions",
		Tenant:      true,
		SQLiteWidth: 22,
		Columns: []sq.Column{
			{Name: "id", SQLite: "TEXT PRIMARY KEY", PG: "TEXT NOT NULL"},
			{Name: "file_key", SQLite: "TEXT NOT NULL", PG: "TEXT NOT NULL"},
			{Name: "file_hash", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "TEXT NOT NULL DEFAULT ''"},
			{Name: "file_size_bytes", SQLite: "INTEGER NOT NULL DEFAULT 0", PG: "BIGINT NOT NULL DEFAULT 0"},
			{Name: "imported_at", SQLite: "TEXT NOT NULL", PG: "TIMESTAMPTZ NOT NULL DEFAULT NOW()"},
			{Name: "imported_by", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "TEXT NOT NULL DEFAULT ''"},
			{Name: "tool_name", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "TEXT NOT NULL DEFAULT ''"},
			{Name: "tool_version", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "TEXT NOT NULL DEFAULT ''"},
			{Name: "seg_type", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "TEXT NOT NULL DEFAULT ''"},
			{Name: "admin_lang", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "TEXT NOT NULL DEFAULT ''"},
			{Name: "src_lang", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "TEXT NOT NULL DEFAULT ''"},
			{Name: "data_type", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "TEXT NOT NULL DEFAULT ''"},
			{Name: "original_format", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "TEXT NOT NULL DEFAULT ''"},
			{Name: "original_encoding", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "TEXT NOT NULL DEFAULT ''"},
			{Name: "entry_count", SQLite: "INTEGER NOT NULL DEFAULT 0", PG: "INTEGER NOT NULL DEFAULT 0"},
			{Name: "properties_json", PGName: "properties", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "JSONB NOT NULL DEFAULT '{}'::jsonb"},
		},
		PGPK: []string{"id"},
		Indexes: []sq.Index{
			{Name: "idx_sessions_file_hash", PGName: "idx_tm_sessions_hash", Cols: []string{"file_hash"}},
			{Name: "idx_sessions_imported_at", PGName: "idx_tm_sessions_time", Cols: []string{"imported_at DESC"}},
		},
	}

	tmEntryOrigins = sq.Table{
		Name:   "tm_entry_origins",
		Tenant: true,
		Columns: []sq.Column{
			{Name: "entry_id", SQLite: "TEXT NOT NULL REFERENCES tm_entries(id) ON DELETE CASCADE", PG: "TEXT NOT NULL"},
			{Name: "ordinal", SQLite: "INTEGER NOT NULL", PG: "INTEGER NOT NULL"},
			{Name: "source", SQLite: "TEXT NOT NULL", PG: "TEXT NOT NULL"},
			{Name: "key", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "TEXT NOT NULL DEFAULT ''"},
			{Name: "reference", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "TEXT NOT NULL DEFAULT ''"},
			{Name: "added_at", SQLite: "TEXT NOT NULL", PG: "TIMESTAMPTZ NOT NULL DEFAULT NOW()"},
			{Name: "added_by", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "TEXT NOT NULL DEFAULT ''"},
			{Name: "session_id", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "TEXT NOT NULL DEFAULT ''"},
		},
		PK:  []string{"entry_id", "ordinal"},
		FKs: []sq.FK{{Cols: []string{"entry_id"}, RefTable: "tm_entries", RefCols: []string{"id"}, SQLiteInline: true}},
		Indexes: []sq.Index{
			{Name: "idx_origins_source", PGName: "idx_tm_origin_source", Cols: []string{"source"}},
			{Name: "idx_origins_key", PGName: "idx_tm_origin_key", Cols: []string{"key"}},
			{Name: "idx_origins_session", PGName: "idx_tm_origin_session", Cols: []string{"session_id"}},
		},
	}
)

// ftsSearchBlock is the SQLite word-search virtual table. The tokenizer is
// resolved by the caller (ICU under cgo, unicode61 under no-cgo).
func ftsSearchBlock(tokenizer string) string {
	return "\t\tCREATE VIRTUAL TABLE IF NOT EXISTS tm_variant_search USING fts5(\n" +
		"\t\t\ttext,\n" +
		"\t\t\tlocale UNINDEXED,\n" +
		"\t\t\tentry_id UNINDEXED,\n" +
		"\t\t\ttokenize='" + tokenizer + "'\n" +
		"\t\t);\n"
}

// ftsTrigramBlock is the SQLite portable trigram virtual table.
const ftsTrigramBlock = "\t\tCREATE VIRTUAL TABLE IF NOT EXISTS tm_variant_trigram USING fts5(\n" +
	"\t\t\tplain, struct_key, general_key,\n" +
	"\t\t\tlocale UNINDEXED,\n" +
	"\t\t\tentry_id UNINDEXED,\n" +
	"\t\t\ttokenize='trigram'\n" +
	"\t\t);\n"

// pgFuzzyBlock is the Postgres pg_trgm GIN + tsvector fuzzy infrastructure on
// tm_variants (the equivalent of the SQLite FTS virtual tables).
const pgFuzzyBlock = "\t\tCREATE EXTENSION IF NOT EXISTS pg_trgm;\n" +
	"\t\tCREATE INDEX idx_tm_var_trgm_plain   ON tm_variants USING gin (plain gin_trgm_ops);\n" +
	"\t\tCREATE INDEX idx_tm_var_trgm_struct  ON tm_variants USING gin (struct_key gin_trgm_ops);\n" +
	"\t\tCREATE INDEX idx_tm_var_trgm_general ON tm_variants USING gin (general_key gin_trgm_ops);\n" +
	"\n" +
	"\t\tALTER TABLE tm_variants ADD COLUMN search_tsv tsvector\n" +
	"\t\t\tGENERATED ALWAYS AS (to_tsvector('simple', plain)) STORED;\n" +
	"\t\tCREATE INDEX idx_tm_var_search_tsv ON tm_variants USING gin (search_tsv);\n"

// sqliteV1Opt renders the historical v1 SQLite layout: two-tab indentation,
// IF NOT EXISTS, aligned index names, has_codes/concept_id excluded (added by
// later migrations).
var sqliteV1Opt = sq.Opt{IfNotExists: true, AlignIndexes: true, Exclude: []string{"has_codes", "concept_id"}}

// RenderTMSQLiteV1 renders the v1 SQLite TM migration body, byte-identical to
// the historical hand-written migration.
func RenderTMSQLiteV1(tokenizer string) string {
	o := sqliteV1Opt
	sections := []string{
		tmEntries.Create(sq.SQLite, o) + tmEntries.CreateIndexes(sq.SQLite, o, "idx_tm_project", "idx_tm_updated", "idx_tm_stream"),
		tmVariants.Create(sq.SQLite, o) + tmVariants.CreateIndexes(sq.SQLite, o),
		ftsSearchBlock(tokenizer),
		ftsTrigramBlock,
		tmEntryEntities.Create(sq.SQLite, o) + tmEntryEntities.CreateIndexes(sq.SQLite, o, "idx_entities_type"),
		tmEntryEntityValues.Create(sq.SQLite, o) + tmEntryEntityValues.CreateIndexes(sq.SQLite, o),
		tmImportSessions.Create(sq.SQLite, o) + tmImportSessions.CreateIndexes(sq.SQLite, o),
		tmEntryOrigins.Create(sq.SQLite, o) + tmEntryOrigins.CreateIndexes(sq.SQLite, o),
	}
	return "\n" + strings.Join(sections, "\n") + "\t\t"
}

// RenderTMSQLiteV2 renders the v2 SQLite migration: concept_id on entity maps.
// SQLite ADD COLUMN takes no IF NOT EXISTS (unlike the index).
func RenderTMSQLiteV2() string {
	return "\n" +
		tmEntryEntities.AddColumn(sq.SQLite, sq.Opt{}, "concept_id") +
		tmEntryEntities.CreateIndexes(sq.SQLite, sq.Opt{IfNotExists: true}, "idx_entities_concept") +
		"\t\t"
}

// RenderTMSQLiteV3 renders the v3 SQLite migration: has_codes facet flag.
func RenderTMSQLiteV3() string {
	o := sq.Opt{}
	return "\n" + tmEntries.AddColumn(sq.SQLite, o, "has_codes") + "\t\t"
}

// RenderTMPostgresCreate renders the fresh-install Postgres TM schema (the body
// of historical migration v4, without the leading DROP statements): all tables
// with workspace_id tenancy plus the pg_trgm/tsvector fuzzy infrastructure.
func RenderTMPostgresCreate() string {
	o := sq.Opt{AlignIndexes: true, Exclude: []string{"has_codes", "concept_id"}}
	var b strings.Builder
	b.WriteString(tmEntries.Create(sq.Postgres, o))
	b.WriteString(tmEntries.CreateIndexes(sq.Postgres, o, "idx_tm_project", "idx_tm_updated", "idx_tm_stream"))
	b.WriteString("\n")
	b.WriteString(tmVariants.Create(sq.Postgres, o))
	b.WriteString(tmVariants.CreateIndexes(sq.Postgres, o))
	b.WriteString("\n")
	b.WriteString(pgFuzzyBlock)
	b.WriteString("\n")
	b.WriteString(tmEntryEntities.Create(sq.Postgres, o))
	b.WriteString(tmEntryEntities.CreateIndexes(sq.Postgres, o, "idx_entities_type"))
	b.WriteString("\n")
	b.WriteString(tmEntryEntityValues.Create(sq.Postgres, o))
	b.WriteString(tmEntryEntityValues.CreateIndexes(sq.Postgres, o))
	b.WriteString("\n")
	b.WriteString(tmImportSessions.Create(sq.Postgres, o))
	b.WriteString(tmImportSessions.CreateIndexes(sq.Postgres, o))
	b.WriteString("\n")
	b.WriteString(tmEntryOrigins.Create(sq.Postgres, o))
	b.WriteString(tmEntryOrigins.CreateIndexes(sq.Postgres, o))
	return b.String()
}

// RenderTMPostgresConceptID renders the Postgres migration adding concept_id to
// entity maps (historical v5).
func RenderTMPostgresConceptID() string {
	o := sq.Opt{IfNotExists: true}
	return tmEntryEntities.AddColumn(sq.Postgres, o, "concept_id") +
		tmEntryEntities.CreateIndexes(sq.Postgres, o, "idx_entities_concept")
}
