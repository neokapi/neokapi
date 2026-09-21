package projectdb

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
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
	// MetaStoreInstance identifies this store file. It is minted on the first
	// ask and never rewritten, so a store deleted and created again is a
	// different one, which is what anything holding a claim about the store's
	// contents needs to be able to tell.
	MetaStoreInstance = "store.instance"
)

// ErrNoStore reports an operation that needs the database on a handle that has
// none — the browser build. Callers whose feature is optional there match it
// and degrade rather than fail.
var ErrNoStore = errors.New("projectdb: this build has no file-backed store")

// Meta reads one metadata value. ok is false when the key was never written.
func (d *DB) Meta(ctx context.Context, key string) (value string, ok bool, err error) {
	if d.projection == nil {
		return "", false, ErrNoStore
	}
	err = d.projection.QueryRowContext(ctx, `SELECT value FROM store_meta WHERE key = ?`, key).Scan(&value)
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
	if d.projection == nil {
		return ErrNoStore
	}
	_, err := d.projection.ExecContext(ctx, `
INSERT INTO store_meta (key, value) VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	if err != nil {
		return fmt.Errorf("projectdb: write metadata %q: %w", key, err)
	}
	return nil
}

// ContextMeta reads one metadata value about the project's CONTEXT, from the
// context store. ok is false when the key was never written.
//
// Separate from Meta because the two pools answer for different things. A
// stamp about the projection is about this checkout's derived state, and a
// fact about the context is true of the project: every checkout reads one
// answer, which is what keeps two of them from disagreeing about which stored
// profile a recipe's binding names.
func (d *DB) ContextMeta(ctx context.Context, key string) (value string, ok bool, err error) {
	if d.context == nil {
		return "", false, ErrNoStore
	}
	err = d.context.QueryRowContext(ctx, `SELECT value FROM store_meta WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("projectdb: read context metadata %q: %w", key, err)
	}
	return value, true, nil
}

// PutContextMeta writes one metadata value about the project's context,
// replacing any previous one.
func (d *DB) PutContextMeta(ctx context.Context, key, value string) error {
	if d.context == nil {
		return ErrNoStore
	}
	_, err := d.context.ExecContext(ctx, `
INSERT INTO store_meta (key, value) VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	if err != nil {
		return fmt.Errorf("projectdb: write context metadata %q: %w", key, err)
	}
	return nil
}

// InstanceID returns this store file's identity, minting one on the first ask.
//
// It exists so a record kept outside the store can say which store it is about.
// The position a project has consumed in a venue's change feed is such a
// record: it lives in the ref cache and vouches for what landed in the store,
// including decisions pulled and staged but not committed. The two are deleted
// independently, and a position carried onto a store that never consumed it
// claims content this project holds nowhere.
//
// Returns ErrNoStore on a build with no file-backed store, where there is no
// file to identify.
func (d *DB) InstanceID(ctx context.Context) (string, error) {
	if d.projection == nil {
		return "", ErrNoStore
	}
	if id, ok, err := d.Meta(ctx, MetaStoreInstance); err != nil {
		return "", err
	} else if ok && strings.TrimSpace(id) != "" {
		return id, nil
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("projectdb: mint store identity: %w", err)
	}
	id := hex.EncodeToString(buf)
	if err := d.PutMeta(ctx, MetaStoreInstance, id); err != nil {
		return "", err
	}
	return id, nil
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
