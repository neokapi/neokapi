package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
	docs  map[string]string
}

type memUnit struct {
	Unit   UnitState `json:"unit"`
	Staged bool      `json:"staged,omitempty"`
}

// memFile is the sidecar serialization of the browser working set.
type memFile struct {
	Units []memUnit         `json:"units"`
	Docs  map[string]string `json:"docs,omitempty"`
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
	sort.Slice(f.Units, func(i, j int) bool {
		if f.Units[i].Unit.Unit != f.Units[j].Unit.Unit {
			return f.Units[i].Unit.Unit < f.Units[j].Unit.Unit
		}
		ki, _ := f.Units[i].Unit.Variant.MarshalText()
		kj, _ := f.Units[j].Unit.Variant.MarshalText()
		return string(ki) < string(kj)
	})
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
			mem := &memWork{
				path:  strings.TrimSuffix(dbPath, filepath.Ext(dbPath)) + ".json",
				units: map[Key]memUnit{},
				docs:  map[string]string{},
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
		return nil, fmt.Errorf("state: open work store: %w", err)
	}
	if err := storage.Migrate(db, "state", workMigrations); err != nil {
		db.Close()
		return nil, fmt.Errorf("state: migrate work store: %w", err)
	}
	w := &WorkStore{db: db, committed: committedPath}

	empty, err := w.isEmpty(ctx)
	if err != nil {
		db.Close()
		return nil, err
	}
	if empty {
		if err := w.seed(ctx); err != nil {
			db.Close()
			return nil, err
		}
	}
	return w, nil
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
	if w.mem != nil {
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
		`SELECT payload FROM unit_state WHERE unit = ? AND variant = ?`,
		k.Unit, string(variant)).Scan(&payload)
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
INSERT INTO unit_state (unit, variant, scope, content_hash, context_hash, payload, staged)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(unit, variant) DO UPDATE SET
    scope = excluded.scope, content_hash = excluded.content_hash,
    context_hash = excluded.context_hash, payload = excluded.payload,
    staged = MAX(unit_state.staged, excluded.staged)`,
		u.Unit, string(variant), u.Scope, u.ContentHash, u.ContextHash, string(payload), flag)
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
	_, err := w.db.ExecContext(ctx, `DELETE FROM unit_state WHERE unit = ? AND variant = ?`, k.Unit, string(variant))
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
	rows, err := w.db.QueryContext(ctx, `SELECT payload FROM unit_state ORDER BY unit, variant`)
	if err != nil {
		return nil, fmt.Errorf("state: list units: %w", err)
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

// PutDocument records where a document currently lives. The key is its durable
// identity; the path is only its address, and moves without it.
func (w *WorkStore) PutDocument(ctx context.Context, key, path string) error {
	if w.mem != nil {
		if w.mem.docs[key] == path {
			return nil
		}
		w.mem.docs[key] = path
		return w.mem.persist()
	}
	_, err := w.db.ExecContext(ctx, `
INSERT INTO document (key, path) VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET path = excluded.path`, key, path)
	if err != nil {
		return fmt.Errorf("state: put document: %w", err)
	}
	return nil
}

// DocumentPaths returns the known documents as key to current path.
func (w *WorkStore) DocumentPaths(ctx context.Context) (map[string]string, error) {
	if w.mem != nil {
		out := make(map[string]string, len(w.mem.docs))
		maps.Copy(out, w.mem.docs)
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

// sortUnits orders by (unit, variant) so a shard's bytes depend only on its
// contents — a stable diff, not an accident of map iteration.
func sortUnits(units []UnitState) {
	sort.Slice(units, func(i, j int) bool {
		if units[i].Unit != units[j].Unit {
			return units[i].Unit < units[j].Unit
		}
		ki, _ := units[i].Variant.MarshalText()
		kj, _ := units[j].Variant.MarshalText()
		return string(ki) < string(kj)
	})
}
