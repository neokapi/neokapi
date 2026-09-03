package jobs

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/core/ref"
	"github.com/neokapi/neokapi/core/venue"
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
		if col == nil || !venue.IsRecipeOwned(col.Owner) {
			continue
		}
		hashes[col.Name] = col.ContextHash
	}
	current := ""
	if len(hashes) > 0 {
		current = venue.ComputeContextHash(hashes)
	}
	return ref.Assert(ref.ComponentContext, expected.Context, current)
}

// decisionLedgerReader is the one verb the assertion needs. Narrowing to it is
// what lets a push assert through the transaction it is about to write in —
// making the check a compare-and-swap — while a caller outside a transition
// passes the store.
type decisionLedgerReader interface {
	ListUnitDecisions(ctx context.Context, projectID, stream string) ([]venue.UnitDecision, error)
}

// assertDecisionsRef refuses a decisions upsert whose asserted decisions
// component no longer matches the ledger.
func assertDecisionsRef(ctx context.Context, ds decisionLedgerReader, projectID, stream string, expected ref.Ref) error {
	if expected.Decisions == "" {
		return nil
	}
	current, err := ds.ListUnitDecisions(ctx, projectID, stream)
	if err != nil {
		return fmt.Errorf("read the ledger for the decisions assertion: %w", err)
	}
	return assertDecisionsHeld(expected, current)
}

// assertDecisionsHeld is the assertion itself, over a ledger the caller has
// already read. The push transition reads it once and asks it two questions,
// so the read is the caller's rather than this function's.
func assertDecisionsHeld(expected ref.Ref, held []venue.UnitDecision) error {
	if expected.Decisions == "" {
		return nil
	}
	return ref.Assert(ref.ComponentDecisions, expected.Decisions, venue.DecisionsComponent(held))
}
