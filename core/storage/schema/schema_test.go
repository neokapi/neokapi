package schema

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tenantTable exercises every dialect divergence the seam covers: a renamed
// Postgres column, a SQLite-only column, a Postgres-only column, an inline
// SQLite foreign key, a renamed index, and per-dialect index visibility.
func tenantTable() Table {
	return Table{
		Name:   "widgets",
		Tenant: true,
		Columns: []Column{
			{Name: "id", SQLite: "TEXT PRIMARY KEY", PG: "TEXT NOT NULL"},
			{Name: "owner_id", SQLite: "TEXT NOT NULL REFERENCES owners(id) ON DELETE CASCADE", PG: "TEXT NOT NULL"},
			{Name: "props", SQLite: "TEXT NOT NULL DEFAULT ''", PG: "JSONB NOT NULL DEFAULT '{}'::jsonb"},
			{Name: "created_at", PGName: "created", SQLite: "TEXT NOT NULL", PG: "TIMESTAMPTZ NOT NULL DEFAULT NOW()"},
			{Name: "has_codes", SQLite: "INTEGER NOT NULL DEFAULT 0", Comment: " -- derived on PG"},
			{Name: "tsv", PG: "tsvector"},
		},
		PGPK: []string{"id"},
		FKs:  []FK{{Cols: []string{"owner_id"}, RefTable: "owners", RefCols: []string{"id"}, SQLiteInline: true}},
		Indexes: []Index{
			{Name: "idx_widgets_owner", PGName: "idx_widgets_ws_owner", Cols: []string{"owner_id"}},
			{Name: "idx_widgets_codes", Cols: []string{"has_codes"}, SQLiteOnly: true},
			{Name: "idx_widgets_tsv", Cols: []string{"tsv"}, PGOnly: true},
		},
	}
}

// --- the tenant prefix -----------------------------------------------------
//
// Multi-tenancy is the seam's one cross-cutting rewrite: on Postgres a Tenant
// table gains a leading workspace_id column and a workspace_id prefix on its
// primary key, each foreign key (both sides), and each index. Those four have
// to agree — a workspace_id in the primary key but not in an index gives a
// lookup that silently reads across tenants, and a prefix on the local FK
// columns but not the referenced ones is a constraint that cannot be satisfied.

func TestTenantPrefixIsAppliedConsistentlyOnPostgres(t *testing.T) {
	tbl := tenantTable()
	create := tbl.Create(Postgres, Opt{})
	indexes := tbl.CreateIndexes(Postgres, Opt{})

	assert.Contains(t, create, "workspace_id TEXT NOT NULL",
		"the tenant discriminator is the leading column")
	assert.Less(t, strings.Index(create, "workspace_id"), strings.Index(create, "id "),
		"workspace_id comes first, so a composite key reads scope-then-key")
	assert.Contains(t, create, "PRIMARY KEY (workspace_id, id)")
	assert.Contains(t, create, "FOREIGN KEY (workspace_id, owner_id) REFERENCES owners(workspace_id, id)",
		"both sides of the reference carry the tenant prefix")

	for _, name := range []string{"idx_widgets_ws_owner", "idx_widgets_tsv"} {
		line := indexLine(t, indexes, name)
		assert.Contains(t, line, "(workspace_id,",
			"every Postgres index on a tenant table must lead with workspace_id, or it reads across tenants")
	}
}

func TestSQLiteNeverCarriesTheTenantColumn(t *testing.T) {
	tbl := tenantTable()
	create := tbl.Create(SQLite, Opt{})
	indexes := tbl.CreateIndexes(SQLite, Opt{})

	// SQLite databases are one file per project, so the discriminator would be
	// a constant column in every row.
	assert.NotContains(t, create, TenantColumn)
	assert.NotContains(t, indexes, TenantColumn)
	// Default width is the longest name (created_at) plus one.
	assert.Contains(t, create, "id         TEXT PRIMARY KEY", "SQLite keeps its inline primary key")
	assert.NotContains(t, create, "PRIMARY KEY (", "PGPK is a Postgres-only table-level key")
}

