package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/core/reconcile"
	"github.com/neokapi/neokapi/core/storage"
)

// The working store is the staging area between a decision being made and the
// project's committed record of it.
//
// It exists because the committed record is a whole-file serialization, and
// writing it once per decision is quadratic: every approval re-encoded and
// rewrote the entire artifact, so a run recording a thousand decisions moved
// gigabytes to record a few hundred kilobytes of change. Worse, two processes
// each read the whole file, mutated their own copy and wrote it back, so the
// second silently discarded the first's decisions — the atomic rename made the
// loss clean and invisible.
//
// So decisions accumulate here, in one transaction-capable place, and are
// serialized once per run by Commit. That is the design the package
// documentation always described — a committed serialization as the source of
// truth, a working store as a derived index over it — and that Pending() was
// written for.
//
// It is deliberately ONE database. Only the working set genuinely needs
// transactions and hash lookups; the rest of what a project caches is file
// artifacts and stays files. A second database would buy nothing (both would sit
// in the same disposable directory) and would cost a second migration ledger.

// WorkStore is a SQLite-backed working set of unit state.
//
// Durability note: while a decision is here and not yet committed, this database
// holds the only copy. It therefore lives beside the derived caches but is not
// one — and the window is kept short by committing at the end of each command
// rather than leaving a staging area to accumulate.
type WorkStore struct {
	db        *storage.DB
	committed string // path of the committed serialization this indexes

	// ownsDB records whether Close may close the pool: true when this store
	// opened its own file, false when it adopted the project's merged store,
	// whose owner closes it once for all four subsystems.
	ownsDB bool

	// mem is the browser fallback: the wasm build has no file-backed SQLite
	// (storage.ErrNoSQLite), yet the review→approve loop must still work in
	// the lab. The working set lives in process memory and persists as a JSON
	// sidecar next to where the database would sit, written through the
	// sandbox filesystem — a decision recorded by one command must survive
	// into the next, exactly as the database gives every other build. nil on
	// every build with a real driver.
	mem *memWork
}

// memWork is the JSON-sidecar working set backing the browser build.
type memWork struct {
	path  string // the sidecar the set persists to
	units map[Key]memUnit
	docs  map[string]memDoc
}

// memDoc is one document's identity in the sidecar: where it lives now, and
// what it held when it was last read — which is what recognises it again after
// a rename.
type memDoc struct {
	Path    string   `json:"path"`
	Content []string `json:"content,omitempty"`
}

type memUnit struct {
	Unit   UnitState `json:"unit"`
	Staged bool      `json:"staged,omitempty"`
}

// memFile is the sidecar serialization of the browser working set.
type memFile struct {
	Units []memUnit         `json:"units"`
	Docs  map[string]memDoc `json:"docs,omitempty"`
}

// load reads the sidecar back into the working set. A missing sidecar is an
// empty set (the caller seeds from the committed record); a malformed one is an
// error, because the set may hold decisions no other copy has.
func (m *memWork) load() (found bool, err error) {
	data, err := os.ReadFile(m.path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("state: read work set %s: %w", m.path, err)
	}
	var f memFile
	if err := json.Unmarshal(data, &f); err != nil {
		return false, fmt.Errorf("state: parse work set %s: %w", m.path, err)
	}
	for _, mu := range f.Units {
		m.units[mu.Unit.Key()] = mu
	}
	if f.Docs != nil {
		m.docs = f.Docs
	}
	return true, nil
}

// persist writes the working set to the sidecar. Called after every mutation:
// while a decision is only here, this file is its only durable copy.
func (m *memWork) persist() error {
	f := memFile{Units: make([]memUnit, 0, len(m.units))}
	for _, mu := range m.units {
		f.Units = append(f.Units, mu)
	}
	sort.Slice(f.Units, func(i, j int) bool { return unitLess(f.Units[i].Unit, f.Units[j].Unit) })
	if len(m.docs) > 0 {
		f.Docs = m.docs
	}
	data, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("state: marshal work set: %w", err)
	}
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("state: write work set: %w", err)
	}
	if err := os.Rename(tmp, m.path); err != nil {
		return fmt.Errorf("state: rename work set: %w", err)
	}
	return nil
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
	//
	// It rebuilds rather than resets: the old table holds at most one row per
	// (unit, variant) and every row already carries the scope it belongs to, so
	// copying across is lossless and by construction cannot collide. A reset
	// would be cheap for everything the committed record reproduces and would
	// throw away the one thing it does not — a decision staged and not yet
	// committed, whose only copy is here.
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
	// orphaned — silently, since nothing fails.
	SQL: `
ALTER TABLE document ADD COLUMN content TEXT NOT NULL DEFAULT '[]';
CREATE INDEX IF NOT EXISTS document_path ON document(path);`,
}}

