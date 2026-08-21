package host

import (
	"sort"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/reconcile"
)

// Identity resolution is the step between reading files and doing anything with
// what was read.
//
// A reader names blocks by what the format says — a structural address like
// `install/p#2`, a JSON key, a message id. Those names are the right thing for a
// reader to report and the wrong thing to record a decision against: for a
// format with no natural key they follow structure, so deleting the first
// paragraph of a section re-addresses every one below it. Resolving identity
// here means content is filed under a key that is MATCHED rather than named, so
// a decision, a translation and a history entry stay attached to the words they
// were made about.
//
// See core/reconcile for the grading, core/formats/markdown/naming.go for the
// limit this exists to answer, and AD-003 for why identity is matched rather
// than named.

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

// Priors is what the project already knows, as identity resolution needs it:
// the documents it holds and the units inside them.
//
// It is a parameter rather than a store because WHERE it comes from is the
// decision that matters, and it is not this function's to make. A producer
// fetches it from the venue, so a fresh clone resolves exactly as a warm one
// does; resolving against uncommitted local state instead is the mistake that
// made a cold CI runner and a reset venue two different kinds of wrong.
type Priors struct {
	Documents []reconcile.DocUnit
	Units     []reconcile.Unit
}

// ResolveIdentity matches a fresh read against what the project already knows
// and returns the durable identity of every block, writing each one onto its
// block as model.Block.Unit.
//
// byPath is what a source scan produces: blocks grouped by the file they came
// from. Documents are resolved first, because a block's context is scoped to its
// document's key — resolving that first is what stops a file rename from
// disturbing the units inside it.
//
// A block that matches a prior takes THAT prior's key, so resolution against a
// venue's existing keys returns those keys unchanged and only genuinely new
// content is minted. There is no re-keying to migrate: what changes is that a
// block whose name shifted keeps the key it had.
func ResolveIdentity(byPath map[string][]*model.Block, priors Priors) []ResolvedDocument {
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
	resolvedDocs := reconcile.Documents(docs, priors.Documents)

	out := make([]ResolvedDocument, 0, len(docs))
	for i, d := range docs {
		scope := resolvedDocs[i].Key
		// Content matching is project-wide on purpose: text moved from one file
		// to another keeps its translation, so every document is matched against
		// every unit in the prior set rather than only its own.
		results := reconcile.Blocks(scope, d.Blocks, priors.Units)

		rd := ResolvedDocument{Path: d.Path, Scope: scope, Kind: resolvedDocs[i].Kind}
		for _, r := range results {
			if r.Block != nil {
				r.Block.Unit = r.Key
			}
			rd.Blocks = append(rd.Blocks, ResolvedBlock{Block: r.Block, Unit: r.Key, Kind: r.Kind})
		}
		out = append(out, rd)
	}
	return out
}
