//go:build integration

package terms_test

// Cross-backend parity: the same fixture corpus and the same lookups must
// produce identical term match sets, scores, ordering and validity filtering on
// the framework SQLite terms and the bowrain Postgres terms. Both stores
// delegate ranking to terms.LookupTiered / terms.LookupAllTiered, so any
// divergence here is a real drift regression.
//
// This suite historically FAILED before the dialect-seam refactor: the Postgres
// terms ignored term validity (valid_from/valid_to) in its fuzzy, exact and
// normalized paths — while SQLite honored it — and its LookupAll omitted the
// (text, position) de-duplication. It is the acceptance gate for keeping the two
// backends in lockstep.
//
// The fuzzy validity fixtures use identical term TEXT differing only in
// validity, so the two backends' distinct fuzzy retrieval engines (SQLite FTS5
// trigram vs Postgres pg_trgm) retrieve the same rows — isolating the shared
// validity/scoring/dedup fix from cross-engine retrieval nuances. The corpus's
// "Install" terms also clear the fuzzy tier's MinScore against "installations"
// (a 0.5385 Levenshtein ratio), so the fuzzy cases below expect them: the
// candidate pool is bounded by terms.FuzzyLengthWindow, which admits exactly
// what the score gate can keep, on both backends. Fixtures set
// no ProjectID: the Postgres terms is workspace-scoped and has no project_id
// column, so project scoping is a SQLite/local concern and is not exercised
// here.
//
// Requires a Postgres (BOWRAIN_TEST_POSTGRES_URL, or the local default) and the
// fts5 build tag for the SQLite side: `go test -tags "integration fts5"`.

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"

	storage "github.com/neokapi/neokapi/bowrain/storage"
	pgtb "github.com/neokapi/neokapi/bowrain/terms"

	"github.com/stretchr/testify/require"
)

// tbRow is the comparable projection of a TermMatch: everything the ranking and
// filtering policy controls, backend-independent.
type tbRow struct {
	ConceptID   string
	Text        string
	Score       float64 // rounded to 4 dp
	MatchType   model.MatchStrategy
	HasValidity bool
	Start       int
	End         int
}

// projectTB projects and canonically sorts matches: ties among equal-score /
// equal-position matches are backend-order-dependent, so the parity comparison
// is a set comparison keyed by (concept, text, position).
func projectTB(matches []terms.TermMatch) []tbRow {
	out := make([]tbRow, 0, len(matches))
	for _, m := range matches {
		out = append(out, tbRow{
			ConceptID:   m.Concept.ID,
			Text:        m.Term.Text,
			Score:       math.Round(m.Score*1e4) / 1e4,
			MatchType:   m.MatchType,
			HasValidity: m.Term.Validity != nil,
			Start:       m.Position.Start,
			End:         m.Position.End,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Start != out[j].Start {
			return out[i].Start < out[j].Start
		}
		if out[i].ConceptID != out[j].ConceptID {
			return out[i].ConceptID < out[j].ConceptID
		}
		return out[i].Text < out[j].Text
	})
	return out
}

// openParitySQLiteTB opens a throwaway in-memory SQLite terms.
func openParitySQLiteTB(t *testing.T) terms.Terminology {
	t.Helper()
	tb, err := terms.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = tb.Close() })
	return tb
}

// maybePostgresTB opens a Postgres terms if one is reachable, returning (nil,
// false) otherwise — unlike openTestPostgresTerms it does NOT skip the test,
// so the SQLite side of the parity fixtures always exercises even without PG.
// The full cross-backend comparison runs only when Postgres is present (CI).
func maybePostgresTB(t *testing.T) (terms.Terminology, bool) {
	t.Helper()
	connStr := os.Getenv("BOWRAIN_TEST_POSTGRES_URL")
	if connStr == "" {
		connStr = "postgres://bowrain:bowrain@localhost:5432/bowrain_test?sslmode=disable"
	}
	db, err := storage.OpenPostgres(connStr)
	if err != nil {
		return nil, false
	}
	wsID := fmt.Sprintf("parity-%s-%d", t.Name(), time.Now().UnixNano())
	tb, err := pgtb.NewPostgresStoreFromDB(db, wsID)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM tb_terms WHERE workspace_id = $1", wsID)
		_, _ = db.Exec("DELETE FROM tb_concepts WHERE workspace_id = $1", wsID)
		_ = db.Close()
	})
	return tb, true
}

