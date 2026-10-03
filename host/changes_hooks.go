package host

import (
	"context"
	"path/filepath"
	"sync"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/history"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/reconcile"
	"github.com/neokapi/neokapi/core/storage"
)

// The hooks the change service calls, each built here and plugged in at this
// one place. A nil hook is what the service runs without: it checks nothing at
// commit, permits every operation, records nothing, or reads no basis and no
// history. An error stops the service from being built, so a surface never
// edits without a hook it was meant to have. The recorder and the block
// history open the project store when a change set, a read or a history first
// needs it, so a service that only reads leaves a project that has none as it
// was.

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

// changeHistory answers two questions from the block history of the project
// at root: a read's, about the basis a derived edition was made from where
// the most recent recorded change to it left the content it holds now, and a
// history's, about every recorded change to an edition. Outside a project
// there is no history, so a read shows no basis and a history lists nothing.
// The history is read from the project store when one is open or exists; a
// project with no store has recorded no change, and neither question creates
// one.
func (a *App) changeHistory(root string) *blockHistory {
	if root == "" {
		return nil
	}
	return &blockHistory{app: a, root: root}
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
		// The database belongs to the driver: in the browser it lives in
		// SQLite's memory, where os.Stat never finds it.
		if held, _ := storage.Exists(projectLayoutAt(abs).StorePath()); !held {
			return nil
		}
	}
	db, err := a.ProjectDB(ctx, abs)
	if err != nil {
		return nil
	}
	return db
}

// blockHistory reads the block history, keyed as the recorder keys a change
// (editRecorder): the document's key, the block's key, and the edition's key
// in its text form.
type blockHistory struct {
	app  *App
	root string

	once sync.Once
	hist *history.Store
	docs DocumentIndex
}

var (
	_ change.EditionStates    = (*blockHistory)(nil)
	_ change.EditionHistories = (*blockHistory)(nil)
)

// open opens the history once, when the project has a store.
func (h *blockHistory) open(ctx context.Context) *history.Store {
	h.once.Do(func() {
		db := h.app.existingProjectDB(ctx, h.root)
		if db == nil {
			return
		}
		h.hist = db.History()
		h.docs = h.app.documentIndexOrEmpty(ctx, h.root)
	})
	return h.hist
}

// key names edition k of b in doc as the recorder recorded it.
func (h *blockHistory) key(doc change.DocInfo, b *model.Block, k model.EditionKey) (docKey, edition string, ok bool) {
	docKey = doc.Doc
	if !reconcile.IsDocumentKey(docKey) {
		docKey = h.docs.Key(docKey)
	}
	text, err := b.EditionKeyOf(k).MarshalText()
	if err != nil || len(text) == 0 {
		return "", "", false
	}
	return docKey, string(text), true
}

// EditionState returns the basis the most recent recorded change to edition k
// of b named, when that change left the edition at the revision it holds now.
// An edition changed since by a writer that recorded nothing (a person's
// editor, another tool) has no basis this history can vouch for.
func (h *blockHistory) EditionState(ctx context.Context, doc change.DocInfo, b *model.Block, k model.EditionKey) (change.EditionState, bool) {
	hist := h.open(ctx)
	if hist == nil {
		return change.EditionState{}, false
	}
	docKey, edition, ok := h.key(doc, b, k)
	if !ok {
		return change.EditionState{}, false
	}
	row, found, err := hist.LastWrite(ctx, docKey, change.BlockKey(b), edition)
	if err != nil || !found || row.Basis == "" || row.After != model.EditionRevision(b, k) {
		return change.EditionState{}, false
	}
	return change.EditionState{Basis: row.Basis}, true
}

// EditionHistory returns the recorded changes to edition k of b, most recent
// first and at most limit of them: who made each, through which surface, when,
// and the revisions around it.
func (h *blockHistory) EditionHistory(ctx context.Context, doc change.DocInfo, b *model.Block, k model.EditionKey, limit int) ([]change.HistoryEntry, error) {
	hist := h.open(ctx)
	if hist == nil {
		return nil, nil
	}
	docKey, edition, ok := h.key(doc, b, k)
	if !ok {
		return nil, nil
	}
	rows, err := hist.Edition(ctx, docKey, change.BlockKey(b), edition)
	if err != nil {
		return nil, err
	}
	if len(rows) > limit {
		rows = rows[:limit]
	}
	out := make([]change.HistoryEntry, 0, len(rows))
	for _, r := range rows {
		e := change.HistoryEntry{Record: r.Op, Before: r.Before, After: r.After, Basis: r.Basis, Origin: r.Origin, At: r.At}
		if r.Actor != "" {
			e.Actor = &change.Actor{Kind: change.ActorKind(r.Actor), Name: r.ActorName, Session: r.Session}
		}
		out = append(out, e)
	}
	return out, nil
}
