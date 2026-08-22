// Package projectdb opens a kapi project's local store.
//
// One file — `.kapi/work/store.db` — holds every local projection the project keeps:
// the content memory, the terms store, the block cache and the unit working
// set. Each subsystem migrates its own schema under its own ledger table
// (`sievepen_migrations`, `termbase_migrations`, `cache_migrations`, `state`),
// which is what storage.Migrate's namespaced bookkeeping was built for, so
// nothing about a subsystem's schema changes by moving in.
//
// It was four files. They could not be written together: an approve-and-promote
// records a decision in the working set and teaches the wording to the content
// memory, and across two databases no transaction covers both — a crash between
// them left a decision blessing wording the memory never learned. Four files
// also meant four opens, four connection pools and four migration orders to get
// right at every entry point. One file, one pool, all migrations at open.
//
// Everything here except staged decisions is a projection rebuilt from
// committed sources, which is why the predecessor sweep migrates nothing: it
// carries the staged decisions forward and deletes the rest (see sweep.go).
//
// # Write discipline
//
// One file means one write lock, so the handle is opened with
// storage.ProjectOptions(): BEGIN IMMEDIATE on every transaction, and an
// in-process FIFO permit every write holds for its whole life. Both halves were
// measured, not assumed. Without them a converge run's content-memory writes
// starved the review loop's drip of unit-state writes almost completely — 32
// operations of 2650 completed, the rest failing SQLITE_BUSY after the five-second
// timeout, because SQLite's busy backoff has no notion of who has waited longest.
// With them the in-process busy count is zero and the starved writer runs at
// roughly nine tenths of what it managed when it had a file to itself.
//
// Two consequences worth knowing at the call site:
//
//   - A block-store session from Blocks() is ONE transaction over a whole
//     purge-and-refill, so it holds the permit from Begin to Commit. Every other
//     writer in the process waits. That is the correct reading of what SQLite
//     makes it anyway — the store has a single writer for the length of an
//     extraction — but it does mean a session must be closed, and that a write
//     issued from the goroutine holding one is a deadlock. It is reported rather
//     than hung: see storage.ErrWriteGateReentrant.
//   - The permit orders writers in THIS process. A second kapi process on the
//     same project still contends at the file level, where IMMEDIATE and
//     busy_timeout are all there is.
//
// Reads are never gated. Under WAL a reader neither blocks a writer nor waits
// for one, so `kapi status` beside a converge run costs nothing.
//
// Browser build: there is no file-backed SQLite driver, so storage.Open reports
// storage.ErrNoSQLite and Open degrades — Work() still functions, backed by the
// JSON sidecar at `.kapi/work/store.json`, while Memory(), Terms() and Blocks()
// return nil. That is not a hole the browser ever falls into: the host injects
// in-memory content-memory, terms and block backends before any browser code
// path reaches a project store. The degraded handle exists so the review→approve
// loop keeps its durability contract in the lab, where a decision recorded by
// one command must survive into the next.
package projectdb

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/blockstore/sqlitestore"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/voice"
)

// Primary tables the presence helpers probe. Presence is a row question, not a
// file question: in a merged store every subsystem's schema exists from the
// first open, so "is there a content memory?" can only mean "has anything been
// written to it?" — the file-existence signal the four-file layout used no
// longer distinguishes anything.
const (
	memoryTable   = "tm_entries"
	termsTable    = "tb_concepts"
	blocksTable   = "blocks"
	voiceTable    = "voice_profiles"
	overlaysTable = "overlays"
)

// DB is the project's open store: one connection pool with every subsystem
// bound to it.
//
// The handle owns the pool. The subsystem handles it hands out do not — do not
// Close() them, or the other three lose their database; Close this instead.
type DB struct {
	layout project.Layout

	// raw is nil on a build with no file-backed SQLite driver, where work is
	// the sidecar-backed store and the other three subsystems are absent.
	raw        *storage.DB
	memory     *memory.SQLiteStore
	terms      *terms.SQLiteStore
	voice      *voice.SQLiteStore
	blocks     blockstore.Store
	blocksAuto blockstore.Store
	work       *state.WorkStore
}

// Open opens (creating it if absent) the project's store at layout.StorePath()
// and runs every subsystem's migrations, having first folded any earlier state
// directory forward and then swept the predecessor four-file layout out of it.
//
// On a build with no file-backed SQLite driver it returns a degraded handle
// rather than an error — see the package documentation.
func Open(ctx context.Context, layout project.Layout) (*DB, error) {
	if layout.StateDir == "" {
		return nil, errors.New("projectdb: layout has no state directory")
	}
	if err := os.MkdirAll(layout.WorkDir(), 0o755); err != nil {
		return nil, fmt.Errorf("projectdb: create work dir: %w", err)
	}
	// Before the store opens: the fold moves the committed decision record the
	// working store seeds from, so it has to land first.
	foldLayoutForward(layout)

	raw, err := storage.OpenWith(layout.StorePath(), storage.ProjectOptions())
	if err != nil {
		if errors.Is(err, storage.ErrNoSQLite) {
			return openDegraded(ctx, layout)
		}
		return nil, fmt.Errorf("projectdb: open %s: %w", layout.StorePath(), err)
	}

	db := &DB{layout: layout, raw: raw}
	if err := db.bind(ctx); err != nil {
		_ = raw.Close()
		return nil, err
	}
	sweepPredecessors(ctx, layout, db)
	return db, nil
}