// enTerm builds a single-English-term concept, optionally with a validity.
func enTerm(id, text string, validity *graph.Validity) terms.Concept {
	return terms.Concept{
		ID:     id,
		Domain: "software",
		Terms: []terms.Term{
			{Text: text, Locale: "en", Status: model.TermApproved, Validity: validity},
		},
	}
}

// paritySeed loads the identical fixture corpus into a store. past/future bound
// two validity windows around "now"; the fuzzy validity fixtures share the same
// term text so both backends retrieve the same rows.
func paritySeed(t *testing.T, tb terms.Terminology) {
	t.Helper()
	ctx := context.Background()
	past := time.Now().Add(-48 * time.Hour)
	future := time.Now().Add(48 * time.Hour)
	expired := &graph.Validity{ValidTo: &past}    // no longer valid at "now"
	notYet := &graph.Validity{ValidFrom: &future} // not yet valid at "now"

	concepts := []terms.Concept{
		// Exact-tier validity: same text, one always-valid, one expired.
		enTerm("c-install-valid", "Install", nil),
		enTerm("c-install-expired", "Install", expired),
		// Fuzzy-tier validity (the headline drift): identical text so both
		// engines retrieve identically; one valid, one expired, one not-yet.
		enTerm("c-installation-valid", "installation", nil),
		enTerm("c-installation-expired", "installation", expired),
		enTerm("c-installation-future", "installation", notYet),
		// LookupAll de-duplication: same term text in two concepts.
		enTerm("c-save-a", "Save", nil),
		enTerm("c-save-b", "Save", nil),
	}
	for _, c := range concepts {
		require.NoError(t, tb.AddConcept(ctx, c))
	}
}

// runParity runs a named lookup against the SQLite store (always) and, when a
// Postgres store is present, asserts the two backends project identical result
// rows. Without Postgres it still exercises the SQLite fixture path so the
// corpus/lookup code is locally covered.
func runParity(t *testing.T, sq, pg terms.Terminology, name string, lookup func(terms.Terminology) ([]terms.TermMatch, error)) {
	t.Helper()
	sqMatches, err := lookup(sq)
	require.NoError(t, err, "%s: sqlite", name)
	if pg == nil {
		return
	}
	pgMatches, err := lookup(pg)
	require.NoError(t, err, "%s: postgres", name)
	require.Equal(t, projectTB(sqMatches), projectTB(pgMatches), "backend drift for %q", name)
}

func TestTermsParity_SQLiteVsPostgres(t *testing.T) {
	sq := openParitySQLiteTB(t)
	pg, hasPG := maybePostgresTB(t)
	paritySeed(t, sq)
	if hasPG {
		paritySeed(t, pg)
	}

	ctx := context.Background()
	now := graph.ScopeAt(time.Now())
	fuzzy := []model.MatchStrategy{model.MatchStrategyFuzzy}

	cases := []struct {
		name   string
		lookup func(terms.Terminology) ([]terms.TermMatch, error)
	}{
		{
			// Exact "Install" under a now-scope: the expired copy is filtered on
			// both backends.
			name: "exact-validity-now",
			lookup: func(tb terms.Terminology) ([]terms.TermMatch, error) {
				return tb.Lookup(ctx, "Install", terms.LookupOptions{SourceLocale: "en", Scope: &now})
			},
		},
		{
			// Exact "Install" with no scope: both copies returned on both backends.
			name: "exact-no-scope",
			lookup: func(tb terms.Terminology) ([]terms.TermMatch, error) {
				return tb.Lookup(ctx, "Install", terms.LookupOptions{SourceLocale: "en"})
			},
		},
		{
			// Fuzzy "installations" under a now-scope: the expired and not-yet
			// copies are filtered — the specific drift the content memory pass flagged as
			// deferred (Postgres fuzzy ignored validity entirely).
			name: "fuzzy-validity-now",
			lookup: func(tb terms.Terminology) ([]terms.TermMatch, error) {
				return tb.Lookup(ctx, "installations", terms.LookupOptions{SourceLocale: "en", MinScore: 0.5, MatchModes: fuzzy, Scope: &now})
			},
		},
		{
			// Fuzzy "installations" with no scope: all three copies returned.
			name: "fuzzy-no-scope",
			lookup: func(tb terms.Terminology) ([]terms.TermMatch, error) {
				return tb.Lookup(ctx, "installations", terms.LookupOptions{SourceLocale: "en", MinScore: 0.5, MatchModes: fuzzy})
			},
		},
		{
			// LookupAll: "Save" occurs once in the text but in two concepts — the
			// (text, position) de-dup collapses it to a single match on both
			// backends.
			name: "lookupall-dedup",
			lookup: func(tb terms.Terminology) ([]terms.TermMatch, error) {
				return tb.LookupAll(ctx, "Please Save now", terms.LookupOptions{SourceLocale: "en"})
			},
		},
		{
			// LookupAll under a now-scope: expired terms are not reported.
			name: "lookupall-validity-now",
			lookup: func(tb terms.Terminology) ([]terms.TermMatch, error) {
				return tb.LookupAll(ctx, "Install the installation", terms.LookupOptions{SourceLocale: "en", Scope: &now})
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runParity(t, sq, pg, tc.name, tc.lookup)
		})
	}
}

