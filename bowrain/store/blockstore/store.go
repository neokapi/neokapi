package blockstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"iter"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/klf"
	"github.com/neokapi/neokapi/core/model"
)

// DB is the subset of *sql.DB the overlay table calls depend on.
// Both the Postgres and SQLite Bowrain stores expose a method that
// returns this surface, so we accept either.
type DB interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Dialect switches between Postgres ($1, $2) and SQLite (?) placeholders.
type Dialect int

const (
	PostgresDialect Dialect = iota
	SQLiteDialect
)

// Options scopes a Store to one project and stream.
type Options struct {
	// ContentStore is the backing Bowrain content store. Used for
	// block reads and writes (the richer model.Block path).
	ContentStore platstore.ContentStore
	// DB is the underlying *sql.DB for overlay table access. Must
	// point at the same database as ContentStore. Supply Postgres or
	// SQLite; both are supported.
	DB DB
	// Dialect selects the SQL placeholder style.
	Dialect Dialect
	// ProjectID scopes the Store to one project.
	ProjectID string
	// Stream scopes the Store to one stream. Empty defaults to "main".
	Stream string
	// DefaultCollection is the collection written to by PutBlock when
	// the caller doesn't supply one. Empty means default-for-project
	// (resolved at Begin).
	DefaultCollection string
}

// New returns a blockstore.Store that adapts the supplied Bowrain
// content store to the core/blockstore interface. Overlays are
// persisted in the `block_overlays` table defined by migration
// #2 (Postgres) / #34 (SQLite).
func New(opts Options) (blockstore.Store, error) {
	if opts.ContentStore == nil {
		return nil, errors.New("bowrain/blockstore: ContentStore is required")
	}
	if opts.DB == nil {
		return nil, errors.New("bowrain/blockstore: DB is required")
	}
	if opts.ProjectID == "" {
		return nil, errors.New("bowrain/blockstore: ProjectID is required")
	}
	if opts.Stream == "" {
		opts.Stream = "main"
	}
	return &store{opts: opts}, nil
}

type store struct {
	opts Options
}

func (s *store) Capabilities() blockstore.Capabilities {
	return blockstore.Capabilities{
		RandomAccess: true,
		Concurrent:   true,
		Remote:       false,
		Writable:     true,
	}
}

func (s *store) Begin(ctx context.Context) (blockstore.Session, error) {
	coll := s.opts.DefaultCollection
	if coll == "" {
		c, err := s.opts.ContentStore.GetDefaultCollection(ctx, s.opts.ProjectID)
		if err == nil && c != nil {
			coll = c.Name
		}
	}
	return &session{
		ctx:        ctx,
		opts:       s.opts,
		collection: coll,
	}, nil
}

func (s *store) Close() error { return nil }

// ─── Session ────────────────────────────────────────────────────

type session struct {
	ctx        context.Context
	opts       Options
	collection string
	closed     bool
}

func (s *session) Capabilities() blockstore.Capabilities {
	return blockstore.Capabilities{
		RandomAccess: true,
		Concurrent:   true,
		Remote:       false,
		Writable:     true,
	}
}

func (s *session) Blocks(filter blockstore.BlockFilter) iter.Seq2[*blockstore.Block, error] {
	return func(yield func(*blockstore.Block, error) bool) {
		if s.closed {
			yield(nil, blockstore.ErrClosed)
			return
		}
		q := platstore.BlockQuery{
			ProjectID: s.opts.ProjectID,
			Stream:    s.opts.Stream,
			Limit:     filter.Limit,
		}
		if filter.Translatable != nil {
			t := *filter.Translatable
			q.Translatable = &t
		}
		// Collection filter: Bowrain collections map to item groupings
		// at the ContentStore level. The simplest correct mapping is
		// to filter items by collection name, then fetch blocks per
		// item. Until the collection filter is pushed into BlockQuery,
		// we just return all blocks when a collection is requested —
		// matching current ContentStore semantics.
		rows, err := s.opts.ContentStore.GetBlocks(s.ctx, q)
		if err != nil {
			yield(nil, fmt.Errorf("bowrain/blockstore: list blocks: %w", err))
			return
		}
		for _, sb := range rows {
			kb := toKLF(sb)
			if kb == nil {
				continue
			}
			if !yield(kb, nil) {
				return
			}
		}
	}
}

func (s *session) GetBlock(hash string) (*blockstore.Block, error) {
	if s.closed {
		return nil, blockstore.ErrClosed
	}
	if hash == "" {
		return nil, blockstore.ErrNotFound
	}
	// Blocks are currently looked up by ID in the ContentStore.
	// blockstore.Session callers pass the Block.Hash as the lookup
	// key (same value StoreBlocks computes on write). We resolve via
	// ContentHash filter + take the first match.
	rows, err := s.opts.ContentStore.GetBlocks(s.ctx, platstore.BlockQuery{
		ProjectID:   s.opts.ProjectID,
		Stream:      s.opts.Stream,
		ContentHash: hash,
		Limit:       1,
	})
	if err != nil {
		return nil, fmt.Errorf("bowrain/blockstore: get block: %w", err)
	}
	if len(rows) == 0 {
		return nil, blockstore.ErrNotFound
	}
	return toKLF(rows[0]), nil
}

func (s *session) PutBlock(collection string, b *blockstore.Block) error {
	if s.closed {
		return blockstore.ErrClosed
	}
	if b == nil {
		return errors.New("bowrain/blockstore: PutBlock: nil block")
	}
	_ = collection // reserved for future use once collections are first-class in BlockQuery
	mb := fromKLF(b)
	return s.opts.ContentStore.StoreBlocks(s.ctx, s.opts.ProjectID, s.opts.Stream, []*model.Block{mb})
}

