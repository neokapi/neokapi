// Package history is the block history of a project: every recorded change to
// an edition of a block, as the context store keeps it.
//
// An edit is recorded as a content.edit operation in the workspace's log
// (core/projector), and the projector projects each operation into the
// block_history table this package owns: one row per edition the edit
// changed, carrying the revisions around the change, the basis a derived
// edition was made from, the identity evidence reconciliation matches on, and
// who made the change through which surface. The table answers "who changed
// this edition, when, and from what", and a rebuild writes the same rows again
// from the log.
//
// The projector is the only writer. Everything else reads.
package history

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/core/reconcile"
	"github.com/neokapi/neokapi/core/storage"
)

// Row is one recorded change to one edition of one block.
type Row struct {
	// Op is the id of the content.edit operation that recorded the change.
	Op string
	// Address is that operation's content address, which every log holding
	// the operation agrees on, whatever id each recorded it under first.
	Address string
	// Doc is the document's key.
	Doc string
	// Block is the block as the read reported it: its durable key where
	// reconciliation assigned one, else the structural name the format gives.
	Block string
	// Key is the durable key reconciliation assigned, when it assigned one.
	Key string
	// Edition is the edition's key in its text form ("en", "fr",
	// "en;channel=short").
	Edition string
	// Before and After are the edition revisions around the change. Before
	// is "absent" for an edition the change created, and After for one it
	// removed.
	Before string
	After  string
	// Basis is the authoritative edition's revision a derived edition was made
	// from, when the change recorded one.
	Basis string
	// ContentHash and ContextHash are the block's identity signals after the
	// change, what core/reconcile matches a later read against.
	ContentHash string
	ContextHash string
	// Actor is who made the change: person, agent or tool, empty when nobody
	// knows (an edit made outside kapi). ActorName and Session say which one.
	Actor     string
	ActorName string
	Session   string
	// Origin is the surface that applied the change: apply, desktop,
	// flow:<name>, merge, pull or observed.
	Origin string
	// At is when the operation was accepted.
	At time.Time
}

// EditionRef names one edition of one block inside a document.
type EditionRef struct {
	Block   string
	Edition string
}

// Store is the block history kept in a project's context store.
type Store struct {
	db *storage.DB
}

var migrations = []storage.Migration{{
	Version:     1,
	Description: "block history",
	// One row per edition an edit changed. The document, the block, the
	// edition and the operation's content address identify it, and the key in
	// that order is also the index every read of one edition's history walks.
	// The address rather than the id, because two logs that recorded one edit
	// under different ids keep the older id once they merge: the row the
	// newer id projected is then the row the older one rewrites, as a rebuild
	// writes it. block_history_reached answers which operation left an
	// edition at a revision without reading the rest of the document.
	SQL: `
CREATE TABLE IF NOT EXISTS block_history (
    op           TEXT NOT NULL,
    address      TEXT NOT NULL,
    doc          TEXT NOT NULL,
    block        TEXT NOT NULL,
    key          TEXT NOT NULL DEFAULT '',
    edition      TEXT NOT NULL DEFAULT '',
    before       TEXT NOT NULL DEFAULT '',
    after        TEXT NOT NULL DEFAULT '',
    basis        TEXT NOT NULL DEFAULT '',
    content_hash TEXT NOT NULL DEFAULT '',
    context_hash TEXT NOT NULL DEFAULT '',
    actor        TEXT NOT NULL DEFAULT '',
    actor_name   TEXT NOT NULL DEFAULT '',
    session      TEXT NOT NULL DEFAULT '',
    origin       TEXT NOT NULL DEFAULT '',
    at           TEXT NOT NULL,
    PRIMARY KEY (doc, block, edition, address)
);
CREATE INDEX IF NOT EXISTS block_history_doc ON block_history(doc, op);
CREATE INDEX IF NOT EXISTS block_history_reached ON block_history(doc, block, edition, after, op);`,
}}

// Open binds the block history to a context database, creating its table.
func Open(db *storage.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("history: no database")
	}
	if err := storage.Migrate(db, "history", migrations); err != nil {
		return nil, fmt.Errorf("history: migrate: %w", err)
	}
	return &Store{db: db}, nil
}

// timeLayout is how an instant is stored: RFC 3339 with nanoseconds in UTC,
// which sorts lexically in time order.
const timeLayout = "2006-01-02T15:04:05.000000000Z"

