// Package reconcile matches a freshly-read set of blocks against the units a
// project already knows about, so identity survives ordinary editing.
//
// The problem it solves. Every reader has to name the blocks it emits, and the
// two available answers both fail. Name a block by its position ("para3") and
// deleting an earlier paragraph renames everything below it, so a recorded
// decision silently names a different block. Name it by its content and editing
// a typo renames it, so the block reads as a deletion plus an addition and
// loses the history it should have kept. Choosing between them only picks which
// way identity breaks.
//
// So identity is not a naming scheme here. It is the OUTPUT of matching this
// read against the previous one, the way rename detection works on files: keep
// both signals, independently, and grade the pair.
//
//	content   context   meaning                    what carries over
//	match     match     untouched                  everything
//	match     differ    moved, or reused elsewhere identity and history
//	differ    match     edited in place            identity and history; target is stale
//	differ    differ    genuinely new              nothing
//
// The third row is the one no single key can express, and the reason this
// package exists. The fourth is honest rather than clever: when both signals
// change at once nothing links the new block to the old one, and a guess would
// attach a real decision to the wrong words.
//
// This lives beside the framework rather than inside the readers on purpose.
// AD-003 puts the reconciling of format-native ids into a stable identifier at
// the layer that persists them, and keeps the readers reporting what the format
// actually says. Readers stay unchanged: their positional names are a perfectly
// good CONTEXT signal, and are consumed as one here.
package reconcile

import (
	"strconv"

	"github.com/neokapi/neokapi/core/model"
)

// Unit is a content unit the project already knows about, as the persisting
// layer remembers it. Callers supply the units carried forward from earlier
// reads — including units whose blocks are not in the current read, since that
// is what lets a removed block return to its own history later.
type Unit struct {
	Key         string
	ContentHash string
	ContextHash string
}

// Kind is how a block in the current read relates to what came before.
type Kind int

const (
	// New has no counterpart: either the project has never seen it, or both of
	// its signals changed at once and nothing reliable links it back.
	New Kind = iota
	// Unchanged matched on both signals.
	Unchanged
	// Moved kept its words while its surroundings changed — an earlier block was
	// added or removed, or the same text is reused elsewhere. Any recorded
	// translation still applies.
	Moved
	// Edited kept its place while its words changed. Identity and history follow
	// it, but an existing translation is now stale and has to be redone.
	Edited
)

func (k Kind) String() string {
	switch k {
	case Unchanged:
		return "unchanged"
	case Moved:
		return "moved"
	case Edited:
		return "edited"
	default:
		return "new"
	}
}

// Result is one block's resolved identity.
type Result struct {
	Block *model.Block
	Key   string
	Kind  Kind
}

// Identify returns the two hashes reconcile matches on, for one block in one
// document.
//
// scope names the document the block was read from. It exists because a block's
// name is only unique inside its own file: every markdown document has a
// "para1" and every plaintext document a "line1". Without it, a paragraph in one
// file could claim the history of an unrelated paragraph in another, which in a
// project of a few hundred documents is not an edge case but the common case.
//
// Scope binds the CONTEXT hash only. The content hash stays document-free on
// purpose, so text moved from one file to another is still recognised as the
// same words and keeps its translation.
func Identify(scope string, b *model.Block) Unit {
	props := make(map[string]string, len(b.Properties)+1)
	for k, v := range b.Properties {
		props[k] = v
	}
	props[scopeProp] = scope
	return Unit{
		ContentHash: model.ComputeContentHash(b.SourceText()),
		ContextHash: model.ComputeContextHash(b.Name, b.Type, props),
	}
}

// scopeProp carries the document into the context hash. It is namespaced so it
// cannot be confused with a property a format actually produced.
const scopeProp = "\x00reconcile.scope"