func (s *session) GetOverlay(kind, blockHash string) (blockstore.Overlay, error) {
	if s.closed {
		return blockstore.Overlay{}, blockstore.ErrClosed
	}
	var (
		payload      []byte
		updatedAtStr string
	)
	row := s.opts.DB.QueryRowContext(s.ctx, s.selectOverlay(),
		s.opts.ProjectID, s.opts.Stream, blockHash, kind,
	)
	if err := row.Scan(&payload, &updatedAtStr); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return blockstore.Overlay{}, blockstore.ErrNotFound
		}
		return blockstore.Overlay{}, fmt.Errorf("bowrain/blockstore: get overlay: %w", err)
	}
	return blockstore.Overlay{
		Kind:      kind,
		BlockHash: blockHash,
		Payload:   payload,
		UpdatedAt: parseTimestamp(updatedAtStr),
	}, nil
}

func (s *session) PutOverlay(o blockstore.Overlay) error {
	if s.closed {
		return blockstore.ErrClosed
	}
	if o.Kind == "" || o.BlockHash == "" {
		return errors.New("bowrain/blockstore: PutOverlay: Kind and BlockHash are required")
	}
	updatedAt := time.Unix(o.UpdatedAt, 0).UTC()
	if o.UpdatedAt == 0 {
		updatedAt = time.Now().UTC()
	}
	payload := o.Payload
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	_, err := s.opts.DB.ExecContext(s.ctx, s.upsertOverlay(),
		s.opts.ProjectID, s.opts.Stream, o.BlockHash, o.Kind, payload, updatedAt,
	)
	if err != nil {
		return fmt.Errorf("bowrain/blockstore: put overlay: %w", err)
	}
	return nil
}

func (s *session) ListOverlays(kind string) iter.Seq2[blockstore.Overlay, error] {
	return func(yield func(blockstore.Overlay, error) bool) {
		if s.closed {
			yield(blockstore.Overlay{}, blockstore.ErrClosed)
			return
		}
		rows, err := s.opts.DB.QueryContext(s.ctx, s.listOverlays(),
			s.opts.ProjectID, s.opts.Stream, kind,
		)
		if err != nil {
			yield(blockstore.Overlay{}, fmt.Errorf("bowrain/blockstore: list overlays: %w", err))
			return
		}
		defer rows.Close()
		for rows.Next() {
			var (
				blockID      string
				payload      []byte
				updatedAtStr string
			)
			if err := rows.Scan(&blockID, &payload, &updatedAtStr); err != nil {
				yield(blockstore.Overlay{}, fmt.Errorf("bowrain/blockstore: list overlays scan: %w", err))
				return
			}
			if !yield(blockstore.Overlay{
				Kind:      kind,
				BlockHash: blockID,
				Payload:   payload,
				UpdatedAt: parseTimestamp(updatedAtStr),
			}, nil) {
				return
			}
		}
		if err := rows.Err(); err != nil {
			yield(blockstore.Overlay{}, fmt.Errorf("bowrain/blockstore: list overlays rows: %w", err))
		}
	}
}

func (s *session) Commit() error {
	s.closed = true
	return nil
}

func (s *session) Rollback() error {
	s.closed = true
	return nil
}

func (s *session) Close() error {
	s.closed = true
	return nil
}

// ─── SQL helpers ────────────────────────────────────────────────

func (s *session) selectOverlay() string {
	switch s.opts.Dialect {
	case SQLiteDialect:
		return `SELECT payload, updated_at FROM block_overlays
			WHERE project_id = ? AND stream = ? AND block_id = ? AND kind = ?`
	default:
		return `SELECT payload, updated_at FROM block_overlays
			WHERE project_id = $1 AND stream = $2 AND block_id = $3 AND kind = $4`
	}
}

func (s *session) listOverlays() string {
	switch s.opts.Dialect {
	case SQLiteDialect:
		return `SELECT block_id, payload, updated_at FROM block_overlays
			WHERE project_id = ? AND stream = ? AND kind = ? ORDER BY block_id`
	default:
		return `SELECT block_id, payload, updated_at FROM block_overlays
			WHERE project_id = $1 AND stream = $2 AND kind = $3 ORDER BY block_id`
	}
}

func (s *session) upsertOverlay() string {
	switch s.opts.Dialect {
	case SQLiteDialect:
		return `INSERT INTO block_overlays (project_id, stream, block_id, kind, payload, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(project_id, stream, block_id, kind) DO UPDATE SET
				payload = excluded.payload,
				updated_at = excluded.updated_at`
	default:
		return `INSERT INTO block_overlays (project_id, stream, block_id, kind, payload, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (project_id, stream, block_id, kind) DO UPDATE SET
				payload = EXCLUDED.payload,
				updated_at = EXCLUDED.updated_at`
	}
}

// parseTimestamp handles both the SQLite `datetime('now')` TEXT format
// ("2006-01-02 15:04:05") and the Postgres TIMESTAMPTZ RFC 3339 form
// that pgx returns as a string over the database/sql surface. Returns
// the Unix-seconds value or 0 if the column was empty/unparseable.
func parseTimestamp(s string) int64 {
	if s == "" {
		return 0
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Unix()
		}
	}
	return 0
}

// Unused import guard — klf is referenced via the aliased Block in
// convert.go; Go's unused-import rule for this file is satisfied.
var _ = klf.SchemaVersion
