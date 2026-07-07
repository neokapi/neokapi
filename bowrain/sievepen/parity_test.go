//go:build integration

package sievepen_test

// Cross-backend parity: the same fixture corpus and the same lookups must
// produce identical match sets, scores and ordering on the framework SQLite TM
// and the bowrain Postgres TM. Both stores delegate ranking to
// sievepen.TieredLookup, so any divergence here is a real drift regression.
//
// This suite historically FAILED before the dialect-seam refactor: the Postgres
// TM lacked the tag-mismatch penalty and the exact-ambiguity demotion, and
// sorted by match-type-then-score instead of score-then-priority-then-ID. It is
// the acceptance gate for keeping the two backends in lockstep.
//
// Requires a Postgres (BOWRAIN_TEST_POSTGRES_URL, or the local default) and the
// fts5 build tag for the SQLite side: `go test -tags "integration fts5"`.

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/sievepen"

	pgtm "github.com/neokapi/neokapi/bowrain/sievepen"
	pgstorage "github.com/neokapi/neokapi/bowrain/storage"

	"github.com/stretchr/testify/require"
)

// resultRow is the comparable projection of a TMMatch: everything the ranking
// policy controls, backend-independent.
type resultRow struct {
	EntryID   string
	Score     float64 // rounded to 4 dp
	MatchType sievepen.MatchType
	Ambiguous bool
	Target    string
}

func project(matches []sievepen.TMMatch, targetLocale model.LocaleID) []resultRow {
	out := make([]resultRow, 0, len(matches))
	for _, m := range matches {
		out = append(out, resultRow{
			EntryID:   m.Entry.ID,
			Score:     math.Round(m.Score*1e4) / 1e4,
			MatchType: m.MatchType,
			Ambiguous: m.Ambiguous,
			Target:    m.Entry.VariantText(targetLocale),
		})
	}
	return out
}

// openParitySQLiteTM opens a throwaway SQLite TM for the parity corpus.
func openParitySQLiteTM(t *testing.T) sievepen.TMStore {
	t.Helper()
	tm, err := sievepen.NewSQLiteTM(filepath.Join(t.TempDir(), "parity.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = tm.Close() })
	return tm
}

// maybePostgresTM opens a Postgres TM if one is reachable, returning (nil,
// false) otherwise — unlike openTestPostgresTM it does NOT skip the test, so
// the SQLite side of the parity fixtures always exercises even without PG. The
// full cross-backend comparison runs only when Postgres is present (CI).
func maybePostgresTM(t *testing.T) (sievepen.TMStore, bool) {
	t.Helper()
	connStr := os.Getenv("BOWRAIN_TEST_POSTGRES_URL")
	if connStr == "" {
		connStr = "postgres://bowrain:bowrain@localhost:5432/bowrain_test?sslmode=disable"
	}
	db, err := pgstorage.OpenPostgres(connStr)
	if err != nil {
		return nil, false
	}
	wsID := fmt.Sprintf("parity-%s-%d", t.Name(), time.Now().UnixNano())
	tm, err := pgtm.NewPostgresTMFromDB(db, wsID)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM tm_entries WHERE workspace_id = $1", wsID)
		_, _ = db.Exec("DELETE FROM tm_import_sessions WHERE workspace_id = $1", wsID)
		_ = db.Close()
	})
	return tm, true
}

// codedEntry is an entry whose source/target carry paired inline codes, so its
// PLAIN key collides with the bare text while its STRUCTURAL key differs.
func codedEntry(id, srcText, tgtText string) sievepen.TMEntry {
	return sievepen.TMEntry{
		ID: id,
		Variants: map[model.LocaleID][]model.Run{
			"en": {
				{PcOpen: &model.PcOpenRun{ID: "m0", Data: "**"}},
				{Text: &model.TextRun{Text: srcText}},
				{PcClose: &model.PcCloseRun{ID: "m0", Data: "**"}},
			},
			"nb": {
				{PcOpen: &model.PcOpenRun{ID: "m0", Data: "**"}},
				{Text: &model.TextRun{Text: tgtText}},
				{PcClose: &model.PcCloseRun{ID: "m0", Data: "**"}},
			},
		},
	}
}

func plainEntry(id, src, tgt string) sievepen.TMEntry {
	return sievepen.TMEntry{
		ID: id,
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: src}}},
			"nb": {{Text: &model.TextRun{Text: tgt}}},
		},
	}
}

