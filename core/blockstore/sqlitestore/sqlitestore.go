//go:build !wasm

// Package sqlitestore provides the SQLite-backed implementation of the
// blockstore.Store interface. It is split out of core/blockstore so the
// interface package (and its importers such as core/flow) stay free of the
// cgo SQLite driver: only callers that actually construct a persistent,
// on-disk block store import this leaf package and pull in core/storage.
package sqlitestore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"strings"
	"sync"
	"time"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/storage"
)

// New opens (or creates) a SQLite-backed block store at the given path.
// Intended use: project-local persistence in the project store,
// `.kapi/work/store.db`, where core/projectdb binds it alongside the other
// subsystems — full random access, append-friendly for concurrent overlay
// writes.
//
// The database schema is internal to this package and versioned by
// `cache_migrations`. Safe to delete and rebuild from another source
// (the file is a cache in the "easily reconstructable" sense).
func New(path string) (blockstore.Store, error) {
	return open(path, false)
}

// NewAutocommit opens the store in AUTOCOMMIT session mode: every session
// read/write runs as its own short statement on the shared pool instead of
// inside one session-long *sql.Tx, and Commit/Rollback are lifecycle no-ops.
// This is the mode for concurrent flow runs (converge fans locales out, each
// file-run holding a session): run-long read+write transactions on one SQLite
// file deadlock-avoid into immediate SQLITE_BUSY, bypassing busy_timeout.
// Overlays/blocks are idempotent per key and durable the moment the write
// returns — matching the executor's long-standing intent (it already opens
// sessions on the parent context precisely so cancellation does NOT roll back
// written overlays). Use New for single-writer, all-or-nothing work
// (extraction's purge-and-refill).
func NewAutocommit(path string) (blockstore.Store, error) {
	return open(path, true)
}

// NewFromDB adopts an already-open database — the project's merged
// `.kapi/work/store.db`, where the block cache is one schema among the content
// memory, the terms store and the unit working set. The migrations and the
// `cache_migrations` ledger are the same ones a standalone file gets, which is
// the whole point of namespaced ledgers: a store opened either way is the same
// store.
//
// The returned Store does NOT own db. Close detaches this handle and leaves the
// connection pool open, because the merged store's owner closes it exactly
// once; a subsystem closing a pool three other subsystems are still using is
// the failure this exists to prevent.
func NewFromDB(db *storage.DB, autocommit bool) (blockstore.Store, error) {
	if db == nil {
		return nil, errors.New("blockstore: adopt cache: nil database")
	}
	if err := storage.Migrate(db, "cache_migrations", cacheMigrations); err != nil {
		return nil, fmt.Errorf("blockstore: migrate cache: %w", err)
	}
	return &cacheStore{db: db, autocommit: autocommit}, nil
}

func open(path string, autocommit bool) (blockstore.Store, error) {
	db, err := storage.Open(path)
	if err != nil {
		return nil, fmt.Errorf("blockstore: open cache: %w", err)
	}
	if err := storage.Migrate(db, "cache_migrations", cacheMigrations); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("blockstore: migrate cache: %w", err)
	}
	return &cacheStore{db: db, autocommit: autocommit, owns: true}, nil
}

type cacheStore struct {
	db         *storage.DB
	autocommit bool
	// owns records whether Close may close the pool: true for a store that
	// opened its own file, false for one that adopted a shared database.
	owns bool
	mu   sync.Mutex // guards Close; per-operation writes serialize via SQLite WAL
}

func (k *cacheStore) Capabilities() blockstore.Capabilities {
	return blockstore.Capabilities{RandomAccess: true, Concurrent: true, Writable: true, Persistent: true}
}

// Begin opens a session. In the default (transactional) mode the session is
// backed by one *sql.Tx: writes become visible at Commit and Rollback
// discards them — the all-or-nothing contract extraction relies on. In
// autocommit mode (NewAutocommit) there is no session transaction; see
// NewAutocommit for why concurrent flow runs need that.
func (k *cacheStore) Begin(ctx context.Context) (blockstore.Session, error) {
	if k.db == nil {
		return nil, blockstore.ErrClosed
	}
	if k.autocommit {
		return &cacheSession{store: k, ctx: ctx}, nil
	}
	tx, err := k.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("blockstore: begin tx: %w", err)
	}
	return &cacheSession{store: k, tx: tx, ctx: ctx}, nil
}

func (k *cacheStore) Close() error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.db == nil {
		return nil
	}
	db := k.db
	k.db = nil
	if !k.owns {
		return nil // adopted database: its owner closes the pool
	}
	return db.Close()
}

// ─── Session ────────────────────────────────────────────────────

