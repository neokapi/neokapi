package venue

import (
	"path"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/reconcile"
)

// The tree is what a venue holds and what a producer read, said in one shape.
//
// A push used to be a negotiation: the producer sent its hashes, the venue
// answered with verdicts, and the producer descended one level per item to
// learn which blocks were wanted — sequentially, once per item, before a byte
// of content moved. A first push has no cache, so every item was new, and a
// few hundred files cost a few hundred round trips.
//
// The producer was never missing the capability to work this out. It was
// missing the venue's side of the comparison, and asking per item is not how
// you get it — fetching is. One request answers with the whole tree at a known
// ref; the producer diffs its scan against that locally and knows exactly which
// blocks are missing. Two network operations, and the content is one of them.
//
// The tree is also what makes a push able to say what is GONE. A payload of
// changed blocks cannot express a deletion — a removed string simply stops
// being sent — so the venue, which upserts what arrives, kept it forever. A
// declared tree says what each item holds NOW, and a declared scope says which
// items the producer is speaking for, so absence within that scope is an
// answer rather than a silence.

// TreeItem is one item's content, reduced to hashes.
//
// The three lists are parallel and in document order. Order is not decoration:
// a rename is recognised by how much of a document's content survived, and a
// map of hashes has no order to compare.
type TreeItem struct {
	Path string `json:"path"`
	// ID is the venue's identity for this item — its name is only the address
	// the item currently sits at. Served by a venue; a producer leaves it empty,
	// because which document an entry IS is the venue's to say.
	//
	// It is the scope block identity resolves under, and that is why it has to
	// travel. Scoping by PATH would undo document resolution: a renamed file
	// would find no priors under its new path and every block inside it would
	// mint a fresh unit — the exact orphaning that resolving documents first
	// exists to prevent.
	ID string `json:"id,omitempty"`
	// Keys are the durable block keys (what the venue stores as source_id),
	// which is what a prune is expressed in.
	Keys []string `json:"keys"`
	// Content is model.ComputeContentHash of each block's source text — the
	// same value the venue stores as a block's content hash, which is what lets
	// a renamed file be recognised by what is inside it.
	Content []string `json:"content"`
	// Record folds content and context together: the transfer hash, and so the
	// answer to "does the venue already hold this block". Omitted from a
	// producer's declaration, which the venue reads for identity rather than
	// for transfer.
	Record []string `json:"record,omitempty"`
	// Context is each block's context hash — its name, type and properties.
	// Served by a venue, omitted by a producer.
	//
	// It is here so a producer can resolve block identity against what the
	// venue holds rather than against its own uncommitted local state. Content
	// and context are the two signals reconciliation grades independently, so
	// both have to travel; Record cannot stand in for Context because it is a
	// fold, and a fold cannot be taken apart.
	Context []string `json:"context,omitempty"`
}

// Tree is a stream's items, keyed by path.
type Tree map[string]TreeItem

// TreeFromBlocks reduces what a producer just read to the tree it declares.
//
// Every item scanned appears, including one that read to nothing: a file whose
// last translatable string was deleted is precisely the case a declaration
// exists for, and dropping it would leave the emptiest item the only one that
// never gets cleaned.
func TreeFromBlocks(blocksByItem map[string][]*model.Block) Tree {
	if len(blocksByItem) == 0 {
		return nil
	}
	out := make(Tree, len(blocksByItem))
	for item, blocks := range blocksByItem {
		if item == "" {
			continue
		}
		ti := TreeItem{Path: item}
		for _, b := range blocks {
			if b == nil {
				continue
			}
			key := convergence.BlockKey(b)
			if key == "" {
				continue
			}
			ti.Keys = append(ti.Keys, key)
			ti.Content = append(ti.Content, model.ComputeContentHash(b.SourceText()))
		}
		out[item] = ti
	}
	return out
}

// Units renders the tree as the prior/current shape document reconciliation
// speaks, in path order so two runs over one tree resolve identically.
//
// A venue's tree carries an id per item and so yields keyed priors; a
// producer's declaration carries none and yields the unkeyed "current" side.
// Identity is the venue's to assign, and the shape says so.
func (t Tree) Units() []reconcile.DocUnit {
	units := make([]reconcile.DocUnit, 0, len(t))
	for _, p := range t.paths() {
		units = append(units, reconcile.DocUnit{
			Key: t[p].ID, Path: p, Content: t[p].Content,
		})
	}
	return units
}

// paths is the tree's paths in a stable order.
func (t Tree) paths() []string {
	out := make([]string, 0, len(t))
	for p := range t {
		out = append(out, p)
	}
	slices.Sort(out)
	return out
}

