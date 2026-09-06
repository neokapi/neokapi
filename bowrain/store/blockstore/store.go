package blockstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"iter"
	"sync"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	corestore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/kbf"
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
//
// On Postgres the returned Store also satisfies blockstore.TextSearcher
// (see pgStore); on SQLite a text search takes core/blockstore's scan.
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
	s := &store{opts: opts}
	if opts.Dialect == PostgresDialect {
		return &pgStore{store: s}, nil
	}
	return s, nil
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
		Persistent:   true,
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
	// rowIDs memoizes overlay-key → `blocks.id` resolutions. A block's id is
	// fixed for as long as the block exists, so one lookup per distinct key
	// serves a session that writes an overlay per block.
	rowMu  sync.Mutex
	rowIDs map[string]string
}

// dialect names this session's SQL flavour the way bowrain/store's overlay
// helpers spell it.
func (s *session) dialect() string {
	if s.opts.Dialect == SQLiteDialect {
		return "sqlite"
	}
	return "pg"
}

func (s *session) Capabilities() blockstore.Capabilities {
	return blockstore.Capabilities{
		RandomAccess: true,
		Concurrent:   true,
		Remote:       false,
		Writable:     true,
		Persistent:   true,
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
		// Collection filter: Bowrain collections group items; items own
		// blocks. Resolve `collection name` → item names in that collection,
		// then narrow the block query to those items. A nonexistent
		// collection yields no blocks (empty iteration, no error —
		// matches the interface contract's "empty filter = no results").
		if filter.Collection != "" {
			coll, err := s.opts.ContentStore.GetCollectionByName(s.ctx, s.opts.ProjectID, filter.Collection, s.opts.Stream)
			if err != nil || coll == nil {
				return
			}
			items, err := s.opts.ContentStore.ListItems(s.ctx, s.opts.ProjectID, s.opts.Stream)
			if err != nil {
				yield(nil, fmt.Errorf("bowrain/blockstore: list items for collection %q: %w", filter.Collection, err))
				return
			}
			var itemNames []string
			for _, it := range items {
				if it != nil && it.CollectionID == coll.ID {
					itemNames = append(itemNames, it.Name)
				}
			}
			if len(itemNames) == 0 {
				return
			}
			// Stream per item to keep memory bounded; ContentStore's
			// BlockQuery takes a single ItemName, so we iterate.
			for _, name := range itemNames {
				iq := q
				iq.ItemName = name
				rows, err := s.opts.ContentStore.GetBlocks(s.ctx, iq)
				if err != nil {
					yield(nil, fmt.Errorf("bowrain/blockstore: list blocks for item %q: %w", name, err))
					return
				}
				for _, sb := range rows {
					kb := toKBF(sb)
					if kb == nil {
						continue
					}
					if !yield(kb, nil) {
						return
					}
				}
			}
			return
		}
		rows, err := s.opts.ContentStore.GetBlocks(s.ctx, q)
		if err != nil {
			yield(nil, fmt.Errorf("bowrain/blockstore: list blocks: %w", err))
			return
		}
		for _, sb := range rows {
			kb := toKBF(sb)
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
	return toKBF(rows[0]), nil
}

func (s *session) PutBlock(collection string, b *blockstore.Block) error {
	if s.closed {
		return blockstore.ErrClosed
	}
	if b == nil {
		return errors.New("bowrain/blockstore: PutBlock: nil block")
	}
	mb := fromKBF(b)
	// When a collection is supplied, route the block through
	// StoreBlocksForItem so items-within-collection bookkeeping stays
	// consistent — the collection is created on first use. Empty
	// collection means "project-level write" and uses plain StoreBlocks.
	if collection == "" {
		return s.opts.ContentStore.StoreBlocks(s.ctx, s.opts.ProjectID, s.opts.Stream, []*model.Block{mb})
	}
	// Collection-scoped blocks live under a synthetic item named for
	// the collection, with the item linked to the collection via
	// CollectionID so subsequent filter.Collection lookups resolve.
	// Callers wanting richer item layouts should use the ContentStore
	// directly.
	collID, err := s.ensureCollection(collection)
	if err != nil {
		return err
	}
	if err := s.ensureCollectionItem(collection, collID); err != nil {
		return err
	}
	return s.opts.ContentStore.StoreBlocksForItem(s.ctx, s.opts.ProjectID, s.opts.Stream, collection, []*model.Block{mb})
}

// ensureCollection returns the ID of a collection with the given name,
// creating it first if it doesn't exist. Idempotent.
func (s *session) ensureCollection(name string) (string, error) {
	if c, err := s.opts.ContentStore.GetCollectionByName(s.ctx, s.opts.ProjectID, name, s.opts.Stream); err == nil && c != nil {
		return c.ID, nil
	}
	c := &platstore.Collection{
		ID:        name,
		ProjectID: s.opts.ProjectID,
		Name:      name,
		Kind:      "uploaded",
		Stream:    s.opts.Stream,
	}
	if err := s.opts.ContentStore.CreateCollection(s.ctx, c); err != nil {
		return "", fmt.Errorf("bowrain/blockstore: create collection %q: %w", name, err)
	}
	return c.ID, nil
}

// ensureCollectionItem makes sure an item with the given name exists
// and is linked to the supplied collection ID — so StoreBlocksForItem
// + filter.Collection queries round-trip.
func (s *session) ensureCollectionItem(itemName, collectionID string) error {
	existing, err := s.opts.ContentStore.GetItem(s.ctx, s.opts.ProjectID, s.opts.Stream, itemName)
	if err == nil && existing != nil {
		if existing.CollectionID == collectionID {
			return nil
		}
		existing.CollectionID = collectionID
		return s.opts.ContentStore.StoreItem(s.ctx, s.opts.ProjectID, s.opts.Stream, existing)
	}
	return s.opts.ContentStore.StoreItem(s.ctx, s.opts.ProjectID, s.opts.Stream, &platstore.Item{
		Name:         itemName,
		CollectionID: collectionID,
	})
}

func (s *session) GetOverlay(kind, blockHash string) (blockstore.Overlay, error) {
	if s.closed {
		return blockstore.Overlay{}, blockstore.ErrClosed
	}
	kind = blockstore.CanonicalOverlayKind(kind)
	switch routeKind(kind) {
	case tableTranslations:
		return s.getTranslation(kind, blockHash)
	case tableAnnotations:
		return s.getAnnotation(kind, blockHash)
	default:
		return s.getExt(kind, blockHash)
	}
}

func (s *session) PutOverlay(o blockstore.Overlay) error {
	if s.closed {
		return blockstore.ErrClosed
	}
	if o.Kind == "" || o.BlockHash == "" {
		return errors.New("bowrain/blockstore: PutOverlay: Kind and BlockHash are required")
	}
	// A `targets/<locale>` kind is filed under the canonical locale, the one
	// every reader asks for, whichever spelling the writer used.
	o.Kind = blockstore.CanonicalOverlayKind(o.Kind)
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
		return s.putAnnotation(o.Kind, o.BlockHash, payload, updatedAt)
	default:
		return s.putExt(o.Kind, o.BlockHash, payload, updatedAt)
	}
}

func (s *session) ListOverlays(kind string) iter.Seq2[blockstore.Overlay, error] {
	kind = blockstore.CanonicalOverlayKind(kind)
	return func(yield func(blockstore.Overlay, error) bool) {
		if s.closed {
			yield(blockstore.Overlay{}, blockstore.ErrClosed)
			return
		}
		switch routeKind(kind) {
		case tableTranslations:
			s.listTranslations(kind, yield)
		case tableAnnotations:
			s.listAnnotations(kind, yield)
		default:
			s.listExt(kind, yield)
		}
	}
}

// ─── table dispatchers ──────────────────────────────────────────
//
// Every dispatcher resolves the caller's key onto the block row it belongs to
// (see blockkey.go), so all three tables address a block the one way the rest
// of the store does. Block and item deletion clear overlays by `blocks.id`; a
// row filed under anything else outlives the block it describes.
//
// A `targets/<locale>` and an `annotations/<name>` overlay are further shared
// with the block round-trip, so each goes through bowrain/store's own writer and
// reader: one key, one spelling of the kind, one payload envelope, whichever
// door the write came through.
//
// The plugin catchall is the one place a caller's own vocabulary survives
// intact. Its kind is opaque — it names no block field and no annotation key —
// and its payload is the tool's bytes verbatim, because nothing but this adapter
// reads `overlays_ext`. That is a deliberate narrowing, not the divergence
// above: the row is still filed under the block's id, so it dies with its block.

func (s *session) getTranslation(kind, blockHash string) (blockstore.Overlay, error) {
	_, locale := splitKindOnce(kind)
	rowID, err := s.resolveBlockRow(blockHash)
	if err != nil {
		return blockstore.Overlay{}, err
	}
	stored, err := corestore.LoadBlockVariantTarget(s.ctx, s.opts.DB, s.dialect(),
		s.opts.ProjectID, s.opts.Stream, rowID, locale)
	if err != nil {
		return blockstore.Overlay{}, fmt.Errorf("bowrain/blockstore: get translation: %w", err)
	}
	if stored == nil {
		return blockstore.Overlay{}, blockstore.ErrNotFound
	}
	payload, err := encodeTargetPayload(stored.Target, stored.Extra)
	if err != nil {
		return blockstore.Overlay{}, fmt.Errorf("bowrain/blockstore: encode translation payload: %w", err)
	}
	return blockstore.Overlay{
		Kind:      kind,
		BlockHash: blockHash,
		Payload:   payload,
		UpdatedAt: stored.UpdatedAt,
	}, nil
}

func (s *session) putTranslation(kind, blockHash string, payload []byte, updatedAt time.Time) error {
	_, locale := splitKindOnce(kind)
	if locale == "" {
		return fmt.Errorf("bowrain/blockstore: put translation: kind %q missing locale", kind)
	}
	rowID, err := s.resolveBlockRow(blockHash)
	if err != nil {
		return err
	}
	target, extra, err := decodeTargetPayload(payload)
	if err != nil {
		return fmt.Errorf("bowrain/blockstore: put translation: %w", err)
	}
	var variant model.VariantKey
	if err := variant.UnmarshalText([]byte(locale)); err != nil {
		return fmt.Errorf("bowrain/blockstore: put translation: decode variant %q: %w", locale, err)
	}
	if err := corestore.UpsertBlockTarget(s.ctx, s.opts.DB, s.dialect(),
		s.opts.ProjectID, s.opts.Stream, rowID, variant, target, extra, updatedAt); err != nil {
		return fmt.Errorf("bowrain/blockstore: put translation: %w", err)
	}
	return nil
}

func (s *session) listTranslations(kind string, yield func(blockstore.Overlay, error) bool) {
	_, locale := splitKindOnce(kind)
	stored, err := corestore.LoadVariantTargets(s.ctx, s.opts.DB, s.dialect(),
		s.opts.ProjectID, s.opts.Stream, locale)
	if err != nil {
		yield(blockstore.Overlay{}, fmt.Errorf("bowrain/blockstore: list translations: %w", err))
		return
	}
	if len(stored) == 0 {
		return
	}
	rowIDs := make([]string, 0, len(stored))
	for _, st := range stored {
		rowIDs = append(rowIDs, st.BlockID)
	}
	keys, err := s.blockKeysByRow(rowIDs)
	if err != nil {
		yield(blockstore.Overlay{}, err)
		return
	}
	for _, st := range stored {
		payload, err := encodeTargetPayload(st.Target, st.Extra)
		if err != nil {
			yield(blockstore.Overlay{}, fmt.Errorf("bowrain/blockstore: encode translation payload: %w", err))
			return
		}
		if !yield(blockstore.Overlay{
			Kind:      kind,
			BlockHash: blockKeyOr(keys, st.BlockID),
			Payload:   payload,
			UpdatedAt: st.UpdatedAt,
		}, nil) {
			return
		}
	}
}

func (s *session) getAnnotation(kind, blockHash string) (blockstore.Overlay, error) {
	key, err := annotationKey(kind)
	if err != nil {
		return blockstore.Overlay{}, err
	}
	rowID, err := s.resolveBlockRow(blockHash)
	if err != nil {
		return blockstore.Overlay{}, err
	}
	stored, err := corestore.LoadBlockAnnotation(s.ctx, s.opts.DB, s.dialect(),
		s.opts.ProjectID, s.opts.Stream, rowID, key)
	if err != nil {
		return blockstore.Overlay{}, fmt.Errorf("bowrain/blockstore: get annotation: %w", err)
	}
	if stored == nil {
		return blockstore.Overlay{}, blockstore.ErrNotFound
	}
	payload, err := encodeAnnotationPayload(stored.Value)
	if err != nil {
		return blockstore.Overlay{}, fmt.Errorf("bowrain/blockstore: encode annotation payload: %w", err)
	}
	return blockstore.Overlay{
		Kind:      kind,
		BlockHash: blockHash,
		Payload:   payload,
		UpdatedAt: stored.UpdatedAt,
	}, nil
}

func (s *session) putAnnotation(kind, blockHash string, payload []byte, updatedAt time.Time) error {
	key, err := annotationKey(kind)
	if err != nil {
		return err
	}
	rowID, err := s.resolveBlockRow(blockHash)
	if err != nil {
		return err
	}
	if err := corestore.UpsertBlockAnnotation(s.ctx, s.opts.DB, s.dialect(),
		s.opts.ProjectID, s.opts.Stream, rowID, key, model.DecodePayload(key, payload), updatedAt); err != nil {
		return fmt.Errorf("bowrain/blockstore: put annotation: %w", err)
	}
	return nil
}

func (s *session) listAnnotations(kind string, yield func(blockstore.Overlay, error) bool) {
	key, err := annotationKey(kind)
	if err != nil {
		yield(blockstore.Overlay{}, err)
		return
	}
	stored, err := corestore.LoadAnnotationsByKey(s.ctx, s.opts.DB, s.dialect(),
		s.opts.ProjectID, s.opts.Stream, key)
	if err != nil {
		yield(blockstore.Overlay{}, fmt.Errorf("bowrain/blockstore: list annotations: %w", err))
		return
	}
	if len(stored) == 0 {
		return
	}
	rowIDs := make([]string, 0, len(stored))
	for _, sa := range stored {
		rowIDs = append(rowIDs, sa.BlockID)
	}
	keys, err := s.blockKeysByRow(rowIDs)
	if err != nil {
		yield(blockstore.Overlay{}, err)
		return
	}
	for _, sa := range stored {
		payload, err := encodeAnnotationPayload(sa.Value)
		if err != nil {
			yield(blockstore.Overlay{}, fmt.Errorf("bowrain/blockstore: encode annotation payload: %w", err))
			return
		}
		if !yield(blockstore.Overlay{
			Kind:      kind,
			BlockHash: blockKeyOr(keys, sa.BlockID),
			Payload:   payload,
			UpdatedAt: sa.UpdatedAt,
		}, nil) {
			return
		}
	}
}

func (s *session) getExt(kind, blockHash string) (blockstore.Overlay, error) {
	rowID, err := s.resolveBlockRow(blockHash)
	if err != nil {
		return blockstore.Overlay{}, err
	}
	var (
		payload      []byte
		updatedAtStr string
	)
	row := s.opts.DB.QueryRowContext(s.ctx, s.sqlSelectExt(),
		s.opts.ProjectID, s.opts.Stream, rowID, kind,
	)
	if err := row.Scan(&payload, &updatedAtStr); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return blockstore.Overlay{}, blockstore.ErrNotFound
		}
		return blockstore.Overlay{}, fmt.Errorf("bowrain/blockstore: get overlays_ext: %w", err)
	}
	return blockstore.Overlay{
		Kind:      kind,
		BlockHash: blockHash,
		Payload:   payload,
		UpdatedAt: corestore.ParseOverlayTimestamp(updatedAtStr),
	}, nil
}

