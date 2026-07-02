package project

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/klf"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/version"
)

// This file centralizes block-store synchronization — the version stamp, the
// per-source drift stamps, and the extract-into-store path — so the CLI's
// convergence loop (`kapi up`) and the desktop's Re-extract share one
// implementation (#1078 C2). The desktop's backend RunExtract is a thin
// binding over ExtractToBlockStore; the CLI auto-extracts on drift before
// each `up` pass via DetectStoreDrift + ExtractToBlockStore.

// BlockStoreVersionStampPath is the sidecar file recording the kapi version
// that last wrote the block store, e.g. `.kapi/cache/blocks.db.kapiversion`.
func BlockStoreVersionStampPath(storePath string) string {
	return storePath + ".kapiversion"
}

// BlockStoreStale reports whether the block store at storePath was written by
// a different kapi version than the running binary — true when the version
// stamp is missing/unreadable, or its contents don't match version.Version.
// Callers invoke this only once the store is known to exist (a missing store
// has nothing to be stale about).
func BlockStoreStale(storePath string) bool {
	data, err := os.ReadFile(BlockStoreVersionStampPath(storePath))
	if err != nil {
		return true
	}
	return strings.TrimSpace(string(data)) != version.Version
}

// StampBlockStoreVersion records the running kapi version as the writer of the
// block store at storePath, so a later binary can tell whether it would have
// extracted different content.
func StampBlockStoreVersion(storePath string) error {
	return os.WriteFile(BlockStoreVersionStampPath(storePath), []byte(version.Version), 0o644)
}

// SourceStamp records the identity of one source file at extract time: its
// content hash plus the (size, mtime) fast-path pair, so drift detection only
// re-hashes files whose stat changed.
type SourceStamp struct {
	// Hash is the source file's content hash ("sha256:..."), as HashFile
	// computes it.
	Hash string `json:"hash"`
	// Size and MTimeNS are the stat fast path: when both match the current
	// file, the bytes are assumed unchanged and the hash is not recomputed.
	Size    int64 `json:"size"`
	MTimeNS int64 `json:"mtimeNs"`
}

// BlockStoreSourceStampsPath is the sidecar recording, per project-relative
// source path, the SourceStamp captured by the last extraction, e.g.
// `.kapi/cache/blocks.db.sources.json`.
func BlockStoreSourceStampsPath(storePath string) string {
	return storePath + ".sources.json"
}

// LoadSourceStamps reads the source-stamp sidecar. A missing or unreadable
// sidecar yields an empty map — every file then reads as drifted, which is the
// safe direction (re-extract).
func LoadSourceStamps(storePath string) map[string]SourceStamp {
	data, err := os.ReadFile(BlockStoreSourceStampsPath(storePath))
	if err != nil {
		return map[string]SourceStamp{}
	}
	var stamps map[string]SourceStamp
	if json.Unmarshal(data, &stamps) != nil || stamps == nil {
		return map[string]SourceStamp{}
	}
	return stamps
}

