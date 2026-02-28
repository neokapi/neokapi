package server

import (
	"fmt"

	"github.com/gokapi/gokapi/bowrain/auth"
	"github.com/gokapi/gokapi/bowrain/storage"
	bstore "github.com/gokapi/gokapi/bowrain/store"
	"github.com/gokapi/gokapi/platform/store"
)

// openPostgresStores opens PostgreSQL-backed ContentStore and AuthStore
// from a postgres:// connection URL, sharing a single connection pool.
func openPostgresStores(databaseURL string) (store.ContentStore, auth.AuthStore, error) {
	db, err := storage.OpenPostgres(databaseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("open PostgreSQL: %w", err)
	}

	cs, err := bstore.NewPostgresStoreFromDB(db)
	if err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("init PostgreSQL content store: %w", err)
	}

	as, err := auth.NewPostgresAuthStoreFromDB(db)
	if err != nil {
		cs.Close()
		return nil, nil, fmt.Errorf("init PostgreSQL auth store: %w", err)
	}

	return cs, as, nil
}