// Put writes rows in one transaction. A row the store already holds, by its
// document, block, edition and operation address, is overwritten with the
// arriving one: the same operation applied again writes the same values, and
// the operation a merge kept in place of one with the same address writes its
// own id, moment and origin over the one it replaced.
func (s *Store) Put(ctx context.Context, rows []Row) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("history: put: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO block_history (op, address, doc, block, key, edition, before, after, basis,
    content_hash, context_hash, actor, actor_name, session, origin, at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(doc, block, edition, address) DO UPDATE SET
    op = excluded.op, key = excluded.key, before = excluded.before, after = excluded.after,
    basis = excluded.basis, content_hash = excluded.content_hash, context_hash = excluded.context_hash,
    actor = excluded.actor, actor_name = excluded.actor_name, session = excluded.session,
    origin = excluded.origin, at = excluded.at`)
	if err != nil {
		return fmt.Errorf("history: put: %w", err)
	}
	defer func() { _ = stmt.Close() }()
	for _, r := range rows {
		if _, err := stmt.ExecContext(ctx,
			r.Op, r.Address, r.Doc, r.Block, r.Key, r.Edition, r.Before, r.After, r.Basis,
			r.ContentHash, r.ContextHash, r.Actor, r.ActorName, r.Session, r.Origin,
			r.At.UTC().Format(timeLayout)); err != nil {
			return fmt.Errorf("history: put %s %s@%s: %w", r.Doc, r.Block, r.Edition, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("history: put: %w", err)
	}
	return nil
}

const columns = `op, address, doc, block, key, edition, before, after, basis, content_hash, context_hash,
    actor, actor_name, session, origin, at`

// Edition returns the recorded changes to one edition of one block, most
// recent first.
func (s *Store) Edition(ctx context.Context, doc, block, edition string) ([]Row, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+columns+` FROM block_history
WHERE doc = ? AND block = ? AND edition = ? ORDER BY op DESC`, doc, block, edition)
	if err != nil {
		return nil, fmt.Errorf("history: read %s %s@%s: %w", doc, block, edition, err)
	}
	return scan(rows)
}

// LastWrite returns the most recent recorded change to one edition of one
// block: who last wrote it, when, and through which surface. found is false
// when nothing has been recorded for it.
func (s *Store) LastWrite(ctx context.Context, doc, block, edition string) (row Row, found bool, err error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+columns+` FROM block_history
WHERE doc = ? AND block = ? AND edition = ? ORDER BY op DESC LIMIT 1`, doc, block, edition)
	if err != nil {
		return Row{}, false, fmt.Errorf("history: read %s %s@%s: %w", doc, block, edition, err)
	}
	out, err := scan(rows)
	if err != nil || len(out) == 0 {
		return Row{}, false, err
	}
	return out[0], true, nil
}

// Document returns every recorded change in one document, most recent first.
func (s *Store) Document(ctx context.Context, doc string) ([]Row, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+columns+` FROM block_history
WHERE doc = ? ORDER BY op DESC, block, edition`, doc)
	if err != nil {
		return nil, fmt.Errorf("history: read %s: %w", doc, err)
	}
	return scan(rows)
}

// Heads returns, for each edition of a document that has a recorded change,
// the operation that recorded its most recent one.
func (s *Store) Heads(ctx context.Context, doc string) (map[EditionRef]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT block, edition, MAX(op) FROM block_history
WHERE doc = ? GROUP BY block, edition`, doc)
	if err != nil {
		return nil, fmt.Errorf("history: read the heads of %s: %w", doc, err)
	}
	defer func() { _ = rows.Close() }()
	out := map[EditionRef]string{}
	for rows.Next() {
		var ref EditionRef
		var op string
		if err := rows.Scan(&ref.Block, &ref.Edition, &op); err != nil {
			return nil, fmt.Errorf("history: read the heads of %s: %w", doc, err)
		}
		out[ref] = op
	}
	return out, rows.Err()
}

// Reach names an edition of a block at one revision.
type Reach struct {
	Block   string
	Edition string
	Rev     string
}

