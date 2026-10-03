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
// So the loop's own basis is the latest row of the block history for the
// edition, and it answers only while two things hold:
//
//   - A tool in a flow wrote it. A translation a person or an agent wrote, or
//     one that was in the tree before kapi ever ran, is not the loop's work: it
//     grades basisNone or basisUnknown and is never re-drafted. The loop does
//     not get to claim authorship of somebody's work by reading it.
//   - The file still holds what the flow wrote: the row's revision after the
//     change is the edition's revision now. A translation somebody has since
//     rewritten describes work they took over.
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
}

// newLoopWrites is the block history of the project at root. A project with
// no store has recorded nothing, and asking creates none.
func (a *App) newLoopWrites(ctx context.Context, root string) *loopWrites {
	if root == "" {
		return nil
	}
	return &loopWrites{app: a, ctx: context.WithoutCancel(ctx), root: root, byDoc: map[string]map[[2]string]history.Row{}}
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
	if w == nil {
		return history.Row{}, false
	}
	if w.open() == nil {
		return history.Row{}, false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
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
// change service names it.
func targetRevision(b *model.Block, locale model.LocaleID) string {
	k := model.EditionKey{Locale: locale}.Canonical()
	if !b.HasTarget(locale) {
		return model.AbsentRevision
	}
	return model.RunsRevision(k, b.TargetRuns(locale))
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
	row, ok := a.newLoopWrites(ctx, root).last(doc, unit, editionText(model.EditionKey{Locale: locale}.Canonical()))
	if !ok || row.Actor != string(change.ActorTool) || row.After != targetRev {
		return state.UnitState{}, false
	}
	return state.UnitState{
		Unit: unit, Variant: model.Variant(locale), Scope: doc,
		Status:               model.TargetStatusTranslated,
		ContentHash:          row.ContentHash,
		Origin:               row.Producer,
		GoverningFingerprint: row.Producer.ContextFingerprint,
	}, true
}