// The store runs its statements on ctx: a WorkStore is a
// local SQLite file with no cancellation semantics yet, and the API predates
// context plumbing — which arrives with the merged-store work rather than as
// nine call sites of ceremony here.

// OpenWork opens the working store at dbPath, seeding it from the committed
// serialization at committedPath when it holds nothing yet.
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
	w := &WorkStore{db: db, committed: committedPath, ownsDB: true}
	if err := w.seedIfEmpty(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return w, nil
}

// OpenWorkFromDB adopts an already-open database — the project's merged
// `.kapi/work/store.db`, where the working set is one schema among the content
// memory, the terms store and the block cache. Same migrations, same `state`
// ledger, same seeding from the committed shards; only the file is shared.
//
// It is what makes an approve-and-promote atomic: the decision and the wording
// the content memory learns from it are now two writes in one transaction on
// one connection pool, which no arrangement of separate files could offer.
//
// The returned store does not own db; its owner closes the pool.
func OpenWorkFromDB(ctx context.Context, db *storage.DB, committedPath string) (*WorkStore, error) {
	if db == nil {
		return nil, errors.New("state: adopt work store: nil database")
	}
	if err := storage.Migrate(db, "state", workMigrations); err != nil {
		return nil, fmt.Errorf("state: migrate work store: %w", err)
	}
	w := &WorkStore{db: db, committed: committedPath}
	if err := w.seedIfEmpty(ctx); err != nil {
		return nil, err
	}
	return w, nil
}

// OpenWorkSidecar opens the JSON-sidecar working set at sidecarPath — the
// browser build's working store, and the only form the set takes where there is
// no file-backed SQLite driver. Callers on a build with a driver reach it only
// to read a set some earlier browser session wrote.
func OpenWorkSidecar(ctx context.Context, sidecarPath, committedPath string) (*WorkStore, error) {
	if err := os.MkdirAll(filepath.Dir(sidecarPath), 0o755); err != nil {
		return nil, fmt.Errorf("state: work dir: %w", err)
	}
	mem := &memWork{
		path:  sidecarPath,
		units: map[Key]memUnit{},
		docs:  map[string]memDoc{},
	}
	w := &WorkStore{committed: committedPath, mem: mem}
	found, err := mem.load()
	if err != nil {
		return nil, err
	}
	if !found {
		if err := w.seed(ctx); err != nil {
			return nil, err
		}
	}
	return w, nil
}

// seedIfEmpty imports the committed record into a working store that holds
// nothing yet.
func (w *WorkStore) seedIfEmpty(ctx context.Context) error {
	empty, err := w.isEmpty(ctx)
	if err != nil {
		return err
	}
	if !empty {
		return nil
	}
	return w.seed(ctx)
}

