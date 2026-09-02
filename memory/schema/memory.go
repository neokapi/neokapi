// Package schema declares the content-memory and (via tb.go) terminology
// table layouts once, as data, and renders them to SQLite or PostgreSQL DDL
// through the dialect seam in core/storage/schema. The two content memory backends —
// framework SQLite (memory) and bowrain Postgres — both build their migration
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

// content memory table descriptors. Column specs are written side by side per dialect so
// type drift is visible at the definition site. Partitioned tables gain a
// leading partition-key column and a partition-key-prefixed PK/FK on Postgres only.
var (
	memoryEntries = sq.Table{
		Name:        "tm_entries",
		Partitioned: true,
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
			// point is added by a later migration (v4 SQLite, v7 Postgres): the
			// context point an answer was approved at. A project resolves it
			// from its recipe and the server renders it from the collection a
			// decision belongs to, so both backends store it and a chain
			// narrows to one place on either.
			{Name: "point", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "TEXT NOT NULL DEFAULT ''"},
			// unit is added by a later migration (v5 SQLite, v7 Postgres): the durable block
			// identity this answer was approved for (model.Block.Unit), and the
			// only thing that links successive approvals of one block into a
			// version chain. The corpus already holds every version — a changed
			// source writes a new entry beside the old rather than replacing it
			// — but keyed by text, nothing said the two were the same block.
			//
			// A unit is resolved by reconciliation over a project's own content
			// and travels the sync protocol to the server, so both backends
			// store it and the same chain is answerable wherever the corpus
			// lives.
			{Name: "unit", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "TEXT NOT NULL DEFAULT ''"},
		},
		PGPK: []string{"id"},
		Indexes: []sq.Index{
			{Name: "idx_tm_project", PGName: "idx_tm_ws_project", Cols: []string{"project_id"}},
			{Name: "idx_tm_updated", PGName: "idx_tm_ws_updated", Cols: []string{"updated_at DESC"}},
			{Name: "idx_tm_stream", PGName: "idx_tm_ws_stream", Cols: []string{"stream"}},
			// The chain lookup's index. (unit, point) in that order because the
			// question is always "this block, near here": a unit alone spans
			// every point the block has ever sat at.
			{Name: "idx_tm_unit", PGName: "idx_tm_ws_unit", Cols: []string{"unit", "point"}},
		},
	}

	memoryVariants = sq.Table{
		Name:        "tm_variants",
		Partitioned: true,
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

	memoryEntryEntities = sq.Table{
		Name:        "tm_entry_entities",
		Partitioned: true,
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

	memoryEntryEntityValues = sq.Table{
		Name:        "tm_entry_entity_values",
		Partitioned: true,
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

	memoryImportSessions = sq.Table{
		Name:        "tm_import_sessions",
		Partitioned: true,
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

	memoryEntryOrigins = sq.Table{
		Name:        "tm_entry_origins",
		Partitioned: true,
		Columns: []sq.Column{
			{Name: "entry_id", SQLite: "TEXT NOT NULL REFERENCES tm_entries(id) ON DELETE CASCADE", PG: "TEXT NOT NULL"},
			{Name: "ordinal", SQLite: "INTEGER NOT NULL", PG: "INTEGER NOT NULL"},
			{Name: "source", SQLite: "TEXT NOT NULL", PG: "TEXT NOT NULL"},
			{Name: "key", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "TEXT NOT NULL DEFAULT ''"},
			{Name: "reference", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "TEXT NOT NULL DEFAULT ''"},
			{Name: "added_at", SQLite: "TEXT NOT NULL", PG: "TIMESTAMPTZ NOT NULL DEFAULT NOW()"},
			{Name: "added_by", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "TEXT NOT NULL DEFAULT ''"},
			{Name: "session_id", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "TEXT NOT NULL DEFAULT ''"},
			// context_fp is added by a later migration (v5 SQLite, v7
			// Postgres): the governing context in force when this answer was
			// produced, as model.Origin.ContextFingerprint records it on the
			// target.
			//
			// The corpus needs it because an answer is only reusable under the
			// rules it was approved under. Without it a prior version can be
			// retrieved but not judged, and a caller offering one as reference
			// would anchor on wording the rules in force may since have
			// rejected. Per origin rather than per entry, so re-absorbing under
			// moved governance appends rather than overwrites.
			{Name: "context_fp", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "TEXT NOT NULL DEFAULT ''"},
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

// pgFuzzyBaselineBlock is pgFuzzyBlock made replayable, for the consolidated
// baseline.
//
// DERIVED from the historical block rather than written out again: the
// historical renderers are a record of how the schema was built and must keep
// producing exactly what they always produced (pinned by the
// semantic-equivalence test in this package), so the record is left alone and
// the baseline states its difference from it in one place.
var pgFuzzyBaselineBlock = strings.NewReplacer(
	"CREATE INDEX idx_tm_var_", "CREATE INDEX IF NOT EXISTS idx_tm_var_",
	"ADD COLUMN search_tsv tsvector", "ADD COLUMN IF NOT EXISTS search_tsv tsvector",

	// An extension belongs to a database but is INSTALLED INTO one schema, and
	// an unqualified CREATE EXTENSION picks whatever search_path points at.
	// Name the schema, so where gin_trgm_ops ends up does not depend on the
	// session that happened to run the migration first: IF NOT EXISTS makes the
	// second caller a no-op, and a caller whose search_path led somewhere else
	// would then find no operator class at all. See pg_trgm's placement being
	// pinned in the schema tests, and the two-live-schemas regression test in
	// bowrain/memory.
	"CREATE EXTENSION IF NOT EXISTS pg_trgm;",
	"CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;",
).Replace(pgFuzzyBlock)

// sqliteV1Opt renders the historical v1 SQLite layout: two-tab indentation,
// IF NOT EXISTS, aligned index names, has_codes/concept_id excluded (added by
// later migrations).
var sqliteV1Opt = sq.Opt{IfNotExists: true, AlignIndexes: true, Exclude: []string{"has_codes", "concept_id", "point", "unit", "context_fp"}}

// RenderMemorySQLiteV1 renders the v1 SQLite content memory migration body, byte-identical to
// the historical hand-written migration.
func RenderMemorySQLiteV1(tokenizer string) string {
	o := sqliteV1Opt
	sections := []string{
		memoryEntries.Create(sq.SQLite, o) + memoryEntries.CreateIndexes(sq.SQLite, o, "idx_tm_project", "idx_tm_updated", "idx_tm_stream"),
		memoryVariants.Create(sq.SQLite, o) + memoryVariants.CreateIndexes(sq.SQLite, o),
		ftsSearchBlock(tokenizer),
		ftsTrigramBlock,
		memoryEntryEntities.Create(sq.SQLite, o) + memoryEntryEntities.CreateIndexes(sq.SQLite, o, "idx_entities_type"),
		memoryEntryEntityValues.Create(sq.SQLite, o) + memoryEntryEntityValues.CreateIndexes(sq.SQLite, o),
		memoryImportSessions.Create(sq.SQLite, o) + memoryImportSessions.CreateIndexes(sq.SQLite, o),
		memoryEntryOrigins.Create(sq.SQLite, o) + memoryEntryOrigins.CreateIndexes(sq.SQLite, o),
	}
	return "\n" + strings.Join(sections, "\n") + "\t\t"
}

// RenderMemorySQLiteV2 renders the v2 SQLite migration: concept_id on entity maps.
// SQLite ADD COLUMN takes no IF NOT EXISTS (unlike the index).
func RenderMemorySQLiteV2() string {
	return "\n" +
		memoryEntryEntities.AddColumn(sq.SQLite, sq.Opt{}, "concept_id") +
		memoryEntryEntities.CreateIndexes(sq.SQLite, sq.Opt{IfNotExists: true}, "idx_entities_concept") +
		"\t\t"
}

// RenderMemorySQLiteV3 renders the v3 SQLite migration: has_codes facet flag.
func RenderMemorySQLiteV3() string {
	o := sq.Opt{}
	return "\n" + memoryEntries.AddColumn(sq.SQLite, o, "has_codes") + "\t\t"
}

// RenderMemorySQLiteV4 renders the v4 SQLite migration: the context point an
// entry's answer was approved at.
func RenderMemorySQLiteV4() string {
	o := sq.Opt{}
	return "\n" + memoryEntries.AddColumn(sq.SQLite, o, "point") + "\t\t"
}

// RenderMemorySQLiteV5 renders the v5 SQLite migration: the durable block
// identity an answer was approved for, and the index the version lookup walks.
//
// The index is on (unit, point) in that order because the question is always
// "this block, near here" — a unit alone is the whole chain across every point
// it has ever sat at, which is never what a caller wants.
func RenderMemorySQLiteV5() string {
	o := sq.Opt{}
	return "\n" + memoryEntries.AddColumn(sq.SQLite, o, "unit") +
		memoryEntries.CreateIndexes(sq.SQLite, sq.Opt{IfNotExists: true}, "idx_tm_unit") +
		memoryEntryOrigins.AddColumn(sq.SQLite, o, "context_fp") + "\t\t"
}

// RenderMemoryPostgresCreate renders the fresh-install Postgres content memory schema (the body
// of historical migration v4, without the leading DROP statements): all tables
// partitioned by the given tenant column plus the pg_trgm/tsvector fuzzy
// infrastructure.
func RenderMemoryPostgresCreate(tenantColumn string) string {
	o := sq.Opt{TenantColumn: tenantColumn, AlignIndexes: true, Exclude: []string{"has_codes", "concept_id", "point", "unit", "context_fp"}}
	var b strings.Builder
	b.WriteString(memoryEntries.Create(sq.Postgres, o))
	b.WriteString(memoryEntries.CreateIndexes(sq.Postgres, o, "idx_tm_project", "idx_tm_updated", "idx_tm_stream"))
	b.WriteString("\n")
	b.WriteString(memoryVariants.Create(sq.Postgres, o))
	b.WriteString(memoryVariants.CreateIndexes(sq.Postgres, o))
	b.WriteString("\n")
	b.WriteString(pgFuzzyBlock)
	b.WriteString("\n")
	b.WriteString(memoryEntryEntities.Create(sq.Postgres, o))
	b.WriteString(memoryEntryEntities.CreateIndexes(sq.Postgres, o, "idx_entities_type"))
	b.WriteString("\n")
	b.WriteString(memoryEntryEntityValues.Create(sq.Postgres, o))
	b.WriteString(memoryEntryEntityValues.CreateIndexes(sq.Postgres, o))
	b.WriteString("\n")
	b.WriteString(memoryImportSessions.Create(sq.Postgres, o))
	b.WriteString(memoryImportSessions.CreateIndexes(sq.Postgres, o))
	b.WriteString("\n")
	b.WriteString(memoryEntryOrigins.Create(sq.Postgres, o))
	b.WriteString(memoryEntryOrigins.CreateIndexes(sq.Postgres, o))
	return b.String()
}

// RenderMemoryPostgresBaseline renders the whole current Postgres content
// memory schema in one pass: every column present from the start, so nothing is
// added by a later ALTER.
//
// This is what the consolidated tm_schema_migrations baseline applies. Two
// differences from RenderMemoryPostgresCreate are deliberate:
//
//   - concept_id is NOT excluded. Historical v5 added it by ALTER; a baseline
//     creates the table with it.
//   - has_codes stays excluded. It is a SQLite-only column — no Postgres
//     migration has ever added it, so including it here would invent a column
//     production does not have.
//
// The leading DROP TABLE statements of historical v4 are deliberately absent.
// They existed to clear the legacy bilingual tables during that one upgrade,
// and a baseline is re-applied by design — carrying them would mean every
// migrate pass silently destroyed the content memory it was meant to preserve.
func RenderMemoryPostgresBaseline(tenantColumn string) string {
	o := sq.Opt{TenantColumn: tenantColumn, AlignIndexes: true, IfNotExists: true, Exclude: []string{"has_codes", "point", "unit", "context_fp"}}
	var b strings.Builder
	b.WriteString(memoryEntries.Create(sq.Postgres, o))
	b.WriteString(memoryEntries.CreateIndexes(sq.Postgres, o, "idx_tm_project", "idx_tm_updated", "idx_tm_stream"))
	b.WriteString("\n")
	b.WriteString(memoryVariants.Create(sq.Postgres, o))
	b.WriteString(memoryVariants.CreateIndexes(sq.Postgres, o))
	b.WriteString("\n")
	b.WriteString(pgFuzzyBaselineBlock)
	b.WriteString("\n")
	b.WriteString(memoryEntryEntities.Create(sq.Postgres, o))
	b.WriteString(memoryEntryEntities.CreateIndexes(sq.Postgres, o, "idx_entities_type", "idx_entities_concept"))
	b.WriteString("\n")
	b.WriteString(memoryEntryEntityValues.Create(sq.Postgres, o))
	b.WriteString(memoryEntryEntityValues.CreateIndexes(sq.Postgres, o))
	b.WriteString("\n")
	b.WriteString(memoryImportSessions.Create(sq.Postgres, o))
	b.WriteString(memoryImportSessions.CreateIndexes(sq.Postgres, o))
	b.WriteString("\n")
	b.WriteString(memoryEntryOrigins.Create(sq.Postgres, o))
	b.WriteString(memoryEntryOrigins.CreateIndexes(sq.Postgres, o))
	return b.String()
}

// RenderMemoryPostgresConceptID renders the Postgres migration adding concept_id to
// entity maps (historical v5).
func RenderMemoryPostgresConceptID(tenantColumn string) string {
	o := sq.Opt{TenantColumn: tenantColumn, IfNotExists: true}
	return memoryEntryEntities.AddColumn(sq.Postgres, o, "concept_id") +
		memoryEntryEntities.CreateIndexes(sq.Postgres, o, "idx_entities_concept")
}

// RenderMemoryPostgresVersionChain renders the Postgres migration that lets the
// server corpus answer a version chain: the block an answer was approved for,
// the point it was approved at, the governing context each origin recorded, and
// the index the chain lookup walks.
//
// ALTER rather than a wider baseline, because the baseline sits below the
// version every live database has already recorded and so never runs again
// there. A column folded into it would reach a fresh database and no other.
func RenderMemoryPostgresVersionChain(tenantColumn string) string {
	o := sq.Opt{TenantColumn: tenantColumn, IfNotExists: true}
	return memoryEntries.AddColumn(sq.Postgres, o, "point") +
		memoryEntries.AddColumn(sq.Postgres, o, "unit") +
		memoryEntryOrigins.AddColumn(sq.Postgres, o, "context_fp") +
		memoryEntries.CreateIndexes(sq.Postgres, o, "idx_tm_unit")
}
