package schema

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// readGolden reads a testdata golden file.
func readGolden(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read golden %s: %v", name, err)
	}
	return string(b)
}

// TestMemorySQLiteGolden asserts the rendered SQLite DDL is byte-identical to the
// historical hand-written migrations, so existing user databases need no
// schema migration. The tokenizer is pinned to "icu" (the cgo build's word
// tokenizer) to match the captured goldens deterministically.
func TestMemorySQLiteGolden(t *testing.T) {
	cases := []struct {
		name string
		got  string
	}{
		{"tm_sqlite_v1.golden.sql", RenderMemorySQLiteV1("icu")},
		{"tm_sqlite_v2.golden.sql", RenderMemorySQLiteV2()},
		{"tm_sqlite_v3.golden.sql", RenderMemorySQLiteV3()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := readGolden(t, tc.name)
			if tc.got != want {
				t.Errorf("rendered DDL differs from golden %s\n--- got ---\n%q\n--- want ---\n%q", tc.name, tc.got, want)
			}
		})
	}
}

// normalizeStatements splits a DDL body into semantic statements: each
// statement whitespace-collapsed, order-insensitive. This captures structural
// equivalence (tables, columns, constraints, indexes) while ignoring
// formatting and statement ordering.
func normalizeStatements(ddl string) []string {
	var out []string
	for stmt := range strings.SplitSeq(ddl, ";") {
		s := strings.Join(strings.Fields(stmt), " ")
		if s != "" {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// pgV4Reference is the historical Postgres v4 create body (post-DROP): the
// known-good fresh-install schema the descriptors must reproduce semantically.
const pgV4Reference = `
	CREATE TABLE tm_entries (
		workspace_id    TEXT NOT NULL,
		id              TEXT NOT NULL,
		project_id      TEXT NOT NULL DEFAULT '',
		stream          TEXT NOT NULL DEFAULT '',
		hint_src_lang   TEXT NOT NULL DEFAULT '',
		properties      JSONB NOT NULL DEFAULT '{}'::jsonb,
		note            TEXT NOT NULL DEFAULT '',
		created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		PRIMARY KEY (workspace_id, id)
	);
	CREATE INDEX idx_tm_ws_project ON tm_entries(workspace_id, project_id);
	CREATE INDEX idx_tm_ws_stream  ON tm_entries(workspace_id, stream);
	CREATE INDEX idx_tm_ws_updated ON tm_entries(workspace_id, updated_at DESC);

	CREATE TABLE tm_variants (
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
	CREATE INDEX idx_tm_var_ws_locale      ON tm_variants(workspace_id, locale);
	CREATE INDEX idx_tm_var_plain_loc      ON tm_variants(workspace_id, plain, locale);
	CREATE INDEX idx_tm_var_struct_loc     ON tm_variants(workspace_id, struct_key, locale);
	CREATE INDEX idx_tm_var_general_loc    ON tm_variants(workspace_id, general_key, locale);

	CREATE EXTENSION IF NOT EXISTS pg_trgm;
	CREATE INDEX idx_tm_var_trgm_plain   ON tm_variants USING gin (plain gin_trgm_ops);
	CREATE INDEX idx_tm_var_trgm_struct  ON tm_variants USING gin (struct_key gin_trgm_ops);
	CREATE INDEX idx_tm_var_trgm_general ON tm_variants USING gin (general_key gin_trgm_ops);

	ALTER TABLE tm_variants ADD COLUMN search_tsv tsvector
		GENERATED ALWAYS AS (to_tsvector('simple', plain)) STORED;
	CREATE INDEX idx_tm_var_search_tsv ON tm_variants USING gin (search_tsv);

	CREATE TABLE tm_entry_entities (
		workspace_id   TEXT NOT NULL,
		entry_id       TEXT NOT NULL,
		placeholder_id TEXT NOT NULL,
		entity_type    TEXT NOT NULL,
		PRIMARY KEY (workspace_id, entry_id, placeholder_id),
		FOREIGN KEY (workspace_id, entry_id) REFERENCES tm_entries(workspace_id, id) ON DELETE CASCADE
	);
	CREATE INDEX idx_tm_entities_type ON tm_entry_entities(workspace_id, entity_type);

	CREATE TABLE tm_entry_entity_values (
		workspace_id   TEXT NOT NULL,
		entry_id       TEXT NOT NULL,
		placeholder_id TEXT NOT NULL,
		locale         TEXT NOT NULL,
		text_value     TEXT NOT NULL DEFAULT '',
		start_pos      INTEGER NOT NULL DEFAULT 0,
		end_pos        INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (workspace_id, entry_id, placeholder_id, locale),
		FOREIGN KEY (workspace_id, entry_id, placeholder_id)
			REFERENCES tm_entry_entities(workspace_id, entry_id, placeholder_id) ON DELETE CASCADE
	);
	CREATE INDEX idx_tm_entity_values_text ON tm_entry_entity_values(workspace_id, text_value, locale);

	CREATE TABLE tm_import_sessions (
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
	CREATE INDEX idx_tm_sessions_hash ON tm_import_sessions(workspace_id, file_hash);
	CREATE INDEX idx_tm_sessions_time ON tm_import_sessions(workspace_id, imported_at DESC);

	CREATE TABLE tm_entry_origins (
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
	CREATE INDEX idx_tm_origin_source  ON tm_entry_origins(workspace_id, source);
	CREATE INDEX idx_tm_origin_key     ON tm_entry_origins(workspace_id, key);
	CREATE INDEX idx_tm_origin_session ON tm_entry_origins(workspace_id, session_id);
`

const pgV5Reference = `
	ALTER TABLE tm_entry_entities ADD COLUMN IF NOT EXISTS concept_id TEXT NOT NULL DEFAULT '';
	CREATE INDEX IF NOT EXISTS idx_tm_entities_concept ON tm_entry_entities(workspace_id, concept_id);
`

// TestMemoryPostgresSemanticEquivalence asserts the descriptor-rendered Postgres
// DDL matches the historical known-good v4/v5 migrations as a set of
// whitespace-collapsed statements — a fresh install produces the same schema.
func TestMemoryPostgresSemanticEquivalence(t *testing.T) {
	assertStmtSetEqual(t, "v4 create", pgV4Reference, RenderMemoryPostgresCreate())
	assertStmtSetEqual(t, "v5 concept_id", pgV5Reference, RenderMemoryPostgresConceptID())
}

func assertStmtSetEqual(t *testing.T, name, want, got string) {
	t.Helper()
	w := normalizeStatements(want)
	g := normalizeStatements(got)
	if len(w) != len(g) {
		t.Errorf("%s: statement count differs: want %d, got %d\nwant=%v\ngot=%v", name, len(w), len(g), w, g)
		return
	}
	for i := range w {
		if w[i] != g[i] {
			t.Errorf("%s: statement %d differs:\nwant: %s\ngot:  %s", name, i, w[i], g[i])
		}
	}
}