func TestNonTenantTableIsUnprefixedOnBothDialects(t *testing.T) {
	tbl := tenantTable()
	tbl.Tenant = false
	tbl.PK = []string{"id"}
	tbl.PGPK = nil

	for _, d := range []Dialect{SQLite, Postgres} {
		create := tbl.Create(d, Opt{})
		assert.NotContains(t, create, TenantColumn)
		assert.Contains(t, create, "PRIMARY KEY (id)")
		assert.NotContains(t, tbl.CreateIndexes(d, Opt{}), TenantColumn)
	}
}

// TestTenantPrefixDoesNotMutateTheDescriptor guards the prepend in pk, fkClause
// and CreateIndexes: each builds a new slice, so rendering Postgres first must
// not leave a workspace_id in the descriptor that SQLite then inherits.
func TestTenantPrefixDoesNotMutateTheDescriptor(t *testing.T) {
	tbl := tenantTable()

	_ = tbl.Create(Postgres, Opt{})
	_ = tbl.CreateIndexes(Postgres, Opt{})

	assert.Equal(t, []string{"owner_id"}, tbl.FKs[0].Cols, "the descriptor's FK columns are unchanged")
	assert.Equal(t, []string{"id"}, tbl.FKs[0].RefCols)
	assert.Equal(t, []string{"owner_id"}, tbl.Indexes[0].Cols)
	assert.Equal(t, []string{"id"}, tbl.PGPK)

	assert.NotContains(t, tbl.Create(SQLite, Opt{}), TenantColumn,
		"a Postgres render must not leak the tenant prefix into a later SQLite render")
	assert.NotContains(t, tbl.CreateIndexes(SQLite, Opt{}), TenantColumn)
}

// TestRenderIsIdempotent catches accidental descriptor mutation generally:
// rendering twice must produce the same bytes.
func TestRenderIsIdempotent(t *testing.T) {
	tbl := tenantTable()
	for _, d := range []Dialect{SQLite, Postgres} {
		firstCreate := tbl.Create(d, Opt{})
		firstIndexes := tbl.CreateIndexes(d, Opt{})
		assert.Equal(t, firstCreate, tbl.Create(d, Opt{}))
		assert.Equal(t, firstIndexes, tbl.CreateIndexes(d, Opt{}))
	}
}

// --- per-dialect presence --------------------------------------------------

func TestDialectOnlyColumnsAreOmittedFromTheOtherDialect(t *testing.T) {
	tbl := tenantTable()
	sqlite := tbl.Create(SQLite, Opt{})
	pg := tbl.Create(Postgres, Opt{})

	assert.Contains(t, sqlite, "has_codes", "an empty PG spec means SQLite-only")
	assert.NotContains(t, pg, "has_codes")

	assert.Contains(t, pg, "tsv", "an empty SQLite spec means Postgres-only")
	assert.NotContains(t, sqlite, "tsv")
}

func TestDialectOnlyIndexesAreFiltered(t *testing.T) {
	tbl := tenantTable()
	sqlite := tbl.CreateIndexes(SQLite, Opt{})
	pg := tbl.CreateIndexes(Postgres, Opt{})

	assert.Contains(t, sqlite, "idx_widgets_codes")
	assert.NotContains(t, pg, "idx_widgets_codes")
	assert.Contains(t, pg, "idx_widgets_tsv")
	assert.NotContains(t, sqlite, "idx_widgets_tsv")
}

func TestPGNameRenamesColumnsAndIndexes(t *testing.T) {
	tbl := tenantTable()

	assert.Contains(t, tbl.Create(SQLite, Opt{}), "created_at")
	pg := tbl.Create(Postgres, Opt{})
	assert.Contains(t, pg, "created ")
	assert.NotContains(t, pg, "created_at")

	assert.Contains(t, tbl.CreateIndexes(SQLite, Opt{}), "idx_widgets_owner ")
	assert.Contains(t, tbl.CreateIndexes(Postgres, Opt{}), "idx_widgets_ws_owner ")
}