type cacheSession struct {
	store *cacheStore
	// tx is nil in autocommit mode: statements run on the pool. On a gated
	// handle it also carries the write permit, taken at Begin and returned at
	// Commit or Rollback — so a session is the project store's single writer
	// for its whole life, which is what a purge-and-refill needs anyway.
	tx   *storage.Tx
	ctx  context.Context
	done bool
}

// q returns the session's statement runner: its transaction, or the shared
// pool in autocommit mode.
func (s *cacheSession) q() interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
} {
	if s.tx != nil {
		return s.tx
	}
	return s.store.db
}

func (s *cacheSession) Capabilities() blockstore.Capabilities { return s.store.Capabilities() }

func (s *cacheSession) Blocks(filter blockstore.BlockFilter) iter.Seq2[*blockstore.Block, error] {
	return func(yield func(*blockstore.Block, error) bool) {
		if s.done {
			yield(nil, blockstore.ErrClosed)
			return
		}
		q := strings.Builder{}
		q.WriteString(`SELECT payload FROM blocks WHERE 1=1`)
		args := []any{}
		if filter.Collection != "" {
			q.WriteString(` AND collection = ?`)
			args = append(args, filter.Collection)
		}
		if filter.Translatable != nil {
			q.WriteString(` AND translatable = ?`)
			args = append(args, boolInt(*filter.Translatable))
		}
		q.WriteString(` ORDER BY hash`)
		if filter.Limit > 0 {
			q.WriteString(` LIMIT ?`)
			args = append(args, filter.Limit)
		}
		rows, err := s.q().QueryContext(s.ctx, q.String(), args...)
		if err != nil {
			yield(nil, fmt.Errorf("blockstore: query blocks: %w", err))
			return
		}
		defer rows.Close()
		for rows.Next() {
			var payload []byte
			if err := rows.Scan(&payload); err != nil {
				yield(nil, fmt.Errorf("blockstore: scan block: %w", err))
				return
			}
			var b blockstore.Block
			if err := json.Unmarshal(payload, &b); err != nil {
				yield(nil, fmt.Errorf("blockstore: decode block: %w", err))
				return
			}
			if !yield(&b, nil) {
				return
			}
		}
		if err := rows.Err(); err != nil {
			yield(nil, fmt.Errorf("blockstore: iterate blocks: %w", err))
		}
	}
}

func (s *cacheSession) GetBlock(hash string) (*blockstore.Block, error) {
	if s.done {
		return nil, blockstore.ErrClosed
	}
	var payload []byte
	err := s.q().QueryRowContext(s.ctx, `SELECT payload FROM blocks WHERE hash = ?`, hash).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, blockstore.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("blockstore: get block: %w", err)
	}
	var b blockstore.Block
	if err := json.Unmarshal(payload, &b); err != nil {
		return nil, fmt.Errorf("blockstore: decode block: %w", err)
	}
	return &b, nil
}

func (s *cacheSession) PutBlock(collection string, b *blockstore.Block) error {
	if s.done {
		return blockstore.ErrClosed
	}
	if b == nil || b.Hash == "" {
		return errors.New("blockstore: block must have a non-empty Hash")
	}
	payload, err := json.Marshal(b)
	if err != nil {
		return fmt.Errorf("blockstore: encode block: %w", err)
	}
	// text_indexed goes back to 0: this block's searchable text is now stale,
	// and the text index reconciles it the next time something searches. The
	// column costs nothing to write and is the whole of extraction's share of
	// the index — see textindex.go for why the build is not on this path.
	_, err = s.q().ExecContext(s.ctx, `
		INSERT INTO blocks (hash, collection, translatable, payload)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(hash) DO UPDATE SET
			collection=excluded.collection,
			translatable=excluded.translatable,
			payload=excluded.payload,
			text_indexed=0
	`, b.Hash, collection, boolInt(b.Translatable), payload)
	if err != nil {
		return fmt.Errorf("blockstore: put block: %w", err)
	}
	return nil
}

// putBlockTexts reconciles one block's searchable text with what the store
// holds: a row per locale that carries text, and nothing left over. Called by
// the index build, never by PutBlock.
func (s *cacheSession) putBlockTexts(b *blockstore.Block) error {
	texts := blockstore.BlockTexts(b)

	// Drop rows for locales this block no longer has. Written first so a block
	// whose text vanished entirely still loses its rows.
	keep := make([]any, 0, len(texts)+1)
	keep = append(keep, b.Hash)
	placeholders := make([]string, 0, len(texts))
	for _, t := range texts {
		keep = append(keep, t.Locale)
		placeholders = append(placeholders, "?")
	}
	del := `DELETE FROM block_texts WHERE block_hash = ?`
	if len(placeholders) > 0 {
		del += ` AND locale NOT IN (` + strings.Join(placeholders, ",") + `)`
	}
	if _, err := s.q().ExecContext(s.ctx, del, keep...); err != nil {
		return fmt.Errorf("blockstore: prune block text: %w", err)
	}

	for _, t := range texts {
		if _, err := s.q().ExecContext(s.ctx, `
			INSERT INTO block_texts (block_hash, locale, text, text_lower)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(block_hash, locale) DO UPDATE SET
				text=excluded.text,
				text_lower=excluded.text_lower
			WHERE block_texts.text IS NOT excluded.text
		`, b.Hash, t.Locale, t.Text, strings.ToLower(t.Text)); err != nil {
			return fmt.Errorf("blockstore: put block text: %w", err)
		}
	}
	return nil
}

