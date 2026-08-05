// Package storage provides a shared SQLite infrastructure layer for
// persistent content memories and terms stores. It handles connection
// management, WAL mode, and common pragmas.
//
// Two SQLite backends are selected at compile time by the cgo build tag:
//
//   - cgo builds (the default on macOS/Linux dev builds) use the native
//     github.com/mattn/go-sqlite3 driver and the statically linked FTS5 ICU
//     tokenizer (see driver_cgo.go and icu_tokenizer.go), so FTS5 word-search
//     tables use tokenize='icu'.
//   - no-cgo builds (CGO_ENABLED=0, e.g. the Windows kapi CLI) use the pure-Go
//     modernc.org/sqlite driver (see driver_nocgo.go). modernc ships only the
//     built-in FTS5 tokenizers, so FTS5 word-search tables use
//     tokenize='unicode61'.
//
// The driver name (sqliteDriver), DSN builder (sqliteDSN), and word-search
// tokenizer (FTSWordTokenizer) all come from the build-specific driver_*.go
// file.
//
// Cross-build .db caveat: an FTS5 word-search table is created with whichever
// tokenizer the building binary supports. A content memory/terms .db whose FTS table was
// created with tokenize='icu' under a cgo build cannot be FTS-word-queried by a
// no-cgo/modernc binary (which lacks the icu tokenizer), and a db created with
// tokenize='unicode61' under no-cgo cannot rely on ICU segmentation under cgo.
// The trigram tables (tokenize='trigram', built into both backends) and all
// non-FTS data remain portable.
package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// DB wraps a sql.DB with shared configuration applied.
//
// It shadows the write entry points of the embedded *sql.DB — ExecContext,
// Exec, BeginTx, Begin — so that a handle opened with Options.SerializeWrites
// gates every write it issues without any store having to remember to ask. See
// write.go.
type DB struct {
	*sql.DB
	path string

	// gate is nil unless the handle was opened with Options.SerializeWrites: an
	// in-process FIFO queue every write transaction on this handle passes
	// through. See gate.go for what it fixes and what it cannot reach.
	gate *writeGate
}

// Options configures how a database handle is opened. The zero value is the
// long-standing behaviour every standalone store still gets; the project store
// (core/projectdb) is the caller that sets both fields.
type Options struct {
	// ImmediateTx begins every transaction on the handle with BEGIN IMMEDIATE
	// instead of SQLite's default BEGIN DEFERRED.
	//
	// A deferred transaction that reads before it writes takes a read lock and
	// must later upgrade it. SQLite refuses a contended upgrade IMMEDIATELY —
	// it returns SQLITE_BUSY without consulting busy_timeout, because waiting
	// could deadlock two transactions that each hold a read lock and each want
	// to write. So on a shared file the busy timeout, the one thing standing
	// between a queue and an error, does not apply to the commonest write shape
	// there is. IMMEDIATE takes the write lock up front, where the busy handler
	// does apply and the wait is legal.
	//
	// It is opt-in because it is not free: an IMMEDIATE transaction that turns
	// out to be read-only still serialized against every other writer. Handles
	// with one writer (a standalone content memory, a named store, a KPZ
	// overlay) pay that for nothing.
	ImmediateTx bool

	// SerializeWrites installs the in-process write gate: one FIFO permit that
	// every write transaction issued through this handle holds for its whole
	// life. It is what keeps a drip of small writes from being starved by a
	// stream of large ones — busy_timeout's backoff is not fair, and no amount
	// of timeout makes it fair. See writeGate.
	SerializeWrites bool
}

// ProjectOptions is what a merged, multi-subsystem store wants: both halves.
// They belong together — the gate orders the writers this process controls, and
// IMMEDIATE keeps the ones it does not (another kapi process on the same file)
// in a queue SQLite will actually wait in rather than refuse.
//
// It is named rather than spelled out at the call site so that the two settings
// stay one decision. core/projectdb is the caller.
func ProjectOptions() Options {
	return Options{ImmediateTx: true, SerializeWrites: true}
}

// pathLocks hands out one mutex per database file, so callers that must
// serialize on a given database never serialize against unrelated ones.
type pathLocks struct {
	mu sync.Mutex
	m  map[string]*sync.Mutex
}

// ErrNoSQLite marks a build with no file-backed SQLite driver at all (the
// browser build). Callers whose feature is OPTIONAL in the browser — status
// reading the decision store, for instance — match it with errors.Is and
// degrade to their empty state instead of failing a command that otherwise
// works.
var ErrNoSQLite = errors.New("a file-backed SQLite database is not available in the browser build")

func (p *pathLocks) get(path string) *sync.Mutex {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.m == nil {
		p.m = map[string]*sync.Mutex{}
	}
	l, ok := p.m[path]
	if !ok {
		l = &sync.Mutex{}
		p.m[path] = l
	}
	return l
}

// openLocks serializes Open's create-and-apply-pragmas sequence per database
// file. Two connections racing to switch a fresh database's journal mode to
// WAL can hit SQLite's deadlock-avoidance path, which returns "database is
// locked" IMMEDIATELY — bypassing busy_timeout entirely — so a concurrent
// first open of the same file (converge workers each opening the project content memory)
// failed spuriously.
//
// The lock is held across applyPragmasRetry, whose retry window is measured in
// seconds when a cross-process opener holds the file, so it must be per-path:
// a process-wide lock would park every other database's Open — content memory, terms,
// block store — behind one worker's wait. In-memory databases are private to
// their pool and need no lock at all.
//
// (Cross-process first-open races remain covered by the DSN busy_timeout,
// which handles the plain-contention case.)
var openLocks pathLocks

