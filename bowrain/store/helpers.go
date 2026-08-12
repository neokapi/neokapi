package store

import (
	"github.com/neokapi/neokapi/bowrain/storage"
)

// MaxChangesPerRequest is the maximum number of change entries returned per query.
const MaxChangesPerRequest = 1000

// MaxHistoryEntries is the default maximum number of history entries returned.
const MaxHistoryEntries = 100

// scanner is an alias for storage.Scanner, the interface shared by *sql.Row
// and *sql.Rows. Used by the scanX helper functions.
type scanner = storage.Scanner
