package host

import (
	"context"
	"os"
	"path/filepath"
	"sync"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/history"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/reconcile"
)

// The hooks the change service calls, each built here and plugged in at this
// one place. A nil hook is what the service runs without: it checks nothing at
// commit, permits every operation, records nothing, or reads no basis. An
// error stops the service from being built, so a surface never edits without
// a hook it was meant to have. The recorder and the edition states open the
// project store when a change set or a read first needs it, so a service that
// only reads leaves a project that has none as it was.

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
//
// The recorder is opened when a change set first needs it: before a commit
// takes its first lock (prepare), so a recorder that cannot be opened stops
// the commit before anything is written, or at the record of a change set
// that wrote no file. Opening it opens the project store, which a read never
// needs.
func (a *App) changeRecorder(ctx context.Context, root string) *lazyRecorder {
	if root == "" {
		return nil
	}
	return &lazyRecorder{open: sync.OnceValues(func() (change.Recorder, error) {
		return a.EditRecorder(context.WithoutCancel(ctx), root)
	})}
}

// lazyRecorder is a project's recorder, opened on first use.
type lazyRecorder struct {
	open func() (change.Recorder, error)
}

var _ change.Recorder = (*lazyRecorder)(nil)

// Record records rec through the project's recorder, opening it first.
func (r *lazyRecorder) Record(ctx context.Context, rec change.Record) (string, error) {
	inner, err := r.open()
	if err != nil {
		return "", err
	}
	return inner.Record(ctx, rec)
}

// before returns prepare followed by opening the recorder, for a home to call
// before it takes its first lock. A nil recorder adds nothing.
func (r *lazyRecorder) before(prepare func() error) func() error {
	if r == nil {
		return prepare
	}
	return func() error {
		if prepare != nil {
			if err := prepare(); err != nil {
				return err
			}
		}
		_, err := r.open()
		return err
	}
}

// changeEditionStates answers a read's question about a derived edition from
// the block history of the project at root: the basis the edition was made
// from, where the most recent recorded change to it left the content it holds
// now. Outside a project there is no history, and a read shows no basis. The
// history is read from the project store when one is open or exists; a
// project with no store has recorded no change, and a read shows no basis
// without creating one.
func (a *App) changeEditionStates(root string) change.EditionStates {
	if root == "" {
		return nil
	}
	return &historyEditionStates{app: a, root: root}
}

// existingProjectDB is the project store of the project at root when this App
// holds it open or its file exists, opened as ProjectDB opens it; nil when
// the project has none, so asking creates nothing.
func (a *App) existingProjectDB(ctx context.Context, root string) *projectdb.DB {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil
	}
	s := a.ensureProjectStores()
	s.mu.Lock()
	_, open := s.dbs[abs]
	s.mu.Unlock()
	if !open {
		if _, err := os.Stat(projectLayoutAt(abs).StorePath()); err != nil {
			return nil
		}
	}
	db, err := a.ProjectDB(ctx, abs)
	if err != nil {
		return nil
	}
	return db
}

// historyEditionStates reads an edition's basis from the block history, keyed
// as the recorder keys a change (editRecorder): the document's key, the
// block's key, and the edition's key in its text form.
type historyEditionStates struct {
	app  *App
	root string

	once sync.Once
	hist *history.Store
	docs DocumentIndex
}

var _ change.EditionStates = (*historyEditionStates)(nil)

// EditionState returns the basis the most recent recorded change to edition k
// of b named, when that change left the edition at the revision it holds now.
// An edition changed since by a writer that recorded nothing (a person's
// editor, another tool) has no basis this history can vouch for.
func (h *historyEditionStates) EditionState(ctx context.Context, doc change.DocInfo, b *model.Block, k model.EditionKey) (change.EditionState, bool) {
	h.once.Do(func() {
		db := h.app.existingProjectDB(ctx, h.root)
		if db == nil {
			return
		}
		h.hist = db.History()
		h.docs = h.app.documentIndexOrEmpty(ctx, h.root)
	})
	if h.hist == nil {
		return change.EditionState{}, false
	}
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
