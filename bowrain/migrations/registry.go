// Package migrations enumerates every PostgreSQL schema a bowrain deployment
// owns, in the order a deployment applies them.
//
// Until this package existed the enumeration was implicit: it lived in the
// sequence of constructor calls in server.initPostgresStores, plus two more
// schemas created lazily on first workspace access. Nothing could state how
// many subsystems there were, which is how a reset scope came to name six
// bookkeeping tables when the database held fifteen — the nine it missed were
// emptied, their migrations replayed against a schema that already had the
// changes, and terms enforcement went offline in production on 2026-08-01.
//
// A registry makes the count answerable. Whatever iterates over All() cannot
// miss a subsystem the way a hand-maintained list can, and a subsystem added
// later joins every consumer at once.
//
// ORDER IS PART OF THE CONTRACT. Content store precedes auth, jobs precede
// their per-job-type extensions, and the two per-workspace schemas come last.
// The order mirrors server.initPostgresStores because that is the order
// production has always applied, and a foreign key can only be created after
// the table it points at exists.
package migrations

import (
	"fmt"

	"github.com/neokapi/neokapi/bowrain/agent"
	"github.com/neokapi/neokapi/bowrain/auth"
	"github.com/neokapi/neokapi/bowrain/billing"
	"github.com/neokapi/neokapi/bowrain/brand"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	"github.com/neokapi/neokapi/bowrain/memory"
	"github.com/neokapi/neokapi/bowrain/platformconfig"
	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/terms"
)

// Subsystem is one migration ledger: a set of migrations and the bookkeeping
// table that records which of them a database has applied.
type Subsystem struct {
	// Name identifies the subsystem in logs and operator output.
	Name string

	// Table is the bookkeeping table — always "<something>_schema_migrations".
	// The suffix is load-bearing: the reset scope matches bookkeeping by
	// pattern rather than by name so it cannot miss one.
	Table string

	// Migrations is the ledger, ascending by version.
	Migrations []storage.Migration

	// PerWorkspace marks a schema created lazily on first workspace access
	// rather than at boot. The tables are global and isolated by a
	// workspace_id column, so applying them once up front is both correct
	// and what makes a reset complete rather than half-built.
	PerWorkspace bool
}

// All returns every subsystem in application order.
//
// Adding a subsystem means adding it here. A schema absent from this list is
// invisible to the migrate-only entrypoint and to the drift tests, which is
// the failure mode this package exists to make impossible.
func All() []Subsystem {
	return []Subsystem{
		// Content store first: projects, streams and blocks are what the rest
		// of the platform points at.
		{Name: "store", Table: "store_schema_migrations", Migrations: store.Migrations},
		{Name: "auth", Table: "auth_schema_migrations", Migrations: auth.Migrations},

		// Jobs and its per-job-type extensions. Quota carries two ledgers:
		// ai_usage and runner_usage were separated so runner accounting could
		// be added without renumbering the quota history.
		{Name: "jobs", Table: "jobs_schema_migrations", Migrations: jobs.JobMigrations},
		{Name: "quota", Table: "quota_schema_migrations", Migrations: jobs.QuotaMigrations},
		{Name: "runner", Table: "runner_schema_migrations", Migrations: jobs.RunnerMigrations},
		{Name: "extraction", Table: "extraction_schema_migrations", Migrations: jobs.ExtractionMigrations},
		{Name: "brand_scan", Table: "brand_scan_schema_migrations", Migrations: jobs.BrandScanMigrations},
		{Name: "model_sweep", Table: "model_sweep_schema_migrations", Migrations: jobs.ModelSweepMigrations},

		{Name: "brand", Table: "brand_schema_migrations", Migrations: brand.Migrations},
		{Name: "knowledge", Table: "knowledge_schema_migrations", Migrations: knowledge.Migrations},
		{Name: "agent", Table: "agent_schema_migrations", Migrations: agent.Migrations},
		{Name: "billing", Table: "billing_schema_migrations", Migrations: billing.Migrations},
		{Name: "platform_config", Table: "platform_config_schema_migrations", Migrations: platformconfig.Migrations},

		// Created on first workspace access rather than at boot, which is why
		// a truncated tb_schema_migrations surfaced as "terms resolution
		// failed" on a translation job rather than as a startup failure.
		{Name: "terms", Table: "tb_schema_migrations", Migrations: terms.Migrations, PerWorkspace: true},
		{Name: "memory", Table: "tm_schema_migrations", Migrations: memory.Migrations, PerWorkspace: true},
	}
}

// Apply brings every subsystem up to its latest version, in registry order.
//
// This is what the migrate-only entrypoint runs. It is deliberately not what
// the server runs at boot: the server builds stores and migrates as a side
// effect of constructing them, and changing that is a larger move than this
// package needs to make.
func Apply(db *storage.PgDB, log func(string, ...any)) error {
	for _, s := range All() {
		if err := storage.MigratePostgresNS(db, s.Table, s.Migrations); err != nil {
			return fmt.Errorf("migrate %s: %w", s.Name, err)
		}
		if log != nil {
			log("migrated", "subsystem", s.Name, "table", s.Table, "version", s.Migrations[len(s.Migrations)-1].Version)
		}
	}
	return nil
}