// TestCreateIndexesSelectsByLogicalName pins that the name filter matches the
// SQLite (logical) name on both dialects: a caller naming "idx_widgets_owner"
// gets the Postgres index even though it renders as idx_widgets_ws_owner.
func TestCreateIndexesSelectsByLogicalName(t *testing.T) {
	tbl := tenantTable()

	got := tbl.CreateIndexes(Postgres, Opt{}, "idx_widgets_owner")
	assert.Contains(t, got, "idx_widgets_ws_owner")
	assert.NotContains(t, got, "idx_widgets_tsv", "only the named index is rendered")

	assert.Empty(t, tbl.CreateIndexes(Postgres, Opt{}, "idx_widgets_ws_owner"),
		"the filter takes logical names; the Postgres spelling selects nothing")
}

func TestInlineForeignKeyIsPostgresOnlyAtTableLevel(t *testing.T) {
	tbl := tenantTable()

	sqlite := tbl.Create(SQLite, Opt{})
	assert.NotContains(t, sqlite, "FOREIGN KEY",
		"SQLiteInline means the reference is already on the column spec")
	assert.Contains(t, sqlite, "REFERENCES owners(id) ON DELETE CASCADE")

	assert.Contains(t, tbl.Create(Postgres, Opt{}), "FOREIGN KEY (workspace_id, owner_id)")
}

func TestCommentsAreSQLiteOnly(t *testing.T) {
	tbl := tenantTable()
	assert.Contains(t, tbl.Create(SQLite, Opt{}), "-- derived on PG")
	// The Postgres equivalence tests tokenize statements, so a stray comment
	// would read as schema drift.
	assert.NotContains(t, tbl.Create(Postgres, Opt{}), "--")
}

// --- Opt -------------------------------------------------------------------

func TestExcludeDropsAColumnFromCreateOnly(t *testing.T) {
	tbl := tenantTable()
	create := tbl.Create(SQLite, Opt{Exclude: []string{"has_codes"}})

	assert.NotContains(t, create, "has_codes")
	assert.Contains(t, tbl.AddColumn(SQLite, Opt{}, "has_codes"), "ADD COLUMN has_codes",
		"an excluded column is the one a later migration adds")
}

func TestIfNotExists(t *testing.T) {
	tbl := tenantTable()
	o := Opt{IfNotExists: true}

	assert.Contains(t, tbl.Create(SQLite, o), "CREATE TABLE IF NOT EXISTS widgets (")
	assert.Contains(t, tbl.CreateIndexes(SQLite, o, "idx_widgets_owner"), "CREATE INDEX IF NOT EXISTS idx_widgets_owner ")
	assert.Contains(t, tbl.AddColumn(Postgres, o, "props"), "ADD COLUMN IF NOT EXISTS props ")

	assert.Contains(t, tbl.Create(SQLite, Opt{}), "CREATE TABLE widgets (")
}

func TestIndentDefaultsToTwoTabs(t *testing.T) {
	tbl := tenantTable()
	assert.True(t, strings.HasPrefix(tbl.Create(SQLite, Opt{}), "\t\tCREATE TABLE"),
		"the default matches the historical migration-literal indentation")
	assert.True(t, strings.HasPrefix(tbl.Create(SQLite, Opt{Indent: "  "}), "  CREATE TABLE"))
}

func TestSQLiteWidthPadsColumnNames(t *testing.T) {
	tbl := tenantTable()
	tbl.SQLiteWidth = 16
	create := tbl.Create(SQLite, Opt{})
	assert.Contains(t, create, "id              TEXT PRIMARY KEY", "names are padded to the fixed width")

	// A name at or beyond the width still gets one separating space.
	tbl.SQLiteWidth = 2
	assert.Contains(t, tbl.Create(SQLite, Opt{}), "id TEXT PRIMARY KEY")
}

func TestAlignIndexesPadsToTheLongestInTheGroup(t *testing.T) {
	tbl := tenantTable()

	aligned := tbl.CreateIndexes(SQLite, Opt{AlignIndexes: true})
	assert.Contains(t, aligned, "idx_widgets_owner ON", "the longest name gets a single space")
	assert.Contains(t, aligned, "idx_widgets_codes ON")

	tbl.Indexes = append(tbl.Indexes, Index{Name: "idx_widgets_much_longer_name", Cols: []string{"props"}, SQLiteOnly: true})
	aligned = tbl.CreateIndexes(SQLite, Opt{AlignIndexes: true})
	assert.Contains(t, aligned, "idx_widgets_owner            ON",
		"a longer name in the group widens every name in it")
}

