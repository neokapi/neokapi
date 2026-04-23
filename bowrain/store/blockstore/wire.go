package blockstore

import (
	"fmt"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	corestore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/blockstore"
)

// Open returns a `blockstore.Store` wired to the given Bowrain
// ContentStore for the specified project/stream. The dialect and raw
// *sql.DB handle are resolved from the concrete ContentStore type —
// PostgresStore and SQLiteStore are supported; anything else is a
// programming error.
//
// Callers that need full control over Options should use New directly.
func Open(cs platstore.ContentStore, projectID, stream string) (blockstore.Store, error) {
	var (
		db      DB
		dialect Dialect
	)
	switch s := cs.(type) {
	case *corestore.PostgresStore:
		db = s.SQLDB()
		dialect = PostgresDialect
	case *sqlitestore.SQLiteStore:
		db = s.DB()
		dialect = SQLiteDialect
	default:
		return nil, fmt.Errorf("bowrain/blockstore: unsupported ContentStore %T", cs)
	}
	return New(Options{
		ContentStore: cs,
		DB:           db,
		Dialect:      dialect,
		ProjectID:    projectID,
		Stream:       stream,
	})
}
