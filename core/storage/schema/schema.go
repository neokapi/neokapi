// Package schema is a minimal dialect seam for storage schemas that ship in
// two SQL dialects: logical tables are described once as Go data and rendered
// to SQLite or PostgreSQL DDL. It is deliberately not an ORM or a query
// builder — it renders CREATE TABLE / CREATE INDEX / ADD COLUMN statements
// and nothing else.
//
// The seam covers exactly the divergences the TM (sievepen) and termbase
// schemas need:
//
//   - per-dialect column types and constraints, written side by side on one
//     Column so type drift between backends is visible at the definition site
//     (TEXT timestamps vs TIMESTAMPTZ, TEXT vs JSONB, INTEGER vs BIGINT);
//   - optional multi-tenancy: tables marked Tenant gain a leading
//     workspace_id column plus composite PRIMARY KEY / FOREIGN KEY prefixes
//     on Postgres, while SQLite (one database file per project) omits it;
//   - dialect-only columns and indexes (e.g. SQLite's has_codes flag, which
//     Postgres derives at query time);
//   - formatting knobs so the rendered SQLite DDL stays byte-identical to
//     the historical hand-written migrations (golden-tested in
//     sievepen/schema and termbase/schema).
//
// Fuzzy-match infrastructure is inherently dialect-specific (FTS5 virtual
// tables + triggers vs pg_trgm GIN indexes + tsvector columns) and stays as
// literal per-dialect blocks next to the table descriptors, not here.
package schema

import (
	"slices"
	"strings"
)

// Dialect selects the SQL dialect DDL is rendered for.
type Dialect int

const (
	// SQLite is the framework's embedded backend dialect.
	SQLite Dialect = iota
	// Postgres is bowrain's server backend dialect.
	Postgres
)

// TenantColumn is the multi-tenancy discriminator every Tenant table carries
// on Postgres. SQLite databases are per-project files and never have it.
const TenantColumn = "workspace_id"

// tenantSpec is the column spec rendered for TenantColumn.
const tenantSpec = "TEXT NOT NULL"

// Column describes one logical column with its per-dialect spelling.
type Column struct {
	Name    string // logical (and SQLite) column name
	PGName  string // Postgres column name when it differs (empty = Name)
	SQLite  string // SQLite type + constraints; empty = column absent on SQLite
	PG      string // Postgres type + constraints; empty = column absent on Postgres
	Comment string // verbatim SQLite trailing comment, appended after the comma
}

func (c Column) name(d Dialect) string {
	if d == Postgres && c.PGName != "" {
		return c.PGName
	}
	return c.Name
}

func (c Column) spec(d Dialect) string {
	if d == Postgres {
		return c.PG
	}
	return c.SQLite
}

// FK is a table-level foreign key. On Postgres, Tenant tables prepend
// workspace_id to both the local and referenced column lists. When
// SQLiteInline is set, the SQLite side already expresses the reference
// inline on the column spec and the table-level clause is Postgres-only.
type FK struct {
	Cols         []string
	RefTable     string
	RefCols      []string
	SQLiteInline bool
}

// Index describes one secondary index. Postgres names may differ from the
// historical SQLite names; Tenant tables prepend workspace_id to the key.
type Index struct {
	Name       string   // SQLite index name
	PGName     string   // Postgres index name when it differs (empty = Name)
	Cols       []string // may carry an inline direction, e.g. "updated_at DESC"
	SQLiteOnly bool
	PGOnly     bool
}

func (ix Index) name(d Dialect) string {
	if d == Postgres && ix.PGName != "" {
		return ix.PGName
	}
	return ix.Name
}

func (ix Index) in(d Dialect) bool {
	switch d {
	case SQLite:
		return !ix.PGOnly
	case Postgres:
		return !ix.SQLiteOnly
	}
	return false
}

// Table describes one logical table.
type Table struct {
	Name string
	// Tenant marks the table as workspace-scoped on Postgres: a leading
	// workspace_id column, and workspace_id prefixes on the composite
	// PRIMARY KEY and every FOREIGN KEY.
	Tenant  bool
	Columns []Column
	// PK is a composite primary key rendered on both dialects.
	PK []string
	// PGPK is a primary key rendered on Postgres only, for tables whose
	// SQLite definition carries PRIMARY KEY inline on a column spec.
	PGPK    []string
	FKs     []FK
	Indexes []Index
	// SQLiteWidth pads SQLite column names to this width (0 = longest
	// name + 1). Names at or beyond the width get a single space.
	SQLiteWidth int
}

// Opt controls rendering.
type Opt struct {
	// Indent is the statement indentation prefix (default two tabs, the
	// historical migration-literal indentation).
	Indent string
	// IfNotExists emits IF NOT EXISTS (CREATE TABLE / CREATE INDEX /
	// ADD COLUMN, as supported by the dialect).
	IfNotExists bool
	// AlignIndexes pads index names to the longest in the rendered group.
	AlignIndexes bool
	// Exclude names logical columns to leave out — columns that a later
	// migration adds via AddColumn.
	Exclude []string
}

func (o Opt) withDefaults() Opt {
	if o.Indent == "" {
		o.Indent = "\t\t"
	}
	return o
}

func (o Opt) excluded(name string) bool {
	return slices.Contains(o.Exclude, name)
}

// column looks up a column by logical name.
func (t Table) column(name string) (Column, bool) {
	for _, c := range t.Columns {
		if c.Name == name {
			return c, true
		}
	}
	return Column{}, false
}

// cols returns the columns rendered for the dialect, honoring Exclude and
// per-dialect absence, with the tenant column prepended on Postgres.
func (t Table) cols(d Dialect, o Opt) []Column {
	var out []Column
	if d == Postgres && t.Tenant {
		out = append(out, Column{Name: TenantColumn, PG: tenantSpec})
	}
	for _, c := range t.Columns {
		if c.spec(d) == "" || o.excluded(c.Name) {
			continue
		}
		out = append(out, c)
	}
	return out
}

