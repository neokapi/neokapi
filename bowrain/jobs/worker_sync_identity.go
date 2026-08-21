package jobs

import (
	"context"
	"fmt"
	"sort"

	"github.com/neokapi/neokapi/bowrain/core/store"
	bowsync "github.com/neokapi/neokapi/bowrain/sync"
	"github.com/neokapi/neokapi/core/reconcile"
	"github.com/neokapi/neokapi/core/venue"
)

// Which document is which, decided here.
//
// A producer computing WHAT CONTENT TO SEND is safe: it is an optimisation, and
// content addressing verifies it, so a wrong answer is a missing object the
// commit catches. Computing WHAT A DOCUMENT IS is not. A rename grafts one
// document's approvals onto another, so "these two files are the same" is a
// governance claim across a trust boundary, and a buggy or hostile producer
// must not get to make it. The venue computes the transition — added, removed,
// renamed — from the tree it was declared, against the content it holds.
//
// The two halves it decides are the two a payload of changed blocks cannot
// express. A file that moved sends no block at all when its content is
// unchanged, and a file that was deleted sends nothing by definition; both are
// invisible to anything that reads only what arrived.

// identityPlan is what this push does to the project's item identities, decided
// against the state the push started from and applied inside its transition.
type identityPlan struct {
	// Renames maps item id → the path the declaration puts it at.
	Renames map[string]string
	// Removals are items the declared scope covers and the declaration does not
	// mention.
	Removals []string
	// Reads the resolved path an item's blocks should be written under, keyed
	// by the path the producer declared. It is the identity, not the address:
	// a renamed item is renamed BEFORE content lands, so the blocks in the pack
	// are written to the document that already carries the approvals.
	StoreAs map[string]string
}

// planIdentity resolves the declared tree against what the venue holds.
//
// It reads pre-push state, like the conflict checks it runs beside: the
// expectations a push carries are about the state it was built against, and
// answering them from a half-applied transition would compare a producer's
// claim against content this same push had just produced.
func planIdentity(
	ctx context.Context,
	deps *WorkerDeps,
	projectID, stream string,
	scope venue.Scope,
	declared venue.Tree,
) (*identityPlan, error) {
	plan := &identityPlan{
		Renames: map[string]string{},
		StoreAs: map[string]string{},
	}
	// A push that declares no tree makes no claim about the project's shape. It
	// adds and updates, exactly as a push did before trees existed.
	if len(declared) == 0 {
		return plan, nil
	}

	diffEngine := bowsync.NewDiffEngine(deps.ContentStore, deps.SyncCache)
	// The venue's whole tree, not the scope's: a file moved INTO the scope from
	// outside it is still the same document, and a scoped read would see only
	// its arrival.
	stored, itemIDs, err := diffEngine.LoadTree(ctx, projectID, stream, nil)
	if err != nil {
		return nil, fmt.Errorf("read the project tree to resolve identity: %w", err)
	}

	resolved := reconcile.DocumentUnits(
		declared.Units(nil),
		stored.Units(func(p string) string { return itemIDs[p] }),
	)

	claimed := map[string]bool{}
	for _, r := range resolved {
		plan.StoreAs[r.Path] = r.Path
		if r.Kind != reconcile.Moved {
			// Unchanged and Edited resolve to the item already at this path;
			// New has no prior to claim. Neither moves anything.
			if r.Kind != reconcile.New {
				claimed[r.Key] = true
			}
			continue
		}
		plan.Renames[r.Key] = r.Path
		claimed[r.Key] = true
	}

	// Whatever the scope covers and the declaration does not name is gone. This
	// is the half that was computed and thrown away for as long as a push had
	// no scope: without one, an item absent from a push means "not looked at"
	// as readily as "removed", and acting on it would be data loss.
	//
	// An item claimed by a rename is not a removal — it is the same document,
	// now at another path, and its old path is vacated by the rename itself.
	for path, id := range itemIDs {
		if declared[path].Path != "" || claimed[id] {
			continue
		}
		if !scope.Covers(path) {
			continue
		}
		plan.Removals = append(plan.Removals, path)
	}
	sort.Strings(plan.Removals)
	return plan, nil
}

// apply performs the plan on the push's transaction.
//
// Renames run first, and before any content lands: a moved file's blocks
// belong to the document that already holds its approvals, and writing them
// under the new path before the item moved there would mint a second one.
// Removals run last, after the pack and the prune, so an item emptied by this
// push is removed by the declaration rather than by the order of operations.
func (p *identityPlan) applyRenames(ctx context.Context, tx store.PushApplier, projectID, stream string) error {
	ids := make([]string, 0, len(p.Renames))
	for id := range p.Renames {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err := tx.RenameItem(ctx, projectID, stream, id, p.Renames[id]); err != nil {
			return err
		}
	}
	return nil
}

func (p *identityPlan) applyRemovals(ctx context.Context, tx store.PushApplier, projectID, stream string) (int, error) {
	for _, path := range p.Removals {
		if err := tx.DeleteItem(ctx, projectID, stream, path); err != nil {
			return 0, fmt.Errorf("remove item %q the source no longer has: %w", path, err)
		}
	}
	return len(p.Removals), nil
}
