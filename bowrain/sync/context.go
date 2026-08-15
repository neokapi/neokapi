package sync

import (
	"context"
	"fmt"
	"slices"

	"github.com/neokapi/neokapi/core/venue"
)

// The server half of the context content type's negotiation: the diff between
// the context a push declares and the collections the server holds.
//
// It is the third level of the same comparison the block path already makes —
// root hash, item hashes, block hashes — and it works the same way: a hash the
// client computed over what it declares, against a hash the server folds from
// what it stored. The one asymmetry is deliberate: an item the client omits is
// reported and ignored, while a collection the client omits is reported and
// ignored *and cannot be inferred away*, because a collection is where content
// lives.

// ContextDiffResult describes how a push's declared context compares with the
// collections the server holds.
type ContextDiffResult struct {
	// Changed reports whether the declared context differs from the server's.
	// False means the recipe's structure and governance are already in force
	// and the manifest need not carry them.
	Changed bool

	// Undeclared lists recipe-owned collections the server holds that this push
	// does not declare. REPORTED, NEVER ACTED ON — nothing in this package or
	// downstream of it deletes a collection on the strength of this list.
	//
	// Workspace-owned collections are deliberately absent: they were never the
	// recipe's to declare, so their absence from a push says nothing.
	Undeclared []string
}

// CompareContext performs the context-level comparison. clientContextHash is
// the fold the client computed over its entries (core/venue.ContextHashOf);
// declared is the collection names it carries.
//
// An empty clientContextHash means the push makes no claim about the declared
// context — a content-only caller with no recipe to read. That is reported as
// unchanged with nothing undeclared, so such a push can never look like a
// recipe that just dropped every collection.
func (d *DiffEngine) CompareContext(ctx context.Context, projectID, stream, clientContextHash string, declared []string) (*ContextDiffResult, error) {
	if clientContextHash == "" {
		return &ContextDiffResult{}, nil
	}

	collections, err := d.contentStore.ListCollections(ctx, projectID, stream)
	if err != nil {
		return nil, fmt.Errorf("load collections for context diff: %w", err)
	}

	serverHashes := make(map[string]string, len(collections))
	for _, col := range collections {
		if col == nil || !venue.IsRecipeOwned(col.Owner) {
			continue
		}
		serverHashes[col.Name] = col.ContextHash
	}

	result := &ContextDiffResult{
		Changed: venue.ComputeContextHash(serverHashes) != clientContextHash,
	}
	for name := range serverHashes {
		if !slices.Contains(declared, name) {
			result.Undeclared = append(result.Undeclared, name)
		}
	}
	slices.Sort(result.Undeclared)
	return result, nil
}
