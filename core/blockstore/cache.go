package blockstore

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

	"github.com/neokapi/neokapi/core/storage"
)

// NewCacheStore opens (or creates) a SQLite-backed block store at the
// given path. Intended use: project-local persistence at
// `.kapi/cache/blocks.db`, full random access, append-friendly for concurrent
// overlay writes.
//
// The database schema is internal to this package and versioned by
// `cache_migrations`. Safe to delete and rebuild from another source
// (the file is a cache in the "easily reconstructable" sense).
func NewCacheStore(path string) (Store, error) {
	db, err := storage.Open(path)
	if err != nil {
		return nil, fmt.Errorf("blockstore: open cache: %w", err)
	}
	if err := storage.Migrate(db, "cache_migrations", cacheMigrations); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("blockstore: migrate cache: %w", err)
	}
	return &cacheStore{db: db}, nil
}

type cacheStore struct {
	db *storage.DB
	mu sync.Mutex // guards Close; write transactions serialize via SQLite WAL
}

func (k *cacheStore) Capabilities() Capabilities {
	return Capabilities{RandomAccess: true, Concurrent: true, Writable: true, Persistent: true}
}

func (k *cacheStore) Begin(ctx context.Context) (Session, error) {
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
	err := k.db.Close()
	k.db = nil
	return err
}

// ─── Session ────────────────────────────────────────────────────

type cacheSession struct {
	store *cacheStore
	tx    *sql.Tx
	ctx   context.Context
	done  bool
}

func (s *cacheSession) Capabilities() Capabilities { return s.store.Capabilities() }

func (s *cacheSession) Blocks(filter BlockFilter) iter.Seq2[*Block, error] {
	return func(yield func(*Block, error) bool) {
		if s.done {
			yield(nil, ErrClosed)
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
		rows, err := s.tx.QueryContext(s.ctx, q.String(), args...)
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
			var b Block
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

func (s *cacheSession) GetBlock(hash string) (*Block, error) {
	if s.done {
		return nil, ErrClosed
	}
	var payload []byte
	err := s.tx.QueryRowContext(s.ctx, `SELECT payload FROM blocks WHERE hash = ?`, hash).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("blockstore: get block: %w", err)
	}
	var b Block
	if err := json.Unmarshal(payload, &b); err != nil {
		return nil, fmt.Errorf("blockstore: decode block: %w", err)
	}
	return &b, nil
}

func (s *cacheSession) PutBlock(collection string, b *Block) error {
	if s.done {
		return ErrClosed
	}
	if b == nil || b.Hash == "" {
		return errors.New("blockstore: block must have a non-empty Hash")
	}
	payload, err := json.Marshal(b)
	if err != nil {
		return fmt.Errorf("blockstore: encode block: %w", err)
	}
	_, err = s.tx.ExecContext(s.ctx, `
		INSERT INTO blocks (hash, collection, translatable, payload)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(hash) DO UPDATE SET
			collection=excluded.collection,
			translatable=excluded.translatable,
			payload=excluded.payload
	`, b.Hash, collection, boolInt(b.Translatable), payload)
	if err != nil {
		return fmt.Errorf("blockstore: put block: %w", err)
	}
	return nil
}

func (s *cacheSession) GetOverlay(kind, blockHash string) (Overlay, error) {
	if s.done {
		return Overlay{}, ErrClosed
	}
	var sc Overlay
	err := s.tx.QueryRowContext(s.ctx, `
		SELECT kind, block_hash, payload, updated_at
		FROM overlays WHERE kind = ? AND block_hash = ?
	`, kind, blockHash).Scan(&sc.Kind, &sc.BlockHash, &sc.Payload, &sc.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Overlay{}, ErrNotFound
	}
	if err != nil {
		return Overlay{}, fmt.Errorf("blockstore: get overlay: %w", err)
	}
	return sc, nil
}

func (s *cacheSession) PutOverlay(sc Overlay) error {
	if s.done {
		return ErrClosed
	}
	if sc.Kind == "" || sc.BlockHash == "" {
		return errors.New("blockstore: overlay needs both Kind and BlockHash")
	}
	if sc.UpdatedAt == 0 {
		sc.UpdatedAt = time.Now().Unix()
	}
	_, err := s.tx.ExecContext(s.ctx, `
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

func (s *cacheSession) ListOverlays(kind string) iter.Seq2[Overlay, error] {
	return func(yield func(Overlay, error) bool) {
		if s.done {
			yield(Overlay{}, ErrClosed)
			return
		}
		rows, err := s.tx.QueryContext(s.ctx, `
			SELECT kind, block_hash, payload, updated_at
			FROM overlays WHERE kind = ? ORDER BY block_hash
		`, kind)
		if err != nil {
			yield(Overlay{}, fmt.Errorf("blockstore: list overlays: %w", err))
			return
		}
		defer rows.Close()
		for rows.Next() {
			var sc Overlay
			if err := rows.Scan(&sc.Kind, &sc.BlockHash, &sc.Payload, &sc.UpdatedAt); err != nil {
				yield(Overlay{}, fmt.Errorf("blockstore: scan overlay: %w", err))
				return
			}
			if !yield(sc, nil) {
				return
			}
		}
		if err := rows.Err(); err != nil {
			yield(Overlay{}, fmt.Errorf("blockstore: iterate overlays: %w", err))
		}
	}
}

func (s *cacheSession) Commit() error {
	if s.done {
		return ErrClosed
	}
	s.done = true
	return s.tx.Commit()
}

func (s *cacheSession) Rollback() error {
	if s.done {
		return nil
	}
	s.done = true
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
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
