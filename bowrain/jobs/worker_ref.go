package jobs

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/bowrain/core/store"
	coresync "github.com/neokapi/neokapi/bowrain/core/sync"
	"github.com/neokapi/neokapi/core/ref"
)

// The worker half of the freshness ref's compare-and-swap.
//
// The commit handler makes the same assertion, and that is the one a waiting
// `kapi push` sees. This one exists because the handler answers 202 and hands
// the work to a queue: between the two, another client's push can land, and the
// governance the first client asserted is no longer what its write is about to
// overwrite. The window is small and the consequence is not — a collection
// rebound to a voice nobody chose, an approval undone — so the assertion is
// made again here, immediately before the write it guards.
//
// Each assertion covers exactly the component the write beside it changes, so a
// push carrying blocks alone is never refused by either check.

// assertContextRef refuses a context reconcile whose asserted context component
// no longer matches the collections the server holds.
func assertContextRef(ctx context.Context, deps *WorkerDeps, projectID, stream string, expected ref.Ref) error {
	if expected.Context == "" || deps.ContentStore == nil {
		return nil
	}
	collections, err := deps.ContentStore.ListCollections(ctx, projectID, stream)
	if err != nil {
		return fmt.Errorf("read collections for the context assertion: %w", err)
	}
	hashes := make(map[string]string, len(collections))
	for _, col := range collections {
		if col == nil || !coresync.IsRecipeOwned(col.Owner) {
			continue
		}
		hashes[col.Name] = col.ContextHash
	}
	current := ""
	if len(hashes) > 0 {
		current = coresync.ComputeContextHash(hashes)
	}
	return ref.Assert(ref.ComponentContext, expected.Context, current)
}

// assertDecisionsRef refuses a decisions upsert whose asserted decisions
// component no longer matches the ledger.
func assertDecisionsRef(ctx context.Context, ds store.DecisionStore, projectID, stream string, expected ref.Ref) error {
	if expected.Decisions == "" {
		return nil
	}
	current, err := ds.ListUnitDecisions(ctx, projectID, stream)
	if err != nil {
		return fmt.Errorf("read the ledger for the decisions assertion: %w", err)
	}
	return ref.Assert(ref.ComponentDecisions, expected.Decisions, coresync.DecisionsComponent(current))
}
