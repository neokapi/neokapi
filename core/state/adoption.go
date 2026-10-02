package state

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/reconcile"
	"github.com/neokapi/neokapi/core/storage"
)

// Document adoptions are the project's record of which key each document has.
//
// A checkout resolves a document's key from the documents it has seen before:
// by path, then by what the file holds (reconcile.DocumentUnits), and mints a
// key from the path when nothing claims the file. A checkout that has seen
// nothing before would mint from wherever the file sits now, and a document
// another checkout has followed through a rename would then carry two keys,
// with every decision filed under one of them invisible to the other.
//
// So every adoption is recorded in the workspace's log as a document.adopt
// operation (core/projector), and the projector writes it into the
// document_adoption table. A checkout resolving a read consults those
// adoptions after its own documents, so a fresh checkout finds the key the
// project already uses, by path or by content, before it mints one.

// Adoption is one document adoption: the key a document has, where it was
// read, and what it held there.
type Adoption struct {
	Key  string `json:"key"`
	Path string `json:"path"`
	// Digest is ContentDigest of Content.
	Digest string `json:"digest"`
	// Content is the content hash of each block the document held, in order,
	// which is what recognises the document at another path.
	Content []string `json:"content,omitempty"`
	// Prev is the id of the adoption the key held when this one was made,
	// empty for a key's first adoption.
	Prev string `json:"prev,omitempty"`
	// At is when the adoption was recorded.
	At time.Time `json:"at"`
}

// AdoptionID identifies an adoption by the key, the path, the content and the
// adoption it follows (Prev). Two checkouts that see a document move from the
// same state make the same adoption, and a document that returns to a path or
// a content it held before makes a new one.
func AdoptionID(a Adoption) string {
	sum := sha256.Sum256([]byte(a.Key + "\x00" + a.Path + "\x00" + a.Digest + "\x00" + a.Prev))
	return hex.EncodeToString(sum[:])
}

// ContentDigest is the digest of a document's content as identity resolution
// sees it: the SHA-256 over the content hash of each block, in order.
func ContentDigest(content []string) string {
	sum := sha256.Sum256([]byte(strings.Join(content, "\n")))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// ApplyAdoptions writes adoptions into the document_adoption table of a
// context database, in one transaction. A key keeps its most recent adoption
// (the latest At, then the greater digest, path and id) and the moment it was
// first adopted, so applying adoptions in any order leaves the same rows.
func ApplyAdoptions(ctx context.Context, db *storage.DB, adoptions []Adoption) error {
	if len(adoptions) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("state: apply adoptions: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, a := range adoptions {
		content, err := json.Marshal(a.Content)
		if err != nil {
			return fmt.Errorf("state: encode adoption %q: %w", a.Key, err)
		}
		at := entryTimeText(a.At)
		if _, err := tx.ExecContext(ctx, `
INSERT INTO document_adoption (key, id, path, digest, content, first_at, at) VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(key) DO UPDATE SET
    first_at = MIN(document_adoption.first_at, excluded.first_at),
    id       = CASE WHEN `+laterAdoption+` THEN excluded.id ELSE document_adoption.id END,
    path     = CASE WHEN `+laterAdoption+` THEN excluded.path ELSE document_adoption.path END,
    digest   = CASE WHEN `+laterAdoption+` THEN excluded.digest ELSE document_adoption.digest END,
    content  = CASE WHEN `+laterAdoption+` THEN excluded.content ELSE document_adoption.content END,
    at       = MAX(document_adoption.at, excluded.at)`,
			a.Key, AdoptionID(a), a.Path, a.Digest, string(content), at, at); err != nil {
			return fmt.Errorf("state: apply adoption %q: %w", a.Key, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("state: apply adoptions: %w", err)
	}
	return nil
}

// laterAdoption is the condition under which an arriving adoption replaces
// the one a key holds.
const laterAdoption = `(excluded.at > document_adoption.at OR
    (excluded.at = document_adoption.at AND (excluded.digest > document_adoption.digest OR
        (excluded.digest = document_adoption.digest AND (excluded.path > document_adoption.path OR
            (excluded.path = document_adoption.path AND excluded.id > document_adoption.id))))))`

// adoption is a document_adoption row.
type adoption struct {
	reconcile.DocUnit
	ID     string
	Digest string
}

// adoptions reads the recorded adoptions, the earliest adopted first. A
// handle with no database holds none.
func (w *WorkStore) adoptions(ctx context.Context) ([]adoption, error) {
	if w.db == nil {
		return nil, nil
	}
	rows, err := w.db.QueryContext(ctx,
		`SELECT key, id, path, digest, content FROM document_adoption ORDER BY first_at, key`)
	if err != nil {
		return nil, fmt.Errorf("state: read document adoptions: %w", err)
	}
	defer rows.Close()
	var out []adoption
	for rows.Next() {
		var a adoption
		var content string
		if err := rows.Scan(&a.Key, &a.ID, &a.Path, &a.Digest, &content); err != nil {
			return nil, fmt.Errorf("state: read document adoptions: %w", err)
		}
		if content != "" && content != "null" {
			if err := json.Unmarshal([]byte(content), &a.Content); err != nil {
				return nil, fmt.Errorf("state: parse adoption %q content: %w", a.Key, err)
			}
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// AdoptedDocuments returns the documents the project has adopted, as every
// checkout recorded them: one per key, at the path and with the content of its
// most recent adoption, the earliest adopted first.
func (w *WorkStore) AdoptedDocuments(ctx context.Context) ([]reconcile.DocUnit, error) {
	held, err := w.adoptions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]reconcile.DocUnit, 0, len(held))
	for _, a := range held {
		out = append(out, a.DocUnit)
	}
	return out, nil
}

// recordAdoptions records the adoptions a resolution made that the project
// does not hold yet: a key it has not adopted, or one now read at another path
// or with other content, which follows the adoption the key holds (Prev). They
// go through the journal where there is one, and straight into the table
// otherwise.
func (w *WorkStore) recordAdoptions(ctx context.Context, held []adoption, resolved []reconcile.DocResult, current []reconcile.DocUnit) error {
	if w.db == nil {
		return nil
	}
	byKey := make(map[string]adoption, len(held))
	for _, a := range held {
		byKey[a.Key] = a
	}
	var fresh []Adoption
	for i, r := range resolved {
		digest := ContentDigest(current[i].Content)
		a, ok := byKey[r.Key]
		if ok && a.Path == r.Path && a.Digest == digest {
			continue
		}
		fresh = append(fresh, Adoption{Key: r.Key, Path: r.Path, Digest: digest, Content: current[i].Content, Prev: a.ID})
	}
	if len(fresh) == 0 {
		return nil
	}
	if w.journal != nil {
		if err := w.journal.RecordAdoptions(ctx, fresh); err != nil {
			return fmt.Errorf("state: record document adoptions: %w", err)
		}
		return nil
	}
	now := w.clock()
	for i := range fresh {
		fresh[i].At = now
	}
	return ApplyAdoptions(ctx, w.db, fresh)
}
