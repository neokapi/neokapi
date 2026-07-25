package memory_test

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/memory"
	"github.com/stretchr/testify/require"
)

// mustCount returns tm.Count, failing the test on error. It keeps the
// context-threaded Count signature out of the inline assertions that used
// to call the old no-arg, error-free Count.
func mustCount(t *testing.T, tm memory.ContentMemory) int {
	t.Helper()
	n, err := tm.Count(context.Background())
	require.NoError(t, err)
	return n
}

// mustGetEntry returns (entry, ok), failing the test on a query error.
func mustGetEntry(t *testing.T, tm memory.Store, id string) (memory.Entry, bool) {
	t.Helper()
	e, ok, err := tm.GetEntry(context.Background(), id)
	require.NoError(t, err)
	return e, ok
}

// mustEntries returns all entries, failing the test on error.
func mustEntries(t *testing.T, tm memory.Store) []memory.Entry {
	t.Helper()
	entries, err := tm.Entries(context.Background())
	require.NoError(t, err)
	return entries
}

// mustGetImportSession returns (session, ok), failing the test on error.
func mustGetImportSession(t *testing.T, tm memory.Store, id string) (memory.ImportSession, bool) {
	t.Helper()
	s, ok, err := tm.GetImportSession(context.Background(), id)
	require.NoError(t, err)
	return s, ok
}

// mustFacetStats returns the full facet stats, failing the test on error.
func mustFacetStats(t *testing.T, tm memory.Store) memory.FacetData {
	t.Helper()
	d, err := tm.FacetStats(context.Background())
	require.NoError(t, err)
	return d
}

// mustLocaleStats returns locale stats, failing the test on error.
func mustLocaleStats(t *testing.T, tm memory.Store) []memory.LocaleFacet {
	t.Helper()
	s, err := tm.LocaleStats(context.Background())
	require.NoError(t, err)
	return s
}

// mustListImportSessions returns all import sessions, failing on error.
func mustListImportSessions(t *testing.T, tm memory.Store) []memory.ImportSession {
	t.Helper()
	s, err := tm.ListImportSessions(context.Background())
	require.NoError(t, err)
	return s
}

// mustFindImportSessionByHash returns (session, ok), failing on error.
func mustFindImportSessionByHash(t *testing.T, tm memory.Store, hash string) (memory.ImportSession, bool) {
	t.Helper()
	s, ok, err := tm.FindImportSessionByHash(context.Background(), hash)
	require.NoError(t, err)
	return s, ok
}
