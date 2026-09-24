// Package projectdb opens a project's projection and context stores.
//
// The projection is per checkout, at .kapi/work/store.db. It holds the block cache,
// flow overlays and extraction stamps derived from the working tree. Extraction
// can rewrite these tables in long transactions.
//
// The context store lives in a workspace (core/workspace), outside the checkout.
// It holds terms, voice profiles, content memory and the unit-state working set
// with staged decisions. Checkouts of the same project share this authored data.
//
// Each subsystem retains its own migration ledger, including sievepen_migrations,
// termbase_migrations, cache_migrations and state, regardless of its database pool.
//
// # Embedded layout
//
// Without a workspace, Open uses .kapi/work/store.db for both context and
// projection tables through one handle. The host supplies the workspace location
// through host.DataDir(); the framework has no default pointing to user data.
//
// Opening an embedded project with a workspace moves staged decisions into the
// context store, re-seeds the other context data from committed .kapi files, and
// drops the projection's context tables. See adopt.go.
//
// # Cross-store queries
//
// DB.Join attaches the projection as main and the context store as context for
// one read, allowing queries such as finding the blocks that use a term.
//
// # Writes
//
// storage.ProjectOptions configures BEGIN IMMEDIATE and an in-process FIFO write
// permit for each database file. SQLite busy retries alone do not order waiting
// writers; repeated short transactions can starve other writes. The FIFO permit
// provides that ordering within a process.
//
// The separate files allow context writes during extraction. A block-store session
// holds the projection permit throughout its purge-and-refill transaction, from
// Begin to Commit. Close every session. Writing to the same pool from a goroutine
// that already holds its permit returns storage.ErrWriteGateReentrant.
//
// Separate processes still contend through SQLite's file locking and busy_timeout.
// Reads bypass the write permit and use WAL concurrency.
//
// # Browser build
//
// Without a file-backed SQLite driver, storage.Open returns storage.ErrNoSQLite.
// Work remains available through .kapi/work/store.json; Memory, Terms and Blocks
// return nil. The host injects in-memory backends for those services. The sidecar
// preserves review decisions between browser commands.
package projectdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/blockstore/sqlitestore"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/reconcile"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/voice"
)

// Primary tables the presence helpers probe. Presence is a row question, not a
// file question: every subsystem's schema exists from the first open, so "is
// there a content memory?" can only mean "has anything been written to it?" —
// the file-existence signal the four-file layout used no longer distinguishes
// anything.
const (
	memoryTable    = "tm_entries"
	termsTable     = "tb_concepts"
	blocksTable    = "blocks"
	voiceTable     = "voice_profiles"
	overlaysTable  = "overlays"
	decisionsTable = "unit_decision"
)

// ContextSchema is the name the context store is attached under inside a Join.
// A query written against a join spells its context tables `context.<table>`.
const ContextSchema = "context"

// Stores names the databases a caller outside the framework has opened for this
// project: the workspace's context store, and the workspace database the
// context graph lives in.
//
// Both handles belong to the caller. The project store reads and writes through
// them and never closes them.
type Stores struct {
	// Context is the project's context store — one database in the workspace,
	// shared by every checkout of the project.
	Context *storage.DB
	// Graph is the database holding the context graph. It is the workspace
	// database, because a graph node's id already carries the project it
	// belongs to and a question that spans projects is one query there.
	Graph *storage.DB
}

// Option configures Open.
type Option func(*settings)

type settings struct {
	stores Stores
}

// WithWorkspace binds this project to a workspace: its context store, and the
// workspace database holding the context graph.
//
// Without it the context tables sit beside the projection in
// `.kapi/work/store.db` and the graph with them, which is the embedded layout
// the package documentation describes.
func WithWorkspace(s Stores) Option {
	return func(o *settings) { o.stores = s }
}

// DB is the project's open store: the projection pool, the context pool, and
// every subsystem bound to whichever holds its tables.
//
// The handle owns the projection pool. It does not own the pools a workspace
// supplied, and it does not own the subsystem handles it hands out — do not
// Close() those, or the others lose their database; Close this instead.
type DB struct {
	layout project.Layout

	// projection is nil on a build with no file-backed SQLite driver, where
	// work is the sidecar-backed store and the other subsystems are absent.
	projection *storage.DB
	// context holds the authored subsystems. It is the projection handle in the
	// embedded layout and the workspace's per-project database otherwise.
	context *storage.DB
	// graph holds the context graph: the workspace database where there is one,
	// the projection otherwise.
	graph *storage.DB
	// ownsContext records whether Close releases the context pool, which it
	// does only in the embedded layout, where the context pool IS the
	// projection.
	ownsContext bool

	memory     *memory.SQLiteStore
	terms      *terms.SQLiteStore
	voice      *voice.SQLiteStore
	blocks     blockstore.Store
	blocksAuto blockstore.Store
	work       *state.WorkStore
}