func (w *WorkStore) isEmpty(ctx context.Context) (bool, error) {
	var n int
	if err := w.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM unit_state`).Scan(&n); err != nil {
		return false, fmt.Errorf("state: count units: %w", err)
	}
	return n == 0, nil
}

// seed imports the committed serialization. A working store is derived, so this
// is a rebuild rather than a migration: deleting the database costs nothing that
// has already been committed.
func (w *WorkStore) seed(ctx context.Context) error {
	units, err := ReadCommitted(w.committed)
	if err != nil {
		return err
	}
	for _, u := range units {
		if err := w.put(ctx, u, false); err != nil {
			return err
		}
	}
	return nil
}

func (w *WorkStore) Close() error {
	if w.mem != nil || !w.ownsDB {
		return nil
	}
	return w.db.Close()
}

// Get returns the state recorded for a unit.
func (w *WorkStore) Get(ctx context.Context, k Key) (UnitState, bool) {
	if w.mem != nil {
		mu, ok := w.mem.units[k]
		return mu.Unit, ok
	}
	variant, _ := k.Variant.MarshalText()
	var payload string
	err := w.db.QueryRowContext(ctx,
		`SELECT payload FROM unit_state WHERE scope = ? AND unit = ? AND variant = ?`,
		k.Scope, k.Unit, string(variant)).Scan(&payload)
	if err != nil {
		return UnitState{}, false
	}
	var u UnitState
	if json.Unmarshal([]byte(payload), &u) != nil {
		return UnitState{}, false
	}
	return u, true
}

// Put records a unit's state and stages it for the next commit.
func (w *WorkStore) Put(ctx context.Context, u UnitState) error { return w.put(ctx, u, true) }

// Record stores what the loop produced rather than what a person decided: a
// unit's basis (the source it translated and the translation it wrote) with no
// Decision on it.
//
// It is deliberately not staged. Staging answers "you have N uncommitted
// decisions", and a convergence pass produces one of these for every unit it
// writes a target for, and counting them there would bury a person's two
// pending approvals under fourteen thousand machine records. The loop persists its own
// output with PersistRecords instead.
func (w *WorkStore) Record(ctx context.Context, u UnitState) error { return w.put(ctx, u, false) }

// PersistRecords writes the committed serialization from the units that are NOT
// staged: everything already committed, plus whatever the loop has recorded
// since.
//
// It exists so a run can make its own output durable without publishing a
// person's pending decisions along with it. Commit is still the only thing that
// moves a staged decision into the committed record, and it still clears the
// staged flag; this writes around it.
func (w *WorkStore) PersistRecords(ctx context.Context) error {
	units, err := w.unstaged(ctx)
	if err != nil {
		return err
	}
	return WriteCommitted(w.committed, units)
}

// unstaged returns every recorded state that is not waiting for a commit.
func (w *WorkStore) unstaged(ctx context.Context) ([]UnitState, error) {
	if w.mem != nil {
		out := make([]UnitState, 0, len(w.mem.units))
		for _, mu := range w.mem.units {
			if !mu.Staged {
				out = append(out, mu.Unit)
			}
		}
		sortUnits(out)
		return out, nil
	}
	rows, err := w.db.QueryContext(ctx,
		`SELECT payload FROM unit_state WHERE staged = 0 ORDER BY scope, unit, variant`)
	if err != nil {
		return nil, fmt.Errorf("state: list unstaged: %w", err)
	}
	defer rows.Close()
	return scanUnits(rows)
}

func (w *WorkStore) put(ctx context.Context, u UnitState, staged bool) error {
	if w.mem != nil {
		k := u.Key()
		staged = staged || w.mem.units[k].Staged
		w.mem.units[k] = memUnit{Unit: u, Staged: staged}
		return w.mem.persist()
	}
	variant, _ := u.Variant.MarshalText()
	payload, err := json.Marshal(u)
	if err != nil {
		return fmt.Errorf("state: marshal unit: %w", err)
	}
	flag := 0
	if staged {
		flag = 1
	}
	_, err = w.db.ExecContext(ctx, `
INSERT INTO unit_state (scope, unit, variant, content_hash, context_hash, payload, staged)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(scope, unit, variant) DO UPDATE SET
    content_hash = excluded.content_hash,
    context_hash = excluded.context_hash, payload = excluded.payload,
    staged = MAX(unit_state.staged, excluded.staged)`,
		u.Scope, u.Unit, string(variant), u.ContentHash, u.ContextHash, string(payload), flag)
	if err != nil {
		return fmt.Errorf("state: put unit: %w", err)
	}
	return nil
}

// Delete removes a unit's state.
func (w *WorkStore) Delete(ctx context.Context, k Key) error {
	if w.mem != nil {
		if _, ok := w.mem.units[k]; !ok {
			return nil
		}
		delete(w.mem.units, k)
		return w.mem.persist()
	}
	variant, _ := k.Variant.MarshalText()
	_, err := w.db.ExecContext(ctx,
		`DELETE FROM unit_state WHERE scope = ? AND unit = ? AND variant = ?`,
		k.Scope, k.Unit, string(variant))
	if err != nil {
		return fmt.Errorf("state: delete unit: %w", err)
	}
	return nil
}

// All returns every recorded state, ordered so the serialization is stable.
func (w *WorkStore) All(ctx context.Context) ([]UnitState, error) {
	if w.mem != nil {
		out := make([]UnitState, 0, len(w.mem.units))
		for _, mu := range w.mem.units {
			out = append(out, mu.Unit)
		}
		sortUnits(out)
		return out, nil
	}
	rows, err := w.db.QueryContext(ctx, `SELECT payload FROM unit_state ORDER BY scope, unit, variant`)
	if err != nil {
		return nil, fmt.Errorf("state: list units: %w", err)
	}
	defer rows.Close()
	return scanUnits(rows)
}

// Staged returns the units decided since the last commit — the same set
// Pending counts, as records rather than a number.
//
// It exists so staged decisions can be carried between working stores: they are
// the one thing a working store holds that the committed record does not, so a
// store being replaced must hand them over before it is deleted. Everything
// else re-seeds.
func (w *WorkStore) Staged(ctx context.Context) ([]UnitState, error) {
	if w.mem != nil {
		var out []UnitState
		for _, mu := range w.mem.units {
			if mu.Staged {
				out = append(out, mu.Unit)
			}
		}
		sortUnits(out)
		return out, nil
	}
	rows, err := w.db.QueryContext(ctx,
		`SELECT payload FROM unit_state WHERE staged = 1 ORDER BY scope, unit, variant`)
	if err != nil {
		return nil, fmt.Errorf("state: list staged: %w", err)
	}
	defer rows.Close()
	return scanUnits(rows)
}

// Priors returns the identity signals for every unit in a document, which is
// what core/reconcile matches a fresh read against.
//
// Scoped to one document because that is how reconcile is called, but content
// matching stays project-wide: pass the whole project's units when text may have
// moved between files.
func (w *WorkStore) Priors(ctx context.Context, scope string) ([]UnitState, error) {
	if w.mem != nil {
		var out []UnitState
		for _, mu := range w.mem.units {
			if mu.Unit.Scope == scope {
				out = append(out, mu.Unit)
			}
		}
		sortUnits(out)
		return out, nil
	}
	rows, err := w.db.QueryContext(ctx,
		`SELECT payload FROM unit_state WHERE scope = ? ORDER BY unit, variant`, scope)
	if err != nil {
		return nil, fmt.Errorf("state: list priors: %w", err)
	}
	defer rows.Close()
	return scanUnits(rows)
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

// Documents returns the documents the project already knows, as identity
// resolution needs them: a durable key, the path it was last seen at, and the
// content hashes it held there.
func (w *WorkStore) Documents(ctx context.Context) ([]reconcile.DocUnit, error) {
	if w.mem != nil {
		out := make([]reconcile.DocUnit, 0, len(w.mem.docs))
		for key, d := range w.mem.docs {
			out = append(out, reconcile.DocUnit{Key: key, Path: d.Path, Content: d.Content})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
		return out, nil
	}
	rows, err := w.db.QueryContext(ctx, `SELECT key, path, content FROM document ORDER BY key`)
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

// AdoptDocuments resolves a fresh read against the documents the project knows,
// records the result, and returns each current path's durable key.
//
// It is the local half of what a venue does on a push, and it exists for the
// same reason: a decision is filed against the document it was made in, so if
// the document's identity is its path, renaming a file orphans every approval
// inside it — silently, because nothing fails and the loop simply re-approves
// from scratch. Resolution here matches on path first and on surviving content
// second, so a file that moved keeps its key, and the decisions filed under its
// old address move onto that key in the same transaction.
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

// rekeyScope moves every decision filed under one scope onto another. A no-op
// when they are already the same, which is the ordinary case: a document whose
// key equals its path (nothing recorded yet) and a document that did not move.
//
// Decisions already under `to` win. Re-keying is a migration of an address into
// an identity, and it must never overwrite a decision that was recorded against
// the identity itself.
func (w *WorkStore) rekeyScope(ctx context.Context, from, to string) error {
	if from == "" || to == "" || from == to {
		return nil
	}
	if w.mem != nil {
		for k, mu := range w.mem.units {
			if k.Scope != from {
				continue
			}
			moved := Key{Scope: to, Unit: k.Unit, Variant: k.Variant}
			if _, taken := w.mem.units[moved]; taken {
				continue
			}
			mu.Unit.Scope = to
			w.mem.units[moved] = mu
			delete(w.mem.units, k)
		}
		return nil
	}
	// The payload carries the scope too, so it is rewritten with the column —
	// a reader that trusted one and not the other would report the document a
	// decision was made in as the path it no longer sits at.
	if _, err := w.db.ExecContext(ctx, `
UPDATE OR IGNORE unit_state
   SET scope   = ?,
       payload = json_set(payload, '$.scope', ?)
 WHERE scope = ?`, to, to, from); err != nil {
		return fmt.Errorf("state: move decisions from %q to %q: %w", from, to, err)
	}
	// Rows the update could not move are ones the target already holds. They
	// are the address's copy of a decision the identity also has, and leaving
	// them would make the same unit answer twice.
	if _, err := w.db.ExecContext(ctx, `DELETE FROM unit_state WHERE scope = ?`, from); err != nil {
		return fmt.Errorf("state: drop superseded decisions at %q: %w", from, err)
	}
	return nil
}

// putDocument records where a document currently lives and what it held. The
// key is its durable identity; the path is only its address, and moves without
// it.
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
INSERT INTO document (key, path, content) VALUES (?, ?, ?)
ON CONFLICT(key) DO UPDATE SET path = excluded.path, content = excluded.content`,
		key, path, string(encoded))
	if err != nil {
		return fmt.Errorf("state: put document: %w", err)
	}
	return nil
}

