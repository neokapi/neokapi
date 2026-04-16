//go:build klzcache

package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/storage"
)

// Open returns a Cache for the archive whose manifest hashes to
// `manifestHash`. If an existing cache entry is found on disk and
// its schema version matches CacheSchemaVersion, it is reopened
// directly. Otherwise the caller must call Build before running
// queries.
func Open(ctx context.Context, manifestHash string) (Cache, error) {
	dir := EntryDir(manifestHash)
	dbPath := filepath.Join(dir, "db.sqlite")
	if _, err := os.Stat(dbPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &missingCache{hash: manifestHash}, nil
		}
		return nil, fmt.Errorf("klz cache: stat: %w", err)
	}
	h, err := storage.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("klz cache: open %s: %w", dbPath, err)
	}
	c := &sqliteCache{db: h, hash: manifestHash, dir: dir}
	if err := c.checkSchemaVersion(ctx); err != nil {
		_ = h.Close()
		// Schema mismatch → drop the stale entry so the next Build
		// produces a fresh one per RFC 0001 §Schema evolution.
		_ = os.RemoveAll(dir)
		return &missingCache{hash: manifestHash}, nil
	}
	return c, nil
}

// Build populates a cache entry from the archive's sources/targets/
// documents. The caller supplies an iterator — the db package does
// not depend on core/klf or core/klz to avoid import cycles — and
// the build writes everything into a temp dir then atomically
// renames into place so concurrent builds don't corrupt each other.
func Build(ctx context.Context, manifestHash string, src Source) error {
	dir := EntryDir(manifestHash)
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return fmt.Errorf("klz cache: mkdir parent: %w", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "db.sqlite")); err == nil {
		// Already built by some other process; nothing to do.
		return nil
	}

	// Build under a unique temp dir, rename into place atomically.
	tmp, err := os.MkdirTemp(filepath.Dir(dir), "build-*")
	if err != nil {
		return fmt.Errorf("klz cache: mkdir temp: %w", err)
	}
	// On error, clean up the temp dir.
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(tmp)
		}
	}()

	dbPath := filepath.Join(tmp, "db.sqlite")
	h, err := storage.Open(dbPath)
	if err != nil {
		return fmt.Errorf("klz cache: open %s: %w", dbPath, err)
	}
	if _, err := h.ExecContext(ctx, Schema); err != nil {
		_ = h.Close()
		return fmt.Errorf("klz cache: apply schema: %w", err)
	}
	c := &sqliteCache{db: h, hash: manifestHash, dir: tmp}
	if err := c.ingest(ctx, src); err != nil {
		_ = h.Close()
		return fmt.Errorf("klz cache: ingest: %w", err)
	}
	if err := c.writeMeta(ctx, manifestHash); err != nil {
		_ = h.Close()
		return fmt.Errorf("klz cache: write meta: %w", err)
	}
	if err := h.Close(); err != nil {
		return fmt.Errorf("klz cache: close db: %w", err)
	}
	// Write a pointer file so debuggers can trace back to the source.
	_ = os.WriteFile(filepath.Join(tmp, "built.at"), []byte(time.Now().UTC().Format(time.RFC3339)), 0o644)

	// Atomic rename. If another process won the race, remove our temp
	// and return the winner.
	if err := os.Rename(tmp, dir); err != nil {
		if _, statErr := os.Stat(filepath.Join(dir, "db.sqlite")); statErr == nil {
			// Someone else won; our work is superseded.
			return nil
		}
		return fmt.Errorf("klz cache: rename %s → %s: %w", tmp, dir, err)
	}
	committed = true
	return nil
}

// Source is the interface core/klz's Reader implements so the db
// package can populate a cache entry without importing core/klz
// (would create an import cycle with internal/).
type Source interface {
	// Blocks yields every Block in the archive with metadata the
	// cache schema needs. Entries are emitted in document order.
	Blocks() ([]BlockRow, error)
	// Targets yields one row per (block, locale) pair carrying the
	// target Run[] JSON and its status/origin.
	Targets() ([]TargetRow, error)
	// SourceLocale is the archive's source locale.
	SourceLocale() string
}

