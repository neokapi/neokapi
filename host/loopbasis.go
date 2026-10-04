package host

import (
	"context"
	"sync"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/history"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
)

// The basis of an undecided translation.
//
// A decision carries the source it blessed, so a source rewrite under a decided
// translation is derived on read: the decision's basis no longer matches the
// wording in front of the reader, the unit reads stale, coverage withholds the
// scope, and the loop re-drafts it. An undecided translation needs the same
// anchor, and the record of the flow that wrote it provides one: every
// document a flow writes is recorded as a content.edit (host/flowchanges.go),
// and each derived edition's transition in the block history carries the
// source it was made from (its content hash, beside the source revision as
// its basis), the revision of the translation it left, and the stamp of the
// tool that produced it.
//
// So the loop's own basis is the row of the block history that left the
// edition at the revision it holds now (history.Store.Wrote), and it answers
// only while two things hold:
//
//   - A tool wrote it from a recorded source: a flow, or kapi pull bringing
//     down a venue's translation whose record names the source the checkout
//     held. A translation a person or an agent wrote, a pulled one the venue
//     made from other wording, or one that was in the tree before kapi ever
//     ran, is not the loop's work: it grades basisNone or basisUnknown and is
//     never re-drafted. The loop does not get to claim authorship of
//     somebody's work by reading it.
//   - The file holds what the flow wrote: a recorded change left the edition
//     at its revision now. A translation somebody has since rewritten
//     describes work they took over. The history is shared by every branch of
//     the checkout, so the change that answers is not always the latest: a
//     checkout of another branch brings back what a pass wrote there, and that
//     pass's record answers for it again.
//
// A decision is never overruled by it: a unit with a recorded decision is
// graded by the decision, whose basis is the decision's.

// loopWrites reads the block history of a project for the last write a flow
// made to each edition, one document at a time and once per document.
type loopWrites struct {
	app  *App
	ctx  context.Context
	root string

	once  sync.Once
	hist  *history.Store
	mu    sync.Mutex
	byDoc map[string]map[[2]string]history.Row
	byRev map[revisionOf]history.Row
}

// revisionOf names an edition of a block in a document at one revision.
type revisionOf struct {
	doc, block, edition, rev string
}

// newLoopWrites is the block history of the project at root. A project with
// no store has recorded nothing, and asking creates none.
func (a *App) newLoopWrites(ctx context.Context, root string) *loopWrites {
	if root == "" {
		return nil
	}
	return &loopWrites{
		app: a, ctx: context.WithoutCancel(ctx), root: root,
		byDoc: map[string]map[[2]string]history.Row{}, byRev: map[revisionOf]history.Row{},
	}
}

// open binds the project's block history, when the project has a store.
func (w *loopWrites) open() *history.Store {
	w.once.Do(func() {
		if db := w.app.existingProjectDB(w.ctx, w.root); db != nil {
			w.hist = db.History()
		}
	})
	return w.hist
}

// recorded reports whether the block history holds any change at all.
func (w *loopWrites) recorded() bool {
	if w == nil || w.open() == nil {
		return false
	}
	empty, err := w.hist.Empty(w.ctx)
	return err == nil && !empty
}

// last returns the latest recorded change to the edition of the block in the
// document, whoever made it.
func (w *loopWrites) last(doc, block, edition string) (history.Row, bool) {
	if w == nil || w.open() == nil {
		return history.Row{}, false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.latestLocked(doc, block, edition)
}

// at returns the recorded change that left the edition of the block in the
// document at revision rev (history.Store.Wrote), and when none did, the
// latest recorded change to it, whose After then names another revision. The
// latest answers without another read when it left rev itself.
func (w *loopWrites) at(doc, block, edition, rev string) (history.Row, bool) {
	if w == nil || w.open() == nil {
		return history.Row{}, false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	latest, ok := w.latestLocked(doc, block, edition)
	if !ok || rev == "" || (latest.After == rev && latest.Origin != history.OriginObserved) {
		return latest, ok
	}
	key := revisionOf{doc: doc, block: block, edition: edition, rev: rev}
	if r, cached := w.byRev[key]; cached {
		return r, true
	}
	r, found, err := w.hist.Wrote(w.ctx, doc, block, edition, rev)
	if err != nil || !found {
		return latest, true
	}
	w.byRev[key] = r
	return r, true
}

// loopWrite reports whether a row records a translation a tool made from a
// recorded source: a flow's draft, or a venue's translation kapi pull brought
// down where the venue's record names the source the checkout held. It is
// re-drafted when that source moves. A pull records no basis for a
// translation the venue made from any other wording (host.WithStatedBases).
func loopWrite(r history.Row) bool {
	return r.Actor == string(change.ActorTool) && r.Basis != ""
}

// latestLocked is last, with w.mu held.
func (w *loopWrites) latestLocked(doc, block, edition string) (history.Row, bool) {
	rows, ok := w.byDoc[doc]
	if !ok {
		rows = map[[2]string]history.Row{}
		latest, err := w.hist.Latest(w.ctx, doc)
		if err == nil {
			for _, r := range latest {
				rows[[2]string{r.Block, r.Edition}] = r
			}
		}
		w.byDoc[doc] = rows
	}
	r, ok := rows[[2]string{block, edition}]
	return r, ok
}

// targetRevision is the revision of the block's translation in locale, as the
// change service names it (model.TargetRevision).
func targetRevision(b *model.Block, locale model.LocaleID) string {
	return model.TargetRevision(b, locale)
}

// loopRecord is the record a flow's write of a translation stands for, read
// from the block history: the source the translation was made from and the
// stamp of the tool that made it, while the translation is the one the flow
// wrote (targetRev, its revision). It reports false for a translation no flow
// wrote, or one somebody rewrote since.
func (a *App) loopRecord(ctx context.Context, root, doc, unit string, locale model.LocaleID, targetRev string) (state.UnitState, bool) {
	if targetRev == "" {
		return state.UnitState{}, false
	}
	row, ok := a.newLoopWrites(ctx, root).at(doc, unit, editionText(model.EditionKey{Locale: locale}.Canonical()), targetRev)
	if !ok || !loopWrite(row) || row.After != targetRev {
		return state.UnitState{}, false
	}
	return state.UnitState{
		Unit: unit, Variant: model.Variant(locale), Scope: doc,
		Status:               model.TargetStatusTranslated,
		ContentHash:          row.ContentHash,
		Basis:                row.Basis,
		Revision:             row.After,
		Origin:               row.Producer,
		GoverningFingerprint: row.Producer.ContextFingerprint,
	}, true
}