// DocumentPaths returns the known documents as key to current path.
func (w *WorkStore) DocumentPaths(ctx context.Context) (map[string]string, error) {
	if w.mem != nil {
		out := make(map[string]string, len(w.mem.docs))
		for key, d := range w.mem.docs {
			out[key] = d.Path
		}
		return out, nil
	}
	rows, err := w.db.QueryContext(ctx, `SELECT key, path FROM document ORDER BY key`)
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

// Pending reports how many decisions are staged and not yet committed — the
// "you have N uncommitted decisions" signal.
func (w *WorkStore) Pending(ctx context.Context) (int, error) {
	if w.mem != nil {
		n := 0
		for _, mu := range w.mem.units {
			if mu.Staged {
				n++
			}
		}
		return n, nil
	}
	var n int
	if err := w.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM unit_state WHERE staged = 1`).Scan(&n); err != nil {
		return 0, fmt.Errorf("state: count staged: %w", err)
	}
	return n, nil
}

// Commit serializes the working set to the project's committed record and clears
// the staged flag. This is the once-per-run write that replaced the
// once-per-decision one.
func (w *WorkStore) Commit(ctx context.Context) error {
	n, err := w.Pending(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return nil
	}
	units, err := w.All(ctx)
	if err != nil {
		return err
	}
	if err := WriteCommitted(w.committed, units); err != nil {
		return err
	}
	if w.mem != nil {
		for k, mu := range w.mem.units {
			mu.Staged = false
			w.mem.units[k] = mu
		}
		return w.mem.persist()
	}
	if _, err := w.db.ExecContext(ctx, `UPDATE unit_state SET staged = 0 WHERE staged = 1`); err != nil {
		return fmt.Errorf("state: clear staged: %w", err)
	}
	return nil
}

// shardOf groups units into one file per document scope, so editing the docs does
// not rewrite the shard holding the interface strings. Units with no scope yet
// land in a shared shard rather than being dropped.
func shardOf(u UnitState) string {
	if s := strings.TrimSpace(u.Scope); s != "" {
		return s
	}
	return "unscoped"
}

// sortUnits orders by the identity key — (scope, unit, variant) — so a shard's
// bytes depend only on its contents, a stable diff rather than an accident of
// map iteration.
func sortUnits(units []UnitState) {
	sort.Slice(units, func(i, j int) bool { return unitLess(units[i], units[j]) })
}

// unitLess is the one ordering over unit records: the identity key, field by
// field.
func unitLess(a, b UnitState) bool {
	if a.Scope != b.Scope {
		return a.Scope < b.Scope
	}
	if a.Unit != b.Unit {
		return a.Unit < b.Unit
	}
	ka, _ := a.Variant.MarshalText()
	kb, _ := b.Variant.MarshalText()
	return string(ka) < string(kb)
}
