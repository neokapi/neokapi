package server

import (
	"fmt"
	"log"

	"github.com/neokapi/neokapi/bowrain/auth"
	platbrand "github.com/neokapi/neokapi/bowrain/brand"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/bowrain/storage"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	corebrand "github.com/neokapi/neokapi/core/brand"
	coreg "github.com/neokapi/neokapi/core/graph"
	platgraph "github.com/neokapi/neokapi/bowrain/graph"
	"github.com/neokapi/neokapi/platform/store"
)

// pgStores holds all PostgreSQL-backed stores opened from a shared connection pool.
type pgStores struct {
	Content    store.ContentStore
	Auth       auth.AuthStore
	Job        jobs.JobStore
	Quota      jobs.QuotaStore
	Brand      corebrand.BrandStore
	GraphStore coreg.GraphStore
	DB         *storage.PgDB // shared connection pool for TM/TB
}

func openPostgresStores(databaseURL string) (*pgStores, error) {
	// Try with AGE graph support first; fall back to plain PG if AGE is unavailable.
	db, err := storage.OpenPostgresWithPool(databaseURL, platgraph.AfterConnect)
	if err != nil {
		log.Printf("WARNING: AGE extension unavailable, opening PostgreSQL without graph support: %v", err)
		db, err = storage.OpenPostgresWithPool(databaseURL, nil)
		if err != nil {
			return nil, fmt.Errorf("open PostgreSQL: %w", err)
		}
	}
	return initPostgresStores(db)
}

// openPostgresStoresAzure opens PostgreSQL-backed stores using Azure
// Managed Identity for authentication (passwordless).
func openPostgresStoresAzure(databaseURL, clientID string) (*pgStores, error) {
	// Try with AGE graph support first; fall back to plain PG if AGE is unavailable.
	db, err := storage.OpenPostgresAzureWithHook(databaseURL, clientID, platgraph.AfterConnect)
	if err != nil {
		log.Printf("WARNING: AGE extension unavailable, opening PostgreSQL without graph support: %v", err)
		db, err = storage.OpenPostgresAzure(databaseURL, clientID)
		if err != nil {
			return nil, fmt.Errorf("open PostgreSQL (Azure): %w", err)
		}
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

	bs, err := platbrand.NewPostgresBrandStore(db)
	if err != nil {
		log.Printf("WARNING: failed to init brand store: %v (brand voice features disabled)", err)
	}

	stores := &pgStores{Content: cs, Auth: as, Job: js, Quota: qs, Brand: bs, DB: db}

	// Initialize graph store if pgxpool is available (AfterConnect was wired).
	if pool := db.Pool(); pool != nil {
		gs := platgraph.NewAGEGraphStore(pool)
		stores.GraphStore = gs
	}

	return stores, nil
}
