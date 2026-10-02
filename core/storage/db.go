// Package storage provides a shared SQLite infrastructure layer for
// persistent content memories and terms stores. It handles connection
// management, WAL mode, and common pragmas.
//
// Three SQLite backends are selected at compile time, and every build has one:
//
//   - cgo builds (the default on macOS/Linux dev builds) use the native
//     github.com/mattn/go-sqlite3 driver and the statically linked FTS5 ICU
//     tokenizer (see driver_cgo.go and icu_tokenizer.go), so FTS5 word-search
//     tables use tokenize='icu'.
//   - no-cgo builds (CGO_ENABLED=0, e.g. the Windows kapi CLI) use the pure-Go
//     modernc.org/sqlite driver (see driver_nocgo.go). modernc ships only the
//     built-in FTS5 tokenizers, so FTS5 word-search tables use
//     tokenize='unicode61'.
//   - the browser build (GOOS=js) uses the official SQLite WebAssembly build,
//     reached synchronously through a JavaScript bridge the host installs
//     before Go starts (see driver_js.go). Its databases live in the module's
//     memory, one connection per file, and FTS5 word search uses
//     tokenize='unicode61'.
//
// The driver name (sqliteDriver), DSN builder (sqliteDSN), word-search
// tokenizer (FTSWordTokenizer) and the driver's Profile all come from the
// build-specific driver_*.go file. A database file belongs to the driver too:
// Exists, Remove, Rename and List answer for it (namespace.go).
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

	// flock is nil unless the handle was opened with
	// Options.CrossProcessWrites: an advisory lock on a file beside the
	// database, taken inside the gate, that orders the writers the gate cannot
	// reach. See filelock.go.
	flock *fileLock

	// cleanup runs after the pool closes. It is set only where the handle is
	// backed by something this package created for it, which today means the
	// snapshot OpenReadOnly takes when a database's own directory is not
	// writable.
	cleanup func()
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

	// ReadOnly opens the database for reading only: the journal-mode switch is
	// skipped, every connection carries PRAGMA query_only=ON, and the pool is
	// held to one connection so that pragma governs every statement.
	//
	// It is what a caller wants where the database's directory may not be
	// writable. Opening still touches the filesystem, so it can fail where the
	// write-ahead log's shared-memory index is absent and cannot be created;
	// OpenReadOnly covers that case too, by reading a snapshot.
	ReadOnly bool

	// CrossProcessWrites installs an advisory lock on a file beside the
	// database, held for the length of every write transaction this handle
	// issues, so writers in DIFFERENT processes wait for each other in the
	// kernel instead of sleeping through SQLite's busy backoff.
	//
	// It is what a file several processes write wants: a project's context
	// store, opened at once by an agent's server, a CLI run and the desktop. A
	// file one process writes pays a lock acquisition for nothing.
	CrossProcessWrites bool
}

// ProjectOptions is what a multi-subsystem store wants: three settings that
// belong together. The gate orders the writers this process controls, the
// advisory lock orders the ones it does not, and IMMEDIATE keeps whatever
// reaches SQLite in a queue it will wait in rather than refuse.
//
// It is named rather than spelled out at the call site so the three stay one
// decision. core/projectdb is the caller, for both of a project's pools: the
// projection several tools in one process write, and the context store several
// processes share.
func ProjectOptions() Options {
	return Options{ImmediateTx: true, SerializeWrites: true, CrossProcessWrites: true}
}

// pathLocks hands out one mutex per database file, so callers that must
// serialize on a given database never serialize against unrelated ones.
type pathLocks struct {
	mu sync.Mutex
	m  map[string]*sync.Mutex
}

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
	profile := DriverProfile()
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
	// connection so all queries share the same in-memory state. A read-only
	// handle is held to one connection for a different reason: query_only is a
	// per-connection pragma with no DSN spelling in either driver, so one
	// connection is what makes it govern every statement the handle issues.
	// A driver whose profile allows one connection per file gets one, kept
	// open for the life of the pool.
	switch {
	case isMemoryDSN(dbPath), opts.ReadOnly, profile.MaxConns <= 1:
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
	default:
		db.SetMaxOpenConns(profile.MaxConns)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(30 * time.Minute)
	}

	if err := applyPragmasRetry(db, opts, profile); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply pragmas: %w", err)
	}

	wrapped := &DB{DB: db, path: dbPath}
	if opts.SerializeWrites {
		wrapped.gate = newWriteGate()
	}
	if opts.CrossProcessWrites && profile.CrossProcessLock && !opts.ReadOnly && !isMemoryDSN(dbPath) {
		lock, lerr := newFileLock(dbPath)
		if lerr != nil {
			db.Close()
			return nil, lerr
		}
		wrapped.flock = lock
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
func applyPragmasRetry(db *sql.DB, opts Options, profile Profile) error {
	const window = 15 * time.Second
	delay := 10 * time.Millisecond
	deadline := time.Now().Add(window)
	for {
		err := applyPragmas(db, opts, profile)
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

// Close releases the pool and then whatever this package created to back it.
// The only such thing today is the snapshot OpenReadOnly takes when a
// database's own directory cannot be written.
func (db *DB) Close() error {
	err := db.DB.Close()
	if cerr := db.flock.close(); cerr != nil && err == nil {
		err = cerr
	}
	if db.cleanup != nil {
		db.cleanup()
		db.cleanup = nil
	}
	return err
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
// driver. Every driver honours these PRAGMA statements via Exec.
//
// The driver's profile decides two of them: a driver without WAL keeps its own
// journal mode, and the busy timeout is the driver's (see busyTimeoutMS), since
// waiting for a lock helps only where another thread or process can release it
// meanwhile.
func applyPragmas(db *sql.DB, opts Options, profile Profile) error {
	busy := fmt.Sprintf("PRAGMA busy_timeout=%d", busyTimeoutMS)
	if opts.ReadOnly {
		// No journal_mode switch: it is a write, and the whole point of this
		// handle is that the database's directory may refuse one. query_only
		// then makes the refusal explicit at the first write rather than at
		// whatever the filesystem happens to allow.
		for _, p := range []string{
			busy,
			"PRAGMA cache_size=" + cacheSize,
			"PRAGMA temp_store=MEMORY",
			"PRAGMA query_only=ON",
		} {
			if _, err := db.Exec(p); err != nil { //nolint:noctx // startup pragmas
				return fmt.Errorf("execute %s: %w", p, err)
			}
		}
		return nil
	}
	// busy_timeout first: subsequent statements (notably the journal_mode=WAL
	// switch, which needs a write lock) then wait for a busy database instead
	// of failing immediately. The DSN sets this per connection too; this is
	// belt-and-suspenders for the connection applyPragmas runs on.
	pragmas := []string{busy}
	if profile.WAL {
		pragmas = append(pragmas, "PRAGMA journal_mode=WAL", "PRAGMA wal_autocheckpoint=10000")
	}
	pragmas = append(pragmas,
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA cache_size="+cacheSize,
		"PRAGMA temp_store=MEMORY",
	)
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil { //nolint:noctx // startup pragmas
			return fmt.Errorf("execute %s: %w", p, err)
		}
	}
	return nil
}
