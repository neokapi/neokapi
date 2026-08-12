package sync

import (
	"context"
	"errors"
	"fmt"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	coresync "github.com/neokapi/neokapi/bowrain/core/sync"
	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/ref"
	"github.com/neokapi/neokapi/terms"
)

// The server half of the freshness ref: computing, from stored state, the ref a
// stream currently stands at.
//
// Every component is DERIVED, never stored. That is the property that makes the
// compare-and-swap correct without a column to keep in step: a component is a
// pure function of the rows it describes, so it moves in the same transaction
// as the write that moved them, and no path exists on which a write lands and
// its component does not follow. A stored component would be a second copy of
// the same fact, and the two would eventually disagree — which is precisely the
// failure the ref replaces.

// TermsSource is the slice of a workspace terminology store the terms component
// reads. terms.Terminology satisfies it; a deployment with no workspace terms
// store passes nil and publishes no terms component.
type TermsSource interface {
	Concepts(ctx context.Context) ([]terms.Concept, error)
	ListRelations(ctx context.Context, scope *graph.Scope) ([]terms.ConceptRelation, error)
}

// The real workspace terminology store satisfies the slice this file reads.
var _ TermsSource = (terms.Terminology)(nil)

// RefSource is what a ref is computed from.
type RefSource struct {
	// Content holds the stream's change feed, its collections and its decision
	// ledger. Required.
	Content platstore.ContentStore
	// Terms holds the workspace's terminology. Optional: an unclaimed project
	// has no workspace to read one from, and its terms component is empty —
	// which compares as unknown, not as "no terminology".
	Terms TermsSource
}

// CurrentRef computes the ref the server publishes for one stream.
//
// A component whose source cannot be read is left empty rather than guessed. An
// empty component compares as unknown, so a degraded read costs a client the
// answer to one question and never gives it a wrong one.
func CurrentRef(ctx context.Context, src RefSource, projectID, stream string) (ref.Ref, error) {
	if src.Content == nil {
		return ref.Ref{}, errors.New("sync: ref requires a content store")
	}

	var out ref.Ref

	cursor, err := src.Content.LatestCursor(ctx, projectID, stream)
	if err != nil {
		return ref.Ref{}, fmt.Errorf("ref content component: %w", err)
	}
	out.Content = cursor

	declared, err := contextComponent(ctx, src.Content, projectID, stream)
	if err != nil {
		return ref.Ref{}, err
	}
	out.Context = declared

	decisions, err := decisionsComponent(ctx, src.Content, projectID, stream)
	if err != nil {
		return ref.Ref{}, err
	}
	out.Decisions = decisions

	vocabulary, err := TermsComponentOf(ctx, src.Terms)
	if err != nil {
		return ref.Ref{}, err
	}
	out.Terms = vocabulary

	return out, nil
}

// contextComponent folds the recipe-owned collections the server holds.
//
// Only recipe-owned rows are folded, and that is the same restriction the push
// negotiation makes: a workspace-owned collection was never the recipe's to
// declare, so counting it would make every project whose hub created one read
// as permanently diverged from a recipe that is perfectly current.
func contextComponent(ctx context.Context, cs platstore.ContentStore, projectID, stream string) (string, error) {
	collections, err := cs.ListCollections(ctx, projectID, stream)
	if err != nil {
		return "", fmt.Errorf("ref context component: %w", err)
	}
	hashes := make(map[string]string, len(collections))
	for _, col := range collections {
		if col == nil || !coresync.IsRecipeOwned(col.Owner) {
			continue
		}
		hashes[col.Name] = col.ContextHash
	}
	if len(hashes) == 0 {
		// A project with no recipe-owned collections makes no claim about the
		// declared context. Folding the empty set would publish the constant
		// empty-fold hash, and a client that declares nothing would then read
		// "current" against a server that has simply never been told.
		return "", nil
	}
	return coresync.ComputeContextHash(hashes), nil
}

// decisionsComponent folds the stream's decision ledger.
func decisionsComponent(ctx context.Context, cs platstore.ContentStore, projectID, stream string) (string, error) {
	ds, ok := cs.(platstore.DecisionStore)
	if !ok {
		return "", nil
	}
	decisions, err := ds.ListUnitDecisions(ctx, projectID, stream)
	if err != nil {
		return "", fmt.Errorf("ref decisions component: %w", err)
	}
	return coresync.DecisionsComponent(decisions), nil
}

// TermsComponentOf folds a workspace's terminology into the terms component. It
// is exported because the terms component is asserted on a governance write
// that never touches a project — a terminology change-set — and that path has a
// terms store and no stream.
func TermsComponentOf(ctx context.Context, source TermsSource) (string, error) {
	if source == nil {
		return "", nil
	}
	concepts, err := source.Concepts(ctx)
	if err != nil {
		return "", fmt.Errorf("ref terms component: %w", err)
	}
	relations, err := source.ListRelations(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("ref terms component relations: %w", err)
	}
	return coresync.TermsComponent(concepts, relations), nil
}
