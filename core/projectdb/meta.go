package projectdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/storage"
)

// The block store's bookkeeping — which extraction semantics wrote it, and what
// each source file looked like when it did — lives in `store_meta`, not in
// sidecar files named after the database. A stamp keyed off a database FILENAME
// means nothing once the block cache is one schema inside a shared file, and a
// sidecar beside a merged store is a third thing to keep consistent with it. In
// `store_meta` a stamp is written by the same transaction that could write the
// blocks it describes.
//
// This is the only implementation: the file-based one is gone and the sweep
// deletes the sidecars it wrote.

// metaMigrationsTable is this package's own ledger. It records only the
// store-wide metadata schema — every subsystem keeps its own.
const metaMigrationsTable = "projectdb_migrations"

var metaMigrations = []storage.Migration{{
	Version:     1,
	Description: "store metadata",
	SQL: `
CREATE TABLE IF NOT EXISTS store_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);`,
}}

// Metadata keys. Namespaced by subsystem so the table stays legible as more
// than the block store records here.
const (
	// MetaBlocksSchemaVersion records the extraction semantics that last wrote
	// the block cache (project.BlockStoreSchemaVersion).
	MetaBlocksSchemaVersion = "blocks.schemaVersion"
	// MetaBlocksSourceStamps records, per project-relative source path, the
	// identity that source had at extract time.
	MetaBlocksSourceStamps = "blocks.sourceStamps"
)

// ErrNoStore reports an operation that needs the database on a handle that has
// none — the browser build. Callers whose feature is optional there match it
// and degrade rather than fail.
var ErrNoStore = errors.New("projectdb: this build has no file-backed store")

// Meta reads one metadata value. ok is false when the key was never written.
func (d *DB) Meta(ctx context.Context, key string) (value string, ok bool, err error) {
	if d.raw == nil {
		return "", false, ErrNoStore
	}
	err = d.raw.QueryRowContext(ctx, `SELECT value FROM store_meta WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("projectdb: read metadata %q: %w", key, err)
	}
	return value, true, nil
}

// PutMeta writes one metadata value, replacing any previous one.
func (d *DB) PutMeta(ctx context.Context, key, value string) error {
	if d.raw == nil {
		return ErrNoStore
	}
	_, err := d.raw.ExecContext(ctx, `
INSERT INTO store_meta (key, value) VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	if err != nil {
		return fmt.Errorf("projectdb: write metadata %q: %w", key, err)
	}
	return nil
}

// StampBlockStoreVersion records the running binary's extraction semantics as
// the writer of the block cache, so a later binary can tell whether it would
// have extracted different content.
func (d *DB) StampBlockStoreVersion(ctx context.Context) error {
	return d.PutMeta(ctx, MetaBlocksSchemaVersion, project.BlockStoreSchemaVersion)
}

// BlockStoreStale reports whether the block cache was written under different
// extraction semantics than the running binary — true when the stamp is
// missing, unreadable, or does not match. Every uncertainty reads as stale,
// which is the safe direction: re-extract.
func (d *DB) BlockStoreStale(ctx context.Context) bool {
	v, ok, err := d.Meta(ctx, MetaBlocksSchemaVersion)
	if err != nil || !ok {
		return true
	}
	return strings.TrimSpace(v) != project.BlockStoreSchemaVersion
}

// LoadSourceStamps reads the per-source extract-time stamps. A missing or
// unreadable value yields an empty map — every file then reads as drifted,
// which is the safe direction.
func (d *DB) LoadSourceStamps(ctx context.Context) map[string]project.SourceStamp {
	v, ok, err := d.Meta(ctx, MetaBlocksSourceStamps)
	if err != nil || !ok {
		return map[string]project.SourceStamp{}
	}
	var stamps map[string]project.SourceStamp
	if json.Unmarshal([]byte(v), &stamps) != nil || stamps == nil {
		return map[string]project.SourceStamp{}
	}
	return stamps
}

// SaveSourceStamps records the per-source extract-time stamps.
func (d *DB) SaveSourceStamps(ctx context.Context, stamps map[string]project.SourceStamp) error {
	data, err := json.Marshal(stamps)
	if err != nil {
		return fmt.Errorf("projectdb: encode source stamps: %w", err)
	}
	return d.PutMeta(ctx, MetaBlocksSourceStamps, string(data))
}

// DetectStoreDrift compares the project's resolved source files against the
// stamps in the store and reports what has drifted.
//
// StoreMissing now means the block cache holds NO blocks, not that a file is
// absent: the store file exists from the first open of any subsystem, so its
// presence stopped being evidence that anything was ever extracted. Read-only
// and best-effort, like its file-based counterpart: an unreadable file counts
// as changed.
func (d *DB) DetectStoreDrift(ctx context.Context, files []project.ResolvedFile) project.StoreDrift {
	var drift project.StoreDrift
	extracted, err := d.HasBlocks(ctx)
	if err != nil || !extracted {
		drift.StoreMissing = true
		return drift
	}
	if d.BlockStoreStale(ctx) {
		drift.VersionStale = true
	}
	drift.Changed, drift.Removed = project.CompareSourceStamps(d.LoadSourceStamps(ctx), files)
	return drift
}
