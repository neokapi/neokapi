package server

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkspaceStores_ConcurrentFirstUseOpensOneStore covers the lazy opening in
// getMemory/getTerms, which is the code every editor, review and knowledge
// handler reaches before it can touch a workspace's content memory or terms.
//
// A page load fires several of those handlers at once, so first use is
// concurrent by construction: the entry is empty for all of them, and if the
// nil check and the assignment are not under one lock, each request opens its
// own store and then overwrites the field the others just wrote. That is a data
// race on the field (-race says so) and, in production where the store is
// PostgreSQL-backed, a burst of duplicate opens — each one running migrations
// and taking connections from the same pool.
//
// The assertions hold without the race detector too: the factory has to be
// called once and every caller has to be handed that same store.
func TestWorkspaceStores_ConcurrentFirstUseOpensOneStore(t *testing.T) {
	const goroutines = 32
	// The in-memory factory returns instantly, which the production one does
	// not — a PostgreSQL store takes a connection and migrates. Without a cost
	// to opening, the first goroutine can finish before the second is
	// scheduled, and an unsynchronized cache passes the count assertion by
	// accident. openCost restores the window the real store has. It is paid
	// once when the cache is correct, so it does not slow the passing run.
	const openCost = 20 * time.Millisecond

	tests := []struct {
		name string
		// install wires a counting factory and returns the accessor under test,
		// boxed to any so both store types share one table.
		install func(ws *workspaceStores, opens *atomic.Int64) func(slug string) (any, error)
	}{
		{
			name: "content memory",
			install: func(ws *workspaceStores, opens *atomic.Int64) func(string) (any, error) {
				ws.memoryFactory = func() memory.Store {
					opens.Add(1)
					time.Sleep(openCost)
					return &testMemoryStore{memory.NewInMemoryStore()}
				}
				return func(slug string) (any, error) { return ws.getMemory(slug) }
			},
		},
		{
			name: "terms",
			install: func(ws *workspaceStores, opens *atomic.Int64) func(string) (any, error) {
				ws.termsFactory = func() terms.Store {
					opens.Add(1)
					time.Sleep(openCost)
					return newTestTermsStore()
				}
				return func(slug string) (any, error) { return ws.getTerms(slug) }
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ws := newWorkspaceStores()
			var opens atomic.Int64
			get := tc.install(ws, &opens)

			var start sync.WaitGroup
			start.Add(1)
			var done sync.WaitGroup
			got := make([]any, goroutines)
			errs := make([]error, goroutines)
			for i := range goroutines {
				done.Add(1)
				go func() {
					defer done.Done()
					start.Wait()
					got[i], errs[i] = get("acme")
				}()
			}
			// Release them together, so every goroutine finds the entry empty.
			start.Done()
			done.Wait()

			for i := range goroutines {
				require.NoError(t, errs[i])
				require.NotNil(t, got[i])
				assert.Same(t, got[0], got[i], "every caller must be handed the same store")
			}
			assert.Equal(t, int64(1), opens.Load(), "the store must be opened exactly once")
		})
	}
}

// TestWorkspaceStores_DistinctWorkspacesGetDistinctStores pins the other half of
// the caching contract the per-entry lock must not break: the entry is keyed by
// workspace slug, so two workspaces never share a store.
func TestWorkspaceStores_DistinctWorkspacesGetDistinctStores(t *testing.T) {
	ws := newWorkspaceStores()
	ws.memoryFactory = func() memory.Store { return &testMemoryStore{memory.NewInMemoryStore()} }
	ws.termsFactory = func() terms.Store { return newTestTermsStore() }

	acmeMemory, err := ws.getMemory("acme")
	require.NoError(t, err)
	globexMemory, err := ws.getMemory("globex")
	require.NoError(t, err)
	assert.NotSame(t, acmeMemory, globexMemory)

	acmeTerms, err := ws.getTerms("acme")
	require.NoError(t, err)
	globexTerms, err := ws.getTerms("globex")
	require.NoError(t, err)
	assert.NotSame(t, acmeTerms, globexTerms)

	// And a second call for a slug already opened returns the cached store
	// rather than opening a new one.
	again, err := ws.getMemory("acme")
	require.NoError(t, err)
	assert.Same(t, acmeMemory, again)
}