// Open opens a SQLite database at the given path with shared pragmas.
// Use ":memory:" for in-memory databases (useful for testing).
// Parent directories must already exist; the file is created on demand.
//
// Transactions are SQLite's default (deferred) and writes are ungated, which is
// what a database with one writer wants. A file several subsystems write
// concurrently wants OpenWith(path, projectOptions()) instead — see Options.
func Open(dbPath string) (*DB, error) {
	return OpenWith(dbPath, Options{})
}

// OpenWith opens a SQLite database with the shared pragmas and the given write
// discipline. Open is OpenWith with the zero Options.
func OpenWith(dbPath string, opts Options) (*DB, error) {
	// Builds without a SQLite driver (wasm) report why before database/sql can
	// blame a missing import.
	if err := driverUnavailable(); err != nil {
		return nil, fmt.Errorf("open database %s: %w", dbPath, err)
	}
	if !isMemoryDSN(dbPath) {
		l := openLocks.get(dbPath)
		l.Lock()
		defer l.Unlock()
	}
	// Set the busy timeout in the DSN so every pooled connection waits for locks
	// from the moment it is established — before any pragma runs. Without this,
	// the very first `PRAGMA journal_mode=WAL` can hit "database is locked" when a
	// second short-lived kapi process (e.g. the verify hook) touches the same DB
	// concurrently, because the per-connection PRAGMA busy_timeout below has not
	// taken effect yet.
	db, err := sql.Open(sqliteDriver, sqliteDSN(dbPath, opts))
	if err != nil {
		return nil, fmt.Errorf("open database %s: %w", dbPath, err)
	}

	// In-memory databases create a separate DB per connection. Force a single
	// connection so all queries share the same in-memory state.
	if isMemoryDSN(dbPath) {
		db.SetMaxOpenConns(1)
	} else {
		db.SetMaxOpenConns(25)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(30 * time.Minute)
	}

	if err := applyPragmasRetry(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply pragmas: %w", err)
	}

	wrapped := &DB{DB: db, path: dbPath}
	if opts.SerializeWrites {
		wrapped.gate = newWriteGate()
	}
	return wrapped, nil
}

// applyPragmasRetry retries applyPragmas while the database reports itself
// busy/locked. A fresh database being migrated by a sibling opener (another
// converge worker in this process, or another kapi process) holds an exclusive
// lock that can surface here as an IMMEDIATE "database is locked" — SQLite's
// deadlock-avoidance path returns without consulting busy_timeout — so a
// bounded retry, not the busy handler, is what absorbs it. First-creation
// migrations complete in at most seconds; anything still locked after the
// window is a real fault and surfaces as the error.
func applyPragmasRetry(db *sql.DB) error {
	const window = 15 * time.Second
	delay := 10 * time.Millisecond
	deadline := time.Now().Add(window)
	for {
		err := applyPragmas(db)
		if err == nil || !isBusyErr(err) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(delay)
		if delay < 500*time.Millisecond {
			delay *= 2
		}
	}
}

// isBusyErr reports whether err is SQLite lock contention (mattn and modernc
// phrase it differently; neither exposes a portable sentinel through
// database/sql).
func isBusyErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "database is locked") || strings.Contains(s, "SQLITE_BUSY") || strings.Contains(s, "database table is locked")
}

// Path returns the database file path.
func (db *DB) Path() string {
	return db.path
}

// isMemoryDSN reports whether dbPath addresses an in-memory database. The
// ":memory:" form and "mode=memory" query parameter behave identically across
// the mattn and modernc drivers.
func isMemoryDSN(dbPath string) bool {
	return dbPath == ":memory:" || strings.Contains(dbPath, ":memory:") || strings.Contains(dbPath, "mode=memory")
}

// applyPragmas runs the shared connection pragmas as belt-and-suspenders on the
// single connection it happens to execute on.
//
// IMPORTANT: a PRAGMA run here via db.Exec configures only ONE of the up-to-25
// pooled connections. Connection-level pragmas — foreign_keys above all, since
// ON DELETE CASCADE silently no-ops where foreign_keys is OFF — are therefore
// set in the DSN (sqliteDSN, per build-tagged driver file), which applies them
// to every connection as it is established. The journal_mode=WAL and
// wal_autocheckpoint pragmas are database-level (persisted in the DB header /
// shared across connections), so running them once is sufficient; they are
// repeated here mainly to switch the journal mode on first open under the cgo
// driver. Both the mattn (cgo) and modernc (no-cgo) drivers honour these PRAGMA
// statements via Exec.
func applyPragmas(db *sql.DB) error {
	pragmas := []string{
		// busy_timeout first: subsequent statements (notably the journal_mode=WAL
		// switch, which needs a write lock) then wait for a busy database instead
		// of failing immediately. The DSN sets this per connection too; this is
		// belt-and-suspenders for the connection applyPragmas runs on.
		"PRAGMA busy_timeout=5000",
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA cache_size=-131072",
		"PRAGMA wal_autocheckpoint=10000",
		"PRAGMA temp_store=MEMORY",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil { //nolint:noctx // startup pragmas
			return fmt.Errorf("execute %s: %w", p, err)
		}
	}
	return nil
}