// Records is every transfer hash the tree holds, across all items.
//
// A push asks this of the venue's tree and then sends only the blocks whose
// record hash is not in it. Global rather than per-item on purpose: a block
// that moved between files is content the venue already has, and asking per
// item would upload it again to a new name.
func (t Tree) Records() map[string]struct{} {
	out := map[string]struct{}{}
	for _, ti := range t {
		for _, r := range ti.Record {
			if r != "" {
				out[r] = struct{}{}
			}
		}
	}
	return out
}

// Priors renders the tree as the prior set block reconciliation matches against.
//
// This is the point of serving context hashes. A producer resolving identity
// needs to know what the venue already holds, and the honest source for that is
// the venue — not the producer's own cache, which a fresh clone does not have
// and another machine's push invalidates. Fetched priors make a cold checkout
// and a warm one resolve to the same units.
//
// The scope is the item's path, which is what qualifies a context match: a
// block's name is only unique inside its own document.
func (t Tree) Priors() []reconcile.Unit {
	var out []reconcile.Unit
	for _, p := range t.paths() {
		ti := t[p]
		// The document's identity, not its address — see TreeItem.ID.
		scope := ti.ID
		if scope == "" {
			scope = p
		}
		for i, key := range ti.Keys {
			if i >= len(ti.Content) || i >= len(ti.Context) {
				// A venue too old to serve context hashes offers no priors to
				// match on. Resolution then mints, which is the same answer a
				// producer reached before it could ask at all.
				break
			}
			out = append(out, reconcile.Unit{
				Key:         key,
				Scope:       scope,
				ContentHash: ti.Content[i],
				ContextHash: ti.Context[i],
			})
		}
	}
	return out
}

// BlockKeys renders the tree as the item → declared keys map a prune reads.
func (t Tree) BlockKeys() map[string][]string {
	if len(t) == 0 {
		return nil
	}
	out := make(map[string][]string, len(t))
	for p, ti := range t {
		keys := slices.Clone(ti.Keys)
		slices.Sort(keys)
		out[p] = slices.Compact(keys)
	}
	return out
}

// Scope is the set of paths a push is authoritative over: the recipe's globs,
// or the resolved paths of a scoped push.
//
// It is what turns absence into an answer. Without it, an item the venue holds
// and the producer did not mention could mean either "deleted" or "this push
// was not looking there", and the venue has to assume the second or risk
// deleting a project. With it, the two are distinguishable by construction —
// which is what makes a scoped push safe because of what it declares rather
// than because of what nobody looked at.
type Scope []string

// Covers reports whether a path falls inside the scope.
//
// An empty scope covers nothing. That is the safe reading and the deliberate
// one: a producer too old to declare a scope makes no claim about what is
// missing, so its push stays purely additive, exactly as it was before scopes
// existed.
//
// A pattern prefixed with `!` excludes, and an exclusion beats every include —
// the gitignore convention, because the thing being carried is the same thing:
// a recipe's globs and the exclusions that qualify them. Without them a build
// directory swept into a glob and then excluded would read as content the
// source had deleted.
func (s Scope) Covers(itemPath string) bool {
	included := false
	for _, pat := range s {
		if pat == "" {
			continue
		}
		if negated, found := strings.CutPrefix(pat, "!"); found {
			if matchScope(negated, itemPath) {
				return false
			}
			continue
		}
		if matchScope(pat, itemPath) {
			included = true
		}
	}
	return included
}

// matchScope accepts a glob (path.Match, with `**` meaning any depth) or a
// directory prefix, so a recipe's globs and a `push <dir>` both say what they
// mean without the caller translating between them.
func matchScope(pattern, itemPath string) bool {
	pattern = strings.TrimSuffix(pattern, "/")
	if pattern == "**" || pattern == "." {
		return true
	}
	// A directory names everything under it.
	if !strings.ContainsAny(pattern, "*?[") {
		return itemPath == pattern || strings.HasPrefix(itemPath, pattern+"/")
	}
	if ok, err := path.Match(pattern, itemPath); err == nil && ok {
		return true
	}
	// `**` spans separators, which path.Match does not: compare the literal
	// segments either side of it.
	if before, after, found := strings.Cut(pattern, "**"); found {
		before = strings.TrimSuffix(before, "/")
		after = strings.TrimPrefix(after, "/")
		if before != "" && !strings.HasPrefix(itemPath, before+"/") && itemPath != before {
			return false
		}
		if after == "" {
			return true
		}
		rest := strings.TrimPrefix(strings.TrimPrefix(itemPath, before), "/")
		for rest != "" {
			if ok, err := path.Match(after, rest); err == nil && ok {
				return true
			}
			_, rest, _ = strings.Cut(rest, "/")
		}
		return false
	}
	return false
}
