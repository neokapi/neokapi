// Package workhome is the workspace home: the home of the change service for
// the editions that have no file. A parked locale's drafts, the drafts a run
// produced and kapi merge has not delivered, and the edits a person or an
// agent makes to them live here, in the project's operation log, until a
// delivery moves them into the file their recipe names.
//
// Every write to the workspace home is one content.edit operation carrying
// the result: the runs, status and origin of each block's edition it leaves.
// The operation names its subject (the document's key and the edition,
// Subject), and a writer appends it only while the subject's head is still
// the one it read (workspace.Backend.RecordIf), so two processes writing one
// edition take turns. The projector folds the operations of each subject into
// tables of the context store, which this package owns:
//
//   - edition_head holds one row per block of each edition the workspace
//     keeps: its revision, its runs (inline, or in a blob when large), its
//     status and origin, the basis it was made from, and the stamp its
//     producer recognizes it by;
//   - edition_subject_head holds one row per subject: the operation its head
//     is at, the latest operation folded, and the operations that did not
//     advance it;
//   - document_head holds one row per document the workspace keeps whole
//     (a document opened for editing that has no file, such as a source a
//     KPZ carries): its revision, format, the blob holding its bytes, and
//     the operation its head is at (docstore.go, dochome.go).
//
// The head a conditional record expects is the local position of the latest
// operation on the subject, which the writer reads from the log itself
// (Log.SubjectHead): a position is local to one log, so no projection a
// checkpoint carries to another machine holds it.
//
// The fold reads a subject's operations in id order, the order every machine
// whose log has been merged agrees on. An operation advances the head when it
// was staged on the head it finds (its base); otherwise it is divergent and
// changes nothing. Every machine therefore reaches the same head whatever
// order its log received the operations in. A divergent operation stays
// listed until the head advances by an operation that rebases it (its cause),
// by a person's or an agent's write to every block it changed, or by a
// release that removes the whole edition.
//
// The projector is the only writer of these tables. Everything else reads.
package workhome

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/storage"
)

// Name is the home as a change result reports it.
const Name = "workspace"

// InlineRuns is the size of the runs an edition_head row keeps inline; larger
// runs are read from the blob the write stored them in.
const InlineRuns = 16 << 10

// Subject spells the subject a write to edition of the document keyed doc
// names in the log: what the workspace home orders the writes to one edition
// by.
func Subject(doc, edition string) string { return doc + "@" + edition }

// Row is one block's edition as the workspace home keeps it.
type Row struct {
	Doc     string
	Edition string
	Block   string
	// Rev is the edition revision (model.RunsRevision) of the runs.
	Rev string
	// Runs is the canonical run JSON (model.CanonicalRunsJSON), nil when the
	// runs are larger than InlineRuns and live in Blob only.
	Runs []byte
	// Blob is the address of the blob holding the runs.
	Blob   string
	Status model.Status
	Origin model.Origin
	// Basis is the authoritative edition's revision the edition was made from.
	Basis string
	// Stamp is what the producer of a draft recognizes it by, to serve it
	// again rather than produce it anew; nil for an edition no producer made.
	Stamp json.RawMessage
	// Op is the operation that wrote the row.
	Op string
}

// Head is the head of one edition the workspace home keeps.
type Head struct {
	Doc     string
	Edition string
	// Path is where the document was when the latest write that advanced the
	// head landed.
	Path string
	// Op is the operation the head is at.
	Op string
	// Last is the largest operation id folded into the head.
	Last string
	// Divergent are the operations that did not advance the head and that no
	// later operation has rebased, in id order.
	Divergent []Divergence
}

// Divergence is an operation that did not advance a head: the blocks it
// changed and the revisions it moved each from and to. Release marks a
// release that did not land, which nothing carries over: the delivery that
// made it handles what the head still holds.
type Divergence struct {
	Op      string `json:"op"`
	Blocks  []Move `json:"blocks"`
	Release bool   `json:"release,omitempty"`
}

// Move is one block's edition as a write moved it.
type Move struct {
	Block  string `json:"block"`
	Before string `json:"before"`
	After  string `json:"after"`
}

// Write is one operation on one edition, as the fold reads it.
type Write struct {
	Op string
	// Doc, Edition and Path name the edition and where its document was.
	Doc     string
	Edition string
	Path    string
	// Base is the operation the head was at when the write was staged; Cause
	// is the divergent operation a rebase carries over.
	Base  string
	Cause string
	// Writer says a person or an agent made the write: when it advances the
	// head, it settles what every divergent write held for the blocks it
	// writes. Release says the write is a delivery's release of the whole
	// edition, which settles every divergent write.
	Writer  bool
	Release bool
	Blocks  []BlockWrite
}

