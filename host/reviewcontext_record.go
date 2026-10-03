package host

import (
	"context"
	"path/filepath"
	"time"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/history"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/reconcile"
	"github.com/neokapi/neokapi/core/review"
	"github.com/neokapi/neokapi/core/state"
)

// recordedProvenance answers the provenance of the edition under review from
// the block history as well as the decision ledger. An edit made through the
// change service is recorded as a content.edit, not as a ledger entry, so:
//
//   - when the most recent recorded change to the edition produced the content
//     in force, its writer is the origin: a person's edit reads as human, an
//     agent's as agent with the agent named. A tool's write keeps the origin
//     the tool stamped.
//   - a decision recorded against a translation that is no longer there is not
//     the decision in force, and its origin describes text that is gone.
func (a *App) recordedProvenance(ctx context.Context, req ReviewContextRequest, block *model.Block, loc model.LocaleID, p review.Provenance) review.Provenance {
	source := isReviewSource(req, loc)
	if !source && req.Unit != nil && req.Unit.Stale(state.TargetHash(block.TargetText(loc))) {
		p.ReviewState, p.By, p.At, p.Note, p.Status = "", "", "", "", ""
		if t := block.Target(loc); t == nil || t.Origin.Kind == "" {
			p.Origin = nil
		}
	}
	row, ok := a.lastRecordedWrite(ctx, req, block, loc, source)
	if !ok {
		return p
	}
	o := model.Origin{Timestamp: row.At.UTC().Format(time.RFC3339)}
	switch change.ActorKind(row.Actor) {
	case change.ActorPerson:
		o.Kind = model.OriginHuman
	case change.ActorAgent:
		o.Kind, o.Engine, o.Reference = model.OriginAgent, row.ActorName, row.Session
	default:
		return p
	}
	p.Origin = &o
	return p
}

// isReviewSource reports whether the language under review is the source
// language, which a source row reviews.
func isReviewSource(req ReviewContextRequest, loc model.LocaleID) bool {
	return loc == "" || (req.SourceLang != "" && model.NormalizeLocale(loc) == model.NormalizeLocale(model.LocaleID(req.SourceLang)))
}

// lastRecordedWrite is the most recent recorded change to the edition under
// review, when it left the edition with the content the block holds now. The
// history is read only where the project has a store, so a review creates
// none.
func (a *App) lastRecordedWrite(ctx context.Context, req ReviewContextRequest, block *model.Block, loc model.LocaleID, source bool) (history.Row, bool) {
	if req.Root == "" || req.SourcePath == "" {
		return history.Row{}, false
	}
	db := a.existingProjectDB(ctx, req.Root)
	if db == nil {
		return history.Row{}, false
	}
	doc := reviewPointPath(req.Root, req.SourcePath)
	if filepath.IsAbs(doc) {
		return history.Row{}, false
	}
	if !reconcile.IsDocumentKey(doc) {
		doc = a.documentIndexOrEmpty(ctx, req.Root).Key(doc)
	}
	// The edition as the change service names and revises it: the source in
	// the project's source language, a translation by its locale.
	key := model.EditionKey{Locale: loc}
	runs := block.TargetRuns(loc)
	if source {
		key, runs = model.EditionKey{Locale: model.LocaleID(req.SourceLang)}, block.SourceRuns()
	}
	edition, err := key.Canonical().MarshalText()
	if err != nil || len(edition) == 0 {
		return history.Row{}, false
	}
	row, found, err := db.History().LastWrite(ctx, doc, req.Key, string(edition))
	if err != nil || !found || row.After != model.RunsRevision(key, runs) {
		return history.Row{}, false
	}
	return row, true
}
