package migrations_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/migrations"
	"github.com/neokapi/neokapi/bowrain/storage"
	corestorage "github.com/neokapi/neokapi/core/storage"
)

// The ledger rules need no database, so they carry no build tag: every PR runs
// them. Their siblings in schema_test.go build a real schema and sit behind
// `integration`, which runs only on main, a lane that reports a broken ledger
// after the merge that broke it, to a database that has already been migrated.

// TestBaselineVersionsSitAboveRetiredOnes checks the rule that outlives any one
// consolidation: a version number, once issued, is spent. Consolidation folded
// each subsystem's history into one migration numbered ABOVE the highest that
// subsystem ever issued, so a database that already recorded the old numbers
// sees the baseline as new work rather than as history it has done.
//
// Reusing a retired number is the sharp failure: a database holding version 14
// would skip a new migration numbered 14 entirely, and drift silently.
func TestBaselineVersionsSitAboveRetiredOnes(t *testing.T) {
	// The highest version each subsystem had ever issued, read from the
	// migration lists at the commit that retired them. A baseline must be
	// strictly greater.
	//
	// Store and jobs sit above the rest because they were consolidated more
	// than once, and every number a fold spent stays spent. A baseline edited
	// in place spends its old number too: auth 8 and 9, knowledge 2 to 4,
	// store 21, jobs 9, billing 7 and extraction 2 were each a baseline before
	// later changes were folded into it.
	highestEverIssued := map[string]int{
		"store": 21, "auth": 9, "jobs": 9, "quota": 3, "runner": 1,
		"extraction": 2, "context_scan": 2, "model_sweep": 1, "voice": 2,
		"knowledge": 4, "agent": 1, "billing": 7, "platform_config": 1,
		"terms": 4, "memory": 5,
	}

	for _, s := range migrations.All() {
		t.Run(s.Name, func(t *testing.T) {
			prior, known := highestEverIssued[s.Name]
			require.True(t, known, "subsystem %q is not in the retired-version ledger; add it", s.Name)
			require.NotEmpty(t, s.Migrations, "subsystem %q has no migrations", s.Name)

			// The invariant is about the version NUMBERS, not the count. Every
			// migration a subsystem carries must sit above every version it
			// retired, and later migrations must keep climbing, so a running
			// max seeded with the highest retired version, which each migration
			// must strictly exceed, catches both a reused number and a
			// non-increasing one. Asserting a single baseline instead would fail
			// the first time a real migration is appended after the fold, the
			// ordinary, sanctioned way the schema evolves, and so mask the reuse
			// it exists to catch.
			last := prior
			for _, m := range s.Migrations {
				assert.Greater(t, m.Version, last,
					"%q version %d must exceed both every retired version and the prior migration (%d); a reused or non-increasing number drifts silently",
					s.Name, m.Version, last)
				last = m.Version
			}
		})
	}
}

// TestVersionsAreUniqueWithinASubsystem is the cheapest guard on the sharpest
// edge. The bookkeeping table's PRIMARY KEY is the version, and the runner
// reads the applied maximum ONCE before its loop, so two migrations sharing a
// number produce one of two outcomes, both silent until they are not:
//
//   - An empty database applies both and then fails to record the second,
//     aborting the whole boot with a duplicate-key error.
//   - A database already at that version skips both, so the schema change
//     never lands and the first query against the missing column is the only
//     symptom.
func TestVersionsAreUniqueWithinASubsystem(t *testing.T) {
	for _, s := range migrations.All() {
		t.Run(s.Name, func(t *testing.T) {
			seen := map[int]string{}
			for _, m := range s.Migrations {
				prior, dup := seen[m.Version]
				require.False(t, dup,
					"version %d is issued twice in %q: %q and %q", m.Version, s.Name, prior, m.Description)
				seen[m.Version] = m.Description
			}
		})
	}
}

// TestEveryLedgerIsValid holds each subsystem's migrations to the rules the
// runner refuses to start without (storage.ValidateMigrations): one migration
// per version, versions climbing without a gap nobody declared Retired, and a
// declared revision on every step written in Go.
func TestEveryLedgerIsValid(t *testing.T) {
	for _, s := range migrations.All() {
		t.Run(s.Name, func(t *testing.T) {
			_, err := corestorage.ValidateMigrations(s.Table, s.Migrations)
			require.NoError(t, err)
		})
	}
}

// TestEveryShippedIdentityIsPinned holds every subsystem's migrations to the
// committed ledger file (bowrain/storage/migration_ledger.json): each version
// is listed under its current name with the identity it executes now. An edit
// to a shipped migration fails here and in the migration guard, before it
// merges; a new migration is listed with make migration-ledger.
func TestEveryShippedIdentityIsPinned(t *testing.T) {
	shipped := storage.ShippedLedgers()
	for _, s := range migrations.All() {
		t.Run(s.Name, func(t *testing.T) {
			pinned, ok := shipped[s.Table]
			require.True(t, ok, "%s is not in the committed ledger file; run make migration-ledger", s.Table)
			for _, m := range s.Migrations {
				if m.Retired {
					continue
				}
				p, ok := pinned[m.Version]
				require.True(t, ok, "%s version %d (%q) is not pinned; run make migration-ledger", s.Table, m.Version, m.Description)
				assert.Equal(t, p.Description, m.Description, "%s version %d", s.Table, m.Version)
				assert.Equal(t, p.Identity, corestorage.MigrationIdentity(m), "%s version %d (%q) no longer has the identity it shipped with: a shipped migration was edited", s.Table, m.Version, m.Description)
			}
		})
	}
}
