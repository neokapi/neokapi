package state

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/neokapi/neokapi/core/reconcile"
	"github.com/neokapi/neokapi/core/storage"
)

// WorkStore is the project's decision ledger and one checkout's view of it.
//
// The ledger is append-only and content-addressed (see ledger.go). It is the
// authority: a decision is durable the moment it is recorded, and nothing is
// ever rewritten in place.
//
// The view answers "which pairing does this unit have HERE". One ledger serves
// every checkout of a project, and several of them are on different branches at
// once, so the store holds one view per checkout and every read goes through
// it. A decision recorded on one branch answers for another exactly when that
// branch's files carry the same source and the same translation, which is what
// keeps one branch's approvals out of another's record without anything having
// to move.
//
// The committed shards under `.kapi/state/` are this checkout's export of the
// view, and an import source for the ledger. `kapi commit` writes them; opening
// the store reads them back. A fresh clone restores its decisions that way, and
// so does a checkout picking up a colleague's after `git pull`.
type WorkStore struct {
	db        *storage.DB
	committed string // this checkout's committed record directory
	checkout  string // the view this handle reads and writes

	// ownsDB records whether Close may close the pool: true when this store
	// opened its own file, false when it adopted the project's merged store,
	// whose owner closes it once for all subsystems.
	ownsDB bool

	// mu guards policy, which a caller may replace while another goroutine
	// records.
	mu     sync.Mutex
	policy Policy

	// now is the clock entries are stamped from. Bound here so a test can pin
	// it and so every stamp in one process comes from one source.
	now func() time.Time

	// journal, when set, is the operation log every ledger entry is recorded
	// in and applied from (core/projector). A store with no journal, which is
	// the embedded layout a test opens, writes its ledger directly.
	journal Journal

	// mem is the browser fallback: the wasm build has no file-backed SQLite
	// (storage.ErrNoSQLite), yet the review loop must still work in the lab.
	// The ledger and the view live in process memory and persist to a JSON
	// sidecar next to where the database would sit. nil on every build with a
	// real driver.
	mem *memWork
}

var workMigrations = []storage.Migration{{
	Version:     1,
	Description: "unit working set",
	SQL: `
CREATE TABLE IF NOT EXISTS unit_state (
    unit         TEXT NOT NULL,
    variant      TEXT NOT NULL,
    scope        TEXT NOT NULL DEFAULT '',
    content_hash TEXT NOT NULL DEFAULT '',
    context_hash TEXT NOT NULL DEFAULT '',
    payload      TEXT NOT NULL,
    staged       INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (unit, variant)
);
CREATE INDEX IF NOT EXISTS unit_state_content ON unit_state(content_hash);
CREATE INDEX IF NOT EXISTS unit_state_context ON unit_state(scope, context_hash);
CREATE INDEX IF NOT EXISTS unit_state_staged  ON unit_state(staged) WHERE staged = 1;`,
}, {
	Version:     2,
	Description: "document identity",
	// A document's PATH cannot be derived from the units it holds, and path is
	// the strongest signal for matching a document, so it is recorded. The key
	// is stable across a rename; the path is wherever the file lives now.
	SQL: `
CREATE TABLE IF NOT EXISTS document (
    key  TEXT NOT NULL PRIMARY KEY,
    path TEXT NOT NULL
);`,
}, {
	Version:     3,
	Description: "unit identity carries its document",
	// The document belongs in the key. A unit id is unique inside its document
	// and nowhere wider, so (unit, variant) made two documents that share an id
	// one row, and the second decision recorded overwrote the first.
	SQL: `
CREATE TABLE unit_state_scoped (
    scope        TEXT NOT NULL DEFAULT '',
    unit         TEXT NOT NULL,
    variant      TEXT NOT NULL,
    content_hash TEXT NOT NULL DEFAULT '',
    context_hash TEXT NOT NULL DEFAULT '',
    payload      TEXT NOT NULL,
    staged       INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (scope, unit, variant)
);
INSERT INTO unit_state_scoped (scope, unit, variant, content_hash, context_hash, payload, staged)
    SELECT scope, unit, variant, content_hash, context_hash, payload, staged FROM unit_state;
DROP TABLE unit_state;
ALTER TABLE unit_state_scoped RENAME TO unit_state;
CREATE INDEX IF NOT EXISTS unit_state_content ON unit_state(content_hash);
CREATE INDEX IF NOT EXISTS unit_state_context ON unit_state(scope, context_hash);
CREATE INDEX IF NOT EXISTS unit_state_staged  ON unit_state(staged) WHERE staged = 1;`,
}, {
	Version:     4,
	Description: "a document records what it held",
	// Path alone cannot recognise a document that moved. What it CONTAINED can:
	// the content hash of each block it held, which is what reconcile grades a
	// candidate against. Without it a rename is indistinguishable from a
	// deletion plus an unrelated new file, and every decision in the file is
	// orphaned, silently, since nothing fails.
	SQL: `
ALTER TABLE document ADD COLUMN content TEXT NOT NULL DEFAULT '[]';
CREATE INDEX IF NOT EXISTS document_path ON document(path);`,
}, {
	Version:     5,
	Description: "the working set records which record it was built from",
	// The committed shards are git-tracked and move under a checkout, so the
	// store holds the digest of the shards it last imported and compares it
	// against the ones on disk.
	SQL: `
CREATE TABLE IF NOT EXISTS state_meta (
    key   TEXT NOT NULL PRIMARY KEY,
    value TEXT NOT NULL
);`,
}, {
	Version:     6,
	Description: "decision ledger and per-checkout view",
	// The ledger is append-only and addressed by content, so recording the same
	// decision twice records it once. The view names the pairing each unit has
	// in one checkout, and every read of a unit's state goes through it: one
	// ledger serves checkouts that sit on different branches at the same time,
	// and a decision answers only where its pairing appears.
	//
	// The document table gains the same dimension for the same reason. Where a
	// document lives is a property of a checkout, not of the project.
	SQL: `
CREATE TABLE IF NOT EXISTS unit_decision (
    id           TEXT NOT NULL PRIMARY KEY,
    scope        TEXT NOT NULL DEFAULT '',
    unit         TEXT NOT NULL,
    variant      TEXT NOT NULL,
    content_hash TEXT NOT NULL DEFAULT '',
    target_hash  TEXT NOT NULL DEFAULT '',
    actor        TEXT NOT NULL DEFAULT '',
    origin       TEXT NOT NULL DEFAULT '',
    recorded_at  TEXT NOT NULL,
    revoked      INTEGER NOT NULL DEFAULT 0,
    payload      TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS unit_decision_pairing
    ON unit_decision(scope, unit, variant, content_hash, target_hash, recorded_at);
CREATE TABLE IF NOT EXISTS unit_view (
    checkout     TEXT NOT NULL,
    scope        TEXT NOT NULL DEFAULT '',
    unit         TEXT NOT NULL,
    variant      TEXT NOT NULL,
    content_hash TEXT NOT NULL DEFAULT '',
    target_hash  TEXT NOT NULL DEFAULT '',
    exported     INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (checkout, scope, unit, variant)
);
CREATE TABLE IF NOT EXISTS checkout (
    id   TEXT NOT NULL PRIMARY KEY,
    path TEXT NOT NULL
);
ALTER TABLE document RENAME TO document_legacy;
CREATE TABLE document (
    checkout TEXT NOT NULL,
    key      TEXT NOT NULL,
    path     TEXT NOT NULL,
    content  TEXT NOT NULL DEFAULT '[]',
    PRIMARY KEY (checkout, key)
);
CREATE INDEX IF NOT EXISTS document_at_path ON document(checkout, path);`,
}}

