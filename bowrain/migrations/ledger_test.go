package migrations_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/migrations"
)

// The ledger rules need no database, so they carry no build tag: every PR runs
// them. Their siblings in schema_test.go build a real schema and sit behind
// `integration`, which runs only on main — a lane that reports a broken ledger
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
	// than once: migrations appended after a fold (store 16-19, jobs 8-9) were
	// themselves folded, and every number they spent stays spent. A baseline
	// edited in place spends its old number too: auth 8 and knowledge 2 were
	// baselines before the machine author identity and the solo self-approval
	// marker were folded into them, store 20 before the audit event key, jobs 9
	// before created_by, and billing 7 before the deduction idempotency index.
	highestEverIssued := map[string]int{
		"store": 20, "auth": 8, "jobs": 9, "quota": 3, "runner": 1,
		"extraction": 1, "brand_scan": 1, "model_sweep": 1, "brand": 2,
		"knowledge": 2, "agent": 1, "billing": 7, "platform_config": 1,
		"terms": 4, "memory": 5,
	}

	for _, s := range migrations.All() {
		t.Run(s.Name, func(t *testing.T) {
			prior, known := highestEverIssued[s.Name]
			require.True(t, known, "subsystem %q is not in the retired-version ledger; add it", s.Name)
			require.NotEmpty(t, s.Migrations, "subsystem %q has no migrations", s.Name)

			assert.Len(t, s.Migrations, 1, "a consolidated subsystem carries exactly one baseline")
			assert.Greater(t, s.Migrations[0].Version, prior,
				"baseline version must sit above every version %q ever issued (%d)", s.Name, prior)
		})
	}
}

// TestVersionsAreUniqueWithinASubsystem is the cheapest guard on the sharpest
// edge. The bookkeeping table's PRIMARY KEY is the version, and the runner
// reads the applied maximum ONCE before its loop — so two migrations sharing a
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