// DeleteBlocks drops every block in this session's transaction, leaving
// overlays intact. Implements blockstore.BlockPurger so a full re-extraction
// can rebuild the block set from scratch without stale rows lingering.
func (s *cacheSession) DeleteBlocks() error {
	if s.done {
		return blockstore.ErrClosed
	}
	// The text rows go first and by their own statement, so the FTS index is
	// maintained by block_texts' own triggers. Cascading them off a trigger on
	// blocks would nest one trigger inside another over a contentless FTS5
	// table, where a duplicated 'delete' command corrupts the index.
	if _, err := s.q().ExecContext(s.ctx, `DELETE FROM block_texts`); err != nil {
		return fmt.Errorf("blockstore: delete block text: %w", err)
	}
	if _, err := s.q().ExecContext(s.ctx, `DELETE FROM blocks`); err != nil {
		return fmt.Errorf("blockstore: delete blocks: %w", err)
	}
	return nil
}

func (s *cacheSession) GetOverlay(kind, blockHash string) (blockstore.Overlay, error) {
	if s.done {
		return blockstore.Overlay{}, blockstore.ErrClosed
	}
	kind = blockstore.CanonicalOverlayKind(kind)
	var sc blockstore.Overlay
	err := s.q().QueryRowContext(s.ctx, `
		SELECT kind, block_hash, payload, updated_at
		FROM overlays WHERE kind = ? AND block_hash = ?
	`, kind, blockHash).Scan(&sc.Kind, &sc.BlockHash, &sc.Payload, &sc.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return blockstore.Overlay{}, blockstore.ErrNotFound
	}
	if err != nil {
		return blockstore.Overlay{}, fmt.Errorf("blockstore: get overlay: %w", err)
	}
	return sc, nil
}

func (s *cacheSession) PutOverlay(sc blockstore.Overlay) error {
	if s.done {
		return blockstore.ErrClosed
	}
	if sc.Kind == "" || sc.BlockHash == "" {
		return errors.New("blockstore: overlay needs both Kind and BlockHash")
	}
	sc.Kind = blockstore.CanonicalOverlayKind(sc.Kind)
	if sc.UpdatedAt == 0 {
		sc.UpdatedAt = time.Now().Unix()
	}
	_, err := s.q().ExecContext(s.ctx, `
		INSERT INTO overlays (kind, block_hash, payload, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(kind, block_hash) DO UPDATE SET
			payload=excluded.payload,
			updated_at=excluded.updated_at
	`, sc.Kind, sc.BlockHash, sc.Payload, sc.UpdatedAt)
	if err != nil {
		return fmt.Errorf("blockstore: put overlay: %w", err)
	}
	return nil
}

func (s *cacheSession) ListOverlays(kind string) iter.Seq2[blockstore.Overlay, error] {
	kind = blockstore.CanonicalOverlayKind(kind)
	return func(yield func(blockstore.Overlay, error) bool) {
		if s.done {
			yield(blockstore.Overlay{}, blockstore.ErrClosed)
			return
		}
		rows, err := s.q().QueryContext(s.ctx, `
			SELECT kind, block_hash, payload, updated_at
			FROM overlays WHERE kind = ? ORDER BY block_hash
		`, kind)
		if err != nil {
			yield(blockstore.Overlay{}, fmt.Errorf("blockstore: list overlays: %w", err))
			return
		}
		defer rows.Close()
		for rows.Next() {
			var sc blockstore.Overlay
			if err := rows.Scan(&sc.Kind, &sc.BlockHash, &sc.Payload, &sc.UpdatedAt); err != nil {
				yield(blockstore.Overlay{}, fmt.Errorf("blockstore: scan overlay: %w", err))
				return
			}
			if !yield(sc, nil) {
				return
			}
		}
		if err := rows.Err(); err != nil {
			yield(blockstore.Overlay{}, fmt.Errorf("blockstore: iterate overlays: %w", err))
		}
	}
}

