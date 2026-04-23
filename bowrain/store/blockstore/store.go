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
	switch routeKind(kind) {
	case tableTranslations:
		return s.getTranslation(kind, blockHash)
	case tableAnnotations:
		return s.getExtOrAnnotation(kind, blockHash, "annotations")
	default:
		return s.getExtOrAnnotation(kind, blockHash, "overlays_ext")
	}
}

func (s *session) PutOverlay(o blockstore.Overlay) error {
	if s.closed {
		return blockstore.ErrClosed
	}
	if o.Kind == "" || o.BlockHash == "" {
		return errors.New("bowrain/blockstore: PutOverlay: Kind and BlockHash are required")
	}
	payload := o.Payload
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	updatedAt := time.Unix(o.UpdatedAt, 0).UTC()
	if o.UpdatedAt == 0 {
		updatedAt = time.Now().UTC()
	}
	switch routeKind(o.Kind) {
	case tableTranslations:
		return s.putTranslation(o.Kind, o.BlockHash, payload, updatedAt)
	case tableAnnotations:
		return s.putExtOrAnnotation("annotations", o.Kind, o.BlockHash, payload, updatedAt)
	default:
		return s.putExtOrAnnotation("overlays_ext", o.Kind, o.BlockHash, payload, updatedAt)
	}
}

func (s *session) ListOverlays(kind string) iter.Seq2[blockstore.Overlay, error] {
	return func(yield func(blockstore.Overlay, error) bool) {
		if s.closed {
			yield(blockstore.Overlay{}, blockstore.ErrClosed)
			return
		}
		switch routeKind(kind) {
		case tableTranslations:
			s.listTranslations(kind, yield)
		case tableAnnotations:
			s.listExtOrAnnotation(kind, "annotations", yield)
		default:
			s.listExtOrAnnotation(kind, "overlays_ext", yield)
		}
	}
}

// ─── table dispatchers ──────────────────────────────────────────

func (s *session) getTranslation(kind, blockHash string) (blockstore.Overlay, error) {
	_, locale := splitKindOnce(kind)
	var (
		text, provider string
		metadata       []byte
		updatedAtStr   string
	)
	row := s.opts.DB.QueryRowContext(s.ctx, s.sqlSelectTranslation(),
		s.opts.ProjectID, s.opts.Stream, blockHash, locale,
	)
	if err := row.Scan(&text, &provider, &metadata, &updatedAtStr); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return blockstore.Overlay{}, blockstore.ErrNotFound
		}
		return blockstore.Overlay{}, fmt.Errorf("bowrain/blockstore: get translation: %w", err)
	}
	payload, err := encodeTranslationPayload(text, provider, metadata)
	if err != nil {
		return blockstore.Overlay{}, fmt.Errorf("bowrain/blockstore: encode translation payload: %w", err)
	}
	return blockstore.Overlay{
		Kind:      kind,
		BlockHash: blockHash,
		Payload:   payload,
		UpdatedAt: parseTimestamp(updatedAtStr),
	}, nil
}

func (s *session) putTranslation(kind, blockHash string, payload []byte, updatedAt time.Time) error {
	_, locale := splitKindOnce(kind)
	if locale == "" {
		return fmt.Errorf("bowrain/blockstore: put translation: kind %q missing locale", kind)
	}
	text, provider, metadata := decodeTranslationPayload(payload)
	if len(metadata) == 0 {
		metadata = []byte("{}")
	}
	_, err := s.opts.DB.ExecContext(s.ctx, s.sqlUpsertTranslation(),
		s.opts.ProjectID, s.opts.Stream, blockHash, locale, text, provider, metadata, updatedAt,
	)
	if err != nil {
		return fmt.Errorf("bowrain/blockstore: put translation: %w", err)
	}
	return nil
}

func (s *session) listTranslations(kind string, yield func(blockstore.Overlay, error) bool) {
	_, locale := splitKindOnce(kind)
	rows, err := s.opts.DB.QueryContext(s.ctx, s.sqlListTranslations(),
		s.opts.ProjectID, s.opts.Stream, locale,
	)
	if err != nil {
		yield(blockstore.Overlay{}, fmt.Errorf("bowrain/blockstore: list translations: %w", err))
		return
	}
	defer rows.Close()
	for rows.Next() {
		var (
			blockID, text, provider string
			metadata                []byte
			updatedAtStr            string
		)
		if err := rows.Scan(&blockID, &text, &provider, &metadata, &updatedAtStr); err != nil {
			yield(blockstore.Overlay{}, fmt.Errorf("bowrain/blockstore: list translations scan: %w", err))
			return
		}
		payload, err := encodeTranslationPayload(text, provider, metadata)
		if err != nil {
			yield(blockstore.Overlay{}, fmt.Errorf("bowrain/blockstore: encode translation payload: %w", err))
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
		yield(blockstore.Overlay{}, fmt.Errorf("bowrain/blockstore: list translations rows: %w", err))
	}
}

func (s *session) getExtOrAnnotation(kind, blockHash, table string) (blockstore.Overlay, error) {
	var (
		payload      []byte
		updatedAtStr string
	)
	row := s.opts.DB.QueryRowContext(s.ctx, s.sqlSelectExtOrAnnotation(table),
		s.opts.ProjectID, s.opts.Stream, blockHash, kind,
	)
	if err := row.Scan(&payload, &updatedAtStr); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return blockstore.Overlay{}, blockstore.ErrNotFound
		}
		return blockstore.Overlay{}, fmt.Errorf("bowrain/blockstore: get %s: %w", table, err)
	}
	return blockstore.Overlay{
		Kind:      kind,
		BlockHash: blockHash,
		Payload:   payload,
		UpdatedAt: parseTimestamp(updatedAtStr),
	}, nil
}

