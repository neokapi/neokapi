package server

import (
	"context"
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
	"github.com/neokapi/neokapi/bowrain/knowledge"
	"github.com/neokapi/neokapi/bowrain/platformconfig"
	"github.com/neokapi/neokapi/bowrain/storage"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	corebrand "github.com/neokapi/neokapi/core/brand"
	coreg "github.com/neokapi/neokapi/core/graph"
)

// pgStores holds all PostgreSQL-backed stores opened from a shared connection pool.
type pgStores struct {
	Content        store.ContentStore
	Auth           auth.AuthStore
	Job            jobs.JobStore
	Extraction     jobs.ExtractionJobStore
	BrandScan      jobs.BrandScanJobStore
	Quota          jobs.QuotaStore
	Sweep          jobs.ModelSweepStore
	Brand          corebrand.BrandStore
	Knowledge      knowledge.Store
	GraphStore     coreg.GraphStore
	Agent          platagent.AgentStore
	Billing        billing.BillingStore
	PlatformConfig *platformconfig.Store
	DB             *storage.PgDB // shared connection pool for TM/TB
}

func openPostgresStores(databaseURL string) (*pgStores, error) {
	// The brand knowledge graph runs on the plain-SQL backend over standard
	// PostgreSQL (Epic 014), so no AfterConnect hook is needed — a stock
	// Postgres/RDS instance works out of the box. (A pgx AfterConnect-based
	// Apache AGE backend existed historically; see bowrain/graph/NOTES.md.)
	db, err := storage.OpenPostgresWithPool(databaseURL, nil)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL: %w", err)
	}
	return initPostgresStores(db)
}

func initPostgresStores(db *storage.PgDB) (*pgStores, error) {
	cs, err := bstore.NewPostgresStoreFromDB(db)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("init PostgreSQL content store: %w", err)
	}

	as, err := auth.NewAuthStoreFromDB(db)
	if err != nil {
		cs.Close()
		return nil, fmt.Errorf("init PostgreSQL auth store: %w", err)
	}

	js, err := jobs.NewJobStore(db)
	if err != nil {
		return nil, fmt.Errorf("init PostgreSQL job store: %w", err)
	}

	qs, err := jobs.NewQuotaStore(db)
	if err != nil {
		return nil, fmt.Errorf("init PostgreSQL quota store: %w", err)
	}

	bs, err := platbrand.NewPostgresBrandStore(db)
	if err != nil {
		slog.Warn("failed to init brand store (brand voice features disabled)", "error", err)
	}

	ks, err := knowledge.NewPostgresKnowledgeStore(db)
	if err != nil {
		slog.Warn("failed to init knowledge store (brand knowledge graph disabled)", "error", err)
		ks = nil
	}

	es, err := jobs.NewExtractionJobStore(db)
	if err != nil {
		return nil, fmt.Errorf("init PostgreSQL extraction job store: %w", err)
	}

	bsj, err := jobs.NewBrandScanJobStore(db)
	if err != nil {
		return nil, fmt.Errorf("init PostgreSQL brand-scan job store: %w", err)
	}

	sws, err := jobs.NewModelSweepStore(db)
	if err != nil {
		return nil, fmt.Errorf("init PostgreSQL model-sweep store: %w", err)
	}

	stores := &pgStores{Content: cs, Auth: as, Job: js, Extraction: es, BrandScan: bsj, Quota: qs, Sweep: sws, Brand: bs, DB: db}
	if ks != nil {
		stores.Knowledge = ks
	}

	// Initialize the graph store if a pgxpool is available. The plain-SQL
	// backend (relational adjacency tables on standard PostgreSQL/RDS) is the
	// only backend. The graph is optional and derived (GraphSyncer rebuilds it
	// from events), so a schema-setup failure degrades to a nil GraphStore
	// rather than failing server startup.
	if pool := db.Pool(); pool != nil {
		gs := platgraph.NewSQLGraphStore(pool)
		if err := gs.EnsureGraph(context.Background()); err != nil {
			slog.Warn("failed to ensure SQL graph schema (brand knowledge graph disabled)", "error", err)
		} else {
			stores.GraphStore = gs
		}
	}

	// Initialize agent store (Bowrain AD-016).
	ags, err := bragent.NewStore(db)
	if err != nil {
		slog.Warn("failed to init agent store (agent features disabled)", "error", err)
	} else {
		stores.Agent = ags
	}

	// Initialize billing store (Bowrain AD-018).
	bils, err := billing.NewPgBillingStore(db)
	if err != nil {
		slog.Warn("failed to init billing store (billing features disabled)", "error", err)
	} else {
		stores.Billing = bils
	}

	// Initialize the instance-wide platform_config store (ctrl-managed settings).
	pcs, err := platformconfig.NewStore(db)
	if err != nil {
		slog.Warn("failed to init platform_config store (using env defaults only)", "error", err)
	} else {
		stores.PlatformConfig = pcs
	}

	return stores, nil
}
