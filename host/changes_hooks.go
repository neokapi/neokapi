package host

import (
	"context"
	"sync"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/history"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/reconcile"
)

// The hooks the change service calls, each built here and plugged in at this
// one place. A nil hook is what the service runs without: it checks nothing at
// commit, permits every operation, records nothing, or reads no basis. An
// error stops the service from being built, so a surface never edits without
// a hook it was meant to have.

// changeCommitCheck is the commit check the service runs over the editions a
// change set changes, before anything is written (App.CommitCheck). cmd names
// the service's project with -p, as every check surface resolves one, and
// carries the --source-lang a caller gave. A service outside a project
// (recipe "") edits documents under a directory of its own, so its check
// holds them to hygiene alone rather than to a project it would discover from
// the working directory.
func (a *App) changeCommitCheck(cmd Command, recipe string) (change.CommitCheck, error) {
	if recipe == "" {
		return &commitCheck{app: a, cmd: cmd, outside: true}, nil
	}
	return a.CommitCheck(cmd), nil
}

// changePolicy decides which operations an actor may send, for the project
// at recipe ("" outside a project): ChangePolicy, whose context policy is the
// one kapi apply's asset entries follow.
func (a *App) changePolicy(_ string) (change.Policy, error) {
	return ChangePolicy{}, nil
}

// changeRecorder records an applied change set for the project whose root
// directory is root (App.EditRecorder). Outside a project (root "") there is
// no log to record into, and nothing is recorded.
func (a *App) changeRecorder(ctx context.Context, root string) (change.Recorder, error) {
	if root == "" {
		return nil, nil
	}
	return a.EditRecorder(ctx, root)
}

// changeEditionStates answers a read's question about a derived edition from
// the block history of the project at root: the basis the edition was made
// from, where the most recent recorded change to it left the content it holds
// now. Outside a project there is no history, and a read shows no basis.
func (a *App) changeEditionStates(ctx context.Context, root string) (change.EditionStates, error) {
	if root == "" {
		return nil, nil
	}
	db, err := a.ProjectDB(ctx, root)
	if err != nil {
		return nil, err
	}
	hist := db.History()
	if hist == nil {
		return nil, nil
	}
	return &historyEditionStates{app: a, root: root, hist: hist}, nil
}

// historyEditionStates reads an edition's basis from the block history, keyed
// as the recorder keys a change (editRecorder): the document's key, the
// block's key, and the edition's key in its text form.
type historyEditionStates struct {
	app  *App
	root string
	hist *history.Store

	once sync.Once
	docs DocumentIndex
}

var _ change.EditionStates = (*historyEditionStates)(nil)

// EditionState returns the basis the most recent recorded change to edition k
// of b named, when that change left the edition at the revision it holds now.
// An edition changed since by a writer that recorded nothing (a person's
// editor, another tool) has no basis this history can vouch for.
func (h *historyEditionStates) EditionState(ctx context.Context, doc change.DocInfo, b *model.Block, k model.EditionKey) (change.EditionState, bool) {
	h.once.Do(func() { h.docs = h.app.documentIndexOrEmpty(ctx, h.root) })
	key := doc.Doc
	if !reconcile.IsDocumentKey(key) {
		key = h.docs.Key(key)
	}
	edition, err := b.EditionKeyOf(k).MarshalText()
	if err != nil || len(edition) == 0 {
		return change.EditionState{}, false
	}
	row, found, err := h.hist.LastWrite(ctx, key, change.BlockKey(b), string(edition))
	if err != nil || !found || row.Basis == "" || row.After != model.EditionRevision(b, k) {
		return change.EditionState{}, false
	}
	return change.EditionState{Basis: row.Basis}, true
}
