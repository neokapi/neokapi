package termbase_test

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/termbase"
	"github.com/stretchr/testify/require"
)

// mustCount returns tb.Count, failing the test on error.
func mustCount(t *testing.T, tb termbase.TermBase) int {
	t.Helper()
	n, err := tb.Count(context.Background())
	require.NoError(t, err)
	return n
}

// mustGetConcept returns (concept, ok), failing the test on a query error.
func mustGetConcept(t *testing.T, tb termbase.TermBase, id string) (termbase.Concept, bool) {
	t.Helper()
	c, ok, err := tb.GetConcept(context.Background(), id)
	require.NoError(t, err)
	return c, ok
}

// mustConcepts returns all concepts, failing the test on error.
func mustConcepts(t *testing.T, tb termbase.TermBase) []termbase.Concept {
	t.Helper()
	c, err := tb.Concepts(context.Background())
	require.NoError(t, err)
	return c
}

// mustLookup returns the term matches, failing the test on error.
func mustLookup(t *testing.T, tb termbase.TermBase, src string, opts termbase.LookupOptions) []termbase.TermMatch {
	t.Helper()
	m, err := tb.Lookup(context.Background(), src, opts)
	require.NoError(t, err)
	return m
}

// mustLookupAll returns all term matches, failing the test on error.
func mustLookupAll(t *testing.T, tb termbase.TermBase, src string, opts termbase.LookupOptions) []termbase.TermMatch {
	t.Helper()
	m, err := tb.LookupAll(context.Background(), src, opts)
	require.NoError(t, err)
	return m
}

// mustSearch returns (concepts, total), failing the test on error.
func mustSearch(t *testing.T, tb termbase.TermBase, query string, src, tgt model.LocaleID, offset, limit int) ([]termbase.Concept, int) {
	t.Helper()
	c, total, err := tb.Search(context.Background(), query, src, tgt, offset, limit)
	require.NoError(t, err)
	return c, total
}

// mustSearchForStream returns (concepts, total), failing the test on error.
func mustSearchForStream(t *testing.T, tb termbase.TBStore, query string, src, tgt model.LocaleID, stream string, chain []string, offset, limit int) ([]termbase.Concept, int) {
	t.Helper()
	c, total, err := tb.SearchForStream(context.Background(), query, src, tgt, stream, chain, offset, limit)
	require.NoError(t, err)
	return c, total
}