// BlockRow carries the per-block metadata the cache indexes. The
// SourceJSON field is the Block.source Run[] marshaled to JSON
// (matches the `sources.source_runs` column).
type BlockRow struct {
	ID                   string
	DocumentPath         string
	Hash                 string
	Type                 string
	Component            string
	JSXPath              string
	OptionalPlaceholders int
	RequiredPlaceholders int
	SourceText           string
	SourceJSON           string
	Context              string
}

// TargetRow carries one (block, locale) target overlay.
type TargetRow struct {
	BlockID      string
	Locale       string
	TargetJSON   string
	Status       string
	Origin       string
	OriginDetail string
}

// ───────── sqliteCache ─────────

type sqliteCache struct {
	db   *storage.DB
	hash string
	dir  string
}

// Close releases the database handle.
func (c *sqliteCache) Close() error { return c.db.Close() }

// BlockByID returns the Block.source Run[] JSON for the given block
// id, or a nil slice when not found.
func (c *sqliteCache) BlockByID(ctx context.Context, id string) ([]byte, error) {
	var out string
	err := c.db.QueryRowContext(ctx,
		`SELECT s.source_runs FROM blocks b
		 JOIN sources s ON s.source_hash = b.hash
		 WHERE b.id = ?
		 LIMIT 1`, id).Scan(&out)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("klz cache: block-by-id: %w", err)
	}
	return []byte(out), nil
}

