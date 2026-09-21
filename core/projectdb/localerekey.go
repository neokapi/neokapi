package projectdb

import (
	"context"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/storage"
)

// Keying the context store's rows back to the canonical locale.
//
// The rows the audit finds in the projection are derived from the working tree
// and a rebuild clears them. The rows it finds in the context store are not:
// an approved term, a reviewed sentence and a voice profile exist there and
// nowhere else, so they are moved to the spelling every lookup asks in rather
// than deleted.
//
// The move is one transaction over the context store, and it decides three
// cases per row:
//
//   - The canonical spelling is free. The row moves to it and is found again.
//   - The canonical spelling holds a row saying exactly the same thing. The two
//     are one row under two spellings, so the non-canonical one is folded into
//     it and the merge is reported.
//   - The canonical spelling holds a row saying something else. Both are kept.
//     The canonical row is the one every lookup already reads, and the other
//     carries wording somebody approved, so neither is thrown away and neither
//     is chosen here: the pair is reported, and a person says which is right.
//
// The last case leaves the audit still reporting the row, which is the honest
// outcome. A repair that silently picked one would be a repair that deleted an
// approval.

// LocaleRekey is what keying one subsystem's rows to the canonical spelling
// did.
type LocaleRekey struct {
	// Subsystem is the store whose rows moved: "content memory" or "terms".
	Subsystem string `json:"subsystem"`
	// Locale is the spelling the rows carried, and Canonical the form every
	// lookup asks in.
	Locale    string `json:"locale"`
	Canonical string `json:"canonical"`
	// Moved counts the rows now keyed by the canonical spelling.
	Moved int `json:"moved"`
	// Merged counts the rows folded into a canonical row saying the same
	// thing: one row that was in the store twice.
	Merged int `json:"merged"`
	// Conflicted counts the rows left where they are, because the canonical
	// spelling already holds something else for the same thing.
	Conflicted int `json:"conflicted"`
}

// String renders the outcome the way a report names it.
func (r LocaleRekey) String() string {
	s := fmt.Sprintf("%s: %d row(s) moved from %q to %q", r.Subsystem, r.Moved, r.Locale, r.Canonical)
	if r.Merged > 0 {
		s += fmt.Sprintf(", %d folded into an identical row", r.Merged)
	}
	if r.Conflicted > 0 {
		s += fmt.Sprintf(", %d left under %q because %q already answers differently",
			r.Conflicted, r.Locale, r.Canonical)
	}
	return s
}

// Rekeyed reports whether anything in the store moved.
func (r LocaleRekey) Rekeyed() bool { return r.Moved > 0 || r.Merged > 0 }

// localeTable is a table whose rows are keyed by a locale.
type localeTable struct {
	name string
	// identity is the rest of the row's key: the columns that, together with
	// the locale, decide which row this is. Empty where the table's key is a
	// synthetic row id, so two rows can differ in nothing but their id and a
	// move can never collide.
	identity []string
	// content is what makes two rows the same row under two spellings. Empty
	// means every column but the locale and the row id, read from the schema,
	// so a table that gains a column cannot drift out of the comparison.
	content []string
}

// contextLocaleTables names, per subsystem the audit reports, every table in
// the context store keyed by a locale.
//
// The content memory's entity values are in it because they belong to the
// variant they describe: a variant keyed back to `nb` whose entity values
// stayed at `nb-NO` would be found again and answer with its placeholders
// unresolved.
var contextLocaleTables = map[string][]localeTable{
	"content memory": {
		{
			name:     "tm_variants",
			identity: []string{"entry_id"},
			content:  []string{"coded", "plain", "struct_key", "general_key"},
		},
		{
			name:     "tm_entry_entity_values",
			identity: []string{"entry_id", "placeholder_id"},
			content:  []string{"text_value", "start_pos", "end_pos"},
		},
	},
	"terms": {
		{name: "tb_terms"},
	},
}