// Reached returns, for each revision named, the content address of the
// operation that most recently left that edition of a document at it. A
// change that starts from a revision extends that operation. A revision no
// recorded change reached is left out.
//
// Each revision is looked up on its own through the block_history_reached
// index, so the cost follows the revisions asked about rather than the length
// of the document's history.
func (s *Store) Reached(ctx context.Context, doc string, revs []Reach) (map[Reach]string, error) {
	out := map[Reach]string{}
	if len(revs) == 0 {
		return out, nil
	}
	type want struct {
		B string `json:"b"`
		E string `json:"e"`
		R string `json:"r"`
	}
	wants := make([]want, len(revs))
	for i, r := range revs {
		wants[i] = want{B: r.Block, E: r.Edition, R: r.Rev}
	}
	list, err := json.Marshal(wants)
	if err != nil {
		return nil, fmt.Errorf("history: read the revisions of %s: %w", doc, err)
	}
	rows, err := s.db.QueryContext(ctx, reachedQuery, string(list), doc)
	if err != nil {
		return nil, fmt.Errorf("history: read the revisions of %s: %w", doc, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var r Reach
		var address sql.NullString
		if err := rows.Scan(&r.Block, &r.Edition, &r.Rev, &address); err != nil {
			return nil, fmt.Errorf("history: read the revisions of %s: %w", doc, err)
		}
		if address.Valid {
			out[r] = address.String
		}
	}
	return out, rows.Err()
}

// reachedQuery is what Reached runs: each revision of a JSON list (?1) looked
// up in one document (?2).
const reachedQuery = `
SELECT json_extract(w.value, '$.b'), json_extract(w.value, '$.e'), json_extract(w.value, '$.r'),
       (SELECT h.address FROM block_history h
         WHERE h.doc = ?2
           AND h.block = json_extract(w.value, '$.b')
           AND h.edition = json_extract(w.value, '$.e')
           AND h.after = json_extract(w.value, '$.r')
         ORDER BY h.op DESC LIMIT 1)
  FROM json_each(?1) w`

// DocumentHead returns the operation that recorded the most recent change in a
// document, and "" when none is recorded. A reader caching what it derived
// from a document's history keys the cache by it.
func (s *Store) DocumentHead(ctx context.Context, doc string) (string, error) {
	var op sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(op) FROM block_history WHERE doc = ?`, doc).Scan(&op); err != nil {
		return "", fmt.Errorf("history: read the head of %s: %w", doc, err)
	}
	return op.String, nil
}

// Priors returns the identity evidence of every block a document's history
// names, as core/reconcile takes it: one unit per block, keyed by its durable
// key (or its name, where it had none), with the content and context hashes
// of its most recent change.
func (s *Store) Priors(ctx context.Context, doc string) ([]reconcile.Unit, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT CASE WHEN key <> '' THEN key ELSE block END AS k, content_hash, context_hash, MAX(op)
  FROM block_history
 WHERE doc = ? AND content_hash <> ''
 GROUP BY k
 ORDER BY k`, doc)
	if err != nil {
		return nil, fmt.Errorf("history: read the priors of %s: %w", doc, err)
	}
	defer func() { _ = rows.Close() }()
	var out []reconcile.Unit
	for rows.Next() {
		var u reconcile.Unit
		var op string
		if err := rows.Scan(&u.Key, &u.ContentHash, &u.ContextHash, &op); err != nil {
			return nil, fmt.Errorf("history: read the priors of %s: %w", doc, err)
		}
		u.Scope = doc
		out = append(out, u)
	}
	return out, rows.Err()
}

// scan reads rows selected with columns, and closes them.
func scan(rows *sql.Rows) ([]Row, error) {
	defer func() { _ = rows.Close() }()
	var out []Row
	for rows.Next() {
		var r Row
		var at string
		if err := rows.Scan(&r.Op, &r.Address, &r.Doc, &r.Block, &r.Key, &r.Edition, &r.Before, &r.After, &r.Basis,
			&r.ContentHash, &r.ContextHash, &r.Actor, &r.ActorName, &r.Session, &r.Origin, &at); err != nil {
			return nil, fmt.Errorf("history: scan: %w", err)
		}
		t, err := time.Parse(timeLayout, at)
		if err != nil {
			return nil, fmt.Errorf("history: read the moment of %s: %w", r.Op, err)
		}
		r.At = t
		out = append(out, r)
	}
	return out, rows.Err()
}