// metaCommittedDigest keys the digest of the shards a checkout's view was
// imported from. One key per checkout: several checkouts share the ledger and
// each holds its own shards.
func metaCommittedDigest(checkout string) string { return "committed.digest:" + checkout }

// OpenWork opens the store at dbPath for the checkout whose committed record is
// at committedPath.
func OpenWork(ctx context.Context, dbPath, committedPath string) (*WorkStore, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("state: work dir: %w", err)
	}
	db, err := storage.Open(dbPath)
	if err != nil {
		if errors.Is(err, storage.ErrNoSQLite) {
			return OpenWorkSidecar(ctx, strings.TrimSuffix(dbPath, filepath.Ext(dbPath))+".json", committedPath)
		}
		return nil, fmt.Errorf("state: open work store: %w", err)
	}
	if err := storage.Migrate(db, "state", workMigrations); err != nil {
		db.Close()
		return nil, fmt.Errorf("state: migrate work store: %w", err)
	}
	w := newStore(db, committedPath)
	w.ownsDB = true
	if err := w.start(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return w, nil
}

// OpenWorkFromDB adopts an already-open database: the project's store, where
// the ledger is one schema among the content memory, the terms store and the
// block cache. Same migrations, same `state` ledger, same import from this
// checkout's shards; only the file is shared.
//
// It is what makes an approve-and-promote atomic: the decision and the wording
// the content memory learns from it are two writes on one connection pool.
//
// The returned store does not own db; its owner closes the pool.
func OpenWorkFromDB(ctx context.Context, db *storage.DB, committedPath string) (*WorkStore, error) {
	if db == nil {
		return nil, errors.New("state: adopt work store: nil database")
	}
	if err := storage.Migrate(db, "state", workMigrations); err != nil {
		return nil, fmt.Errorf("state: migrate work store: %w", err)
	}
	w := newStore(db, committedPath)
	if err := w.start(ctx); err != nil {
		return nil, err
	}
	return w, nil
}

// OpenLedger adopts an already-open context database to read and write the
// decision ledger with no checkout in hand.
//
// A workspace keeps one context store per project, outside every checkout, and
// a project registered there may have no checkout on this machine at all. A
// whole-workspace export or restore therefore reaches the ledger directly:
// this handle registers no checkout, reads no committed record, and points no
// view at what it records. Ledger is what it reads, and a decision it writes
// answers for a checkout as soon as one opens the store and imports its own
// record.
//
// The returned store does not own db; its owner closes the pool.
func OpenLedger(_ context.Context, db *storage.DB) (*WorkStore, error) {
	if db == nil {
		return nil, errors.New("state: open ledger: nil database")
	}
	if err := storage.Migrate(db, "state", workMigrations); err != nil {
		return nil, fmt.Errorf("state: migrate work store: %w", err)
	}
	return newStore(db, ""), nil
}

// OpenWorkSidecar opens the JSON-sidecar store at sidecarPath: the browser
// build's ledger, and the only form it takes where there is no file-backed
// SQLite driver. Callers on a build with a driver reach it only to read a
// sidecar some earlier browser session wrote.
func OpenWorkSidecar(ctx context.Context, sidecarPath, committedPath string) (*WorkStore, error) {
	if err := os.MkdirAll(filepath.Dir(sidecarPath), 0o755); err != nil {
		return nil, fmt.Errorf("state: work dir: %w", err)
	}
	w := newStore(nil, committedPath)
	w.mem = newMemWork(sidecarPath)
	if err := w.mem.load(w.now()); err != nil {
		return nil, err
	}
	if err := w.start(ctx); err != nil {
		return nil, err
	}
	return w, nil
}

func newStore(db *storage.DB, committedPath string) *WorkStore {
	return &WorkStore{
		db:        db,
		committed: committedPath,
		checkout:  checkoutID(committedPath),
		policy:    AllowAny,
		now:       time.Now,
	}
}

// start brings a freshly opened handle up: it carries any pre-ledger rows
// across and registers the checkout.
//
// It reads no committed record. A project's decisions live in the ledger, and
// a directory of shards in the checkout is what `kapi context export` wrote;
// `kapi context import` reads one back. An open that imported them would let
// whichever branch a checkout sits on decide what the whole project holds.
func (w *WorkStore) start(ctx context.Context) error {
	if err := w.carryLegacyRows(ctx); err != nil {
		return err
	}
	return w.registerCheckout(ctx)
}

// checkoutID names the view a handle reads: the SHA-256 of the absolute path
// its committed record directory would sit at, `.kapi/state/` inside the
// checkout. Two worktrees, two clones and two branches checked out side by side
// each have their own, so each has its own view of one ledger.
//
// The value is the path STRING. Nothing is read there and the directory need
// not exist; what the hash buys is an identity per checkout that survives a
// process, which is what lets a decision recorded on one branch stay out of
// another branch's view.
//
// A handle opened with no committed record directory (OpenLedger) names no
// view. It reads and writes the ledger and leaves every checkout's view alone.
func checkoutID(committedPath string) string {
	if committedPath == "" {
		return ""
	}
	abs, err := filepath.Abs(committedPath)
	if err != nil {
		abs = committedPath
	}
	sum := sha256.Sum256([]byte(filepath.Clean(abs)))
	return hex.EncodeToString(sum[:16])
}