// conceptIDs collects the (sorted, unique) concept IDs of a match set.
func conceptIDs(matches []terms.TermMatch) []string {
	seen := map[string]bool{}
	var ids []string
	for _, m := range matches {
		if !seen[m.Concept.ID] {
			seen[m.Concept.ID] = true
			ids = append(ids, m.Concept.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

// TestTermsParity_ExpectedSets pins the concrete result sets the parity fixtures
// must produce, so the drift-fix behavior (validity filtering on every tier,
// LookupAll de-duplication) is locked even when Postgres is not present. When
// Postgres IS present, TestTermsParity_SQLiteVsPostgres additionally asserts the
// Postgres backend produces byte-for-byte the same rows.
func TestTermsParity_ExpectedSets(t *testing.T) {
	sq := openParitySQLiteTB(t)
	pg, hasPG := maybePostgresTB(t)
	paritySeed(t, sq)
	if hasPG {
		paritySeed(t, pg)
	}

	ctx := context.Background()
	now := graph.ScopeAt(time.Now())
	fuzzy := []model.MatchStrategy{model.MatchStrategyFuzzy}

	cases := []struct {
		name   string
		want   []string
		lookup func(terms.Terminology) ([]terms.TermMatch, error)
	}{
		{
			name: "exact-validity-now excludes expired",
			want: []string{"c-install-valid"},
			lookup: func(tb terms.Terminology) ([]terms.TermMatch, error) {
				return tb.Lookup(ctx, "Install", terms.LookupOptions{SourceLocale: "en", Scope: &now})
			},
		},
		{
			name: "exact-no-scope keeps expired",
			want: []string{"c-install-expired", "c-install-valid"},
			lookup: func(tb terms.Terminology) ([]terms.TermMatch, error) {
				return tb.Lookup(ctx, "Install", terms.LookupOptions{SourceLocale: "en"})
			},
		},
		{
			// MinScore 0.5 is low enough to admit the "Install" fixtures too
			// (LevenshteinRatio("installations", "install") = 0.5385), so this
			// case pins validity filtering across BOTH texts: the expired and
			// not-yet copies drop, the always-valid ones stay.
			name: "fuzzy-validity-now excludes expired and not-yet",
			want: []string{"c-install-valid", "c-installation-valid"},
			lookup: func(tb terms.Terminology) ([]terms.TermMatch, error) {
				return tb.Lookup(ctx, "installations", terms.LookupOptions{SourceLocale: "en", MinScore: 0.5, MatchModes: fuzzy, Scope: &now})
			},
		},
		{
			name: "fuzzy-no-scope keeps every scoring copy",
			want: []string{"c-install-expired", "c-install-valid", "c-installation-expired", "c-installation-future", "c-installation-valid"},
			lookup: func(tb terms.Terminology) ([]terms.TermMatch, error) {
				return tb.Lookup(ctx, "installations", terms.LookupOptions{SourceLocale: "en", MinScore: 0.5, MatchModes: fuzzy})
			},
		},
		{
			name: "lookupall-dedup collapses to one concept",
			want: []string{"c-save-a"},
			lookup: func(tb terms.Terminology) ([]terms.TermMatch, error) {
				return tb.LookupAll(ctx, "Please Save now", terms.LookupOptions{SourceLocale: "en"})
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sqMatches, err := tc.lookup(sq)
			require.NoError(t, err)
			require.Equal(t, tc.want, conceptIDs(sqMatches), "sqlite %s", tc.name)
			if hasPG {
				pgMatches, err := tc.lookup(pg)
				require.NoError(t, err)
				require.Equal(t, tc.want, conceptIDs(pgMatches), "postgres %s", tc.name)
			}
		})
	}
}
