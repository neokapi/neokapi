package server

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sqlterms "github.com/neokapi/neokapi/bowrain/terms"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
)

// TestPostgresTerms_EveryWriteMovesTheRevision pins the invalidation key the
// term gate's snapshot cache keeps a snapshot under: every write through the
// PostgreSQL terms store gives the workspace's terms a revision never issued
// before, a read leaves it, a write that fails leaves it, and a workspace's
// revision is its own.
func TestPostgresTerms_EveryWriteMovesTheRevision(t *testing.T) {
	db := pgtest.NewTestDB(t)
	ctx := t.Context()
	tb, err := sqlterms.NewPostgresStoreFromDB(db, "ws-revision")
	require.NoError(t, err)
	other, err := sqlterms.NewPostgresStoreFromDB(db, "ws-other")
	require.NoError(t, err)

	rev, err := tb.Revision(ctx)
	require.NoError(t, err)
	require.Empty(t, rev, "no write has recorded a revision yet")

	seen := map[string]bool{}
	moved := func(t *testing.T, what string) {
		t.Helper()
		got, err := tb.Revision(ctx)
		require.NoError(t, err)
		require.NotEmpty(t, got, what)
		assert.False(t, seen[got], "%s gives a revision never issued before", what)
		seen[got] = true
		rev = got
	}
	held := func(t *testing.T, what string) {
		t.Helper()
		got, err := tb.Revision(ctx)
		require.NoError(t, err)
		assert.Equal(t, rev, got, "%s leaves the revision", what)
	}

	require.NoError(t, tb.AddConcept(ctx, termConcept("c-kapi", "kapi")))
	moved(t, "adding a concept")
	require.NoError(t, tb.AddConcept(ctx, termConcept("c-shop", "shop")))
	moved(t, "adding another")
	require.NoError(t, tb.AddConcept(ctx, termConcept("c-kapi", "Kapi")))
	moved(t, "rewriting a concept's terms")
	require.NoError(t, tb.AddRelation(ctx, terms.ConceptRelation{ID: "r-1", SourceID: "c-kapi", TargetID: "c-shop", RelationType: graph.LabelRelated}))
	moved(t, "adding a relation")
	require.NoError(t, tb.DeleteRelation(ctx, "r-1"))
	moved(t, "deleting a relation")

	_, err = tb.Concepts(ctx)
	require.NoError(t, err)
	_, err = tb.LookupAll(ctx, "kapi", terms.LookupOptions{SourceLocale: model.LocaleEnglish})
	require.NoError(t, err)
	held(t, "reading the terms")
	require.Error(t, tb.DeleteConcept(ctx, "c-missing"))
	held(t, "a delete that fails")
	require.NoError(t, other.AddConcept(ctx, termConcept("c-other", "other")))
	held(t, "another workspace's write")

	require.NoError(t, tb.DeleteConcept(ctx, "c-shop"))
	moved(t, "deleting a concept")
}

// countingPostgresTerms counts the reads of a PostgreSQL terms store's whole
// terms.
type countingPostgresTerms struct {
	*sqlterms.PostgresStore
	reads int
}

func (c *countingPostgresTerms) Concepts(ctx context.Context) ([]terms.Concept, error) {
	c.reads++
	return c.PostgresStore.Concepts(ctx)
}

// TestTermSnapshotCache_ReadsPostgresTermsOncePerRevision drives the cache
// over a PostgreSQL terms store of 1,000 concepts: the terms are read once
// while nothing writes them, and again after a write, which the snapshot then
// holds. It logs what a call costs read whole and through the cache.
func TestTermSnapshotCache_ReadsPostgresTermsOncePerRevision(t *testing.T) {
	db := pgtest.NewTestDB(t)
	ctx := t.Context()
	pg, err := sqlterms.NewPostgresStoreFromDB(db, "ws-cache")
	require.NoError(t, err)
	for i := range 1000 {
		require.NoError(t, pg.AddConcept(ctx, terms.Concept{ID: fmt.Sprintf("c-%04d", i), Source: terms.TermSourceTerminology,
			Terms: []terms.Term{
				{Text: fmt.Sprintf("term %d", i), Locale: "en", Status: model.TermPreferred},
				{Text: fmt.Sprintf("terme %d", i), Locale: "fr", Status: model.TermPreferred},
				{Text: fmt.Sprintf("old term %d", i), Locale: "en", Status: model.TermForbidden},
			}}))
	}
	tb := &countingPostgresTerms{PostgresStore: pg}
	var c termSnapshotCache

	timed := func(fn func()) time.Duration {
		best := time.Duration(1 << 62)
		for range 5 {
			start := time.Now()
			fn()
			best = min(best, time.Since(start))
		}
		return best
	}
	whole := timed(func() { _, _ = snapshotTerms(ctx, tb) })
	tb.reads = 0

	snap, fp := c.snapshot(ctx, "ws-cache", tb)
	require.NotNil(t, snap)
	cached := timed(func() { _, _ = c.snapshot(ctx, "ws-cache", tb) })
	assert.Equal(t, 1, tb.reads, "the terms are read once while nothing writes them")
	t.Logf("a call reading 1,000 concepts whole: %s; through the cache at an unchanged revision: %s", whole, cached)
	assert.Less(t, cached, whole, "a call at an unchanged revision reads only the revision")

	require.NoError(t, pg.AddConcept(ctx, termConcept("c-new", "fresh")))
	snap2, fp2 := c.snapshot(ctx, "ws-cache", tb)
	assert.Equal(t, 2, tb.reads, "a write moves the revision, so the next call reads again")
	assert.NotEqual(t, fp, fp2)
	got, err := snap2.Concepts(ctx)
	require.NoError(t, err)
	assert.Len(t, got, 1001, "and the snapshot holds the write")
}