// Open opens (creating it if absent) the project's projection at
// layout.StorePath(), binds the context store a workspace supplied or falls
// back to the embedded layout, and runs every subsystem's migrations, having
// first folded any earlier state directory forward and then swept the
// predecessor four-file layout out of it.
//
// On a build with no file-backed SQLite driver it returns a degraded handle
// rather than an error — see the package documentation.
func Open(ctx context.Context, layout project.Layout, opts ...Option) (*DB, error) {
	if layout.StateDir == "" {
		return nil, errors.New("projectdb: layout has no state directory")
	}
	var cfg settings
	for _, opt := range opts {
		opt(&cfg)
	}
	if err := os.MkdirAll(layout.WorkDir(), 0o755); err != nil {
		return nil, fmt.Errorf("projectdb: create work dir: %w", err)
	}
	// Before the store opens: the fold moves the committed decision record the
	// working store seeds from, so it has to land first.
	foldLayoutForward(layout)

	projection, err := storage.OpenWith(layout.StorePath(), storage.ProjectOptions())
	if err != nil {
		if errors.Is(err, storage.ErrNoSQLite) {
			return openDegraded(ctx, layout)
		}
		return nil, fmt.Errorf("projectdb: open %s: %w", layout.StorePath(), err)
	}

	db := &DB{layout: layout, projection: projection, context: cfg.stores.Context, graph: cfg.stores.Graph}
	if db.context == nil {
		db.context, db.ownsContext = projection, true
	}
	if db.graph == nil {
		db.graph = projection
	}

	// Whether the context store has ever held a table decides whether this open
	// adopts a project out of the embedded layout, and binding creates those
	// tables, so the question is asked first.
	adopting := false
	if !db.ownsContext {
		adopting, err = isEmptyDatabase(ctx, db.context)
		if err != nil {
			_ = projection.Close()
			return nil, err
		}
	}

	if err := db.bind(ctx); err != nil {
		_ = projection.Close()
		return nil, err
	}
	if adopting {
		adoptEmbeddedContext(ctx, db)
	}
	sweepPredecessors(ctx, layout, db)
	return db, nil
}

// bind runs every subsystem's migrations against the pool that holds its tables
// and attaches its handle. Order within a pool is only bookkeeping — the
// ledgers are independent — but it is fixed so a failure always names the same
// first offender.
func (d *DB) bind(ctx context.Context) error {
	if err := storage.Migrate(d.projection, metaMigrationsTable, metaMigrations); err != nil {
		return fmt.Errorf("projectdb: migrate store metadata: %w", err)
	}
	// The same table in the context pool, for the facts that are true of the
	// project rather than of this checkout. In the embedded layout the two
	// pools are one handle and the second migration is the same one.
	if !d.ownsContext {
		if err := storage.Migrate(d.context, metaMigrationsTable, metaMigrations); err != nil {
			return fmt.Errorf("projectdb: migrate context metadata: %w", err)
		}
	}
	blocks, err := sqlitestore.NewFromDB(d.projection, false)
	if err != nil {
		return fmt.Errorf("projectdb: bind block store: %w", err)
	}
	auto, err := sqlitestore.NewFromDB(d.projection, true)
	if err != nil {
		return fmt.Errorf("projectdb: bind block store (autocommit): %w", err)
	}

	mem, err := memory.NewSQLiteStoreFromDB(d.context)
	if err != nil {
		return fmt.Errorf("projectdb: bind content memory: %w", err)
	}
	tb, err := terms.NewSQLiteStoreFromDB(d.context)
	if err != nil {
		return fmt.Errorf("projectdb: bind terms store: %w", err)
	}
	vc, err := voice.NewSQLiteStore(d.context)
	if err != nil {
		return fmt.Errorf("projectdb: bind voice store: %w", err)
	}
	work, err := state.OpenWorkFromDB(ctx, d.context, d.layout.Export().UnitStateDir())
	if err != nil {
		return fmt.Errorf("projectdb: bind working store: %w", err)
	}
	d.memory, d.terms, d.voice, d.blocks, d.blocksAuto, d.work = mem, tb, vc, blocks, auto, work
	return nil
}