func (s *session) putExt(kind, blockHash string, payload []byte, updatedAt time.Time) error {
	rowID, err := s.resolveBlockRow(blockHash)
	if err != nil {
		return err
	}
	if _, err := s.opts.DB.ExecContext(s.ctx, s.sqlUpsertExt(),
		s.opts.ProjectID, s.opts.Stream, rowID, kind, payload, updatedAt,
	); err != nil {
		return fmt.Errorf("bowrain/blockstore: put overlays_ext: %w", err)
	}
	return nil
}

func (s *session) listExt(kind string, yield func(blockstore.Overlay, error) bool) {
	rows, err := s.opts.DB.QueryContext(s.ctx, s.sqlListExt(),
		s.opts.ProjectID, s.opts.Stream, kind,
	)
	if err != nil {
		yield(blockstore.Overlay{}, fmt.Errorf("bowrain/blockstore: list overlays_ext: %w", err))
		return
	}
	type row struct {
		blockID   string
		payload   []byte
		updatedAt int64
	}
	var collected []row
	for rows.Next() {
		var (
			blockID      string
			payload      []byte
			updatedAtStr string
		)
		if err := rows.Scan(&blockID, &payload, &updatedAtStr); err != nil {
			rows.Close()
			yield(blockstore.Overlay{}, fmt.Errorf("bowrain/blockstore: list overlays_ext scan: %w", err))
			return
		}
		collected = append(collected, row{blockID, payload, corestore.ParseOverlayTimestamp(updatedAtStr)})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		yield(blockstore.Overlay{}, fmt.Errorf("bowrain/blockstore: list overlays_ext rows: %w", err))
		return
	}
	if len(collected) == 0 {
		return
	}
	rowIDs := make([]string, 0, len(collected))
	for _, r := range collected {
		rowIDs = append(rowIDs, r.blockID)
	}
	keys, err := s.blockKeysByRow(rowIDs)
	if err != nil {
		yield(blockstore.Overlay{}, err)
		return
	}
	for _, r := range collected {
		if !yield(blockstore.Overlay{
			Kind:      kind,
			BlockHash: blockKeyOr(keys, r.blockID),
			Payload:   r.payload,
			UpdatedAt: r.updatedAt,
		}, nil) {
			return
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

// The plugin catchall is the only table this adapter still writes directly:
// targets and annotations go through bowrain/store's own statements, so the
// block round-trip and this adapter cannot spell a row two ways.

func (s *session) sqlSelectExt() string {
	switch s.opts.Dialect {
	case SQLiteDialect:
		return `SELECT payload, updated_at FROM overlays_ext
			WHERE project_id = ? AND stream = ? AND block_id = ? AND kind = ?`
	default:
		return `SELECT payload, updated_at FROM overlays_ext
			WHERE project_id = $1 AND stream = $2 AND block_id = $3 AND kind = $4`
	}
}

func (s *session) sqlListExt() string {
	switch s.opts.Dialect {
	case SQLiteDialect:
		return `SELECT block_id, payload, updated_at FROM overlays_ext
			WHERE project_id = ? AND stream = ? AND kind = ? ORDER BY block_id`
	default:
		return `SELECT block_id, payload, updated_at FROM overlays_ext
			WHERE project_id = $1 AND stream = $2 AND kind = $3 ORDER BY block_id`
	}
}

func (s *session) sqlUpsertExt() string {
	switch s.opts.Dialect {
	case SQLiteDialect:
		return `INSERT INTO overlays_ext (project_id, stream, block_id, kind, payload, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(project_id, stream, block_id, kind) DO UPDATE SET
				payload = excluded.payload,
				updated_at = excluded.updated_at`
	default:
		return `INSERT INTO overlays_ext (project_id, stream, block_id, kind, payload, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (project_id, stream, block_id, kind) DO UPDATE SET
				payload = EXCLUDED.payload,
				updated_at = EXCLUDED.updated_at`
	}
}

// Unused import guard — kbf is referenced via the aliased Block in
// convert.go; Go's unused-import rule for this file is satisfied.
var _ = kbf.SchemaVersion