// bind runs every subsystem's migrations against the shared pool and attaches
// its handle. Order is only bookkeeping — the ledgers are independent — but it
// is fixed so a failure always names the same first offender.
func (d *DB) bind(ctx context.Context) error {
	if err := storage.Migrate(d.raw, metaMigrationsTable, metaMigrations); err != nil {
		return fmt.Errorf("projectdb: migrate store metadata: %w", err)
	}
	mem, err := memory.NewSQLiteStoreFromDB(d.raw)
	if err != nil {
		return fmt.Errorf("projectdb: bind content memory: %w", err)
	}
	tb, err := terms.NewSQLiteStoreFromDB(d.raw)
	if err != nil {
		return fmt.Errorf("projectdb: bind terms store: %w", err)
	}
	vc, err := voice.NewSQLiteStore(d.raw)
	if err != nil {
		return fmt.Errorf("projectdb: bind voice store: %w", err)
	}
	blocks, err := sqlitestore.NewFromDB(d.raw, false)
	if err != nil {
		return fmt.Errorf("projectdb: bind block store: %w", err)
	}
	auto, err := sqlitestore.NewFromDB(d.raw, true)
	if err != nil {
		return fmt.Errorf("projectdb: bind block store (autocommit): %w", err)
	}
	work, err := state.OpenWorkFromDB(ctx, d.raw, d.layout.UnitStateDir())
	if err != nil {
		return fmt.Errorf("projectdb: bind working store: %w", err)
	}
	d.memory, d.terms, d.voice, d.blocks, d.blocksAuto, d.work = mem, tb, vc, blocks, auto, work
	return nil
}

// openDegraded builds the browser build's handle: a sidecar-backed working
// store and nothing else.
func openDegraded(ctx context.Context, layout project.Layout) (*DB, error) {
	work, err := state.OpenWorkSidecar(ctx, layout.StoreSidecarPath(), layout.UnitStateDir())
	if err != nil {
		return nil, fmt.Errorf("projectdb: open working set sidecar: %w", err)
	}
	db := &DB{layout: layout, work: work}
	sweepPredecessors(ctx, layout, db)
	return db, nil
}

// Layout returns the project layout this store was opened for.
func (d *DB) Layout() project.Layout { return d.layout }

// Path returns the store file, whether or not this build can open one.
func (d *DB) Path() string { return d.layout.StorePath() }

// Raw returns the shared connection pool, for work that spans subsystems — an
// approve-and-promote writing a decision and a content-memory entry in one
// transaction is the reason it is exported. nil on a build with no file-backed
// SQLite driver.
func (d *DB) Raw() *storage.DB { return d.raw }

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

// Work returns the unit working set. Unlike the other three it is present on
// every build: where there is no SQLite driver it is backed by the JSON
// sidecar, because a staged decision has no other copy.
func (d *DB) Work() *state.WorkStore { return d.work }

// HasMemory reports whether the content memory holds any entry.
func (d *DB) HasMemory(ctx context.Context) (bool, error) {
	return d.hasRows(ctx, memoryTable)
}

// HasTerms reports whether the terms store holds any concept.
func (d *DB) HasTerms(ctx context.Context) (bool, error) {
	return d.hasRows(ctx, termsTable)
}

// HasVoice reports whether the voice store holds any profile.
func (d *DB) HasVoice(ctx context.Context) (bool, error) {
	return d.hasRows(ctx, voiceTable)
}

// HasBlocks reports whether the block cache holds any extracted block. It
// answers what a stat of `blocks.db` used to: whether the project has been
// extracted at all.
func (d *DB) HasBlocks(ctx context.Context) (bool, error) {
	return d.hasRows(ctx, blocksTable)
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
	if has, err := d.hasRows(ctx, blocksTable); err != nil || has {
		return has, err
	}
	return d.hasRows(ctx, overlaysTable)
}

// hasRows probes a subsystem's primary table for any row. table is always one
// of this package's constants, never caller input.
func (d *DB) hasRows(ctx context.Context, table string) (bool, error) {
	if d.raw == nil {
		return false, nil
	}
	var present int
	if err := d.raw.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM "+table+")").Scan(&present); err != nil {
		return false, fmt.Errorf("projectdb: probe %s: %w", table, err)
	}
	return present == 1, nil
}

// Close releases the connection pool. Idempotent; the subsystem handles are
// detached rather than closed, since they never owned the pool.
func (d *DB) Close() error {
	d.memory, d.terms, d.voice, d.blocks, d.blocksAuto, d.work = nil, nil, nil, nil, nil, nil
	if d.raw == nil {
		return nil
	}
	raw := d.raw
	d.raw = nil
	return raw.Close()
}