// --- AddColumn -------------------------------------------------------------

func TestAddColumn(t *testing.T) {
	tbl := tenantTable()

	assert.Equal(t, "\t\tALTER TABLE widgets ADD COLUMN has_codes INTEGER NOT NULL DEFAULT 0;\n",
		tbl.AddColumn(SQLite, Opt{}, "has_codes"))
	assert.Equal(t, "\t\tALTER TABLE widgets ADD COLUMN created TIMESTAMPTZ NOT NULL DEFAULT NOW();\n",
		tbl.AddColumn(Postgres, Opt{}, "created_at"),
		"AddColumn takes the logical name and renders the dialect's spelling")
}

func TestAddColumnPanicsOnAMismatch(t *testing.T) {
	tbl := tenantTable()

	tests := []struct {
		name    string
		dialect Dialect
		column  string
	}{
		{"unknown column", SQLite, "nope"},
		{"column absent on Postgres", Postgres, "has_codes"},
		{"column absent on SQLite", SQLite, "tsv"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Descriptors and migrations are compile-time-adjacent data, so a
			// mismatch is a programming error, not a runtime condition.
			assert.Panics(t, func() { _ = tbl.AddColumn(tt.dialect, Opt{}, tt.column) })
		})
	}
}

// --- primary keys ----------------------------------------------------------

func TestPKAppliesToBothDialectsAndPGPKToPostgresOnly(t *testing.T) {
	shared := Table{
		Name:    "t",
		Columns: []Column{{Name: "a", SQLite: "TEXT", PG: "TEXT"}, {Name: "b", SQLite: "TEXT", PG: "TEXT"}},
		PK:      []string{"a", "b"},
	}
	assert.Contains(t, shared.Create(SQLite, Opt{}), "PRIMARY KEY (a, b)")
	assert.Contains(t, shared.Create(Postgres, Opt{}), "PRIMARY KEY (a, b)")

	pgOnly := shared
	pgOnly.PK = nil
	pgOnly.PGPK = []string{"a"}
	assert.NotContains(t, pgOnly.Create(SQLite, Opt{}), "PRIMARY KEY",
		"PGPK exists for tables whose SQLite key is inline on a column spec")
	assert.Contains(t, pgOnly.Create(Postgres, Opt{}), "PRIMARY KEY (a)")

	both := shared
	both.PGPK = []string{"a"}
	assert.Contains(t, both.Create(Postgres, Opt{}), "PRIMARY KEY (a, b)",
		"PK wins over PGPK; PGPK is the fallback, not an override")
}

// --- structural well-formedness -------------------------------------------
//
// The seam emits DDL as text, so a descriptor mistake shows up as a statement
// that parses but names something that is not there. These hold the rendering
// to the shape both dialects require.

func TestCreateBodyIsCommaSeparatedWithNoTrailingComma(t *testing.T) {
	tbl := tenantTable()
	for _, d := range []Dialect{SQLite, Postgres} {
		create := tbl.Create(d, Opt{})
		require.True(t, strings.HasSuffix(create, ");\n"))

		body := create[strings.Index(create, "(\n")+2 : strings.LastIndex(create, "\t\t);")]
		lines := nonEmptyLines(body)
		require.NotEmpty(t, lines)
		for i, l := range lines {
			// The SQLite FK clause is the one deliberately two-line body entry.
			if strings.HasPrefix(strings.TrimSpace(l), "REFERENCES ") {
				continue
			}
			if i < len(lines)-1 && !strings.HasPrefix(strings.TrimSpace(lines[i+1]), "REFERENCES ") {
				assert.True(t, strings.HasSuffix(strings.TrimRight(l, " "), ",") || strings.Contains(l, "--"),
					"body line %q should end in a comma", l)
			}
		}
		last := strings.TrimSpace(lines[len(lines)-1])
		assert.False(t, strings.HasSuffix(last, ","), "the final body line takes no comma: %q", last)
	}
}