func (s *cacheSession) AllOverlays() iter.Seq2[blockstore.Overlay, error] {
	return func(yield func(blockstore.Overlay, error) bool) {
		if s.done {
			yield(blockstore.Overlay{}, blockstore.ErrClosed)
			return
		}
		rows, err := s.q().QueryContext(s.ctx, `
			SELECT kind, block_hash, payload, updated_at
			FROM overlays ORDER BY kind, block_hash
		`)
		if err != nil {
			yield(blockstore.Overlay{}, fmt.Errorf("blockstore: list all overlays: %w", err))
			return
		}
		defer rows.Close()
		for rows.Next() {
			var sc blockstore.Overlay
			if err := rows.Scan(&sc.Kind, &sc.BlockHash, &sc.Payload, &sc.UpdatedAt); err != nil {
				yield(blockstore.Overlay{}, fmt.Errorf("blockstore: scan overlay: %w", err))
				return
			}
			if !yield(sc, nil) {
				return
			}
		}
		if err := rows.Err(); err != nil {
			yield(blockstore.Overlay{}, fmt.Errorf("blockstore: iterate overlays: %w", err))
		}
	}
}

func (s *cacheSession) Commit() error {
	if s.done {
		return blockstore.ErrClosed
	}
	s.done = true
	if s.tx == nil {
		return nil // autocommit session: every write is already durable
	}
	return s.tx.Commit()
}

func (s *cacheSession) Rollback() error {
	if s.done {
		return nil
	}
	s.done = true
	if s.tx == nil {
		// Autocommit session: completed writes stay (idempotent per-key
		// caches — valid work a later pass reuses). Rollback only closes.
		return nil
	}
	return s.tx.Rollback()
}

func (s *cacheSession) Close() error {
	if !s.done {
		return s.Rollback()
	}
	return nil
}

// ─── Migrations ─────────────────────────────────────────────────

var cacheMigrations = []storage.Migration{
	{
		Version:     1,
		Description: "blocks + overlays",
		SQL: `
			CREATE TABLE blocks (
				hash         TEXT PRIMARY KEY,
				collection   TEXT NOT NULL,
				translatable INTEGER NOT NULL,
				payload      BLOB NOT NULL
			);
			CREATE INDEX blocks_collection_idx ON blocks (collection);

			CREATE TABLE overlays (
				kind       TEXT NOT NULL,
				block_hash TEXT NOT NULL,
				payload    BLOB NOT NULL,
				updated_at INTEGER NOT NULL,
				PRIMARY KEY (kind, block_hash)
			);
			CREATE INDEX overlays_hash_idx ON overlays (block_hash);
		`,
	},
	{
		Version:     2,
		Description: "block text index (FTS5 trigram over source and target text)",
		SQL: `
			CREATE TABLE block_texts (
				block_hash TEXT NOT NULL,
				locale     TEXT NOT NULL,
				text       TEXT NOT NULL,
				text_lower TEXT NOT NULL,
				PRIMARY KEY (block_hash, locale)
			);

			-- Contentless FTS5 trigram over block_texts.text_lower, modelled on
			-- the terms index: the tokenizer is identical across cgo and no-cgo
			-- builds, so a store written by one binary stays queryable by the
			-- other.
			CREATE VIRTUAL TABLE block_texts_trigram USING fts5(
				text_lower,
				content='block_texts', content_rowid='rowid',
				tokenize='trigram'
			);

			CREATE TRIGGER block_texts_trigram_ai AFTER INSERT ON block_texts BEGIN
				INSERT INTO block_texts_trigram(rowid, text_lower) VALUES (new.rowid, new.text_lower);
			END;
			CREATE TRIGGER block_texts_trigram_ad AFTER DELETE ON block_texts BEGIN
				INSERT INTO block_texts_trigram(block_texts_trigram, rowid, text_lower)
				VALUES ('delete', old.rowid, old.text_lower);
			END;
			CREATE TRIGGER block_texts_trigram_au AFTER UPDATE ON block_texts BEGIN
				INSERT INTO block_texts_trigram(block_texts_trigram, rowid, text_lower)
				VALUES ('delete', old.rowid, old.text_lower);
				INSERT INTO block_texts_trigram(rowid, text_lower) VALUES (new.rowid, new.text_lower);
			END;

			-- text_indexed marks a block whose text rows are current. Rows that
			-- predate this migration are 0, so the first search files them.
			--
			-- Deliberately UNINDEXED. A partial index on text_indexed = 0
			-- would make the staleness probe a seek, but every block write
			-- would maintain it, a cost on extraction, which writes the whole
			-- project, to save one scan on a query that runs occasionally. The
			-- scan stays where it belongs.
			ALTER TABLE blocks ADD COLUMN text_indexed INTEGER NOT NULL DEFAULT 0;
		`,
	},
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