// BlockWrite is one block's edition as a write leaves it.
type BlockWrite struct {
	Block  string
	Before string
	After  string
	// Runs is the canonical run JSON of the edition after the write, nil for
	// a write that removes it or one whose runs live in Ref alone.
	Runs   []byte
	Ref    string
	Status model.Status
	Origin model.Origin
	Basis  string
	Stamp  json.RawMessage
}

// Store is the workspace home's projection in a project's context store.
type Store struct {
	db *storage.DB
}

var migrations = []storage.Migration{{
	Version:     1,
	Description: "the workspace home",
	SQL: `
CREATE TABLE IF NOT EXISTS edition_head (
    doc     TEXT NOT NULL,
    edition TEXT NOT NULL,
    block   TEXT NOT NULL,
    rev     TEXT NOT NULL,
    runs    TEXT NOT NULL DEFAULT '',
    blob    TEXT NOT NULL DEFAULT '',
    status  TEXT NOT NULL DEFAULT '',
    origin  TEXT NOT NULL DEFAULT '',
    basis   TEXT NOT NULL DEFAULT '',
    stamp   TEXT NOT NULL DEFAULT '',
    op      TEXT NOT NULL,
    PRIMARY KEY (doc, edition, block)
);
CREATE TABLE IF NOT EXISTS edition_subject_head (
    doc       TEXT NOT NULL,
    edition   TEXT NOT NULL,
    path      TEXT NOT NULL DEFAULT '',
    op        TEXT NOT NULL DEFAULT '',
    last      TEXT NOT NULL DEFAULT '',
    divergent TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (doc, edition)
);`,
}, docMigrations[0]}

// Tables are the tables the store keeps, which a rebuild empties and a
// checkpoint carries.
var Tables = []string{"edition_head", "edition_subject_head", "document_head"}

// Open binds the store to a context database, creating its tables.
func Open(db *storage.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("workhome: no database")
	}
	if err := storage.Migrate(db, "workhome", migrations); err != nil {
		return nil, fmt.Errorf("workhome: migrate: %w", err)
	}
	return &Store{db: db}, nil
}

// Fold folds every write to one edition, in id order, into its head and the
// rows it leaves. It is what a projection of the writes holds whatever order
// they were received in.
func Fold(writes []Write) (Head, map[string]Row) {
	writes = slices.Clone(writes)
	slices.SortStableFunc(writes, func(a, b Write) int { return cmp.Compare(a.Op, b.Op) })
	var h Head
	rows := map[string]Row{}
	for _, w := range writes {
		if h.Doc == "" {
			h.Doc, h.Edition = w.Doc, w.Edition
		}
		if w.Op == h.Last && w.Op != "" {
			continue
		}
		fold(&h, rows, w)
	}
	return h, rows
}

// fold applies one write that sorts after every write h has folded.
func fold(h *Head, rows map[string]Row, w Write) {
	h.Last = w.Op
	if w.Base != h.Op {
		moves := make([]Move, 0, len(w.Blocks))
		for _, b := range w.Blocks {
			moves = append(moves, Move{Block: b.Block, Before: b.Before, After: b.After})
		}
		h.Divergent = append(h.Divergent, Divergence{Op: w.Op, Blocks: moves, Release: w.Release})
		return
	}
	for _, b := range w.Blocks {
		if b.After == model.AbsentRevision {
			delete(rows, b.Block)
			continue
		}
		rows[b.Block] = rowOf(w, b)
	}
	h.Op, h.Path = w.Op, w.Path
	switch {
	case w.Release:
		// The edition went to its file: nothing a divergent write held is
		// carried over onto a head the workspace no longer keeps.
		h.Divergent = nil
	case w.Cause != "":
		h.Divergent = slices.DeleteFunc(h.Divergent, func(d Divergence) bool { return d.Op == w.Cause })
	}
	if w.Writer {
		settleWritten(h, w)
	}
}

// settleWritten drops, from every divergent write h lists, the blocks a
// person's or an agent's write that advanced the head wrote: a write made
// with the conflict in view decides those blocks. A divergent write left with
// no block is settled.
func settleWritten(h *Head, w Write) {
	if len(h.Divergent) == 0 {
		return
	}
	written := make(map[string]bool, len(w.Blocks))
	for _, b := range w.Blocks {
		written[b.Block] = true
	}
	out := h.Divergent[:0]
	for _, d := range h.Divergent {
		d.Blocks = slices.DeleteFunc(slices.Clone(d.Blocks), func(m Move) bool { return written[m.Block] })
		if len(d.Blocks) > 0 {
			out = append(out, d)
		}
	}
	h.Divergent = out
	if len(h.Divergent) == 0 {
		h.Divergent = nil
	}
}