// openDegraded builds the browser build's handle: a sidecar-backed working
// store and nothing else.
func openDegraded(ctx context.Context, layout project.Layout) (*DB, error) {
	work, err := state.OpenWorkSidecar(ctx, layout.StoreSidecarPath(), layout.Export().UnitStateDir())
	if err != nil {
		return nil, fmt.Errorf("projectdb: open working set sidecar: %w", err)
	}
	db := &DB{layout: layout, work: work}
	sweepPredecessors(ctx, layout, db)
	return db, nil
}

// Layout returns the project layout this store was opened for.
func (d *DB) Layout() project.Layout { return d.layout }

// Path returns the projection file, whether or not this build can open one.
func (d *DB) Path() string { return d.layout.StorePath() }

// ContextPath returns the file holding the project's authored context: the
// workspace's per-project database, or the projection in the embedded layout.
// Empty on a build with no file-backed SQLite driver.
func (d *DB) ContextPath() string {
	if d.context == nil {
		return ""
	}
	return d.context.Path()
}

// Raw returns the connection pool the authored subsystems share — the content
// memory, the terms store, the voice store and the unit working set.
//
// It is exported for work that spans them: an approve-and-promote writing a
// decision and a content-memory entry in one transaction is the reason. nil on
// a build with no file-backed SQLite driver.
func (d *DB) Raw() *storage.DB { return d.context }

// Projection returns the pool holding what this checkout derived: the block
// cache, the overlays and the store metadata. nil on a build with no
// file-backed SQLite driver.
func (d *DB) Projection() *storage.DB { return d.projection }

// Graph returns the pool the context graph is migrated into: the workspace
// database where this project belongs to one, the projection otherwise. nil on
// a build with no file-backed SQLite driver.
func (d *DB) Graph() *storage.DB { return d.graph }

// Memory returns the project's content memory, or nil where the build has no
// file-backed SQLite driver.
func (d *DB) Memory() *memory.SQLiteStore { return d.memory }

// Terms returns the project's terms store, or nil where the build has no
// file-backed SQLite driver.
func (d *DB) Terms() *terms.SQLiteStore { return d.terms }

// Voice returns the project's voice store — the profiles a recipe's
// `voice: profile:` binding names — or nil where the build has no file-backed
// SQLite driver.
func (d *DB) Voice() *voice.SQLiteStore { return d.voice }

// Blocks returns the session-transactional block store: a session is one
// *sql.Tx, so writes land all-or-nothing at Commit. This is the mode for
// extraction's purge-and-refill. nil where the build has no file-backed SQLite
// driver.
func (d *DB) Blocks() blockstore.Store { return d.blocks }

// BlocksAutocommit returns the autocommit block store, where every session
// read/write is its own statement on the shared pool. This is the mode for
// concurrent flow runs, whose run-long read+write transactions would otherwise
// deadlock-avoid into immediate SQLITE_BUSY. nil where the build has no
// file-backed SQLite driver.
func (d *DB) BlocksAutocommit() blockstore.Store { return d.blocksAuto }

// Work returns the unit working set. Unlike the others it is present on every
// build: where there is no SQLite driver it is backed by the JSON sidecar,
// because a staged decision has no other copy.
func (d *DB) Work() *state.WorkStore { return d.work }