// SaveSourceStamps writes the source-stamp sidecar next to the block store.
func SaveSourceStamps(storePath string, stamps map[string]SourceStamp) error {
	data, err := json.MarshalIndent(stamps, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(BlockStoreSourceStampsPath(storePath), data, 0o644)
}

// StoreDrift describes how the block store has fallen behind the working tree.
type StoreDrift struct {
	// StoreMissing: the store file does not exist (never extracted, or the
	// cache was cleared).
	StoreMissing bool `json:"storeMissing,omitempty"`
	// VersionStale: the store exists but was written by a different kapi
	// version (missing or mismatched version stamp), so its content may be
	// extracted differently by the running binary.
	VersionStale bool `json:"versionStale,omitempty"`
	// Changed lists the project-relative source paths whose bytes drifted
	// from their extract-time stamps (edited, or never stamped).
	Changed []string `json:"changed,omitempty"`
	// Removed lists stamped source paths that no longer resolve from the
	// project's content patterns (deleted or de-scoped files whose blocks
	// linger in the store).
	Removed []string `json:"removed,omitempty"`
}

// Any reports whether any drift was detected.
func (d StoreDrift) Any() bool {
	return d.StoreMissing || d.VersionStale || len(d.Changed) > 0 || len(d.Removed) > 0
}

// DetectStoreDrift compares the project's resolved source files against the
// block store's sidecar stamps and reports what has drifted: a missing store, a
// version-stamp mismatch, edited/new source files (stat fast path, content-hash
// confirm), and stamped files that no longer resolve. It is read-only and
// best-effort: an unreadable file counts as changed (the safe direction).
func DetectStoreDrift(storePath string, files []ResolvedFile) StoreDrift {
	var d StoreDrift
	if info, err := os.Stat(storePath); err != nil || info.IsDir() {
		d.StoreMissing = true
		return d
	}
	if BlockStoreStale(storePath) {
		d.VersionStale = true
	}

	stamps := LoadSourceStamps(storePath)
	seen := make(map[string]bool, len(files))
	for _, rf := range files {
		seen[rf.Relative] = true
		stamp, ok := stamps[rf.Relative]
		if !ok {
			d.Changed = append(d.Changed, rf.Relative)
			continue
		}
		info, err := os.Stat(rf.Path)
		if err != nil {
			d.Changed = append(d.Changed, rf.Relative)
			continue
		}
		if info.Size() == stamp.Size && info.ModTime().UnixNano() == stamp.MTimeNS {
			continue // stat fast path: unchanged
		}
		hash, err := HashFile(rf.Path)
		if err != nil || hash != stamp.Hash {
			d.Changed = append(d.Changed, rf.Relative)
		}
		// Touched but byte-identical: not drift. The refreshed (size, mtime)
		// pair is re-stamped by the next extraction; until then the hash
		// compare re-runs, which is correct just not maximally cheap.
	}
	for rel := range stamps {
		if !seen[rel] {
			d.Removed = append(d.Removed, rel)
		}
	}
	return d
}

// ExtractSkip records one file that extraction could not process.
type ExtractSkip struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// ExtractStats summarises one extraction into the block store.
type ExtractStats struct {
	// Files is the number of source files successfully extracted.
	Files int `json:"files"`
	// Blocks is the total number of blocks written to the store.
	Blocks int `json:"blocks"`
	// Skipped lists files that could not be extracted (no reader, read error)
	// with a short reason. Extraction is best-effort per file.
	Skipped []ExtractSkip `json:"skipped,omitempty"`
}

// CollectionLabel maps a collection name to its block-store label; unnamed
// collections share the "(unnamed)" bucket. One definition, shared by every
// store writer and reader so coverage queries always match extraction.
func CollectionLabel(name string) string {
	if name == "" {
		return "(unnamed)"
	}
	return name
}

// ExtractToBlockStore extracts the given resolved source files into the
// project's persistent block store — the single extract-into-store path shared
// by the desktop's Re-extract and the CLI's auto-extract-on-drift.
//
// Blocks are a pure cache re-derived from source: the prior block set is
// purged first (stale rows must not linger under content-unique keys) and
// every resolved file is re-read, so the store always mirrors the current
// working tree. Target overlays live in a separate table and are preserved.
// Extraction is best-effort per file: a file with no reader or a read error is
// recorded in Skipped rather than failing the run. On success the store is
// stamped with the running kapi version and the per-source drift stamps.
func ExtractToBlockStore(
	ctx context.Context,
	reg *registry.FormatRegistry,
	pctx *ProjectContext,
	store blockstore.Store,
	storePath string,
	files []ResolvedFile,
) (ExtractStats, error) {
	var stats ExtractStats
	if pctx == nil {
		return stats, errors.New("project: extract to block store: nil project context")
	}

	sess, err := store.Begin(ctx)
	if err != nil {
		return stats, fmt.Errorf("open block store session: %w", err)
	}

	if purger, ok := sess.(blockstore.BlockPurger); ok {
		if perr := purger.DeleteBlocks(); perr != nil {
			_ = sess.Rollback()
			return stats, fmt.Errorf("clear block store: %w", perr)
		}
	}

	stamps := make(map[string]SourceStamp, len(files))
	for _, rf := range files {
		if rf.Format == "" {
			stats.Skipped = append(stats.Skipped, ExtractSkip{
				Path:   rf.Relative,
				Reason: "no format detected (plugin may not be installed)",
			})
			continue
		}
		// Per-format project defaults, applied the same way a run configures
		// the reader — block numbering must match the CLI's.
		var cfg map[string]any
		if fd, ok := pctx.FormatDefaults[rf.Format]; ok {
			cfg = fd.Config
		}
		blocks, _, rerr := ReadSourceBlocks(ctx, reg, rf.Format, rf.Path, pctx.SourceLocale, "", cfg)
		if rerr != nil {
			stats.Skipped = append(stats.Skipped, ExtractSkip{Path: rf.Relative, Reason: rerr.Error()})
			continue
		}
		collection := CollectionLabel(rf.Collection)
		for _, b := range blocks {
			// Key the block globally-unique per (source file, in-file id) so
			// blocks from different files/collections don't collide in the
			// hash-keyed store.
			kb := &klf.Block{
				ID:           b.ID,
				Hash:         BlockStoreHash(rf.Relative, b.ID, b.SourceText()),
				Translatable: b.Translatable,
				Source:       b.Source,
			}
			if perr := sess.PutBlock(collection, kb); perr != nil {
				_ = sess.Rollback()
				return stats, fmt.Errorf("write block from %q: %w", rf.Relative, perr)
			}
			stats.Blocks++
		}
		stats.Files++

		if stamp, serr := sourceStampFor(rf.Path); serr == nil {
			stamps[rf.Relative] = stamp
		}
	}

	if err := sess.Commit(); err != nil {
		return stats, fmt.Errorf("commit extraction: %w", err)
	}

	// Stamp the store with the version that wrote it plus the per-source drift
	// stamps. Best-effort: a failed write only means the next drift check reads
	// the store as stale/drifted and re-extracts.
	_ = StampBlockStoreVersion(storePath)
	_ = SaveSourceStamps(storePath, stamps)
	return stats, nil
}

// sourceStampFor captures a source file's drift stamp (hash + stat fast path).
func sourceStampFor(path string) (SourceStamp, error) {
	info, err := os.Stat(path)
	if err != nil {
		return SourceStamp{}, err
	}
	hash, err := HashFile(path)
	if err != nil {
		return SourceStamp{}, err
	}
	return SourceStamp{Hash: hash, Size: info.Size(), MTimeNS: info.ModTime().UnixNano()}, nil
}