// RekeyContextLocales keys the context store's rows to the canonical locale,
// in place, and reports what it did to each subsystem.
//
// It writes nothing outside the context store: the projection's rows are a
// reading of the working tree, and deleting it is what clears those. A store
// with no drift reports nothing and opens no transaction.
//
// The caller rebuilds the content memory's search indexes afterwards. They are
// built from the variant rows and carry the locale beside each one, so a move
// leaves them answering for a spelling the rows no longer have.
func (d *DB) RekeyContextLocales(ctx context.Context) ([]LocaleRekey, error) {
	if d.context == nil {
		return nil, nil
	}
	drift, err := d.NonCanonicalLocales(ctx)
	if err != nil {
		return nil, err
	}
	var wanted []LocaleDrift
	for _, row := range drift {
		if row.Pool == PoolContext && len(contextLocaleTables[row.Subsystem]) > 0 {
			wanted = append(wanted, row)
		}
	}
	if len(wanted) == 0 {
		return nil, nil
	}

	tx, err := d.context.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("projectdb: key the context store's locales: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	out := make([]LocaleRekey, 0, len(wanted))
	for _, row := range wanted {
		done := LocaleRekey{Subsystem: row.Subsystem, Locale: row.Locale, Canonical: row.Canonical}
		for _, table := range contextLocaleTables[row.Subsystem] {
			moved, merged, left, terr := rekeyLocaleRows(ctx, tx, table, row.Locale, row.Canonical)
			if terr != nil {
				return nil, terr
			}
			done.Moved += moved
			done.Merged += merged
			done.Conflicted += left
		}
		out = append(out, done)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("projectdb: key the context store's locales: %w", err)
	}
	return out, nil
}

// rekeyLocaleRows moves one table's rows from one locale spelling to another.
func rekeyLocaleRows(ctx context.Context, tx *storage.Tx, table localeTable, from, to string) (moved, merged, conflicted int, err error) {
	content := table.content
	if len(content) == 0 {
		if content, err = comparableColumns(ctx, tx, table.name); err != nil {
			return 0, 0, 0, err
		}
	}

	same := make([]string, 0, len(table.identity)+len(content))
	for _, col := range append(append([]string{}, table.identity...), content...) {
		same = append(same, fmt.Sprintf("c.%s IS r.%s", col, col))
	}
	res, err := tx.ExecContext(ctx, fmt.Sprintf(`
DELETE FROM %[1]s AS r
 WHERE r.locale = ?
   AND EXISTS (SELECT 1 FROM %[1]s AS c WHERE c.locale = ? AND %[2]s)`,
		table.name, strings.Join(same, " AND ")), from, to)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("projectdb: fold %s rows keyed %q into %q: %w", table.name, from, to, err)
	}
	if n, aerr := res.RowsAffected(); aerr == nil {
		merged = int(n)
	}

	args := []any{to, from}
	guard := ""
	if len(table.identity) > 0 {
		keyed := make([]string, 0, len(table.identity))
		for _, col := range table.identity {
			keyed = append(keyed, fmt.Sprintf("c.%s IS r.%s", col, col))
		}
		guard = fmt.Sprintf(
			"\n   AND NOT EXISTS (SELECT 1 FROM %s AS c WHERE c.locale = ? AND %s)",
			table.name, strings.Join(keyed, " AND "))
		args = append(args, to)
	}
	res, err = tx.ExecContext(ctx, fmt.Sprintf(
		"UPDATE %s AS r SET locale = ? WHERE r.locale = ?%s", table.name, guard), args...)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("projectdb: key %s rows from %q to %q: %w", table.name, from, to, err)
	}
	if n, aerr := res.RowsAffected(); aerr == nil {
		moved = int(n)
	}

	if err := tx.QueryRowContext(ctx,
		fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE locale = ?", table.name), from).Scan(&conflicted); err != nil {
		return 0, 0, 0, fmt.Errorf("projectdb: count the %s rows left under %q: %w", table.name, from, err)
	}
	return moved, merged, conflicted, nil
}

// comparableColumns is every column of a table but its locale and its row id:
// what two rows have to agree on to be the same row under two spellings.
//
// It reads the schema rather than naming the columns, so a table that gains one
// in a later migration is compared on it without anything here being edited. It
// is used only for a table whose primary key is a synthetic row id, where the
// id says nothing about which row this is.
func comparableColumns(ctx context.Context, tx *storage.Tx, table string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, fmt.Errorf("projectdb: read the columns of %s: %w", table, err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var cid, notNull, pk int
		var name, declared string
		var dflt any
		if err := rows.Scan(&cid, &name, &declared, &notNull, &dflt, &pk); err != nil {
			return nil, fmt.Errorf("projectdb: read the columns of %s: %w", table, err)
		}
		if name == "locale" || pk > 0 {
			continue
		}
		out = append(out, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("projectdb: read the columns of %s: %w", table, err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("projectdb: %s has no columns to compare", table)
	}
	return out, nil
}