func (t Table) colWidth(d Dialect, cols []Column) int {
	if d == SQLite && t.SQLiteWidth > 0 {
		return t.SQLiteWidth
	}
	w := 0
	for _, c := range cols {
		if n := len(c.name(d)); n > w {
			w = n
		}
	}
	return w + 1
}

func pad(name string, width int) string {
	if len(name) >= width {
		return name + " "
	}
	return name + strings.Repeat(" ", width-len(name))
}

// pk returns the table-level primary-key column list for the dialect
// (nil when the dialect has no table-level PK).
func (t Table) pk(d Dialect) []string {
	switch d {
	case SQLite:
		return t.PK
	case Postgres:
		pk := t.PK
		if len(pk) == 0 {
			pk = t.PGPK
		}
		if len(pk) == 0 {
			return nil
		}
		if t.Tenant {
			return append([]string{TenantColumn}, pk...)
		}
		return pk
	}
	return nil
}

// Create renders the CREATE TABLE statement, ending with ");\n".
func (t Table) Create(d Dialect, o Opt) string {
	o = o.withDefaults()

	type bodyLine struct {
		text    string
		comment string
	}
	var lines []bodyLine

	cols := t.cols(d, o)
	width := t.colWidth(d, cols)
	for _, c := range cols {
		lines = append(lines, bodyLine{pad(c.name(d), width) + c.spec(d), c.Comment})
	}
	if pk := t.pk(d); len(pk) > 0 {
		lines = append(lines, bodyLine{"PRIMARY KEY (" + strings.Join(pk, ", ") + ")", ""})
	}
	for _, fk := range t.FKs {
		if d == SQLite && fk.SQLiteInline {
			continue
		}
		lines = append(lines, bodyLine{t.fkClause(d, fk, o), ""})
	}

	var b strings.Builder
	b.WriteString(o.Indent)
	b.WriteString("CREATE TABLE ")
	if o.IfNotExists {
		b.WriteString("IF NOT EXISTS ")
	}
	b.WriteString(t.Name)
	b.WriteString(" (\n")
	for i, l := range lines {
		b.WriteString(o.Indent)
		b.WriteString("\t")
		b.WriteString(l.text)
		if i < len(lines)-1 {
			b.WriteString(",")
		}
		b.WriteString(l.comment)
		b.WriteString("\n")
	}
	b.WriteString(o.Indent)
	b.WriteString(");\n")
	return b.String()
}

// fkClause renders one FOREIGN KEY clause. SQLite uses the historical
// two-line layout; Postgres renders a single line with the tenant prefix.
func (t Table) fkClause(d Dialect, fk FK, o Opt) string {
	cols, refCols := fk.Cols, fk.RefCols
	if d == Postgres && t.Tenant {
		cols = append([]string{TenantColumn}, cols...)
		refCols = append([]string{TenantColumn}, refCols...)
	}
	ref := "REFERENCES " + fk.RefTable + "(" + strings.Join(refCols, ", ") + ") ON DELETE CASCADE"
	head := "FOREIGN KEY (" + strings.Join(cols, ", ") + ")"
	if d == SQLite {
		return head + "\n" + o.Indent + "\t\t" + ref
	}
	return head + " " + ref
}

// CreateIndexes renders CREATE INDEX statements for the named indexes
// (all of the dialect's indexes when names is empty), one per line.
func (t Table) CreateIndexes(d Dialect, o Opt, names ...string) string {
	o = o.withDefaults()

	var group []Index
	for _, ix := range t.Indexes {
		if !ix.in(d) {
			continue
		}
		if len(names) > 0 && !slices.Contains(names, ix.Name) {
			continue
		}
		group = append(group, ix)
	}

	width := 0
	if o.AlignIndexes {
		for _, ix := range group {
			if n := len(ix.name(d)); n > width {
				width = n
			}
		}
		width++
	}

	var b strings.Builder
	for _, ix := range group {
		cols := ix.Cols
		if d == Postgres && t.Tenant {
			cols = append([]string{TenantColumn}, cols...)
		}
		name := ix.name(d)
		if o.AlignIndexes {
			name = pad(name, width)
		} else {
			name += " "
		}
		b.WriteString(o.Indent)
		b.WriteString("CREATE INDEX ")
		if o.IfNotExists {
			b.WriteString("IF NOT EXISTS ")
		}
		b.WriteString(name)
		b.WriteString("ON ")
		b.WriteString(t.Name)
		b.WriteString("(")
		b.WriteString(strings.Join(cols, ", "))
		b.WriteString(");\n")
	}
	return b.String()
}

// AddColumn renders the ALTER TABLE … ADD COLUMN statement for the named
// logical column. It panics when the column does not exist or is absent in
// the dialect — descriptors and migrations are compile-time-adjacent data,
// so a mismatch is a programming error surfaced by the golden tests.
func (t Table) AddColumn(d Dialect, o Opt, name string) string {
	o = o.withDefaults()
	c, ok := t.column(name)
	if !ok || c.spec(d) == "" {
		panic("schema: table " + t.Name + " has no " + name + " column for this dialect")
	}
	var b strings.Builder
	b.WriteString(o.Indent)
	b.WriteString("ALTER TABLE ")
	b.WriteString(t.Name)
	b.WriteString(" ADD COLUMN ")
	if o.IfNotExists {
		b.WriteString("IF NOT EXISTS ")
	}
	b.WriteString(c.name(d))
	b.WriteString(" ")
	b.WriteString(c.spec(d))
	b.WriteString(";\n")
	return b.String()
}