// SetPolicy replaces the rule that decides whether an actor may record a
// transition. The default is AllowAny.
func (w *WorkStore) SetPolicy(p Policy) {
	if p == nil {
		p = AllowAny
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.policy = p
}

func (w *WorkStore) currentPolicy() Policy {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.policy
}

// SetClock binds the clock entries are stamped from. Tests use it to make
// recorded order explicit.
func (w *WorkStore) SetClock(now func() time.Time) {
	if now == nil {
		now = time.Now
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.now = now
}

func (w *WorkStore) clock() time.Time {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.now()
}

func (w *WorkStore) Close() error {
	if w.mem != nil || !w.ownsDB {
		return nil
	}
	return w.db.Close()
}

// registerCheckout records which record directory a view id stands for, so the
// table is readable by someone looking at a project store from outside.
func (w *WorkStore) registerCheckout(ctx context.Context) error {
	if w.mem != nil {
		return nil
	}
	abs, err := filepath.Abs(w.committed)
	if err != nil {
		abs = w.committed
	}
	_, err = w.db.ExecContext(ctx, `
INSERT INTO checkout (id, path) VALUES (?, ?)
ON CONFLICT(id) DO UPDATE SET path = excluded.path`, w.checkout, filepath.Clean(abs))
	if err != nil {
		return fmt.Errorf("state: register checkout: %w", err)
	}
	return nil
}

// Import records this checkout's committed shards in the ledger and rebuilds
// its view from them.
//
// Recording is idempotent: a line the ledger already holds has the same content
// address, so importing the same record any number of times holds it once. The
// digest of the shards is stamped per checkout, so an import that would find
// nothing new costs one pass over the directory and stops there.
//
// The view is rebuilt from the shards rather than merged with them, because the
// shards are git-tracked and a branch switch replaces all of them. One thing
// survives that rebuild: a row this checkout has recorded and not yet written
// out, which no record supplies. Such a row follows the person who made it, and
// the entry behind it answers only where its pairing appears, so a decision
// carried onto another branch writes a line there and claims nothing about that
// branch's wording.
//
// It runs at every open and again before every write of the record, so a
// process holding the store open across a branch switch exports the record this
// checkout holds.
func (w *WorkStore) Import(ctx context.Context) error {
	if w.committed == "" {
		// A handle with no checkout (OpenLedger) has no record to read.
		return nil
	}
	digest, err := CommittedDigest(w.committed)
	if err != nil {
		return err
	}
	stamp, stamped, err := w.readMeta(ctx, metaCommittedDigest(w.checkout))
	if err != nil {
		return err
	}
	if stamped && stamp == digest {
		return nil
	}
	units, err := ReadCommitted(w.committed)
	if err != nil {
		return err
	}
	if err := w.importUnits(ctx, units); err != nil {
		return err
	}
	return w.stampCommitted(ctx, digest)
}

// importUnits is the body of an import: the ledger gains every line that is
// newer than what already answers for its pairing, and the view is rebuilt
// around whatever this checkout has recorded since its last export.
//
// A line older than the entry in force is not recorded. That is the same
// last-writer-wins rule a venue pull follows, and it is what keeps a
// colleague's earlier line from displacing a decision made here since.
func (w *WorkStore) importUnits(ctx context.Context, units []UnitState) error {
	if w.mem != nil {
		keep := map[Key]memViewRow{}
		for k, r := range w.mem.view {
			if !r.Exported {
				keep[k] = r
			}
		}
		w.mem.view = map[Key]memViewRow{}
		now := w.clock()
		for _, u := range units {
			if w.arrivalStands(ctx, u) {
				if err := w.mem.record(u, u.Decision.By, OriginImport, false, entryTimeText(now), false); err != nil {
					return err
				}
			}
			w.mem.view[u.Key()] = memViewRow{
				Scope: u.Scope, Unit: u.Unit, Variant: u.Variant,
				ContentHash: u.ContentHash, TargetHash: u.TargetHash, Exported: true,
			}
		}
		maps.Copy(w.mem.view, keep)
		return w.mem.persist()
	}

	keep, err := w.unexportedRows(ctx)
	if err != nil {
		return err
	}
	// Resolved before the transaction opens: the write gate is not reentrant,
	// so a read that takes a permit cannot run inside one.
	stands := make([]bool, len(units))
	for i, u := range units {
		stands[i] = w.arrivalStands(ctx, u)
	}
	now := w.clock()
	if w.journal != nil {
		// The ledger's side goes through the journal first; what is left for
		// this transaction is the checkout's view.
		var entries []JournalEntry
		for i, u := range units {
			if !stands[i] {
				continue
			}
			entry, changes, err := w.entryFor(ctx, u, u.Decision.By, OriginImport, false, now, false)
			if err != nil {
				return err
			}
			if changes {
				entries = append(entries, entry)
			}
		}
		if err := w.journal.RecordEntries(ctx, entries); err != nil {
			return fmt.Errorf("state: import record: %w", err)
		}
		stands = make([]bool, len(units))
	}

	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("state: import record: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM unit_view WHERE checkout = ?`, w.checkout); err != nil {
		return fmt.Errorf("state: clear checkout view: %w", err)
	}
	for i, u := range units {
		if stands[i] {
			if err := insertEntry(ctx, tx, u, u.Decision.By, OriginImport, false, now, false); err != nil {
				return err
			}
		}
		if err := putView(ctx, tx, w.checkout, u.Pairing(), true); err != nil {
			return err
		}
	}
	for _, r := range keep {
		if err := putView(ctx, tx, w.checkout, r, false); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("state: import record: %w", err)
	}
	return nil
}

// arrivalStands reports whether a record coming in from elsewhere is newer
// than whatever answers for its pairing now.
func (w *WorkStore) arrivalStands(ctx context.Context, u UnitState) bool {
	inForce, applied := w.applies(ctx, u.Pairing())
	return !applied || Supersedes(u, inForce)
}

// unexportedRows returns the view rows this checkout has recorded since its
// last export.
func (w *WorkStore) unexportedRows(ctx context.Context) ([]Pairing, error) {
	rows, err := w.db.QueryContext(ctx, `
SELECT scope, unit, variant, content_hash, target_hash
  FROM unit_view WHERE checkout = ? AND exported = 0`, w.checkout)
	if err != nil {
		return nil, fmt.Errorf("state: read checkout view: %w", err)
	}
	defer rows.Close()
	return scanPairings(rows)
}

func scanPairings(rows *sql.Rows) ([]Pairing, error) {
	var out []Pairing
	for rows.Next() {
		var p Pairing
		var variant string
		if err := rows.Scan(&p.Key.Scope, &p.Key.Unit, &variant, &p.ContentHash, &p.TargetHash); err != nil {
			return nil, fmt.Errorf("state: scan view row: %w", err)
		}
		if err := p.Key.Variant.UnmarshalText([]byte(variant)); err != nil {
			return nil, fmt.Errorf("state: parse variant %q: %w", variant, err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// RecordEntry appends one entry to the ledger and points this checkout's view
// at its pairing.
//
// The entry is durable at once. There is no tier between recording and the
// project's record: `kapi commit` writes the shards, and what it writes is what
// the ledger already holds.
func (w *WorkStore) RecordEntry(ctx context.Context, u UnitState, actor string, origin EntryOrigin) error {
	return w.append(ctx, u, actor, origin, false)
}

// Put records a decision reached in this checkout.
func (w *WorkStore) Put(ctx context.Context, u UnitState) error {
	return w.append(ctx, u, u.Decision.By, OriginLocal, false)
}

// Record stores what the loop produced rather than what a person decided: a
// unit's basis, the source it translated and the translation it wrote, with no
// Decision on it.
func (w *WorkStore) Record(ctx context.Context, u UnitState) error {
	return w.append(ctx, u, u.Decision.By, OriginRun, false)
}

// Delete withdraws whatever applies to the unit in this checkout: a revocation
// entry at the unit's current pairing, and the view row removed. The ledger
// keeps everything it held, so the unit's history stays readable and a pairing
// that comes back comes back to what was decided about it.
func (w *WorkStore) Delete(ctx context.Context, k Key) error {
	p, ok, err := w.pairingOf(ctx, k)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	held, _ := w.applies(ctx, p)
	held.Scope, held.Unit, held.Variant = k.Scope, k.Unit, k.Variant
	held.ContentHash, held.TargetHash = p.ContentHash, p.TargetHash
	if err := w.append(ctx, held, held.Decision.By, OriginLocal, true); err != nil {
		return err
	}
	return w.dropView(ctx, k)
}

// append is the one writer: it resolves what applies at the entry's pairing,
// puts the transition to the policy, and records.
//
// Recording the entry that already answers for its pairing changes nothing in
// the ledger: the entry keeps its place and its moment, and only this
// checkout's view is pointed at the pairing. An entry the ledger holds that no
// longer answers is re-asserted, which moves its moment forward so it answers
// again.
func (w *WorkStore) append(ctx context.Context, u UnitState, actor string, origin EntryOrigin, revoked bool) error {
	p := u.Pairing()
	applies, applied := w.applies(ctx, p)
	err := w.currentPolicy()(Transition{
		Pairing: p, Actor: actor, Origin: origin,
		Applies: applies, Applied: applied,
		Proposed: u, Revoke: revoked,
	})
	if err != nil {
		return fmt.Errorf("state: record %s/%s: %w", u.Scope, u.Unit, err)
	}
	stamp := w.clock()

	if w.journal != nil && w.mem == nil {
		return w.appendJournaled(ctx, u, actor, origin, revoked, stamp)
	}

	if w.mem != nil {
		if rerr := w.mem.record(u, actor, origin, revoked, entryTimeText(stamp), true); rerr != nil {
			return rerr
		}
		if !revoked {
			w.mem.view[u.Key()] = memViewRow{
				Scope: u.Scope, Unit: u.Unit, Variant: u.Variant,
				ContentHash: u.ContentHash, TargetHash: u.TargetHash,
			}
		}
		return w.mem.persist()
	}

	id, err := Address(u, actor, revoked)
	if err != nil {
		return err
	}
	answering, err := w.answering(ctx, p)
	if err != nil {
		return err
	}
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("state: record decision: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if answering != id {
		if err := insertEntry(ctx, tx, u, actor, origin, revoked, stamp, true); err != nil {
			return err
		}
	}
	if !revoked && w.checkout != "" {
		if err := putView(ctx, tx, w.checkout, p, false); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("state: record decision: %w", err)
	}
	return nil
}

// insertEntry writes one entry. An address the ledger already holds is left
// exactly as it was, which is what makes an import and a repeated pull cost
// nothing.
func insertEntry(ctx context.Context, tx *storage.Tx, u UnitState, actor string, origin EntryOrigin, revoked bool, stamp time.Time, reassert bool) error {
	id, err := Address(u, actor, revoked)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(u)
	if err != nil {
		return fmt.Errorf("state: marshal unit: %w", err)
	}
	variant, _ := u.Variant.MarshalText()
	flag := 0
	if revoked {
		flag = 1
	}
	conflict := "ON CONFLICT(id) DO NOTHING"
	if reassert {
		conflict = "ON CONFLICT(id) DO UPDATE SET recorded_at = excluded.recorded_at"
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO unit_decision
    (id, scope, unit, variant, content_hash, target_hash, actor, origin, recorded_at, revoked, payload)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`+conflict,
		id, u.Scope, u.Unit, string(variant), u.ContentHash, u.TargetHash,
		actor, string(origin), entryTimeText(stamp), flag, string(payload))
	if err != nil {
		return fmt.Errorf("state: record entry: %w", err)
	}
	return nil
}

// putView points a checkout's view at a pairing.
func putView(ctx context.Context, tx *storage.Tx, checkout string, p Pairing, exported bool) error {
	variant, _ := p.Key.Variant.MarshalText()
	flag := 0
	if exported {
		flag = 1
	}
	_, err := tx.ExecContext(ctx, `
INSERT INTO unit_view (checkout, scope, unit, variant, content_hash, target_hash, exported)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(checkout, scope, unit, variant) DO UPDATE SET
    content_hash = excluded.content_hash,
    target_hash  = excluded.target_hash,
    exported     = excluded.exported`,
		checkout, p.Key.Scope, p.Key.Unit, string(variant), p.ContentHash, p.TargetHash, flag)
	if err != nil {
		return fmt.Errorf("state: point view at pairing: %w", err)
	}
	return nil
}

// ClearView empties this checkout's view, so the checkout holds no unit state
// until something puts it back. The ledger keeps every entry, and a pairing
// that comes back comes back to what was decided about it.
//
// It is what "replace the project's context with this bundle" means for
// decisions: the bundle decides what the checkout holds, and the decisions it
// does not carry stop answering here without being erased from the record of
// what was decided.
func (w *WorkStore) ClearView(ctx context.Context) error {
	if w.mem != nil {
		w.mem.view = map[Key]memViewRow{}
		return w.mem.persist()
	}
	if _, err := w.db.ExecContext(ctx,
		`DELETE FROM unit_view WHERE checkout = ?`, w.checkout); err != nil {
		return fmt.Errorf("state: clear checkout view: %w", err)
	}
	return nil
}

func (w *WorkStore) dropView(ctx context.Context, k Key) error {
	if w.mem != nil {
		delete(w.mem.view, k)
		return w.mem.persist()
	}
	variant, _ := k.Variant.MarshalText()
	_, err := w.db.ExecContext(ctx,
		`DELETE FROM unit_view WHERE checkout = ? AND scope = ? AND unit = ? AND variant = ?`,
		w.checkout, k.Scope, k.Unit, string(variant))
	if err != nil {
		return fmt.Errorf("state: drop view row: %w", err)
	}
	return nil
}

// pairingOf returns the pairing a unit has in this checkout.
func (w *WorkStore) pairingOf(ctx context.Context, k Key) (Pairing, bool, error) {
	if w.mem != nil {
		r, ok := w.mem.view[k]
		if !ok {
			return Pairing{}, false, nil
		}
		return r.pairing(), true, nil
	}
	variant, _ := k.Variant.MarshalText()
	p := Pairing{Key: k}
	err := w.db.QueryRowContext(ctx, `
SELECT content_hash, target_hash FROM unit_view
 WHERE checkout = ? AND scope = ? AND unit = ? AND variant = ?`,
		w.checkout, k.Scope, k.Unit, string(variant)).Scan(&p.ContentHash, &p.TargetHash)
	if errors.Is(err, sql.ErrNoRows) {
		return Pairing{}, false, nil
	}
	if err != nil {
		return Pairing{}, false, fmt.Errorf("state: read view row: %w", err)
	}
	return p, true, nil
}

// applies returns the record in force at a pairing: the most recent entry
// recorded for it, unless that entry withdraws.
func (w *WorkStore) applies(ctx context.Context, p Pairing) (UnitState, bool) {
	if w.mem != nil {
		return w.mem.applies(p)
	}
	variant, _ := p.Key.Variant.MarshalText()
	var payload string
	var revoked int
	err := w.db.QueryRowContext(ctx, `
SELECT payload, revoked FROM unit_decision
 WHERE scope = ? AND unit = ? AND variant = ? AND content_hash = ? AND target_hash = ?
 ORDER BY recorded_at DESC, rowid DESC LIMIT 1`,
		p.Key.Scope, p.Key.Unit, string(variant), p.ContentHash, p.TargetHash).Scan(&payload, &revoked)
	if err != nil || revoked == 1 {
		return UnitState{}, false
	}
	var u UnitState
	if json.Unmarshal([]byte(payload), &u) != nil {
		return UnitState{}, false
	}
	return u, true
}

// Lookup answers what applies to a unit at a given pairing, which is the
// question every reader of unit state is really asking: this unit, with this
// source and this translation in front of me, what has been decided about it.
//
// A caller holding the file content passes its hashes and gets the entry that
// blessed exactly that pairing, whatever any other checkout has decided about
// the same unit.
func (w *WorkStore) Lookup(ctx context.Context, k Key, contentHash, targetHash string) (UnitState, bool) {
	return w.applies(ctx, Pairing{Key: k, ContentHash: contentHash, TargetHash: targetHash})
}

// Get returns what applies to a unit in this checkout: the entry recorded for
// the pairing the unit has here. A unit this checkout does not hold has no
// answer, however much the ledger holds about it under another branch's
// pairing.
func (w *WorkStore) Get(ctx context.Context, k Key) (UnitState, bool) {
	p, ok, err := w.pairingOf(ctx, k)
	if err != nil || !ok {
		return UnitState{}, false
	}
	return w.applies(ctx, p)
}

// All returns what applies to every unit in this checkout, ordered by the
// identity key so a serialization of it is stable.
func (w *WorkStore) All(ctx context.Context) ([]UnitState, error) {
	return w.resolveView(ctx, "")
}

// Ledger returns the entry in force at every pairing the ledger holds,
// whatever checkout recorded it and whether or not a view still points at it.
// A pairing whose most recent entry revokes it is left out. Ordered by
// identity, so a serialization of the result is stable.
//
// It is what a backup of a project's decisions carries. A view belongs to one
// checkout, and a workspace holds projects whose checkouts sit on another
// machine or nowhere, so the ledger is the only reading of "what this project
// has decided" that does not need a working tree in hand.
func (w *WorkStore) Ledger(ctx context.Context) ([]UnitState, error) {
	if w.mem != nil {
		out := make([]UnitState, 0, len(w.mem.latest))
		for p := range w.mem.latest {
			if u, ok := w.mem.applies(p); ok {
				out = append(out, u)
			}
		}
		sortUnits(out)
		return out, nil
	}
	const query = `
WITH latest AS (
  SELECT scope, unit, variant, payload, revoked,
         ROW_NUMBER() OVER (
             PARTITION BY scope, unit, variant, content_hash, target_hash
             ORDER BY recorded_at DESC, rowid DESC) AS nth
    FROM unit_decision
)
SELECT payload FROM latest
 WHERE nth = 1 AND revoked = 0
 ORDER BY scope, unit, variant`
	rows, err := w.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("state: read the decision ledger: %w", err)
	}
	defer rows.Close()
	units, err := scanUnits(rows)
	if err != nil {
		return nil, err
	}
	sortUnits(units)
	return units, nil
}

// Priors returns the identity signals for every unit in a document, which is
// what core/reconcile matches a fresh read against.
//
// Scoped to one document because that is how reconcile is called, but content
// matching stays project-wide: pass the whole project's units when text may
// have moved between files.
func (w *WorkStore) Priors(ctx context.Context, scope string) ([]UnitState, error) {
	return w.resolveView(ctx, scope)
}

// resolveView answers this checkout's view against the ledger. scope limits it
// to one document; empty takes the whole view.
func (w *WorkStore) resolveView(ctx context.Context, scope string) ([]UnitState, error) {
	if w.mem != nil {
		out := make([]UnitState, 0, len(w.mem.view))
		for _, r := range w.mem.rows() {
			if scope != "" && r.Scope != scope {
				continue
			}
			if u, ok := w.mem.applies(r.pairing()); ok {
				out = append(out, u)
			}
		}
		return out, nil
	}
	// The window function picks each pairing's most recent entry once, over the
	// whole ledger, rather than asking the same question again for every unit
	// in the view.
	const query = `
WITH latest AS (
  SELECT scope, unit, variant, content_hash, target_hash, payload, revoked,
         ROW_NUMBER() OVER (
             PARTITION BY scope, unit, variant, content_hash, target_hash
             ORDER BY recorded_at DESC, rowid DESC) AS nth
    FROM unit_decision
)
SELECT l.payload
  FROM unit_view v
  JOIN latest l
    ON l.scope = v.scope AND l.unit = v.unit AND l.variant = v.variant
   AND l.content_hash = v.content_hash AND l.target_hash = v.target_hash
 WHERE v.checkout = ? AND l.nth = 1 AND l.revoked = 0 AND (? = '' OR v.scope = ?)
 ORDER BY v.scope, v.unit, v.variant`
	rows, err := w.db.QueryContext(ctx, query, w.checkout, scope, scope)
	if err != nil {
		return nil, fmt.Errorf("state: read checkout record: %w", err)
	}
	defer rows.Close()
	return scanUnits(rows)
}

// Entries returns every entry the ledger holds for a unit, most recent first.
// It is the unit's decision history, which the ledger keeps because nothing is
// ever rewritten.
func (w *WorkStore) Entries(ctx context.Context, k Key) ([]Entry, error) {
	if w.mem != nil {
		var out []Entry
		for _, e := range slices.Backward(w.mem.entries) {
			if e.State.Key() != k {
				continue
			}
			stamp, _ := time.Parse(entryTimeLayout, e.Recorded)
			out = append(out, Entry{
				ID: e.ID, State: e.State, Actor: e.Actor,
				Origin: e.Origin, Recorded: stamp, Revoked: e.Revoked,
			})
		}
		sort.SliceStable(out, func(i, j int) bool { return out[i].Recorded.After(out[j].Recorded) })
		return out, nil
	}
	variant, _ := k.Variant.MarshalText()
	rows, err := w.db.QueryContext(ctx, `
SELECT id, actor, origin, recorded_at, revoked, payload FROM unit_decision
 WHERE scope = ? AND unit = ? AND variant = ?
 ORDER BY recorded_at DESC, rowid DESC`, k.Scope, k.Unit, string(variant))
	if err != nil {
		return nil, fmt.Errorf("state: read unit history: %w", err)
	}
	defer rows.Close()

	var out []Entry
	for rows.Next() {
		var e Entry
		var origin, stamp, payload string
		var revoked int
		if err := rows.Scan(&e.ID, &e.Actor, &origin, &stamp, &revoked, &payload); err != nil {
			return nil, fmt.Errorf("state: scan entry: %w", err)
		}
		if err := json.Unmarshal([]byte(payload), &e.State); err != nil {
			return nil, fmt.Errorf("state: parse entry: %w", err)
		}
		e.Origin = EntryOrigin(origin)
		e.Revoked = revoked == 1
		e.Recorded, _ = time.Parse(entryTimeLayout, stamp)
		out = append(out, e)
	}
	return out, rows.Err()
}

func scanUnits(rows *sql.Rows) ([]UnitState, error) {
	var out []UnitState
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, fmt.Errorf("state: scan unit: %w", err)
		}
		var u UnitState
		if err := json.Unmarshal([]byte(payload), &u); err != nil {
			return nil, fmt.Errorf("state: parse unit: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// readMeta reads one of this store's own stamps. ok is false when the key was
// never written.
func (w *WorkStore) readMeta(ctx context.Context, key string) (value string, ok bool, err error) {
	if w.mem != nil {
		if w.mem.committed == "" {
			return "", false, nil
		}
		return w.mem.committed, true, nil
	}
	err = w.db.QueryRowContext(ctx, `SELECT value FROM state_meta WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("state: read %q: %w", key, err)
	}
	return value, true, nil
}

// stampCommitted records the digest of the shards this checkout's view now
// projects.
func (w *WorkStore) stampCommitted(ctx context.Context, digest string) error {
	if w.mem != nil {
		w.mem.committed = digest
		return w.mem.persist()
	}
	_, err := w.db.ExecContext(ctx, `
INSERT INTO state_meta (key, value) VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value`, metaCommittedDigest(w.checkout), digest)
	if err != nil {
		return fmt.Errorf("state: stamp committed record: %w", err)
	}
	return nil
}

// RecordDiff is what writing the committed shards would change in them.
type RecordDiff struct {
	// Written are the records this checkout holds that the shards do not carry
	// line for line: a decision made since the last write, a record a pull
	// brought in, a basis a run produced.
	Written []UnitState
	// Removed counts the lines the shards carry for units this checkout no
	// longer holds, which a write drops.
	Removed int
}

// Changed reports how many lines a write of the record would touch.
func (d RecordDiff) Changed() int { return len(d.Written) + d.Removed }

// RecordDiff compares what this checkout holds against its committed shards. It
// is what `kapi commit` is about to write and what a dry run reports.
func (w *WorkStore) RecordDiff(ctx context.Context) (RecordDiff, error) {
	if err := w.Import(ctx); err != nil {
		return RecordDiff{}, err
	}
	return w.recordDiff(ctx)
}

func (w *WorkStore) recordDiff(ctx context.Context) (RecordDiff, error) {
	held, err := w.All(ctx)
	if err != nil {
		return RecordDiff{}, err
	}
	onDisk, err := ReadCommitted(w.committed)
	if err != nil {
		return RecordDiff{}, err
	}
	lines := make(map[Key]string, len(onDisk))
	for _, u := range onDisk {
		line, merr := json.Marshal(u)
		if merr != nil {
			return RecordDiff{}, fmt.Errorf("state: marshal unit %s: %w", u.Unit, merr)
		}
		lines[u.Key()] = string(line)
	}
	var diff RecordDiff
	for _, u := range held {
		line, merr := json.Marshal(u)
		if merr != nil {
			return RecordDiff{}, fmt.Errorf("state: marshal unit %s: %w", u.Unit, merr)
		}
		if lines[u.Key()] != string(line) {
			diff.Written = append(diff.Written, u)
		}
		delete(lines, u.Key())
	}
	diff.Removed = len(lines)
	return diff, nil
}

// Staged returns the records this checkout holds that its committed shards do
// not carry.
//
// It exists so a predecessor store's work can be carried into a replacement
// before the old one is deleted: everything the shards supply comes back by
// import, and this is what they do not supply.
func (w *WorkStore) Staged(ctx context.Context) ([]UnitState, error) {
	diff, err := w.recordDiff(ctx)
	return diff.Written, err
}

// Commit writes this checkout's record to the committed shards: for every unit
// the checkout holds, the ledger entry that applies to its current pairing.
//
// The write is deterministic. Lines are sorted within a shard, a shard whose
// bytes are unchanged is left untouched, and the payload is the record as it
// was decided, so running it twice over an unchanged project writes the same
// bytes and leaves the same files.
//
// It never prunes on another checkout's behalf: the units it covers are the
// ones in this checkout's view, and the shards it removes are the ones that
// view no longer names.
func (w *WorkStore) Commit(ctx context.Context) error {
	if w.committed == "" {
		// A handle with no checkout (OpenLedger) has no record to write, and
		// the directory it would write into is the process's own.
		return nil
	}
	if err := w.Import(ctx); err != nil {
		return err
	}
	units, err := w.All(ctx)
	if err != nil {
		return err
	}
	if err := WriteCommitted(w.committed, units); err != nil {
		return err
	}
	if err := w.markExported(ctx); err != nil {
		return err
	}
	return w.restamp(ctx)
}

// PersistRecords writes the committed shards from what this checkout holds. It
// is Commit under the name a convergence pass calls it by, so a run makes its
// own output durable through the same one write.
func (w *WorkStore) PersistRecords(ctx context.Context) error { return w.Commit(ctx) }

// markExported records that the shards now carry every row of this checkout's
// view, so a later import rebuilds the view from them rather than treating them
// as work recorded since.
func (w *WorkStore) markExported(ctx context.Context) error {
	if w.mem != nil {
		for k, r := range w.mem.view {
			r.Exported = true
			w.mem.view[k] = r
		}
		return w.mem.persist()
	}
	_, err := w.db.ExecContext(ctx,
		`UPDATE unit_view SET exported = 1 WHERE checkout = ? AND exported = 0`, w.checkout)
	if err != nil {
		return fmt.Errorf("state: mark view exported: %w", err)
	}
	return nil
}

// restamp records the digest of the shards as they stand after this store wrote
// them, so the next open reads the view as current rather than as moved.
func (w *WorkStore) restamp(ctx context.Context) error {
	digest, err := CommittedDigest(w.committed)
	if err != nil {
		return err
	}
	return w.stampCommitted(ctx, digest)
}

// Documents returns the documents this checkout knows, as identity resolution
// needs them: a durable key, the path it was last seen at, and the content
// hashes it held there.
func (w *WorkStore) Documents(ctx context.Context) ([]reconcile.DocUnit, error) {
	if w.mem != nil {
		return w.mem.documents(), nil
	}
	rows, err := w.db.QueryContext(ctx,
		`SELECT key, path, content FROM document WHERE checkout = ? ORDER BY key`, w.checkout)
	if err != nil {
		return nil, fmt.Errorf("state: list documents: %w", err)
	}
	defer rows.Close()

	var out []reconcile.DocUnit
	for rows.Next() {
		var u reconcile.DocUnit
		var content string
		if err := rows.Scan(&u.Key, &u.Path, &content); err != nil {
			return nil, fmt.Errorf("state: scan document: %w", err)
		}
		// A document written before content was recorded reads as holding
		// nothing, which grades as "not a rename" rather than as an error: the
		// path pass still recognises it where it stands, and the next read
		// records what it holds.
		if content != "" && content != "[]" {
			if uerr := json.Unmarshal([]byte(content), &u.Content); uerr != nil {
				return nil, fmt.Errorf("state: parse document %q content: %w", u.Key, uerr)
			}
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// AdoptDocuments resolves a fresh read against the documents this checkout
// knows, records the result, and returns each current path's durable key.
//
// It is the local half of what a venue does on a push, and it exists for the
// same reason: a decision is filed against the document it was made in, so if
// the document's identity is its path, renaming a file orphans every approval
// inside it, silently, because nothing fails and the loop simply re-approves
// from scratch. Resolution here matches on path first and on surviving content
// second, so a file that moved keeps its key, and the decisions filed under its
// old address move onto that key.
//
// current is what was just read, in any order. The returned map is keyed by
// DocUnit.Path.
func (w *WorkStore) AdoptDocuments(ctx context.Context, current []reconcile.DocUnit) (map[string]string, error) {
	prior, err := w.Documents(ctx)
	if err != nil {
		return nil, err
	}
	resolved := reconcile.DocumentUnits(current, prior)

	// Where each key used to live, so a key resolved at a new path can carry
	// the decisions filed under the old one.
	priorPath := make(map[string]string, len(prior))
	for _, u := range prior {
		priorPath[u.Key] = u.Path
	}

	out := make(map[string]string, len(resolved))
	for i, r := range resolved {
		out[r.Path] = r.Key
		content := current[i].Content

		// The address is swept every time, not only on first sight. Anything
		// that could not resolve a key — a fresh checkout, a build with no
		// store, a surface reading a project before its first extraction —
		// files its decision under the path, so the path keeps acquiring
		// decisions that belong to the identity long after it has one.
		if rerr := w.rekeyScope(ctx, r.Path, r.Key); rerr != nil {
			return nil, rerr
		}
		// And a key that moved carries its decisions with it.
		if was, known := priorPath[r.Key]; known && was != r.Path {
			if rerr := w.rekeyScope(ctx, was, r.Key); rerr != nil {
				return nil, rerr
			}
		}
		if perr := w.putDocument(ctx, r.Key, r.Path, content); perr != nil {
			return nil, perr
		}
	}
	if w.mem != nil {
		if perr := w.mem.persist(); perr != nil {
			return nil, perr
		}
	}
	return out, nil
}

// rekeyScope moves every decision this checkout holds under one scope onto
// another. A no-op when they are already the same, which is the ordinary case:
// a document whose key equals its path (nothing recorded yet) and a document
// that did not move.
//
// The ledger is append-only, so the move is a fresh entry per unit carrying the
// record under its new scope, and the view row follows. Rows already under `to`
// win: re-keying is a migration of an address into an identity, and it must
// never displace a decision recorded against the identity itself.
func (w *WorkStore) rekeyScope(ctx context.Context, from, to string) error {
	if from == "" || to == "" || from == to {
		return nil
	}
	moving, err := w.Priors(ctx, from)
	if err != nil {
		return err
	}
	if len(moving) == 0 {
		return nil
	}
	for _, u := range moving {
		k := Key{Scope: to, Unit: u.Unit, Variant: u.Variant}
		if _, taken, perr := w.pairingOf(ctx, k); perr != nil {
			return perr
		} else if taken {
			// The identity already answers for this unit. The address's copy is
			// the older one, and leaving it would make the same unit answer twice.
			if derr := w.dropView(ctx, u.Key()); derr != nil {
				return derr
			}
			continue
		}
		moved := u
		moved.Scope = to
		if aerr := w.append(ctx, moved, moved.Decision.By, OriginLocal, false); aerr != nil {
			return aerr
		}
		if derr := w.dropView(ctx, u.Key()); derr != nil {
			return derr
		}
	}
	return nil
}

// putDocument records where a document currently lives in this checkout and
// what it held. The key is its durable identity; the path is only its address,
// and moves without it.
func (w *WorkStore) putDocument(ctx context.Context, key, path string, content []string) error {
	if w.mem != nil {
		w.mem.docs[key] = memDoc{Path: path, Content: content}
		return nil
	}
	encoded, err := json.Marshal(content)
	if err != nil {
		return fmt.Errorf("state: encode document %q content: %w", key, err)
	}
	_, err = w.db.ExecContext(ctx, `
INSERT INTO document (checkout, key, path, content) VALUES (?, ?, ?, ?)
ON CONFLICT(checkout, key) DO UPDATE SET path = excluded.path, content = excluded.content`,
		w.checkout, key, path, string(encoded))
	if err != nil {
		return fmt.Errorf("state: put document: %w", err)
	}
	return nil
}

// DocumentPaths returns the documents this checkout knows as key to current
// path.
func (w *WorkStore) DocumentPaths(ctx context.Context) (map[string]string, error) {
	if w.mem != nil {
		out := make(map[string]string, len(w.mem.docs))
		for key, d := range w.mem.docs {
			out[key] = d.Path
		}
		return out, nil
	}
	rows, err := w.db.QueryContext(ctx,
		`SELECT key, path FROM document WHERE checkout = ? ORDER BY key`, w.checkout)
	if err != nil {
		return nil, fmt.Errorf("state: list documents: %w", err)
	}
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var key, path string
		if err := rows.Scan(&key, &path); err != nil {
			return nil, fmt.Errorf("state: scan document: %w", err)
		}
		out[key] = path
	}
	return out, rows.Err()
}

// carryLegacyRows moves a pre-ledger database across: every unit row becomes a
// ledger entry and a view row for this checkout, and every document row becomes
// this checkout's.
//
// The rows are authored work, so they are carried rather than reset. A row that
// was staged and never committed has no other copy, and the ledger is where it
// belongs now.
func (w *WorkStore) carryLegacyRows(ctx context.Context) error {
	if w.mem != nil {
		return nil
	}
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("state: carry pre-ledger rows: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	carried, err := carryLegacyUnits(ctx, tx, w.checkout, w.clock())
	if err != nil {
		return err
	}
	moved, err := carryLegacyDocuments(ctx, tx, w.checkout)
	if err != nil {
		return err
	}
	if !carried && !moved {
		return nil
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("state: carry pre-ledger rows: %w", err)
	}
	return nil
}

func carryLegacyUnits(ctx context.Context, tx *storage.Tx, checkout string, now time.Time) (bool, error) {
	present, err := tableExists(ctx, tx, "unit_state")
	if err != nil || !present {
		return false, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT payload, staged FROM unit_state`)
	if err != nil {
		return false, fmt.Errorf("state: read pre-ledger units: %w", err)
	}
	type legacy struct {
		unit   UnitState
		staged bool
	}
	var held []legacy
	for rows.Next() {
		var payload string
		var staged int
		if serr := rows.Scan(&payload, &staged); serr != nil {
			rows.Close()
			return false, fmt.Errorf("state: scan pre-ledger unit: %w", serr)
		}
		var u UnitState
		if uerr := json.Unmarshal([]byte(payload), &u); uerr != nil {
			rows.Close()
			return false, fmt.Errorf("state: parse pre-ledger unit: %w", uerr)
		}
		held = append(held, legacy{unit: u, staged: staged == 1})
	}
	if rerr := rows.Err(); rerr != nil {
		rows.Close()
		return false, fmt.Errorf("state: read pre-ledger units: %w", rerr)
	}
	rows.Close()

	for _, l := range held {
		origin := OriginImport
		if l.staged {
			origin = OriginLocal
		}
		if ierr := insertEntry(ctx, tx, l.unit, l.unit.Decision.By, origin, false, now, false); ierr != nil {
			return false, ierr
		}
		if verr := putView(ctx, tx, checkout, l.unit.Pairing(), !l.staged); verr != nil {
			return false, verr
		}
	}
	if _, derr := tx.ExecContext(ctx, `DROP TABLE unit_state`); derr != nil {
		return false, fmt.Errorf("state: retire pre-ledger units: %w", derr)
	}
	return true, nil
}

func carryLegacyDocuments(ctx context.Context, tx *storage.Tx, checkout string) (bool, error) {
	present, err := tableExists(ctx, tx, "document_legacy")
	if err != nil || !present {
		return false, err
	}
	// The WHERE clause is what tells SQLite that ON CONFLICT belongs to the
	// INSERT rather than to the SELECT it takes its rows from.
	if _, err := tx.ExecContext(ctx, `
INSERT INTO document (checkout, key, path, content)
SELECT ?, key, path, content FROM document_legacy WHERE true
ON CONFLICT(checkout, key) DO NOTHING`, checkout); err != nil {
		return false, fmt.Errorf("state: carry pre-ledger documents: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DROP TABLE document_legacy`); err != nil {
		return false, fmt.Errorf("state: retire pre-ledger documents: %w", err)
	}
	return true, nil
}

func tableExists(ctx context.Context, tx *storage.Tx, name string) (bool, error) {
	var found string
	err := tx.QueryRowContext(ctx,
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("state: look for table %q: %w", name, err)
	}
	return true, nil
}

// shardOf groups units into one file per document scope, so editing the docs
// does not rewrite the shard holding the interface strings. Units with no scope
// yet land in a shared shard rather than being dropped.
func shardOf(u UnitState) string {
	if s := strings.TrimSpace(u.Scope); s != "" {
		return s
	}
	return "unscoped"
}

// sortUnits orders by the identity key, (scope, unit, variant), so a shard's
// bytes depend only on its contents.
func sortUnits(units []UnitState) {
	sort.Slice(units, func(i, j int) bool { return unitLess(units[i], units[j]) })
}

// unitLess is the one ordering over unit records: the identity key, field by
// field, then the pairing's hashes.
//
// The hashes settle an order the key alone leaves open. A checkout's view holds
// one row per key, so they never decide anything there; the ledger holds an
// entry per PAIRING, so one unit can appear under several, and a serialization
// of the ledger needs a total order to be stable.
func unitLess(a, b UnitState) bool {
	if a.Scope != b.Scope {
		return a.Scope < b.Scope
	}
	if a.Unit != b.Unit {
		return a.Unit < b.Unit
	}
	ka, _ := a.Variant.MarshalText()
	kb, _ := b.Variant.MarshalText()
	if string(ka) != string(kb) {
		return string(ka) < string(kb)
	}
	if a.ContentHash != b.ContentHash {
		return a.ContentHash < b.ContentHash
	}
	return a.TargetHash < b.TargetHash
}
