package host

import (
	"context"
	"errors"
	"sort"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/reconcile"
	"github.com/neokapi/neokapi/core/state"
)

// Identity resolution is the step between reading files and doing anything with
// what was read.
//
// A reader names blocks by what the format says — "para3", "line5", a JSON key.
// Those names are the right thing for a reader to report and the wrong thing to
// record a decision against: for a format with no natural key they follow
// position, so deleting a paragraph renames everything below it. Resolving
// identity here means every path that ingests content — push, extract, review —
// records decisions against the same durable unit.
//
// See core/reconcile for the grading, and AD-003 for why identity is matched
// rather than named.

// ResolvedBlock is one block's durable identity and how it relates to the last
// time the project saw it.
type ResolvedBlock struct {
	Block *model.Block
	Unit  string
	Kind  reconcile.Kind
}

// ResolvedDocument is one file's durable identity and its blocks'.
type ResolvedDocument struct {
	Path   string
	Scope  string
	Kind   reconcile.Kind
	Blocks []ResolvedBlock
}

// ResolveIdentity matches a fresh read against what the project already knows
// and persists the result, returning the durable identity of every block.
//
// byPath is what a source scan produces: blocks grouped by the file they came
// from. Documents are resolved first, because a block's context is scoped to its
// document's key — resolving that first is what stops a file rename from
// disturbing the units inside it.
//
// Identity is written back to the working store so the next read has priors to
// match against. It is staged, not committed: the caller commits once, at the
// end of the run.
func ResolveIdentity(ctx context.Context, st *state.WorkStore, byPath map[string][]*model.Block) ([]ResolvedDocument, error) {
	if st == nil {
		return nil, errors.New("resolve identity: no state store")
	}

	// Deterministic order, so two runs over the same tree mint the same keys and
	// a project's identity does not depend on map iteration.
	paths := make([]string, 0, len(byPath))
	for p := range byPath {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	docs := make([]reconcile.Document, 0, len(paths))
	for _, p := range paths {
		docs = append(docs, reconcile.Document{Path: p, Blocks: byPath[p]})
	}

	priorDocs, err := documentPriors(ctx, st)
	if err != nil {
		return nil, err
	}
	resolvedDocs := reconcile.Documents(docs, priorDocs)

	// Content matching is project-wide on purpose: text moved from one file to
	// another keeps its translation, so every document is matched against every
	// unit the project holds, not just its own.
	priors, err := unitPriors(ctx, st)
	if err != nil {
		return nil, err
	}

	out := make([]ResolvedDocument, 0, len(docs))
	for i, d := range docs {
		scope := resolvedDocs[i].Key
		results := reconcile.Blocks(scope, d.Blocks, priors)

		if err := st.PutDocument(ctx, scope, d.Path); err != nil {
			return nil, err
		}
		rd := ResolvedDocument{Path: d.Path, Scope: scope, Kind: resolvedDocs[i].Kind}
		for _, r := range results {
			if err := stageIdentity(ctx, st, scope, r); err != nil {
				return nil, err
			}
			rd.Blocks = append(rd.Blocks, ResolvedBlock{Block: r.Block, Unit: r.Key, Kind: r.Kind})
		}
		out = append(out, rd)
	}
	return out, nil
}

// stageIdentity records a block's resolved identity, preserving any decision
// already held for the unit.
//
// An edited block is the case that matters: its identity and history follow it,
// but the translation that was approved was approved for different words, so the
// decision is cleared rather than left looking current. The previous wording
// stays reachable through the content memory, which is where recycling belongs.
func stageIdentity(ctx context.Context, st *state.WorkStore, scope string, r reconcile.Result) error {
	id := reconcile.Identify(scope, r.Block)
	key := state.Key{Scope: scope, Unit: r.Key, Variant: model.VariantKey{}}

	u, _ := st.Get(ctx, key)
	u.Unit = r.Key
	u.Scope = scope
	u.ContentHash = id.ContentHash
	u.ContextHash = id.ContextHash
	u.Updated = nowRFC3339()

	if r.Kind == reconcile.Edited {
		u.Decision = state.Decision{}
		u.TargetHash = ""
		u.Status = ""
		u.AIReview = nil
	}
	return st.Put(ctx, u)
}

// unitPriors is every unit the project knows, as reconcile's prior set.
func unitPriors(ctx context.Context, st *state.WorkStore) ([]reconcile.Unit, error) {
	all, err := st.All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]reconcile.Unit, 0, len(all))
	for _, u := range all {
		if u.ContentHash == "" && u.ContextHash == "" {
			continue // recorded before identity was tracked; nothing to match on
		}
		out = append(out, reconcile.Unit{
			Key: u.Unit, ContentHash: u.ContentHash, ContextHash: u.ContextHash,
		})
	}
	return out, nil
}

// documentPriors rebuilds the known documents: their contents from the units
// they hold, their current path from the document record.
//
// The path has to be stored. It cannot be derived from the units — nothing in a
// unit says where its file lives — and path is the strongest signal for matching
// a document, so a prior without one would fall through to content similarity
// every time and the ordering that makes a rename cheap would never fire.
func documentPriors(ctx context.Context, st *state.WorkStore) ([]reconcile.DocUnit, error) {
	all, err := st.All(ctx)
	if err != nil {
		return nil, err
	}
	byScope := map[string][]string{}
	for _, u := range all {
		if u.Scope == "" || u.ContentHash == "" {
			continue
		}
		byScope[u.Scope] = append(byScope[u.Scope], u.ContentHash)
	}

	scopes := make([]string, 0, len(byScope))
	for s := range byScope {
		scopes = append(scopes, s)
	}
	sort.Strings(scopes)

	paths, err := st.DocumentPaths(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]reconcile.DocUnit, 0, len(scopes))
	for _, s := range scopes {
		out = append(out, reconcile.DocUnit{Key: s, Path: paths[s], Content: byScope[s]})
	}
	return out, nil
}
