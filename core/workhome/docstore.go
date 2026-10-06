package workhome

import (
	"cmp"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/neokapi/neokapi/core/storage"
)

// The workspace home keeps a whole native document as well as editions: a
// document opened for editing that has no file of its own, such as a source
// document a KPZ carries. Each write to one is a content.edit operation whose
// subject is the document (DocumentSubject) and which names the blob holding
// the document's bytes after the write. The projector folds those operations
// into document_head, one row per document: its revision (the digest of its
// bytes), its format, the blob, and the operation the head is at. The fold
// follows the rule of the edition heads: a write staged on the head advances
// it, and any other is divergent and changes nothing until a rebase or a
// discard settles it (docdiverge.go).

// DocumentSubject spells the subject a write to the whole document keyed key
// names in the log.
func DocumentSubject(key string) string { return "document:" + key }

// DocumentRevision is the revision of a document's bytes: the digest a file
// home reports for the same bytes, so a document read from a file and from
// the workspace home names one revision.
func DocumentRevision(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// DocHead is the head of one document the workspace home keeps whole.
type DocHead struct {
	Key string
	// Path is where the document was when the write that advanced the head
	// landed: the reference a result echoes.
	Path string
	// Rev is the revision of the document's bytes (DocumentRevision), Format
	// the format that reads and writes them, and Blob the address of the blob
	// holding them in the log.
	Rev    string
	Format string
	Blob   string
	// Op is the operation the head is at, and Last the largest operation id
	// folded into it.
	Op   string
	Last string
	// Divergent are the writes that did not advance the head, in id order.
	Divergent []DocDivergence
}

// DocDivergence is a write to a whole document that did not advance its head:
// two machines wrote the document from one head, and the write that sorts
// later is listed here until a person or an agent rebases or discards it
// (Documents.Rebase, Documents.Discard).
type DocDivergence struct {
	Op     string `json:"op"`
	Before string `json:"before"`
	After  string `json:"after"`
	// Contested are the blocks a rebase of the write could not carry over,
	// because the head changed them too. Empty until the write is rebased;
	// a rebase that carries every block over settles the write.
	Contested []DocBlock `json:"contested,omitempty"`
}

// Rebased reports whether a rebase has carried the write over and left the
// blocks it lists for a person to decide.
func (d DocDivergence) Rebased() bool { return len(d.Contested) > 0 }

// DocBlock names one edition of one block of a document the workspace home
// keeps whole. Edition is empty for the document's own edition.
type DocBlock struct {
	Block   string `json:"block"`
	Edition string `json:"edition,omitempty"`
}

// DocWrite is one operation on a whole document, as the fold reads it.
type DocWrite struct {
	Op   string
	Key  string
	Path string
	// Base is the operation the head was at when the write was staged.
	Base string
	// Before and After are the document's revisions around the write.
	Before string
	After  string
	Format string
	Blob   string
	// Writer says a person or an agent made the write, and Decided names the
	// blocks it wrote or kept: when it advances the head, it settles those
	// blocks of every rebased divergent write.
	Writer  bool
	Decided []DocBlock
	// Cause, for a write that settles a divergent write (a rebase or a
	// discard), names that write, and Contested lists the blocks the rebase
	// left for a person to decide, none when it settles the write whole.
	// Such a write leaves the head where it is.
	Cause     string
	Contested []DocBlock
}

var docMigrations = []storage.Migration{{
	Version:     2,
	Description: "documents the workspace home keeps whole",
	SQL: `
CREATE TABLE IF NOT EXISTS document_head (
    key       TEXT NOT NULL PRIMARY KEY,
    path      TEXT NOT NULL DEFAULT '',
    rev       TEXT NOT NULL DEFAULT '',
    format    TEXT NOT NULL DEFAULT '',
    blob      TEXT NOT NULL DEFAULT '',
    op        TEXT NOT NULL DEFAULT '',
    last      TEXT NOT NULL DEFAULT '',
    divergent TEXT NOT NULL DEFAULT ''
);`,
}}

// FoldDocument folds every write to one document, in id order, into its head.
func FoldDocument(writes []DocWrite) DocHead {
	writes = slices.Clone(writes)
	slices.SortStableFunc(writes, func(a, b DocWrite) int { return cmp.Compare(a.Op, b.Op) })
	var h DocHead
	for _, w := range writes {
		if h.Key == "" {
			h.Key = w.Key
		}
		if w.Op == h.Last && w.Op != "" {
			continue
		}
		foldDocument(&h, w)
	}
	return h
}

// foldDocument applies one write that sorts after every write h has folded.
// A write that settles a divergent write changes that write's entry alone.
// Any other write advances the head when it was staged on it and is divergent
// otherwise. A person's or an agent's write that advances the head settles the
// blocks it decided of every rebased divergent write.
func foldDocument(h *DocHead, w DocWrite) {
	h.Last = w.Op
	if w.Cause != "" {
		settleDivergence(h, w.Cause, w.Contested)
		return
	}
	if w.Base != h.Op {
		h.Divergent = append(h.Divergent, DocDivergence{Op: w.Op, Before: w.Before, After: w.After})
		return
	}
	h.Op, h.Path, h.Rev, h.Format, h.Blob = w.Op, w.Path, w.After, w.Format, w.Blob
	if w.Writer && len(w.Decided) > 0 {
		decideBlocks(h, w.Decided)
	}
}

// settleDivergence records a rebase or a discard of the divergent write op:
// the write is dropped when contested is empty, and otherwise left with the
// blocks contested names for a person to decide.
func settleDivergence(h *DocHead, op string, contested []DocBlock) {
	i := slices.IndexFunc(h.Divergent, func(d DocDivergence) bool { return d.Op == op })
	if i < 0 {
		return
	}
	if len(contested) == 0 {
		h.Divergent = slices.Delete(h.Divergent, i, i+1)
	} else {
		h.Divergent[i].Contested = slices.Clone(contested)
	}
	if len(h.Divergent) == 0 {
		h.Divergent = nil
	}
}

// decideBlocks removes the blocks a person's or an agent's write decided from
// every rebased divergent write, and drops a write left with none. A write
// not yet rebased waits for its rebase or its discard.
func decideBlocks(h *DocHead, decided []DocBlock) {
	out := h.Divergent[:0]
	for _, d := range h.Divergent {
		if d.Rebased() {
			d.Contested = slices.DeleteFunc(slices.Clone(d.Contested), func(b DocBlock) bool { return slices.Contains(decided, b) })
			if len(d.Contested) == 0 {
				continue
			}
		}
		out = append(out, d)
	}
	h.Divergent = out
	if len(h.Divergent) == 0 {
		h.Divergent = nil
	}
}

// ApplyDocuments folds writes to whole documents into the projection, each
// after every write its document already folded. A write that sorts before
// the latest write its document folded arrived out of order: it is not
// applied, and the document's key is returned for the caller to fold again
// from every write it holds (ReplaceDocument).
func (s *Store) ApplyDocuments(ctx context.Context, writes []DocWrite) (refold []string, err error) {
	if len(writes) == 0 {
		return nil, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("workhome: apply documents: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	heads := map[string]*DocHead{}
	stale := map[string]bool{}
	var order []string
	for _, w := range writes {
		if stale[w.Key] {
			continue
		}
		h, ok := heads[w.Key]
		if !ok {
			held, found, err := readDocHead(ctx, tx, w.Key)
			if err != nil {
				return nil, err
			}
			if !found {
				held = DocHead{Key: w.Key}
			}
			h = &held
			heads[w.Key] = h
			order = append(order, w.Key)
		}
		switch {
		case w.Op == h.Last:
			continue
		case w.Op < h.Last:
			stale[w.Key] = true
			refold = append(refold, w.Key)
			continue
		}
		foldDocument(h, w)
	}
	for _, key := range order {
		if stale[key] {
			continue
		}
		if err := putDocHead(ctx, tx, *heads[key]); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("workhome: apply documents: %w", err)
	}
	return refold, nil
}

// ReplaceDocument writes the head FoldDocument returned for one document in
// place of what the projection held for it.
func (s *Store) ReplaceDocument(ctx context.Context, h DocHead) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("workhome: replace document: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := putDocHead(ctx, tx, h); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("workhome: replace document: %w", err)
	}
	return nil
}

func putDocHead(ctx context.Context, tx *storage.Tx, h DocHead) error {
	divergent := ""
	if len(h.Divergent) > 0 {
		data, err := json.Marshal(h.Divergent)
		if err != nil {
			return fmt.Errorf("workhome: put the head of %s: %w", h.Key, err)
		}
		divergent = string(data)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO document_head (key, path, rev, format, blob, op, last, divergent) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(key) DO UPDATE SET
    path = excluded.path, rev = excluded.rev, format = excluded.format, blob = excluded.blob,
    op = excluded.op, last = excluded.last, divergent = excluded.divergent`,
		h.Key, h.Path, h.Rev, h.Format, h.Blob, h.Op, h.Last, divergent); err != nil {
		return fmt.Errorf("workhome: put the head of %s: %w", h.Key, err)
	}
	return nil
}

const docHeadColumns = `key, path, rev, format, blob, op, last, divergent`

func scanDocHead(sc interface{ Scan(...any) error }) (DocHead, error) {
	var h DocHead
	var divergent string
	if err := sc.Scan(&h.Key, &h.Path, &h.Rev, &h.Format, &h.Blob, &h.Op, &h.Last, &divergent); err != nil {
		return DocHead{}, err
	}
	if divergent != "" {
		if err := json.Unmarshal([]byte(divergent), &h.Divergent); err != nil {
			return DocHead{}, fmt.Errorf("workhome: read the divergent writes of %s: %w", h.Key, err)
		}
	}
	return h, nil
}

func readDocHead(ctx context.Context, q querier, key string) (DocHead, bool, error) {
	h, err := scanDocHead(q.QueryRowContext(ctx, `SELECT `+docHeadColumns+` FROM document_head WHERE key = ?`, key))
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return DocHead{}, false, nil
	case err != nil:
		return DocHead{}, false, fmt.Errorf("workhome: read the head of %s: %w", key, err)
	}
	return h, true, nil
}

// Document returns the head of one document the workspace home keeps whole;
// found is false for a document it has never kept, or one whose head no
// write has reached yet.
func (s *Store) Document(ctx context.Context, key string) (DocHead, bool, error) {
	h, found, err := readDocHead(ctx, s.db, key)
	if err != nil || !found || h.Op == "" {
		return DocHead{}, false, err
	}
	return h, true, nil
}

// Documents returns the head of every document the workspace home keeps
// whole, by key.
func (s *Store) Documents(ctx context.Context) ([]DocHead, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+docHeadColumns+` FROM document_head WHERE op != '' ORDER BY key`)
	if err != nil {
		return nil, fmt.Errorf("workhome: read the document heads: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []DocHead
	for rows.Next() {
		h, err := scanDocHead(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
