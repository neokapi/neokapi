package host

import (
	"context"

	"github.com/neokapi/neokapi/core/reconcile"
)

// DocumentIndex answers which document a source file IS, for one run.
//
// It is resolved once and read many times. Every decision recorded during a run
// and every decision read back has to name the same document the same way, and
// a per-unit lookup would ask the store the same question once per block.
//
// The zero value answers every path with the key that path derives to
// (reconcile.DocumentKeyFor), which is the key the next extraction mints for it.
// So a project that has recorded nothing yet and one that has recorded
// everything name the same file the same way, and a fresh clone can read its own
// committed record before it has extracted anything.
type DocumentIndex struct {
	byPath map[string]string
}

// DocumentIndex reads the project's document identities. A project with no
// store, or a build with none, yields the zero index rather than an error —
// resolving a decision's scope must not be the thing that fails a run.
func (a *App) DocumentIndex(ctx context.Context, root string) (DocumentIndex, error) {
	if root == "" {
		return DocumentIndex{}, nil
	}
	db, err := a.ProjectDB(ctx, root)
	if err != nil {
		return DocumentIndex{}, err
	}
	work := db.Work()
	if work == nil {
		return DocumentIndex{}, nil
	}
	docs, err := work.Documents(ctx)
	if err != nil {
		return DocumentIndex{}, err
	}
	if len(docs) == 0 {
		return DocumentIndex{}, nil
	}
	byPath := make(map[string]string, len(docs))
	for _, d := range docs {
		byPath[d.Path] = d.Key
	}
	return DocumentIndex{byPath: byPath}, nil
}

// documentIndexOrEmpty resolves the index and swallows a store failure, for the
// read paths where a missing index means "decisions are keyed by path" rather
// than "this run cannot continue".
func (a *App) documentIndexOrEmpty(ctx context.Context, root string) DocumentIndex {
	idx, err := a.DocumentIndex(ctx, root)
	if err != nil {
		return DocumentIndex{}
	}
	return idx
}

// DocumentScope is the one-shot form, for a caller resolving a single unit
// rather than walking a project: it reads the index and asks it. A caller in a
// loop should hold a DocumentIndex instead, so the store is asked once.
func (a *App) DocumentScope(ctx context.Context, root, sourcePath string) string {
	return a.documentIndexOrEmpty(ctx, root).Scope(root, sourcePath)
}

// Scope is the document a unit's decisions belong to: the durable key the
// project resolved for its SOURCE file, or the file's project-relative path
// where no key is known.
//
// It is half of a decision's identity, not a label beside it. A reader names
// blocks by what the format gives it, and for prose those names follow position
// — so every page of a docs collection carries an `h`, a `p` and an `fm_title`,
// and a decision recorded against the id alone belongs to whichever page was
// processed last. The source path rather than the target's is what the unit key
// lives in and what the connector names items by.
//
// The key rather than the path, because a path is where a document lives and
// not what it is. Keying on the address means renaming a file detaches every
// approval inside it, silently — the venue stopped doing that when it learned
// to reconcile a declared tree, and this is the same fix on the local side.
// A file the store has no key for is named by the key its path derives to,
// which is the one the next extraction mints for it. Falling back to the path
// instead made a project's own committed record unreadable until it had been
// extracted once: on a fresh clone every decision and every recorded basis is
// filed under a document key, the store that maps keys to paths is derived and
// absent, and a run reading paths matched none of them. Deriving the key is
// exact wherever the store is silent, because silence means nothing prior
// claims the file and minting is a function of the path.
func (d DocumentIndex) Scope(root, sourcePath string) string {
	rel := DecisionScope(root, sourcePath)
	if key, ok := d.byPath[rel]; ok && key != "" {
		return key
	}
	return reconcile.DocumentKeyFor(rel)
}
