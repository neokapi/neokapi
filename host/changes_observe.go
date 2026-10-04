package host

import (
	"context"
	"log/slog"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/history"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/reconcile"
)

// editObserver records what a read of a project's document finds changed
// outside kapi (change.Observer). It compares each edition the read shows
// with the changes the block history records for it. An edition whose
// revision no recorded change left was changed by something that recorded
// nothing: a person's editor, a tool that writes files. The read records each
// such edition as one transition of an observed content.edit for the
// document, with the actor history.ActorExternal and the origin observed,
// hash-only.
//
// A revision an earlier recorded change left is explained, whatever was
// recorded after it: the history is shared by every branch of the checkout,
// and a checkout of another branch brings back what was written there. So a
// branch switch records nothing, and "who wrote this" and a translation's
// basis are read from the change that left the revision (history.Store.Wrote).
//
// An edition the history has never recorded is left alone: its first record
// is whatever change kapi applies or observes after a change it recorded.
type editObserver struct {
	hist *blockHistory
	rec  change.Recorder
}

var _ change.Observer = (*editObserver)(nil)

// Observe implements change.Observer: it reads the latest recorded change to
// each edition of the document, once per read, and nothing when the project
// has no store or the document no recorded change.
func (o *editObserver) Observe(ctx context.Context, doc change.DocInfo) change.Observation {
	hist := o.hist.open(ctx)
	if hist == nil {
		return nil
	}
	key := doc.Doc
	if !reconcile.IsDocumentKey(key) {
		key = o.hist.docs.Key(key)
	}
	rows, err := hist.Latest(ctx, key)
	if err != nil || len(rows) == 0 {
		return nil
	}
	latest := make(map[[2]string]history.Row, len(rows))
	for _, r := range rows {
		latest[[2]string{r.Block, r.Edition}] = r
	}
	return &observedRead{o: o, hist: hist, key: key, doc: doc, latest: latest}
}

// observedRead is one read the observer follows.
type observedRead struct {
	o      *editObserver
	hist   *history.Store
	key    string
	doc    change.DocInfo
	latest map[[2]string]history.Row
	found  []observedChange
	seen   map[[2]string]bool
}

// observedChange is an edition whose revision the latest recorded change to
// it did not leave.
type observedChange struct {
	at history.Reach
	t  change.Transition
}

// Saw compares each edition of b with the latest recorded change to it, and
// keeps the ones that change did not leave for Done to look up.
func (r *observedRead) Saw(b *model.Block, editions []model.EditionKey) {
	block := change.BlockKey(b)
	auth := b.EditionKeyOf(b.Authoritative(model.AuthorityPolicy{}))
	for _, k := range editions {
		k = b.EditionKeyOf(k)
		text, err := k.MarshalText()
		if err != nil || len(text) == 0 {
			continue
		}
		at := [2]string{block, string(text)}
		row, ok := r.latest[at]
		if !ok || r.seen[at] {
			continue
		}
		rev := model.EditionRevision(b, k)
		if row.After == rev {
			continue
		}
		if r.seen == nil {
			r.seen = map[[2]string]bool{}
		}
		r.seen[at] = true
		role := change.RoleDerived
		if k == auth {
			role = change.RoleAuthoritative
		}
		ch := change.EditionChange{
			Ref:       change.Ref{Doc: r.doc.Doc, Block: block, Edition: k},
			Role:      role,
			Key:       b.Key,
			BeforeRev: row.After,
			AfterRev:  rev,
			Block:     b,
		}
		r.found = append(r.found, observedChange{
			at: history.Reach{Block: block, Edition: string(text), Rev: rev},
			t:  change.Transition{EditionChange: ch},
		})
	}
}

// Done records, as one observed edit of the document, the editions whose
// revision no recorded change left. It asks the history when the read ends
// rather than when it started, so a writer that recorded its own change while
// the read ran (kapi up committing a file the desktop is reading) explains
// that change. A record that fails leaves the read as it was: the next read
// finds the same transitions and records them.
func (r *observedRead) Done(ctx context.Context) {
	if len(r.found) == 0 {
		return
	}
	ctx = context.WithoutCancel(ctx)
	asks := make([]history.Reach, len(r.found))
	for i, f := range r.found {
		asks[i] = f.at
	}
	reached, err := r.hist.Reached(ctx, r.key, asks)
	if err != nil {
		slog.Debug("read the changes an observed edit may repeat", "doc", r.doc.Doc, "error", err)
		return
	}
	var transitions []change.Transition
	for _, f := range r.found {
		if _, explained := reached[f.at]; !explained {
			transitions = append(transitions, f.t)
		}
	}
	if len(transitions) == 0 {
		return
	}
	if _, err := r.o.rec.Record(ctx, change.Record{
		Actor:       change.Actor{Kind: change.ActorKind(history.ActorExternal)},
		Origin:      history.OriginObserved,
		Docs:        []change.DocResult{{Doc: r.doc.Doc, Home: "file"}},
		Transitions: transitions,
	}); err != nil {
		slog.Debug("record an edit made outside kapi", "doc", r.doc.Doc, "error", err)
	}
}
