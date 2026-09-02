//go:build integration

package memory_test

// Cross-backend parity: the same fixture corpus and the same lookups must
// produce identical match sets, scores and ordering on the framework SQLite content memory
// and the bowrain Postgres content memory. Both stores delegate ranking to
// memory.TieredLookup, so any divergence here is a real drift regression.
//
// This suite historically FAILED before the dialect-seam refactor: the Postgres
// content memory lacked the tag-mismatch penalty and the exact-ambiguity demotion, and
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
	"github.com/neokapi/neokapi/memory"

	pgmemory "github.com/neokapi/neokapi/bowrain/memory"
	pgstorage "github.com/neokapi/neokapi/bowrain/storage"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resultRow is the comparable projection of a Match: everything the ranking
// policy controls, backend-independent.
type resultRow struct {
	EntryID   string
	Score     float64 // rounded to 4 dp
	MatchType memory.MatchType
	Ambiguous bool
	Target    string
}

func project(matches []memory.Match, targetLocale model.LocaleID) []resultRow {
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

// openParitySQLiteMemory opens a throwaway SQLite content memory for the parity corpus.
func openParitySQLiteMemory(t *testing.T) memory.Store {
	t.Helper()
	tm, err := memory.NewSQLiteStore(filepath.Join(t.TempDir(), "parity.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = tm.Close() })
	return tm
}

// maybePostgresMemory opens a Postgres content memory if one is reachable, returning (nil,
// false) otherwise — unlike openTestPostgresMemory it does NOT skip the test, so
// the SQLite side of the parity fixtures always exercises even without PG. The
// full cross-backend comparison runs only when Postgres is present (CI).
func maybePostgresMemory(t *testing.T) (memory.Store, bool) {
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
	tm, err := pgmemory.NewPostgresStoreFromDB(db, wsID)
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
func codedEntry(id, srcText, tgtText string) memory.Entry {
	return memory.Entry{
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

func plainEntry(id, src, tgt string) memory.Entry {
	return memory.Entry{
		ID: id,
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: src}}},
			"nb": {{Text: &model.TextRun{Text: tgt}}},
		},
	}
}

// paritySeed loads the identical fixture corpus into a store.
func paritySeed(t *testing.T, tm memory.Store) {
	t.Helper()
	ctx := context.Background()
	entries := []memory.Entry{
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
func runParity(t *testing.T, sq, pg memory.Store, name string, target model.LocaleID, lookup func(memory.Store) ([]memory.Match, error)) {
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

func TestMemoryParity_SQLiteVsPostgres(t *testing.T) {
	sq := openParitySQLiteMemory(t)
	pg, hasPG := maybePostgresMemory(t)
	paritySeed(t, sq)
	if hasPG {
		paritySeed(t, pg)
	}

	ctx := context.Background()
	opts := memory.LookupOptions{MaxResults: 10, MinScore: 0.3}

	cases := []struct {
		name   string
		target model.LocaleID
		lookup func(memory.Store) ([]memory.Match, error)
	}{
		{
			// Plain query "Install": the structurally-identical entry wins at
			// 1.0; the coded entry takes the tag-mismatch penalty. Postgres
			// used to give both 1.0.
			name:   "tag-mismatch-penalty",
			target: "nb",
			lookup: func(tm memory.Store) ([]memory.Match, error) {
				return tm.LookupText(ctx, "Install", "en", "nb", opts)
			},
		},
		{
			// Two exacts with differing targets both demote to ScoreNearExact
			// and flag Ambiguous. Postgres used to keep them at 1.0.
			name:   "ambiguity-demotion",
			target: "nb",
			lookup: func(tm memory.Store) ([]memory.Match, error) {
				return tm.LookupText(ctx, "Save", "en", "nb", opts)
			},
		},
		{
			// Fuzzy neighbourhood: score-then-priority-then-ID ordering must
			// match. Postgres used to sort by match-type first.
			name:   "fuzzy-ranking",
			target: "nb",
			lookup: func(tm memory.Store) ([]memory.Match, error) {
				return tm.LookupText(ctx, "Hello world", "en", "nb", opts)
			},
		},
		{
			// Structural exact via a coded query block against the coded entry.
			name:   "structural-exact-coded-query",
			target: "nb",
			lookup: func(tm memory.Store) ([]memory.Match, error) {
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

// TestMemoryParity_ExactOnlyPolicy asserts the exact-only (MinScore 1.0) policy —
// which drops ambiguous demoted matches — is identical across backends.
func TestMemoryParity_ExactOnlyPolicy(t *testing.T) {
	sq := openParitySQLiteMemory(t)
	pg, hasPG := maybePostgresMemory(t)
	paritySeed(t, sq)
	if hasPG {
		paritySeed(t, pg)
	}

	ctx := context.Background()
	strict := memory.LookupOptions{MinScore: 1.0, MaxResults: 10}
	runParity(t, sq, pg, "exact-only-ambiguous-drop", "nb", func(tm memory.Store) ([]memory.Match, error) {
		return tm.LookupText(ctx, "Save", "en", "nb", strict)
	})
	runParity(t, sq, pg, "exact-only-clean-hit", "nb", func(tm memory.Store) ([]memory.Match, error) {
		return tm.LookupText(ctx, "Install", "en", "nb", strict)
	})
}

// --- version chain ---

// versionStore is what a backend must be to answer a chain: a content memory
// that also implements the optional VersionReader capability. Both backends
// under test do, and the type assertion in openVersionParityStores is what says
// so at the point a dropped method would otherwise pass unnoticed.
type versionStore interface {
	memory.Store
	memory.VersionReader
}

// openVersionParityStores opens the SQLite store (always) and the Postgres one
// (when reachable), each as a chain-answering store.
func openVersionParityStores(t *testing.T) (versionStore, versionStore) {
	t.Helper()
	sq, ok := openParitySQLiteMemory(t).(versionStore)
	require.True(t, ok, "the SQLite content memory must answer a version chain")

	pg, hasPG := maybePostgresMemory(t)
	if !hasPG {
		return sq, nil
	}
	pgv, ok := pg.(versionStore)
	require.True(t, ok, "the Postgres content memory must answer a version chain")
	return sq, pgv
}

// versionAnswer builds one approved answer for a block: a version in the chain.
// Same shape as the framework's own fixture (memory/versions_test.go), so the
// two suites hold the backends to the same corpus.
func versionAnswer(id, unit, point, source, target, fingerprint string, at time.Time) memory.Entry {
	return memory.Entry{
		ID:          id,
		Unit:        unit,
		Point:       point,
		HintSrcLang: "en",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: source}}},
			"nb": {{Text: &model.TextRun{Text: target}}},
		},
		Origins: []memory.Origin{{
			Source:             "tool",
			AddedAt:            at,
			ContextFingerprint: fingerprint,
		}},
		CreatedAt: at,
		UpdatedAt: at,
	}
}

// versionRow is the comparable projection of a Version: the answer's identity
// and the governance that travels with it.
type versionRow struct {
	EntryID            string
	Source             string
	Target             string
	Point              string
	ContextFingerprint string
}

func projectVersions(versions []memory.Version) []versionRow {
	out := make([]versionRow, 0, len(versions))
	for _, v := range versions {
		out = append(out, versionRow{
			EntryID:            v.Entry.ID,
			Source:             v.Entry.VariantText("en"),
			Target:             v.Entry.VariantText("nb"),
			Point:              v.Entry.Point,
			ContextFingerprint: v.ContextFingerprint,
		})
	}
	return out
}

// seedVersionChain loads the identical chain corpus into a store.
func seedVersionChain(t *testing.T, tm versionStore, t0 time.Time, acme, other string) {
	t.Helper()
	ctx := t.Context()
	for _, e := range []memory.Entry{
		// One block, three successive answers at the same point.
		versionAnswer("v1", "u-1", acme, "Get started", "Kom i gang", "fp-old", t0),
		versionAnswer("v2", "u-1", acme, "Get started now", "Kom i gang nå", "fp-now", t0.Add(time.Hour)),
		versionAnswer("v3", "u-1", acme, "Get started today", "Kom i gang i dag", "fp-now", t0.Add(2*time.Hour)),
		// A different block at the same point, and the same block at another
		// point. Neither belongs to u-1's chain at acme.
		versionAnswer("o1", "u-2", acme, "Sign in", "Logg inn", "fp-now", t0),
		versionAnswer("m1", "u-1", other, "Get started", "Sett i gang", "fp-now", t0),
		// Approved before the chain existed: no unit, and so no block's history.
		versionAnswer("pre", "", "", "Get going", "Kom i vei", "", t0),
	} {
		require.NoError(t, tm.Add(ctx, e))
	}
}

// runVersionParity runs one chain query against SQLite (always) and, when a
// Postgres store is present, asserts the two backends project the same chain in
// the same order. It returns the SQLite rows so a case can also assert what the
// chain holds rather than only that the backends agree.
func runVersionParity(t *testing.T, sq, pg versionStore, name string, q memory.VersionQuery, excludeID string) []versionRow {
	t.Helper()
	sqVersions, err := sq.Versions(t.Context(), q, excludeID)
	require.NoError(t, err, "%s: sqlite", name)
	if pg == nil {
		return projectVersions(sqVersions)
	}
	pgVersions, err := pg.Versions(t.Context(), q, excludeID)
	require.NoError(t, err, "%s: postgres", name)
	require.Equal(t, projectVersions(sqVersions), projectVersions(pgVersions),
		"backend drift for %q", name)
	return projectVersions(sqVersions)
}

// TestMemoryParity_VersionChain holds the Postgres corpus to the same
// version-chain semantics as the framework's SQLite one (memory/versions_test.go).
//
// The server binds the content memory with a point and reads the chain from it
// twice: the platform translate assembly puts the prior version in the prompt,
// and the review context carries it as history. Both go empty when the backend
// stores no unit, which is a silent answer rather than a failure, so the parity
// suite is where it gets caught.
func TestMemoryParity_VersionChain(t *testing.T) {
	sq, pg := openVersionParityStores(t)
	t0 := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	acme := memory.NewPoint("acme", "support", "acme-help")
	other := memory.NewPoint("other", "email", "other-mail")

	seedVersionChain(t, sq, t0, acme, other)
	if pg != nil {
		seedVersionChain(t, pg, t0, acme, other)
	}

	t.Run("the chain is the block's prior answers, newest first", func(t *testing.T) {
		got := runVersionParity(t, sq, pg, "chain-newest-first",
			memory.VersionQuery{Unit: "u-1", Point: acme}, "v3")
		require.Len(t, got, 2)
		assert.Equal(t, "v2", got[0].EntryID)
		assert.Equal(t, "v1", got[1].EntryID)
		assert.Equal(t, "Kom i gang nå", got[0].Target)
	})

	t.Run("the answer in force is excluded", func(t *testing.T) {
		got := runVersionParity(t, sq, pg, "exclude-in-force",
			memory.VersionQuery{Unit: "u-1", Point: acme}, "v3")
		for _, v := range got {
			assert.NotEqual(t, "v3", v.EntryID, "the caller already has this one")
		}
	})

	t.Run("another block's answers are not in the chain", func(t *testing.T) {
		got := runVersionParity(t, sq, pg, "other-block-excluded",
			memory.VersionQuery{Unit: "u-1", Point: acme}, "")
		for _, v := range got {
			assert.NotEqual(t, "o1", v.EntryID)
		}
	})

	t.Run("a point narrows the chain to answers approved there", func(t *testing.T) {
		got := runVersionParity(t, sq, pg, "point-narrows",
			memory.VersionQuery{Unit: "u-1", Point: acme}, "")
		require.Len(t, got, 3)
		for _, v := range got {
			assert.Equal(t, acme, v.Point)
		}

		// No point returns the block's whole history, across every place it has
		// sat: what a report on a moved block wants.
		all := runVersionParity(t, sq, pg, "point-omitted",
			memory.VersionQuery{Unit: "u-1"}, "")
		assert.Len(t, all, 4)
	})

	t.Run("the governing context travels with the answer", func(t *testing.T) {
		got := runVersionParity(t, sq, pg, "governing-context",
			memory.VersionQuery{Unit: "u-1", Point: acme}, "v3")
		require.Len(t, got, 2)
		assert.Equal(t, "fp-now", got[0].ContextFingerprint)
		assert.Equal(t, "fp-old", got[1].ContextFingerprint)
	})

	t.Run("a limit trims from the newest end", func(t *testing.T) {
		got := runVersionParity(t, sq, pg, "limit",
			memory.VersionQuery{Unit: "u-1", Point: acme, Limit: 1}, "")
		require.Len(t, got, 1)
		assert.Equal(t, "v3", got[0].EntryID)
	})

	t.Run("answers approved before the chain existed are nobody's history", func(t *testing.T) {
		got := runVersionParity(t, sq, pg, "pre-chain-entries",
			memory.VersionQuery{Unit: "u-1"}, "")
		for _, v := range got {
			assert.NotEqual(t, "pre", v.EntryID, "an entry with no unit is not every block")
		}
	})

	t.Run("a chain needs a block", func(t *testing.T) {
		_, err := sq.Versions(t.Context(), memory.VersionQuery{Point: acme}, "")
		assert.ErrorIs(t, err, memory.ErrVersionQueryNeedsUnit)
		if pg == nil {
			return
		}
		_, err = pg.Versions(t.Context(), memory.VersionQuery{Point: acme}, "")
		assert.ErrorIs(t, err, memory.ErrVersionQueryNeedsUnit)
	})
}

// TestMemoryParity_VersionChainIsWorkspaceScoped asserts one workspace's chain
// never reaches another's. The unit is resolved per project and two workspaces
// can hold the same one, so a chain query that forgot the tenant column would
// answer with a stranger's wording and look entirely plausible doing it.
func TestMemoryParity_VersionChainIsWorkspaceScoped(t *testing.T) {
	_, pg := openVersionParityStores(t)
	if pg == nil {
		t.Skip("no Postgres reachable (BOWRAIN_TEST_POSTGRES_URL)")
	}
	_, neighbour := openVersionParityStores(t)
	require.NotNil(t, neighbour)

	t0 := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	point := memory.NewPoint("acme", "support", "acme-help")
	ctx := t.Context()
	require.NoError(t, pg.Add(ctx, versionAnswer("mine", "u-1", point, "Get started", "Kom i gang", "fp", t0)))
	require.NoError(t, neighbour.Add(ctx, versionAnswer("theirs", "u-1", point, "Get started", "Sett i gang", "fp", t0)))

	got, err := pg.Versions(ctx, memory.VersionQuery{Unit: "u-1", Point: point}, "")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "mine", got[0].Entry.ID)
}