// SimilarSources returns up to `limit` block ids whose source text
// is most similar to query, ranked by FTS5 bm25().
func (c *sqliteCache) SimilarSources(ctx context.Context, query, locale string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := c.db.QueryContext(ctx,
		`SELECT b.id
		 FROM sources_fts f
		 JOIN sources s ON s.id = f.rowid
		 JOIN blocks b ON b.hash = s.source_hash
		 WHERE sources_fts MATCH ?
		   AND s.source_locale = ?
		 ORDER BY bm25(sources_fts)
		 LIMIT ?`, sanitizeFTSQuery(query), locale, limit)
	if err != nil {
		return nil, fmt.Errorf("klz cache: similar-sources: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// sanitizeFTSQuery escapes FTS5 syntax so callers can pass arbitrary
// text without triggering syntax errors. Replaces every non-word
// rune with a space so the query decays to an OR of tokens.
func sanitizeFTSQuery(q string) string {
	var b strings.Builder
	for _, r := range q {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == ' ':
			b.WriteRune(r)
		default:
			b.WriteByte(' ')
		}
	}
	return strings.TrimSpace(b.String())
}

// ingest runs a single transaction that populates sources, blocks,
// source_hashes, and sources_fts from the Source iterator.
func (c *sqliteCache) ingest(ctx context.Context, src Source) error {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	srcLocale := src.SourceLocale()
	blocks, err := src.Blocks()
	if err != nil {
		return err
	}

	now := time.Now().UTC().Unix()

	// Populate sources (dedup by hash/context) and blocks.
	hashToSourceID := make(map[string]int64, len(blocks))
	hashToBlockIDs := make(map[string][]string, len(blocks))

	sourceStmt, err := tx.PrepareContext(ctx,
		`INSERT INTO sources (source_locale, source_hash, source_text, source_runs, context, block_type, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer sourceStmt.Close()

	ftsStmt, err := tx.PrepareContext(ctx,
		`INSERT INTO sources_fts (rowid, source_text) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer ftsStmt.Close()

	blockStmt, err := tx.PrepareContext(ctx,
		`INSERT INTO blocks (id, document_path, hash, type, component, jsx_path, optional_placeholders, required_placeholders)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer blockStmt.Close()

	for _, b := range blocks {
		if _, dup := hashToSourceID[b.Hash]; !dup {
			res, err := sourceStmt.ExecContext(ctx,
				srcLocale, b.Hash, b.SourceText, b.SourceJSON, b.Context, b.Type, now)
			if err != nil {
				return fmt.Errorf("insert source %q: %w", b.ID, err)
			}
			sid, err := res.LastInsertId()
			if err != nil {
				return err
			}
			hashToSourceID[b.Hash] = sid
			if _, err := ftsStmt.ExecContext(ctx, sid, b.SourceText); err != nil {
				return fmt.Errorf("insert fts %q: %w", b.ID, err)
			}
		}

		if _, err := blockStmt.ExecContext(ctx,
			b.ID, b.DocumentPath, b.Hash, b.Type, b.Component, b.JSXPath,
			b.OptionalPlaceholders, b.RequiredPlaceholders); err != nil {
			return fmt.Errorf("insert block %q: %w", b.ID, err)
		}
		hashToBlockIDs[b.Hash] = append(hashToBlockIDs[b.Hash], b.ID)
	}

	// Populate source_hashes (JSON array of block ids sharing a hash).
	shStmt, err := tx.PrepareContext(ctx,
		`INSERT INTO source_hashes (source_hash, block_ids) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer shStmt.Close()
	for hash, ids := range hashToBlockIDs {
		sort.Strings(ids)
		raw, err := json.Marshal(ids)
		if err != nil {
			return err
		}
		if _, err := shStmt.ExecContext(ctx, hash, string(raw)); err != nil {
			return fmt.Errorf("insert source_hashes: %w", err)
		}
	}

	// Populate targets.
	targets, err := src.Targets()
	if err != nil {
		return err
	}
	if len(targets) > 0 {
		targetStmt, err := tx.PrepareContext(ctx,
			`INSERT INTO targets (source_id, locale, target_runs, status, origin, origin_detail, created_at, updated_at)
			 SELECT s.id, ?, ?, ?, ?, ?, ?, ?
			 FROM sources s
			 JOIN blocks b ON b.hash = s.source_hash
			 WHERE b.id = ?`)
		if err != nil {
			return err
		}
		defer targetStmt.Close()
		for _, t := range targets {
			if _, err := targetStmt.ExecContext(ctx,
				t.Locale, t.TargetJSON, t.Status, t.Origin, t.OriginDetail, now, now, t.BlockID); err != nil {
				return fmt.Errorf("insert target (%s, %s): %w", t.BlockID, t.Locale, err)
			}
		}
	}

	return tx.Commit()
}

func (c *sqliteCache) writeMeta(ctx context.Context, manifestHash string) error {
	kv := [][2]string{
		{"cache_schema_version", CacheSchemaVersion},
		{"source_klz_manifest_sha256", manifestHash},
		{"built_at", time.Now().UTC().Format(time.RFC3339)},
		{"built_by", "neokapi core/klz/internal/db"},
	}
	for _, p := range kv {
		if _, err := c.db.ExecContext(ctx,
			`INSERT INTO cache_meta (key, value) VALUES (?, ?)`, p[0], p[1]); err != nil {
			return err
		}
	}
	return nil
}

func (c *sqliteCache) checkSchemaVersion(ctx context.Context) error {
	var v string
	err := c.db.QueryRowContext(ctx,
		`SELECT value FROM cache_meta WHERE key = 'cache_schema_version'`).Scan(&v)
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if v != CacheSchemaVersion {
		return fmt.Errorf("schema version mismatch: got %q want %q", v, CacheSchemaVersion)
	}
	return nil
}

// missingCache is returned by Open when no on-disk entry exists.
// Its query methods return ErrCacheMissing so callers can fall back
// to the iteration-side (linear-scan) code paths.
type missingCache struct{ hash string }

func (c *missingCache) BlockByID(_ context.Context, _ string) ([]byte, error) {
	return nil, &ErrNotImplemented{Op: "BlockByID (cache not built; call Build first)"}
}

func (c *missingCache) SimilarSources(_ context.Context, _, _ string, _ int) ([]string, error) {
	return nil, &ErrNotImplemented{Op: "SimilarSources (cache not built; call Build first)"}
}

func (c *missingCache) Close() error { return nil }

// ───────── GC ─────────

// GC evicts cache entries to bring the total cache size under
// maxBytes. Entries are ranked by access atime (oldest first). A
// value of zero disables the size cap; the caller can still pass a
// maxAge to prune stale entries.
func GC(ctx context.Context, maxBytes int64, maxAge time.Duration) (GCReport, error) {
	root := CacheRoot()
	report := GCReport{Root: root}
	if _, err := os.Stat(root); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return report, nil
		}
		return report, err
	}
	entries, err := scanEntries(root)
	if err != nil {
		return report, err
	}
	report.TotalEntries = len(entries)
	for _, e := range entries {
		report.TotalBytes += e.Bytes
	}

	// Prune by age first.
	now := time.Now()
	if maxAge > 0 {
		keep := entries[:0]
		for _, e := range entries {
			if !e.Mtime.IsZero() && now.Sub(e.Mtime) > maxAge {
				if err := removeEntry(e.Path); err != nil {
					return report, err
				}
				report.EvictedEntries++
				report.EvictedBytes += e.Bytes
				continue
			}
			keep = append(keep, e)
		}
		entries = keep
	}

	if maxBytes > 0 {
		// Sort oldest-atime first so we evict least-recently-used entries.
		sort.Slice(entries, func(i, j int) bool { return entries[i].Atime.Before(entries[j].Atime) })
		var remaining int64
		for _, e := range entries {
			remaining += e.Bytes
		}
		for _, e := range entries {
			if remaining <= maxBytes {
				break
			}
			if err := removeEntry(e.Path); err != nil {
				return report, err
			}
			remaining -= e.Bytes
			report.EvictedEntries++
			report.EvictedBytes += e.Bytes
		}
	}
	return report, nil
}

// GCReport is the summary of one GC pass.
type GCReport struct {
	Root           string
	TotalEntries   int
	TotalBytes     int64
	EvictedEntries int
	EvictedBytes   int64
}

// Entry describes one on-disk cache entry.
type Entry struct {
	Path  string
	Hash  string
	Bytes int64
	Atime time.Time
	Mtime time.Time
}

// Entries enumerates the on-disk cache entries (for `kapi cache info`).
func Entries() ([]Entry, error) {
	root := CacheRoot()
	if _, err := os.Stat(root); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return scanEntries(root)
}

func scanEntries(root string) ([]Entry, error) {
	shards, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var out []Entry
	for _, shard := range shards {
		if !shard.IsDir() || len(shard.Name()) != 2 || strings.HasPrefix(shard.Name(), ".") {
			continue
		}
		shardPath := filepath.Join(root, shard.Name())
		dirs, err := os.ReadDir(shardPath)
		if err != nil {
			continue
		}
		for _, d := range dirs {
			if !d.IsDir() {
				continue
			}
			e := Entry{
				Path: filepath.Join(shardPath, d.Name()),
				Hash: shard.Name() + d.Name(),
			}
			bytes, atime, mtime := dirStats(e.Path)
			e.Bytes = bytes
			e.Atime = atime
			e.Mtime = mtime
			out = append(out, e)
		}
	}
	return out, nil
}

func dirStats(path string) (size int64, atime time.Time, mtime time.Time) {
	_ = filepath.Walk(path, func(p string, info fs.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if !info.IsDir() {
			size += info.Size()
			if m := info.ModTime(); m.After(mtime) {
				mtime = m
			}
		}
		return nil
	})
	// Use directory mtime as an atime proxy since Go doesn't expose
	// atime portably. This is adequate for LRU eviction because
	// builds touch the directory on every rename and queries update
	// mtime via journal writes.
	if info, err := os.Stat(path); err == nil {
		atime = info.ModTime()
	}
	return
}

func removeEntry(path string) error {
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

// sanityHashEncoding is a tiny guard used at package boundaries to
// keep code paths that route arbitrary strings through the cache
// directory layout from accepting non-hex values. Kept as a package
// helper so callers don't re-implement it.
func sanityHashEncoding(hash string) bool {
	_, err := hex.DecodeString(hash)
	return err == nil && len(hash) == 2*sha256.Size
}

var _ = sanityHashEncoding