// Blocks resolves each block in current to a stable key, in the order given.
//
// scope names the document these blocks were read from; see Identify. Callers
// reconcile one document at a time, but may pass the whole project's prior
// units, which is what lets content moved between files keep its identity.
//
// Matching runs strongest signal first — both hashes, then context alone, then
// content alone — and each pass consumes the priors it claims. A prior is
// therefore claimed at most once: two blocks can never resolve to the same key,
// which is what keeps one block's approval from silently approving another.
func Blocks(scope string, current []*model.Block, prior []Unit) []Result {
	pool := newPool(prior)
	out := make([]Result, len(current))
	ids := make([]Unit, len(current))
	for i, b := range current {
		ids[i] = Identify(scope, b)
		out[i] = Result{Block: b}
	}

	// Strongest signal first, so an exact match is never stolen by a weaker one.
	// Both passes below run over the blocks left unresolved by the pass above.
	resolve(out, ids, func(id Unit) (string, bool) {
		return pool.take(bothKey(id.ContentHash, id.ContextHash))
	}, Unchanged)

	// Content before context, because the two signals are not equally strong. A
	// content hash identifies the exact words; a context hash for a prose format
	// is mostly a positional name like "para2", which collides freely across
	// blocks that have nothing to do with each other.
	//
	// The ordering decides a genuinely ambiguous case. A paragraph is deleted, so
	// the one below it slides up into the vacated slot: its words match the old
	// lower block, its position matches the old deleted one. Taking content first
	// reads that as the block moving, which is what happened. Taking context
	// first would read it as the deleted block being rewritten into the words of
	// its neighbour, and would then hand the neighbour's own history to whatever
	// arrived next.
	//
	// An edit in place is not weakened by this: its words have changed, so it has
	// no content match to lose, and the context pass below still claims it.
	resolve(out, ids, func(id Unit) (string, bool) {
		return pool.take(contentKey(id.ContentHash))
	}, Moved)

	resolve(out, ids, func(id Unit) (string, bool) {
		return pool.take(ctxKey(id.ContextHash))
	}, Edited)

	mint(out, ids)
	return out
}

// resolve fills in every still-unresolved block that lookup can claim.
func resolve(out []Result, ids []Unit, lookup func(Unit) (string, bool), kind Kind) {
	for i := range out {
		if out[i].Key != "" {
			continue
		}
		if key, ok := lookup(ids[i]); ok {
			out[i].Key, out[i].Kind = key, kind
		}
	}
}

// mint assigns keys to the blocks nothing claimed. Derived from both hashes so
// a project with no history reconciles to the same keys on a second run, with
// an ordinal for blocks that are identical in both signals.
func mint(out []Result, ids []Unit) {
	seen := map[string]int{}
	for i := range out {
		if out[i].Key != "" {
			continue
		}
		key := "u-" + model.ComputeContentHash(ids[i].ContentHash + "|" + ids[i].ContextHash)[:16]
		seen[key]++
		if n := seen[key]; n > 1 {
			key += "-" + strconv.Itoa(n)
		}
		out[i].Key, out[i].Kind = key, New
	}
}

// pool hands out each prior unit at most once, across all three passes.
type pool struct {
	byKey map[string][]string // lookup key -> unclaimed unit keys, in order
	taken map[string]bool
}

func newPool(prior []Unit) *pool {
	p := &pool{byKey: map[string][]string{}, taken: map[string]bool{}}
	for _, u := range prior {
		p.byKey[bothKey(u.ContentHash, u.ContextHash)] = append(p.byKey[bothKey(u.ContentHash, u.ContextHash)], u.Key)
		p.byKey[ctxKey(u.ContextHash)] = append(p.byKey[ctxKey(u.ContextHash)], u.Key)
		p.byKey[contentKey(u.ContentHash)] = append(p.byKey[contentKey(u.ContentHash)], u.Key)
	}
	return p
}

// take returns the first unit under lookup that no earlier pass has claimed.
// A unit appears under three lookups, so entries already taken are skipped
// rather than removed when claimed elsewhere.
func (p *pool) take(lookup string) (string, bool) {
	queue := p.byKey[lookup]
	for i, key := range queue {
		if p.taken[key] {
			continue
		}
		p.taken[key] = true
		p.byKey[lookup] = queue[i+1:]
		return key, true
	}
	p.byKey[lookup] = nil
	return "", false
}

func bothKey(content, context string) string { return "b:" + content + "|" + context }
func ctxKey(context string) string           { return "x:" + context }
func contentKey(content string) string       { return "c:" + content }