func (s *session) putExtOrAnnotation(table, kind, blockHash string, payload []byte, updatedAt time.Time) error {
	_, err := s.opts.DB.ExecContext(s.ctx, s.sqlUpsertExtOrAnnotation(table),
		s.opts.ProjectID, s.opts.Stream, blockHash, kind, payload, updatedAt,
	)
	if err != nil {
		return fmt.Errorf("bowrain/blockstore: put %s: %w", table, err)
	}
	return nil
}

func (s *session) listExtOrAnnotation(kind, table string, yield func(blockstore.Overlay, error) bool) {
	rows, err := s.opts.DB.QueryContext(s.ctx, s.sqlListExtOrAnnotation(table),
		s.opts.ProjectID, s.opts.Stream, kind,
	)
	if err != nil {
		yield(blockstore.Overlay{}, fmt.Errorf("bowrain/blockstore: list %s: %w", table, err))
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
			yield(blockstore.Overlay{}, fmt.Errorf("bowrain/blockstore: list %s scan: %w", table, err))
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
		yield(blockstore.Overlay{}, fmt.Errorf("bowrain/blockstore: list %s rows: %w", table, err))
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

func (s *session) sqlSelectTranslation() string {
	switch s.opts.Dialect {
	case SQLiteDialect:
		return `SELECT text, provider, metadata, updated_at FROM translations
			WHERE project_id = ? AND stream = ? AND block_id = ? AND locale = ?`
	default:
		return `SELECT text, provider, metadata, updated_at FROM translations
			WHERE project_id = $1 AND stream = $2 AND block_id = $3 AND locale = $4`
	}
}

func (s *session) sqlListTranslations() string {
	switch s.opts.Dialect {
	case SQLiteDialect:
		return `SELECT block_id, text, provider, metadata, updated_at FROM translations
			WHERE project_id = ? AND stream = ? AND locale = ? ORDER BY block_id`
	default:
		return `SELECT block_id, text, provider, metadata, updated_at FROM translations
			WHERE project_id = $1 AND stream = $2 AND locale = $3 ORDER BY block_id`
	}
}

func (s *session) sqlUpsertTranslation() string {
	switch s.opts.Dialect {
	case SQLiteDialect:
		return `INSERT INTO translations (project_id, stream, block_id, locale, text, provider, metadata, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(project_id, stream, block_id, locale) DO UPDATE SET
				text = excluded.text,
				provider = excluded.provider,
				metadata = excluded.metadata,
				updated_at = excluded.updated_at`
	default:
		return `INSERT INTO translations (project_id, stream, block_id, locale, text, provider, metadata, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (project_id, stream, block_id, locale) DO UPDATE SET
				text = EXCLUDED.text,
				provider = EXCLUDED.provider,
				metadata = EXCLUDED.metadata,
				updated_at = EXCLUDED.updated_at`
	}
}

// Annotations + overlays_ext share a SQL shape — one set of helpers
// with the table name interpolated keeps them in lockstep.

func (s *session) sqlSelectExtOrAnnotation(table string) string {
	switch s.opts.Dialect {
	case SQLiteDialect:
		return `SELECT payload, updated_at FROM ` + table + `
			WHERE project_id = ? AND stream = ? AND block_id = ? AND kind = ?`
	default:
		return `SELECT payload, updated_at FROM ` + table + `
			WHERE project_id = $1 AND stream = $2 AND block_id = $3 AND kind = $4`
	}
}

func (s *session) sqlListExtOrAnnotation(table string) string {
	switch s.opts.Dialect {
	case SQLiteDialect:
		return `SELECT block_id, payload, updated_at FROM ` + table + `
			WHERE project_id = ? AND stream = ? AND kind = ? ORDER BY block_id`
	default:
		return `SELECT block_id, payload, updated_at FROM ` + table + `
			WHERE project_id = $1 AND stream = $2 AND kind = $3 ORDER BY block_id`
	}
}

func (s *session) sqlUpsertExtOrAnnotation(table string) string {
	switch s.opts.Dialect {
	case SQLiteDialect:
		return `INSERT INTO ` + table + ` (project_id, stream, block_id, kind, payload, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(project_id, stream, block_id, kind) DO UPDATE SET
				payload = excluded.payload,
				updated_at = excluded.updated_at`
	default:
		return `INSERT INTO ` + table + ` (project_id, stream, block_id, kind, payload, updated_at)
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