// paritySeed loads the identical fixture corpus into a store.
func paritySeed(t *testing.T, tm sievepen.TMStore) {
	t.Helper()
	ctx := context.Background()
	entries := []sievepen.TMEntry{
		// Tag-mismatch: same plain key "Install", different code structure.
		plainEntry("e-plain-install", "Install", "Installering"),
		codedEntry("e-coded-install", "Install", "Installer"),
		// Ambiguity: two structurally-identical exacts with differing targets.
		plainEntry("e-save-a", "Save", "Lagre"),
		plainEntry("e-save-b", "Save", "Lagret"),
		// Fuzzy neighbourhood around "Hello world".
		plainEntry("e-hello-1", "Hello world", "Hei verden"),
		plainEntry("e-hello-2", "Hello word", "Hei ord"),
		plainEntry("e-hello-3", "Hello worlds", "Hei verdener"),
	}
	for _, e := range entries {
		require.NoError(t, tm.Add(ctx, e))
	}
}

// runParity runs a named lookup against the SQLite store (always) and, when a
// Postgres store is present, asserts the two backends project identical result
// rows (set + order). Without Postgres it still exercises the SQLite fixture
// path so the corpus/lookup code is locally covered.
func runParity(t *testing.T, sq, pg sievepen.TMStore, name string, target model.LocaleID, lookup func(sievepen.TMStore) ([]sievepen.TMMatch, error)) {
	t.Helper()
	sqMatches, err := lookup(sq)
	require.NoError(t, err, "%s: sqlite", name)
	if pg == nil {
		return
	}
	pgMatches, err := lookup(pg)
	require.NoError(t, err, "%s: postgres", name)
	require.Equal(t, project(sqMatches, target), project(pgMatches, target),
		"backend drift for %q", name)
}

func TestTMParity_SQLiteVsPostgres(t *testing.T) {
	sq := openParitySQLiteTM(t)
	pg, hasPG := maybePostgresTM(t)
	paritySeed(t, sq)
	if hasPG {
		paritySeed(t, pg)
	}

	ctx := context.Background()
	opts := sievepen.LookupOptions{MaxResults: 10, MinScore: 0.3}

	cases := []struct {
		name   string
		target model.LocaleID
		lookup func(sievepen.TMStore) ([]sievepen.TMMatch, error)
	}{
		{
			// Plain query "Install": the structurally-identical entry wins at
			// 1.0; the coded entry takes the tag-mismatch penalty. Postgres
			// used to give both 1.0.
			name:   "tag-mismatch-penalty",
			target: "nb",
			lookup: func(tm sievepen.TMStore) ([]sievepen.TMMatch, error) {
				return tm.LookupText(ctx, "Install", "en", "nb", opts)
			},
		},
		{
			// Two exacts with differing targets both demote to ScoreNearExact
			// and flag Ambiguous. Postgres used to keep them at 1.0.
			name:   "ambiguity-demotion",
			target: "nb",
			lookup: func(tm sievepen.TMStore) ([]sievepen.TMMatch, error) {
				return tm.LookupText(ctx, "Save", "en", "nb", opts)
			},
		},
		{
			// Fuzzy neighbourhood: score-then-priority-then-ID ordering must
			// match. Postgres used to sort by match-type first.
			name:   "fuzzy-ranking",
			target: "nb",
			lookup: func(tm sievepen.TMStore) ([]sievepen.TMMatch, error) {
				return tm.LookupText(ctx, "Hello world", "en", "nb", opts)
			},
		},
		{
			// Structural exact via a coded query block against the coded entry.
			name:   "structural-exact-coded-query",
			target: "nb",
			lookup: func(tm sievepen.TMStore) ([]sievepen.TMMatch, error) {
				block := &model.Block{Source: []model.Run{
					{PcOpen: &model.PcOpenRun{ID: "m0", Data: "**"}},
					{Text: &model.TextRun{Text: "Install"}},
					{PcClose: &model.PcCloseRun{ID: "m0", Data: "**"}},
				}}
				return tm.Lookup(ctx, block, "en", "nb", opts)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runParity(t, sq, pg, tc.name, tc.target, tc.lookup)
		})
	}
}

// TestTMParity_ExactOnlyPolicy asserts the exact-only (MinScore 1.0) policy —
// which drops ambiguous demoted matches — is identical across backends.
func TestTMParity_ExactOnlyPolicy(t *testing.T) {
	sq := openParitySQLiteTM(t)
	pg, hasPG := maybePostgresTM(t)
	paritySeed(t, sq)
	if hasPG {
		paritySeed(t, pg)
	}

	ctx := context.Background()
	strict := sievepen.LookupOptions{MinScore: 1.0, MaxResults: 10}
	runParity(t, sq, pg, "exact-only-ambiguous-drop", "nb", func(tm sievepen.TMStore) ([]sievepen.TMMatch, error) {
		return tm.LookupText(ctx, "Save", "en", "nb", strict)
	})
	runParity(t, sq, pg, "exact-only-clean-hit", "nb", func(tm sievepen.TMStore) ([]sievepen.TMMatch, error) {
		return tm.LookupText(ctx, "Install", "en", "nb", strict)
	})
}
