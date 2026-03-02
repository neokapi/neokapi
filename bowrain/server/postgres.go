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
	return initPostgresStores(db)
}

// openPostgresStoresAzure opens PostgreSQL-backed stores using Azure
// Managed Identity for authentication (passwordless).
func openPostgresStoresAzure(databaseURL, clientID string) (store.ContentStore, auth.AuthStore, error) {
	db, err := storage.OpenPostgresAzure(databaseURL, clientID)
	if err != nil {
		return nil, nil, fmt.Errorf("open PostgreSQL (Azure): %w", err)
	}
	return initPostgresStores(db)
}

func initPostgresStores(db *storage.PgDB) (store.ContentStore, auth.AuthStore, error) {
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