// rowOf is the row a write leaves for one block.
func rowOf(w Write, b BlockWrite) Row {
	r := Row{Doc: w.Doc, Edition: w.Edition, Block: b.Block, Rev: b.After, Blob: b.Ref,
		Status: b.Status, Origin: b.Origin, Basis: b.Basis, Stamp: b.Stamp, Op: w.Op}
	if b.Runs != nil && (len(b.Runs) <= InlineRuns || b.Ref == "") {
		r.Runs = b.Runs
	}
	return r
}

// Apply folds writes into the projection, each after every write its subject
// already folded. A write that sorts before the latest write its subject
// folded arrived out of order (a merge brought in an older operation): it is
// not applied, and its subject is returned, for the caller to fold again
// from every write it holds (Replace).
func (s *Store) Apply(ctx context.Context, writes []Write) (refold [][2]string, err error) {
	if len(writes) == 0 {
		return nil, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("workhome: apply: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	heads := map[[2]string]*Head{}
	stale := map[[2]string]bool{}
	for _, w := range writes {
		key := [2]string{w.Doc, w.Edition}
		if stale[key] {
			continue
		}
		h, ok := heads[key]
		if !ok {
			held, found, err := readHead(ctx, tx, w.Doc, w.Edition)
			if err != nil {
				return nil, err
			}
			if !found {
				held = Head{Doc: w.Doc, Edition: w.Edition}
			}
			h = &held
			heads[key] = h
		}
		switch {
		case w.Op == h.Last:
			continue
		case w.Op < h.Last:
			stale[key] = true
			refold = append(refold, key)
			continue
		}
		rows := map[string]Row{}
		was := h.Op
		fold(h, rows, w)
		if h.Op == was {
			continue
		}
		for _, b := range w.Blocks {
			if b.After == model.AbsentRevision {
				if _, err := tx.ExecContext(ctx, `DELETE FROM edition_head WHERE doc = ? AND edition = ? AND block = ?`,
					w.Doc, w.Edition, b.Block); err != nil {
					return nil, fmt.Errorf("workhome: apply %s: %w", w.Op, err)
				}
				continue
			}
			if err := putRow(ctx, tx, rows[b.Block]); err != nil {
				return nil, err
			}
		}
	}
	for key, h := range heads {
		if stale[key] {
			continue
		}
		if err := putHead(ctx, tx, *h); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("workhome: apply: %w", err)
	}
	return refold, nil
}

// Replace writes the head and rows Fold returned for one edition in place of
// what the projection held for it.
func (s *Store) Replace(ctx context.Context, h Head, rows map[string]Row) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("workhome: replace: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM edition_head WHERE doc = ? AND edition = ?`, h.Doc, h.Edition); err != nil {
		return fmt.Errorf("workhome: replace %s: %w", Subject(h.Doc, h.Edition), err)
	}
	for _, key := range slices.Sorted(maps.Keys(rows)) {
		if err := putRow(ctx, tx, rows[key]); err != nil {
			return err
		}
	}
	if err := putHead(ctx, tx, h); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("workhome: replace: %w", err)
	}
	return nil
}

func putRow(ctx context.Context, tx *storage.Tx, r Row) error {
	origin := ""
	if r.Origin != (model.Origin{}) {
		data, err := json.Marshal(r.Origin)
		if err != nil {
			return fmt.Errorf("workhome: put %s: %w", r.Block, err)
		}
		origin = string(data)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO edition_head (doc, edition, block, rev, runs, blob, status, origin, basis, stamp, op)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(doc, edition, block) DO UPDATE SET
    rev = excluded.rev, runs = excluded.runs, blob = excluded.blob, status = excluded.status,
    origin = excluded.origin, basis = excluded.basis, stamp = excluded.stamp, op = excluded.op`,
		r.Doc, r.Edition, r.Block, r.Rev, string(r.Runs), r.Blob, string(r.Status), origin, r.Basis, string(r.Stamp), r.Op); err != nil {
		return fmt.Errorf("workhome: put %s of %s: %w", r.Block, Subject(r.Doc, r.Edition), err)
	}
	return nil
}

func putHead(ctx context.Context, tx *storage.Tx, h Head) error {
	divergent := ""
	if len(h.Divergent) > 0 {
		data, err := json.Marshal(h.Divergent)
		if err != nil {
			return fmt.Errorf("workhome: put the head of %s: %w", Subject(h.Doc, h.Edition), err)
		}
		divergent = string(data)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO edition_subject_head (doc, edition, path, op, last, divergent) VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(doc, edition) DO UPDATE SET
    path = excluded.path, op = excluded.op, last = excluded.last, divergent = excluded.divergent`,
		h.Doc, h.Edition, h.Path, h.Op, h.Last, divergent); err != nil {
		return fmt.Errorf("workhome: put the head of %s: %w", Subject(h.Doc, h.Edition), err)
	}
	return nil
}

