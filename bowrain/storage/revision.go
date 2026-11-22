package storage

import (
	"errors"
	"fmt"

	"github.com/neokapi/neokapi/core/id"
)

// ErrStale is a write refused because the record has changed since the writer
// read it: the revision the writer named is no longer the record's.
var ErrStale = errors.New("the record changed after it was read")

// NewRevision is the token a write gives the record it writes. Tokens never
// repeat, so a record deleted and created again under the same id never
// matches a write based on the old one.
func NewRevision() string { return id.New() }

// RevisionColumn is the migrations that give an existing table a revision
// column: a token the store replaces on every write, which a writer names to
// say what it read, so a store refuses a write based on a record that has
// changed since. first is the first version the migrations take; they use
// five.
//
// The steps never rewrite the table, because a volatile default on ADD
// COLUMN would rewrite every row under an exclusive lock:
//
//  1. Add the column nullable, and give rows inserted later a token by
//     default. Catalog changes only: no row is read or written.
//  2. Give each existing row a token of its own. A row lock on each row it
//     updates; reads go on, and only a write to the same row waits.
//  3. Add a not-null check NOT VALID, which checks new and updated rows only.
//  4. Validate the check. It reads every row under SHARE UPDATE EXCLUSIVE,
//     which lets reads and writes go on while it scans.
//  5. SET NOT NULL, which the validated check proves without a scan, then drop
//     the check, which is then redundant.
func RevisionColumn(table string, first int) []Migration {
	check := table + "_revision_not_null"
	return []Migration{
		{
			Version:     first,
			Description: fmt.Sprintf("a revision column on %s", table),
			SQL: fmt.Sprintf(`
				ALTER TABLE %[1]s ADD COLUMN IF NOT EXISTS revision TEXT;
				ALTER TABLE %[1]s ALTER COLUMN revision SET DEFAULT gen_random_uuid()::text;
			`, table),
		},
		{
			Version:     first + 1,
			Description: fmt.Sprintf("a revision for each existing row of %s", table),
			SQL:         fmt.Sprintf(`UPDATE %s SET revision = gen_random_uuid()::text WHERE revision IS NULL;`, table),
		},
		{
			Version:     first + 2,
			Description: fmt.Sprintf("a not-null check on %s revisions, not yet validated", table),
			SQL:         fmt.Sprintf(`ALTER TABLE %s ADD CONSTRAINT %s CHECK (revision IS NOT NULL) NOT VALID;`, table, check),
		},
		{
			Version:     first + 3,
			Description: fmt.Sprintf("validate the %s revision check", table),
			SQL:         fmt.Sprintf(`ALTER TABLE %s VALIDATE CONSTRAINT %s;`, table, check),
		},
		{
			Version:     first + 4,
			Description: fmt.Sprintf("%s revisions are not null", table),
			SQL: fmt.Sprintf(`
				ALTER TABLE %[1]s ALTER COLUMN revision SET NOT NULL;
				ALTER TABLE %[1]s DROP CONSTRAINT %[2]s;
			`, table, check),
		},
	}
}
