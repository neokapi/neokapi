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

// TestTermsSQLiteGolden asserts the rendered SQLite DDL is byte-identical to the
// historical hand-written terms migrations, so existing user databases need
// no schema migration.
func TestTermsSQLiteGolden(t *testing.T) {
	cases := []struct {
		name string
		got  string
	}{
		{"tb_sqlite_v1.golden.sql", RenderTermsSQLiteV1()},
		{"tb_sqlite_v2.golden.sql", RenderTermsSQLiteV2()},
		{"tb_sqlite_v3.golden.sql", RenderTermsSQLiteV3()},
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

// splitTopLevel splits s on commas that sit at paren depth 0.
func splitTopLevel(s string) []string {
	var parts []string
	depth, start := 0, 0
	for i, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// canonicalizeCreateTable sorts the top-level column/constraint items inside a
// whitespace-collapsed CREATE TABLE statement, so column order (which is
// cosmetic — queries reference columns by name) does not register as drift.
func canonicalizeCreateTable(stmt string) string {
	if !strings.HasPrefix(stmt, "CREATE TABLE") {
		return stmt
	}
	open := strings.Index(stmt, "(")
	closeIdx := strings.LastIndex(stmt, ")")
	if open < 0 || closeIdx < 0 || closeIdx < open {
		return stmt
	}
	head := strings.TrimSpace(stmt[:open])
	items := splitTopLevel(stmt[open+1 : closeIdx])
	for i := range items {
		items[i] = strings.TrimSpace(items[i])
	}
	sort.Strings(items)
	return head + " ( " + strings.Join(items, ", ") + " )"
}

// normalizeStatements splits a DDL body into semantic statements: each
// statement whitespace-collapsed, CREATE TABLE column order canonicalized, and
// the whole set order-insensitive. This captures structural equivalence
// (tables, columns, constraints, indexes) while ignoring formatting, column
// order, and statement ordering.
func normalizeStatements(ddl string) []string {
	var out []string
	for stmt := range strings.SplitSeq(ddl, ";") {
		s := strings.Join(strings.Fields(stmt), " ")
		if s == "" {
			continue
		}
		out = append(out, canonicalizeCreateTable(s))
	}
	sort.Strings(out)
	return out
}

// The historical known-good Postgres terms migrations (bowrain/terms),
// retyped here as the reference the descriptors must reproduce. A fresh install
// applying the descriptor-rendered migrations produces the same schema.
const (
	pgV1Reference = `
		CREATE TABLE IF NOT EXISTS tb_concepts (
			id           TEXT NOT NULL,
			workspace_id TEXT NOT NULL,
			domain       TEXT NOT NULL DEFAULT '',
			definition   TEXT NOT NULL DEFAULT '',
			properties   TEXT,
			created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (workspace_id, id)
		);
		CREATE TABLE IF NOT EXISTS tb_terms (
			id            SERIAL PRIMARY KEY,
			workspace_id  TEXT NOT NULL,
			concept_id    TEXT NOT NULL,
			text          TEXT NOT NULL,
			text_lower    TEXT NOT NULL,
			locale        TEXT NOT NULL,
			status        TEXT NOT NULL DEFAULT 'approved',
			part_of_speech TEXT NOT NULL DEFAULT '',
			gender        TEXT NOT NULL DEFAULT '',
			note          TEXT NOT NULL DEFAULT '',
			FOREIGN KEY (workspace_id, concept_id) REFERENCES tb_concepts(workspace_id, id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_tb_terms_ws_concept ON tb_terms(workspace_id, concept_id);
		CREATE INDEX IF NOT EXISTS idx_tb_terms_ws_locale ON tb_terms(workspace_id, locale);
		CREATE INDEX IF NOT EXISTS idx_tb_terms_ws_text ON tb_terms(workspace_id, text_lower, locale);
	`

	pgV2Reference = `
		ALTER TABLE tb_concepts ADD COLUMN stream TEXT NOT NULL DEFAULT '';
		CREATE INDEX IF NOT EXISTS idx_tb_concepts_ws_stream ON tb_concepts(workspace_id, stream);
	`

	pgV3Reference = `
		CREATE EXTENSION IF NOT EXISTS pg_trgm;
		CREATE INDEX IF NOT EXISTS idx_tb_terms_trgm ON tb_terms USING gin (text_lower gin_trgm_ops);
		ALTER TABLE tb_terms ADD COLUMN search_tsv tsvector;
		UPDATE tb_terms SET search_tsv = to_tsvector('simple', text_lower);
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

	pgV4Reference = `
		ALTER TABLE tb_concepts ADD COLUMN source TEXT NOT NULL DEFAULT 'terminology';
		ALTER TABLE tb_terms ADD COLUMN competitor_term BOOLEAN NOT NULL DEFAULT FALSE;
		ALTER TABLE tb_terms ADD COLUMN valid_from TIMESTAMPTZ;
		ALTER TABLE tb_terms ADD COLUMN valid_to TIMESTAMPTZ;
		ALTER TABLE tb_terms ADD COLUMN tags JSONB NOT NULL DEFAULT '{}';
		CREATE TABLE IF NOT EXISTS tb_relations (
			id           TEXT NOT NULL,
			workspace_id TEXT NOT NULL,
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
	`
)

// TestTermsPostgresSemanticEquivalence asserts the descriptor-rendered Postgres DDL
// matches the historical known-good v1-v4 migrations as a set of
// whitespace-collapsed, column-order-insensitive statements — a fresh install
// produces the same schema.
func TestTermsPostgresSemanticEquivalence(t *testing.T) {
	assertStmtSetEqual(t, "v1 create", pgV1Reference, RenderTermsPostgresV1("workspace_id"))
	assertStmtSetEqual(t, "v2 stream", pgV2Reference, RenderTermsPostgresV2("workspace_id"))
	assertStmtSetEqual(t, "v3 fuzzy", pgV3Reference, RenderTermsPostgresV3())
	assertStmtSetEqual(t, "v4 brand graph", pgV4Reference, RenderTermsPostgresV4("workspace_id"))
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
