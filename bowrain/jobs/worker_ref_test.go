package jobs

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/core/store"
	coresync "github.com/neokapi/neokapi/bowrain/core/sync"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/ref"
)

// The worker's compare-and-swap, immediately before the writes it guards.
//
// The commit handler makes the same assertion and answers the waiting client;
// this one exists because the handler answers before the work is queued, and
// another client's push can land in between. A real store, because the check is
// a read of the rows the write is about to change.

func newRefWorkerStore(t *testing.T) (*WorkerDeps, string) {
	t.Helper()
	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "content.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })

	p := &store.Project{Name: "refs", DefaultSourceLanguage: model.LocaleEnglish}
	require.NoError(t, cs.CreateProject(t.Context(), p))
	return &WorkerDeps{ContentStore: cs}, p.ID
}

func declare(t *testing.T, deps *WorkerDeps, projectID, name, hash, owner string) {
	t.Helper()
	require.NoError(t, deps.ContentStore.CreateCollection(t.Context(), &store.Collection{
		ProjectID: projectID, Name: name, Kind: store.CollectionConnected,
		Stream: "main", Owner: owner, ContextHash: hash,
	}))
}

func TestAssertContextRef(t *testing.T) {
	deps, projectID := newRefWorkerStore(t)
	declare(t, deps, projectID, "docs", "h-docs", coresync.ContextOwnerRecipe)

	inForce := coresync.ComputeContextHash(map[string]string{"docs": "h-docs"})

	tests := []struct {
		name     string
		expected ref.Ref
		conflict bool
	}{
		{name: "no assertion always passes", expected: ref.Ref{}},
		{name: "the component in force passes", expected: ref.Ref{Context: inForce}},
		{name: "a component that moved conflicts", expected: ref.Ref{Context: "stale"}, conflict: true},
		{
			name:     "a position that moved is not asserted",
			expected: ref.Ref{Content: 999_999, Context: inForce},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := assertContextRef(t.Context(), deps, projectID, "main", tt.expected)
			if !tt.conflict {
				require.NoError(t, err)
				return
			}
			var conflict *ref.Conflict
			require.ErrorAs(t, err, &conflict)
			assert.Equal(t, ref.ComponentContext, conflict.Component)
		})
	}
}

// TestAssertContextRef_IgnoresWorkspaceOwnedCollections: a collection created
// in the hub is not the recipe's, so it must not refuse a recipe's push.
func TestAssertContextRef_IgnoresWorkspaceOwnedCollections(t *testing.T) {
	deps, projectID := newRefWorkerStore(t)
	declare(t, deps, projectID, "docs", "h-docs", coresync.ContextOwnerRecipe)
	observed := ref.Ref{Context: coresync.ComputeContextHash(map[string]string{"docs": "h-docs"})}

	declare(t, deps, projectID, "hub-only", "h-hub", coresync.ContextOwnerWorkspace)
	require.NoError(t, assertContextRef(t.Context(), deps, projectID, "main", observed))
}

func TestAssertDecisionsRef(t *testing.T) {
	deps, projectID := newRefWorkerStore(t)
	ds, ok := deps.ContentStore.(store.DecisionStore)
	require.True(t, ok)

	first := store.UnitDecision{
		ItemName: "en.json", Unit: "greeting", Variant: "fr",
		Status: "reviewed", ReviewState: "approved", DecidedBy: "ana", Updated: "2026-08-01T00:00:00Z",
	}
	_, err := ds.UpsertUnitDecisions(t.Context(), projectID, "main", []store.UnitDecision{first})
	require.NoError(t, err)

	ledger, err := ds.ListUnitDecisions(t.Context(), projectID, "main")
	require.NoError(t, err)
	inForce := coresync.DecisionsComponent(ledger)
	require.NotEmpty(t, inForce)

	require.NoError(t, assertDecisionsRef(t.Context(), ds, projectID, "main", ref.Ref{}))
	require.NoError(t, assertDecisionsRef(t.Context(), ds, projectID, "main", ref.Ref{Decisions: inForce}))

	// Another client's approval lands.
	second := first
	second.Unit = "farewell"
	second.Updated = "2026-08-02T00:00:00Z"
	_, err = ds.UpsertUnitDecisions(t.Context(), projectID, "main", []store.UnitDecision{second})
	require.NoError(t, err)

	var conflict *ref.Conflict
	require.ErrorAs(t,
		assertDecisionsRef(t.Context(), ds, projectID, "main", ref.Ref{Decisions: inForce}),
		&conflict)
	assert.Equal(t, ref.ComponentDecisions, conflict.Component)
}

// TestClientLedgerFoldMatchesTheServers is the symmetry the decisions component
// depends on: a client that folds the record it sent and a server that folds
// the ledger it stored must agree, or a push would carry the ledger forever and
// every assertion would conflict.
func TestClientLedgerFoldMatchesTheServers(t *testing.T) {
	deps, projectID := newRefWorkerStore(t)
	ds := deps.ContentStore.(store.DecisionStore)

	sent := []store.UnitDecision{
		{ItemName: "en.json", Unit: "farewell", Variant: "fr", Status: "translated",
			ReviewState: "approved", DecidedBy: "ben", Updated: "2026-08-02T00:00:00Z"},
		{ItemName: "en.json", Unit: "greeting", Variant: "fr", Status: "reviewed",
			ReviewState: "approved", DecidedBy: "ana", Note: "house wording", Updated: "2026-08-01T00:00:00Z"},
	}
	_, err := ds.UpsertUnitDecisions(t.Context(), projectID, "main", sent)
	require.NoError(t, err)

	stored, err := ds.ListUnitDecisions(t.Context(), projectID, "main")
	require.NoError(t, err)
	assert.Equal(t, coresync.DecisionsComponent(sent), coresync.DecisionsComponent(stored))
}
