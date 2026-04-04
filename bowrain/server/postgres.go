package server

import (
	"fmt"
	"log/slog"

	bragent "github.com/neokapi/neokapi/bowrain/agent"
	"github.com/neokapi/neokapi/bowrain/auth"
	"github.com/neokapi/neokapi/bowrain/billing"
	platbrand "github.com/neokapi/neokapi/bowrain/brand"
	platagent "github.com/neokapi/neokapi/bowrain/core/agent"
	"github.com/neokapi/neokapi/bowrain/core/store"
	platgraph "github.com/neokapi/neokapi/bowrain/graph"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/bowrain/storage"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	corebrand "github.com/neokapi/neokapi/core/brand"
	coreg "github.com/neokapi/neokapi/core/graph"
)

// pgStores holds all PostgreSQL-backed stores opened from a shared connection pool.
type pgStores struct {
	Content    store.ContentStore
	Auth       auth.AuthStore
	Job        jobs.JobStore
	Extraction jobs.ExtractionJobStore
	Quota      jobs.QuotaStore
	Brand      corebrand.BrandStore
	GraphStore coreg.GraphStore
	Agent      platagent.AgentStore
	Billing    billing.BillingStore
	DB         *storage.PgDB // shared connection pool for TM/TB
}

func openPostgresStores(databaseURL string) (*pgStores, error) {
	db, err := storage.OpenPostgresWithPool(databaseURL, platgraph.AfterConnect)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL: %w", err)
	}
	return initPostgresStores(db)
}

// openPostgresStoresAzure opens PostgreSQL-backed stores using Azure
// Managed Identity for authentication (passwordless).
func openPostgresStoresAzure(databaseURL, clientID string) (*pgStores, error) {
	db, err := storage.OpenPostgresAzureWithHook(databaseURL, clientID, platgraph.AfterConnect)
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

	bs, err := platbrand.NewPostgresBrandStore(db)
	if err != nil {
		slog.Warn("failed to init brand store (brand voice features disabled)", "error", err)
	}

	es, err := jobs.NewPgExtractionJobStore(db)
	if err != nil {
		return nil, fmt.Errorf("init PostgreSQL extraction job store: %w", err)
	}

	stores := &pgStores{Content: cs, Auth: as, Job: js, Extraction: es, Quota: qs, Brand: bs, DB: db}

	// Initialize graph store if pgxpool is available (AfterConnect was wired).
	if pool := db.Pool(); pool != nil {
		gs := platgraph.NewAGEGraphStore(pool)
		stores.GraphStore = gs
	}

	// Initialize agent store (AD-028).
	ags, err := bragent.NewPostgresStore(db)
	if err != nil {
		slog.Warn("failed to init agent store (agent features disabled)", "error", err)
	} else {
		stores.Agent = ags
	}

	// Initialize billing store (AD-030).
	bils, err := billing.NewPgBillingStore(db)
	if err != nil {
		slog.Warn("failed to init billing store (billing features disabled)", "error", err)
	} else {
		stores.Billing = bils
	}

	return stores, nil
}