// TestEveryRenderedKeyColumnExists is the check the seam does not make for
// itself: a PRIMARY KEY, FOREIGN KEY or INDEX that names a column the CREATE
// TABLE did not emit is DDL that fails on a fresh install but never on an
// already-migrated database, so it survives every environment except a new one.
func TestEveryRenderedKeyColumnExists(t *testing.T) {
	tbl := tenantTable()
	for _, d := range []Dialect{SQLite, Postgres} {
		create := tbl.Create(d, Opt{})
		declared := declaredColumns(create)

		for _, ref := range keyColumns(create) {
			assert.Contains(t, declared, ref, "PRIMARY KEY / FOREIGN KEY names an undeclared column on dialect %d", d)
		}
		for _, ref := range indexedColumns(tbl.CreateIndexes(d, Opt{})) {
			assert.Contains(t, declared, ref, "CREATE INDEX names an undeclared column on dialect %d", d)
		}
	}
}

// TestExcludingAnIndexedColumnLeavesADanglingIndex documents the one gap a
// caller has to close by hand: Exclude drops the column from CREATE TABLE but
// CreateIndexes keeps rendering indexes over it, so every call site that
// excludes a column must also narrow its index name list. memory/schema and
// terms/schema both do this — RenderMemorySQLiteV1 excludes concept_id and
// asks for "idx_entities_type" alone — and their golden tests hold them to it.
func TestExcludingAnIndexedColumnLeavesADanglingIndex(t *testing.T) {
	tbl := tenantTable()
	o := Opt{Exclude: []string{"has_codes"}}

	require.NotContains(t, tbl.Create(SQLite, o), "has_codes")
	assert.Contains(t, tbl.CreateIndexes(SQLite, o), "has_codes",
		"Exclude is not consulted by CreateIndexes; callers narrow the name list instead")

	// The safe form: name only the indexes whose columns survive the exclusion.
	assert.NotContains(t, tbl.CreateIndexes(SQLite, o, "idx_widgets_owner"), "has_codes")
}

// --- helpers ---------------------------------------------------------------

func nonEmptyLines(s string) []string {
	var out []string
	for l := range strings.SplitSeq(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

func indexLine(t *testing.T, ddl, name string) string {
	t.Helper()
	for l := range strings.SplitSeq(ddl, "\n") {
		if strings.Contains(l, name+" ") {
			return l
		}
	}
	t.Fatalf("no CREATE INDEX line for %q in:\n%s", name, ddl)
	return ""
}

var (
	pkFKRe  = regexp.MustCompile(`(?:PRIMARY KEY|FOREIGN KEY) \(([^)]*)\)`)
	idxRe   = regexp.MustCompile(`CREATE INDEX (?:IF NOT EXISTS )?\S+\s+ON \w+\(([^)]*)\)`)
	identRe = regexp.MustCompile(`^\w+`)
)

// declaredColumns pulls the column names out of a CREATE TABLE body.
func declaredColumns(create string) []string {
	var out []string
	body := create[strings.Index(create, "(\n")+2:]
	for l := range strings.SplitSeq(body, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "PRIMARY KEY") || strings.HasPrefix(l, "FOREIGN KEY") ||
			strings.HasPrefix(l, "REFERENCES") || strings.HasPrefix(l, ");") {
			continue
		}
		if m := identRe.FindString(l); m != "" {
			out = append(out, m)
		}
	}
	return out
}

func keyColumns(create string) []string {
	var out []string
	for _, m := range pkFKRe.FindAllStringSubmatch(create, -1) {
		out = append(out, splitCols(m[1])...)
	}
	return out
}

func indexedColumns(ddl string) []string {
	var out []string
	for _, m := range idxRe.FindAllStringSubmatch(ddl, -1) {
		out = append(out, splitCols(m[1])...)
	}
	return out
}

// splitCols takes a column list and drops any inline direction (e.g. "x DESC").
func splitCols(list string) []string {
	var out []string
	for c := range strings.SplitSeq(list, ",") {
		if f := strings.Fields(c); len(f) > 0 {
			out = append(out, f[0])
		}
	}
	return out
}