// Join runs fn on one connection that sees both of the project's databases: the
// projection as `main` and the context store as `context` (ContextSchema). It
// is how a question that spans them — which blocks use this term, which of them
// carry a decision — stays one query after the split.
//
// The connection is read-only for the length of fn. A join reads; a write that
// has to span the two pools is two transactions, and the subsystem that needs
// them atomic keeps its tables in one pool for that reason.
//
// Reports ErrNoStore on a build with no file-backed SQLite driver.
func (d *DB) Join(ctx context.Context, fn func(context.Context, *sql.Conn) error) error {
	if d.projection == nil || d.context == nil {
		return ErrNoStore
	}
	// The pool's own connection, not a gated write path: ATTACH takes no write
	// lock and every statement inside fn is a read.
	conn, err := d.projection.DB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("projectdb: open a joined connection: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx,
		`ATTACH DATABASE `+quoteSQLiteString(d.context.Path())+` AS `+ContextSchema); err != nil {
		return fmt.Errorf("projectdb: attach the context store: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `PRAGMA query_only=ON`); err != nil {
		return fmt.Errorf("projectdb: seal the joined connection: %w", err)
	}

	runErr := fn(ctx, conn)

	// Both settings are per connection and the connection goes back to a shared
	// pool, so leaving either in place would hand the next caller a sealed
	// connection with somebody else's database attached.
	var cleanupErr error
	if _, err := conn.ExecContext(ctx, `PRAGMA query_only=OFF`); err != nil {
		cleanupErr = fmt.Errorf("projectdb: unseal the joined connection: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `DETACH DATABASE `+ContextSchema); err != nil && cleanupErr == nil {
		cleanupErr = fmt.Errorf("projectdb: detach the context store: %w", err)
	}
	if runErr != nil {
		return runErr
	}
	return cleanupErr
}

// quoteSQLiteString renders a path as a SQL string literal. ATTACH takes a
// parameter in most drivers, but not on a raw connection under every build, and
// a path is the one value here that is not caller input in the injection sense
// — it comes from the layout — so doubling the quote is the whole escape.
func quoteSQLiteString(s string) string {
	out := make([]byte, 0, len(s)+2)
	out = append(out, '\'')
	for i := range len(s) {
		if s[i] == '\'' {
			out = append(out, '\'')
		}
		out = append(out, s[i])
	}
	return string(append(out, '\''))
}

// AdoptDocuments resolves which document each freshly read source file is,
// against what the project already knows, and records the result — the
// project.DocumentAdopter an extraction looks for.
//
// It is on the handle rather than reached through Work() so extraction names a
// capability instead of a store, the same way it does for the drift stamps.
func (d *DB) AdoptDocuments(ctx context.Context, current []reconcile.DocUnit) (map[string]string, error) {
	if d.work == nil {
		return nil, ErrNoStore
	}
	return d.work.AdoptDocuments(ctx, current)
}

// HasMemory reports whether the content memory holds any entry.
func (d *DB) HasMemory(ctx context.Context) (bool, error) {
	return hasRows(ctx, d.context, memoryTable)
}

// HasTerms reports whether the terms store holds any concept.
func (d *DB) HasTerms(ctx context.Context) (bool, error) {
	return hasRows(ctx, d.context, termsTable)
}

// HasVoice reports whether the voice store holds any profile.
func (d *DB) HasVoice(ctx context.Context) (bool, error) {
	return hasRows(ctx, d.context, voiceTable)
}

// HasDecisions reports whether the ledger holds any decision, whichever
// checkout recorded it. A degraded build keeps its decisions in the sidecar
// rather than a table, and answers from there.
func (d *DB) HasDecisions(ctx context.Context) (bool, error) {
	if d.context == nil {
		if d.work == nil {
			return false, nil
		}
		held, err := d.work.Ledger(ctx)
		return len(held) > 0, err
	}
	return hasRows(ctx, d.context, decisionsTable)
}

// HasBlocks reports whether the block cache holds any extracted block. It
// answers what a stat of `blocks.db` used to: whether the project has been
// extracted at all.
func (d *DB) HasBlocks(ctx context.Context) (bool, error) {
	return hasRows(ctx, d.projection, blocksTable)
}

// HasBlockCache reports whether the block-cache subsystem holds anything worth
// carrying — blocks or overlays.
//
// It is deliberately wider than HasBlocks. A flow run persists the overlays it
// produced without caching the blocks it parsed, so a project can hold
// translated variants and no rows in `blocks`. Under the four-file layout that
// project still had a `blocks.db` on disk and packed fine; a gate on `blocks`
// alone silently dropped its work. HasBlocks keeps the narrower "has this been
// extracted?" meaning its own callers need.
func (d *DB) HasBlockCache(ctx context.Context) (bool, error) {
	if has, err := hasRows(ctx, d.projection, blocksTable); err != nil || has {
		return has, err
	}
	return hasRows(ctx, d.projection, overlaysTable)
}

// hasRows probes a subsystem's primary table for any row. table is always one
// of this package's constants, never caller input.
func hasRows(ctx context.Context, db *storage.DB, table string) (bool, error) {
	if db == nil {
		return false, nil
	}
	var present int
	if err := db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM "+table+")").Scan(&present); err != nil {
		return false, fmt.Errorf("projectdb: probe %s: %w", table, err)
	}
	return present == 1, nil
}

// Close releases the pools this handle owns. Idempotent; the subsystem handles
// are detached rather than closed, since they never owned a pool, and a context
// store a workspace supplied is left open for its owner.
func (d *DB) Close() error {
	d.memory, d.terms, d.voice, d.blocks, d.blocksAuto, d.work = nil, nil, nil, nil, nil, nil
	projection := d.projection
	d.projection, d.context, d.graph = nil, nil, nil
	if projection == nil {
		return nil
	}
	return projection.Close()
}
