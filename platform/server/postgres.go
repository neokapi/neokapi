package server

import (
	"fmt"

	"github.com/neokapi/neokapi/bowrain/auth"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/bowrain/storage"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/platform/store"
)

// openPostgresStores opens PostgreSQL-backed ContentStore and AuthStore
// from a postgres:// connection URL, sharing a single connection pool.
// pgStores holds all PostgreSQL-backed stores opened from a shared connection pool.
type pgStores struct {
	Content store.ContentStore
	Auth    auth.AuthStore
	Job     jobs.JobStore
	Quota   jobs.QuotaStore
	DB      *storage.PgDB // shared connection pool for TM/TB
}

func openPostgresStores(databaseURL string) (*pgStores, error) {
	db, err := storage.OpenPostgres(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL: %w", err)
	}
	return initPostgresStores(db)
}

// openPostgresStoresAzure opens PostgreSQL-backed stores using Azure
// Managed Identity for authentication (passwordless).
func openPostgresStoresAzure(databaseURL, clientID string) (*pgStores, error) {
	db, err := storage.OpenPostgresAzure(databaseURL, clientID)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL (Azure): %w", err)
	}
	return initPostgresStores(db)
}

func initPostgresStores(db *storage.PgDB) (*pgStores, error) {
	cs, err := bstore.NewPostgresStoreFromDB(db)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("init PostgreSQL content store: %w", err)
	}

	as, err := auth.NewPostgresAuthStoreFromDB(db)
	if err != nil {
		cs.Close()
		return nil, fmt.Errorf("init PostgreSQL auth store: %w", err)
	}

	js, err := jobs.NewPgJobStore(db)
	if err != nil {
		return nil, fmt.Errorf("init PostgreSQL job store: %w", err)
	}

	qs, err := jobs.NewPgQuotaStore(db)
	if err != nil {
		return nil, fmt.Errorf("init PostgreSQL quota store: %w", err)
	}

	return &pgStores{Content: cs, Auth: as, Job: js, Quota: qs, DB: db}, nil
}
