package server

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

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
	Brand          corebrand.BrandStore
	Knowledge      knowledge.Store
	GraphStore     coreg.GraphStore
	Agent          platagent.AgentStore
	Billing        billing.BillingStore
	PlatformConfig *platformconfig.Store
	DB             *storage.PgDB // shared connection pool for TM/TB
}

func openPostgresStores(databaseURL string) (*pgStores, error) {
	db, err := storage.OpenPostgresWithPool(databaseURL, graphAfterConnect())
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL: %w", err)
	}
	return initPostgresStores(db)
}

// Graph backend selection (Epic 014). The brand knowledge graph
// (core/graph.GraphStore) is optional and derived — a GraphSyncer rebuilds it
// from relational events — and no consumer uses raw Cypher, so it runs on a
// plain-SQL backend over standard PostgreSQL. That SQL backend is the default,
// which lets development and managed RDS/Aurora (which do not support the
// Apache AGE extension) share the same standard-Postgres image.
//
// Set BOWRAIN_GRAPH_BACKEND=age to opt into the Apache AGE backend (requires an
// AGE-enabled PostgreSQL, e.g. the apache/age image). AGE remains the
// documented upgrade path for workloads that eventually need native graph
// traversal; see bowrain/graph/NOTES.md.
const (
	graphBackendEnv = "BOWRAIN_GRAPH_BACKEND"
	graphBackendSQL = "sql"
	graphBackendAGE = "age"
)

// graphBackend returns the configured graph backend, defaulting to "sql".
func graphBackend() string {
	if strings.EqualFold(strings.TrimSpace(os.Getenv(graphBackendEnv)), graphBackendAGE) {
		return graphBackendAGE
	}
	return graphBackendSQL
}

// graphAfterConnect returns the pgx AfterConnect hook required by the selected
// graph backend. Only the AGE backend needs the ag_catalog search_path hook;
// the SQL backend (and any standard PostgreSQL/RDS instance) must NOT install
// it, so a non-AGE Postgres works out of the box.
func graphAfterConnect() storage.AfterConnectFunc {
	if graphBackend() == graphBackendAGE {
		return platgraph.AfterConnect
	}
	return nil
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

	stores := &pgStores{Content: cs, Auth: as, Job: js, Extraction: es, BrandScan: bsj, Quota: qs, Brand: bs, DB: db}
	if ks != nil {
		stores.Knowledge = ks
	}

	// Initialize the graph store if a pgxpool is available. The backend is
	// selected by BOWRAIN_GRAPH_BACKEND (default "sql"): the SQL backend runs on
	// standard PostgreSQL/RDS, while "age" uses Apache AGE (whose ag_catalog
	// AfterConnect hook was wired at pool-open time). The graph is optional and
	// derived (GraphSyncer rebuilds it from events), so a schema-setup failure
	// degrades to a nil GraphStore rather than failing server startup.
	if pool := db.Pool(); pool != nil {
		switch graphBackend() {
		case graphBackendAGE:
			gs := platgraph.NewAGEGraphStore(pool)
			if err := gs.EnsureGraph(context.Background()); err != nil {
				slog.Warn("failed to ensure AGE graph (brand knowledge graph disabled)", "error", err)
			} else {
				stores.GraphStore = gs
			}
		default:
			gs := platgraph.NewSQLGraphStore(pool)
			if err := gs.EnsureGraph(context.Background()); err != nil {
				slog.Warn("failed to ensure SQL graph schema (brand knowledge graph disabled)", "error", err)
			} else {
				stores.GraphStore = gs
			}
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