// querier is a database handle or a transaction.
type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

const headColumns = `doc, edition, path, op, last, divergent`

func scanHead(sc interface{ Scan(...any) error }) (Head, error) {
	var h Head
	var divergent string
	if err := sc.Scan(&h.Doc, &h.Edition, &h.Path, &h.Op, &h.Last, &divergent); err != nil {
		return Head{}, err
	}
	if divergent != "" {
		if err := json.Unmarshal([]byte(divergent), &h.Divergent); err != nil {
			return Head{}, fmt.Errorf("workhome: read the divergent writes of %s: %w", Subject(h.Doc, h.Edition), err)
		}
	}
	return h, nil
}

func readHead(ctx context.Context, q querier, doc, edition string) (Head, bool, error) {
	h, err := scanHead(q.QueryRowContext(ctx, `SELECT `+headColumns+` FROM edition_subject_head WHERE doc = ? AND edition = ?`, doc, edition))
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Head{}, false, nil
	case err != nil:
		return Head{}, false, fmt.Errorf("workhome: read the head of %s: %w", Subject(doc, edition), err)
	}
	return h, true, nil
}

// Head returns the head of one edition the workspace home keeps; found is
// false for an edition it has never kept.
func (s *Store) Head(ctx context.Context, doc, edition string) (Head, bool, error) {
	return readHead(ctx, s.db, doc, edition)
}

// Heads returns the head of every edition the workspace home has kept, by
// document and edition.
func (s *Store) Heads(ctx context.Context) ([]Head, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+headColumns+` FROM edition_subject_head ORDER BY doc, edition`)
	if err != nil {
		return nil, fmt.Errorf("workhome: read the heads: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Head
	for rows.Next() {
		h, err := scanHead(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// Rows returns every block of one edition the workspace home keeps, by block
// key.
func (s *Store) Rows(ctx context.Context, doc, edition string) (map[string]Row, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT doc, edition, block, rev, runs, blob, status, origin, basis, stamp, op
  FROM edition_head WHERE doc = ? AND edition = ?`, doc, edition)
	if err != nil {
		return nil, fmt.Errorf("workhome: read %s: %w", Subject(doc, edition), err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]Row{}
	for rows.Next() {
		var (
			r                    Row
			runs, status, origin string
			stamp                string
		)
		if err := rows.Scan(&r.Doc, &r.Edition, &r.Block, &r.Rev, &runs, &r.Blob, &status, &origin, &r.Basis, &stamp, &r.Op); err != nil {
			return nil, fmt.Errorf("workhome: read %s: %w", Subject(doc, edition), err)
		}
		if runs != "" {
			r.Runs = []byte(runs)
		}
		r.Status = model.Status(status)
		if origin != "" {
			if err := json.Unmarshal([]byte(origin), &r.Origin); err != nil {
				return nil, fmt.Errorf("workhome: read the origin of %s in %s: %w", r.Block, Subject(doc, edition), err)
			}
		}
		if stamp != "" {
			r.Stamp = json.RawMessage(stamp)
		}
		out[r.Block] = r
	}
	return out, rows.Err()
}

// Held reports whether the workspace home keeps any block of one edition.
func (s *Store) Held(ctx context.Context, doc, edition string) (bool, error) {
	var one int
	switch err := s.db.QueryRowContext(ctx, `SELECT 1 FROM edition_head WHERE doc = ? AND edition = ? LIMIT 1`, doc, edition).Scan(&one); {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("workhome: read %s: %w", Subject(doc, edition), err)
	}
	return true, nil
}

// Digest is the digest of an edition the workspace home keeps: what a result
// reports as the edition's before and after. It is "" for an edition with no
// block.
func Digest(rows map[string]Row) string {
	entries := make(map[string]string, len(rows))
	for key, r := range rows {
		entries[key] = filehome.KeptEntryOf(r.Rev, r.Status, r.Origin)
	}
	return filehome.KeptDigestOf(entries)
}
