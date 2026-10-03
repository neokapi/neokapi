package host

import (
	"context"
	"path/filepath"
	"slices"
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

	mu   sync.Mutex
	hist *history.Store
	docs DocumentIndex
}

var (
	_ change.EditionStates    = (*blockHistory)(nil)
	_ change.EditionHistories = (*blockHistory)(nil)
)

// open opens the history once the project has a store. A service built
// before the project had one, as a long-lived surface's is, finds the store a
// later change created.
func (h *blockHistory) open(ctx context.Context) *history.Store {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.hist == nil {
		db := h.app.existingProjectDB(ctx, h.root)
		if db == nil {
			return nil
		}
		h.hist = db.History()
		h.docs = h.app.documentIndexOrEmpty(ctx, h.root)
	}
	return h.hist
}

// key names edition k of b in doc as the recorder recorded it.
func (h *blockHistory) key(doc change.DocInfo, b *model.Block, k model.EditionKey) (docKey, edition string, ok bool) {
	edition = editionText(b.EditionKeyOf(k))
	return h.docKey(doc), edition, edition != ""
}

// docKey is doc's key as the recorder records it.
func (h *blockHistory) docKey(doc change.DocInfo) string {
	if reconcile.IsDocumentKey(doc.Doc) {
		return doc.Doc
	}
	return h.docs.Key(doc.Doc)
}

// Document returns where the derived editions of doc stand, from the most
// recent recorded change to each, for one read of it that shows at most
// shows blocks (zero for every block). A project with no store, or a history
// that cannot be read, vouches for no basis.
func (h *blockHistory) Document(ctx context.Context, doc change.DocInfo, shows int) change.DocumentStates {
	return &documentStates{ctx: ctx, h: h, doc: doc, byBlock: isShortRead(shows)}
}

// isShortRead reports whether a read that shows at most shows blocks (zero
// for every block) looks up each block's editions one at a time.
func isShortRead(shows int) bool { return shows > 0 && shows <= shortRead }

// shortRead is the most blocks a read may show and still look up each
// block's editions one at a time. A seek of the history costs about the same
// whatever the document's history holds, so a short read (a review pane, an
// agent naming a block, a page) costs what it shows. A longer read reads the
// most recent change to every block of the editions it shows in one pass over
// the document's history, which costs what the document's history holds.
const shortRead = change.DefaultReadLimit

// editionHeads reads the most recent recorded change to an edition, of one
// block or of every block of a document (history.Store).
type editionHeads interface {
	LastWrite(ctx context.Context, doc, block, edition string) (history.Row, bool, error)
	Latest(ctx context.Context, doc string, editions ...string) ([]history.Row, error)
}

// documentStates answers for the editions of one document during one read,
// from the most recent recorded change to each.
type documentStates struct {
	ctx context.Context
	h   *blockHistory
	doc change.DocInfo

	// byBlock says the read is short enough to look up each block.
	byBlock bool

	mu     sync.Mutex
	hist   editionHeads
	docKey string
	// read holds the editions read whole: block to most recent change.
	read map[string]map[string]history.Row
}

// EditionState returns the basis the most recent recorded change to edition k
// of b named, when that change left the edition at the revision it holds now.
// An edition changed since by a writer that recorded nothing (a person's
// editor, another tool) has no basis this history can vouch for.
func (d *documentStates) EditionState(b *model.Block, k model.EditionKey) (change.EditionState, bool) {
	edition := editionText(b.EditionKeyOf(k))
	if edition == "" {
		return change.EditionState{}, false
	}
	row, found := d.last(b, edition)
	if !found || row.Basis == "" || row.After != model.EditionRevision(b, k) {
		return change.EditionState{}, false
	}
	return change.EditionState{Basis: row.Basis}, true
}

// last returns the most recent recorded change to edition of b.
func (d *documentStates) last(b *model.Block, edition string) (history.Row, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.hist == nil {
		hist := d.h.open(d.ctx)
		if hist == nil {
			return history.Row{}, false
		}
		d.hist, d.docKey = hist, d.h.docKey(d.doc)
	}
	block := change.BlockKey(b)
	if d.byBlock {
		row, found, err := d.hist.LastWrite(d.ctx, d.docKey, block, edition)
		return row, err == nil && found
	}
	if rows, ok := d.read[edition]; ok {
		row, found := rows[block]
		return row, found
	}
	// A read asks about every derived edition of each block it shows, so the
	// editions b holds are read with the one asked about, in one query.
	want := []string{edition}
	for _, k := range b.Editions() {
		if b.IsSourceEdition(k) {
			continue
		}
		e := editionText(b.EditionKeyOf(k))
		if _, read := d.read[e]; e != "" && !read && !slices.Contains(want, e) {
			want = append(want, e)
		}
	}
	rows, err := d.hist.Latest(d.ctx, d.docKey, want...)
	if d.read == nil {
		d.read = map[string]map[string]history.Row{}
	}
	for _, e := range want {
		d.read[e] = map[string]history.Row{}
	}
	if err == nil {
		for _, r := range rows {
			d.read[r.Edition][r.Block] = r
		}
	}
	row, found := d.read[edition][block]
	return row, found
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
	rows, err := hist.Edition(ctx, docKey, change.BlockKey(b), edition, limit)
	if err != nil {
		return nil, err
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
