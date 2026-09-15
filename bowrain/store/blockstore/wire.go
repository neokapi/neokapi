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
// *sql.DB handle are resolved from the concrete store beneath any
// decorators, such as the event decorator a running server wraps its
// store in. PostgresStore and SQLiteStore are supported; anything else
// is a programming error. The blockstore keeps cs itself for what it
// reads and writes through the ContentStore interface, so a decorator
// still sees those calls.
//
// Callers that need full control over Options should use New directly.
func Open(cs platstore.ContentStore, projectID, stream string) (blockstore.Store, error) {
	var (
		db      DB
		dialect Dialect
	)
	switch s := concreteStore(cs).(type) {
	case *corestore.PostgresStore:
		db = s.SQLDB()
		dialect = PostgresDialect
	case *sqlitestore.SQLiteStore:
		db = s.DB()
		dialect = SQLiteDialect
	default:
		return nil, fmt.Errorf("bowrain/blockstore: unsupported ContentStore %T", s)
	}
	return New(Options{
		ContentStore: cs,
		DB:           db,
		Dialect:      dialect,
		ProjectID:    projectID,
		Stream:       stream,
	})
}

// concreteStore returns the store beneath cs's decorators, following each
// one's Unwrap until a store has none.
func concreteStore(cs platstore.ContentStore) platstore.ContentStore {
	for {
		d, ok := cs.(interface{ Unwrap() platstore.ContentStore })
		if !ok {
			return cs
		}
		inner := d.Unwrap()
		if inner == nil {
			return cs
		}
		cs = inner
	}
}
