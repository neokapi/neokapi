package server

import (
	"testing"

	"github.com/gokapi/gokapi/bowrain/auth"
	"github.com/gokapi/gokapi/bowrain/service"
	bstore "github.com/gokapi/gokapi/bowrain/store"
	"github.com/gokapi/gokapi/core/sievepen"
	"github.com/gokapi/gokapi/core/termbase"
	"github.com/stretchr/testify/require"
)

// initTestStores wires up in-memory SQLite stores on the server for testing.
// The server is PostgreSQL-only in production, but tests use SQLite for speed
// and isolation. It also installs factory functions on wsStores so that
// getTM/getTB create in-memory stores instead of requiring PostgreSQL.
func initTestStores(t *testing.T, srv *Server) {
	t.Helper()

	cs, err := bstore.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	srv.ContentStore = cs
	srv.Services = service.NewServices(cs, srv.ConnectorReg, srv.FormatRegistry, srv.ToolRegistry)

	if srv.Config.JWTSecret != "" {
		as, err := auth.NewSQLiteAuthStore(":memory:")
		require.NoError(t, err)
		srv.AuthStore = as
		srv.Services.Auth = service.NewAuthService(as, srv.Config.JWTSecret)
	}

	// Install factory functions for in-memory TM/TB stores.
	srv.wsStores.tmFactory = func() sievepen.TMStore {
		return &testTMStore{sievepen.NewInMemoryTM()}
	}
	srv.wsStores.tbFactory = func() termbase.TBStore {
		return &testTermStore{termbase.NewInMemoryTermBase()}
	}
}

// testTMStore wraps InMemoryTM to satisfy the TMStore interface for tests.
type testTMStore struct {
	*sievepen.InMemoryTM
}

func (t *testTMStore) AddWithStream(entry sievepen.TMEntry, _ string) error {
	return t.Add(entry)
}

func (t *testTMStore) SearchEntries(query, sourceLocale, targetLocale string, offset, limit int) ([]sievepen.TMEntry, int) {
	all := t.Entries()
	return filterTMEntries(all, query, sourceLocale, targetLocale, offset, limit)
}

func (t *testTMStore) SearchEntriesForStream(query, sourceLocale, targetLocale, _ string, _ []string, offset, limit int) ([]sievepen.TMEntry, int) {
	return t.SearchEntries(query, sourceLocale, targetLocale, offset, limit)
}

func (t *testTMStore) GetEntry(id string) (sievepen.TMEntry, bool) {
	for _, e := range t.Entries() {
		if e.ID == id {
			return e, true
		}
	}
	return sievepen.TMEntry{}, false
}

func filterTMEntries(entries []sievepen.TMEntry, query, sourceLocale, targetLocale string, offset, limit int) ([]sievepen.TMEntry, int) {
	var filtered []sievepen.TMEntry
	for _, e := range entries {
		if sourceLocale != "" && string(e.SourceLocale) != sourceLocale {
			continue
		}
		if targetLocale != "" && string(e.TargetLocale) != targetLocale {
			continue
		}
		filtered = append(filtered, e)
	}
	total := len(filtered)
	if offset >= total {
		return nil, total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return filtered[offset:end], total
}

// testTermStore wraps InMemoryTermBase to satisfy the TBStore interface for tests.
type testTermStore struct {
	*termbase.InMemoryTermBase
}

func (t *testTermStore) AddConceptWithStream(concept termbase.Concept, _ string) error {
	return t.AddConcept(concept)
}

func (t *testTermStore) SearchForStream(query, sourceLocale, targetLocale, _ string, _ []string, offset, limit int) ([]termbase.Concept, int) {
	return t.Search(query, sourceLocale, targetLocale, offset, limit)
}
