package project

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/kbf"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/reconcile"
	"github.com/neokapi/neokapi/core/registry"
)

// This file centralizes block-store synchronization — the version stamp, the
// per-source drift stamps, and the extract-into-store path — so the CLI's
// convergence loop (`kapi up`) and the desktop's Re-extract share one
// implementation (#1078 C2). The desktop's backend RunExtract is a thin
// binding over ExtractToBlockStore; the CLI auto-extracts on drift before
// each `up` pass via DetectStoreDrift + ExtractToBlockStore.

// BlockStoreSchemaVersion identifies the block-store extraction semantics:
// how blocks are read, numbered, keyed (BlockStoreHash), and glob-expanded
// into the store. Bump it whenever a change makes previously extracted stores
// wrong (e.g. the historical `**`-glob fix), so they re-extract. Staleness is
// stamped by THIS version, not the binary version: a CLI and a desktop built
// from different releases with the same extraction semantics share one cache
// instead of endlessly invalidating each other's.
const BlockStoreSchemaVersion = "2026-07-05.1"

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

// StoreDrift describes how the block store has fallen behind the working tree.
type StoreDrift struct {
	// StoreMissing: nothing has been extracted yet (a fresh project, or the
	// block cache was cleared).
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

// CompareSourceStamps compares resolved source files against their extract-time
// stamps and reports which drifted and which are stamped but no longer resolve.
// It is the half of drift detection that does not care where the stamps came
// from, so this package and the store that holds them (core/projectdb) share one
// implementation rather than two that agree until they don't.
func CompareSourceStamps(stamps map[string]SourceStamp, files []ResolvedFile) (changed, removed []string) {
	seen := make(map[string]bool, len(files))
	for _, rf := range files {
		seen[rf.Relative] = true
		stamp, ok := stamps[rf.Relative]
		if !ok {
			changed = append(changed, rf.Relative)
			continue
		}
		info, err := os.Stat(rf.Path)
		if err != nil {
			changed = append(changed, rf.Relative)
			continue
		}
		if info.Size() == stamp.Size && info.ModTime().UnixNano() == stamp.MTimeNS {
			continue // stat fast path: unchanged
		}
		hash, err := HashFile(rf.Path)
		if err != nil || hash != stamp.Hash {
			changed = append(changed, rf.Relative)
		}
		// Touched but byte-identical: not drift. The refreshed (size, mtime)
		// pair is re-stamped by the next extraction; until then the hash
		// compare re-runs, which is correct just not maximally cheap.
	}
	for rel := range stamps {
		if !seen[rel] {
			removed = append(removed, rel)
		}
	}
	return changed, removed
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
	// Warnings records faults that did not endanger the extracted content —
	// today, a failure to record the store's version or drift stamps after the
	// blocks were already committed. Those writes stay best-effort (see
	// ExtractToBlockStore), but best-effort is not the same as unreportable:
	// the self-healing story assumes the NEXT write succeeds, and a permanent
	// cause makes every future run re-extract the whole project while looking
	// perfectly healthy. This is the channel that lets a caller say so.
	//
	// core is deliberately logger-free, so a returned, serialisable value is
	// the framework's way to report — it is also the only form a test can
	// assert on.
	Warnings []string `json:"warnings,omitempty"`
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

// BlockStoreStamper records the bookkeeping an extraction leaves behind: the
// extraction semantics that wrote the block cache, and what each source file
// looked like when it did.
//
// It is an interface because the stamps live in the project store's `store_meta`
// table (core/projectdb), and that package imports this one — so extraction can
// name the capability it needs but never the store that provides it.
type BlockStoreStamper interface {
	// StampBlockStoreVersion records BlockStoreSchemaVersion as the writer of
	// the block cache.
	StampBlockStoreVersion(ctx context.Context) error
	// SaveSourceStamps records the per-source, extract-time identity stamps.
	SaveSourceStamps(ctx context.Context, stamps map[string]SourceStamp) error
}

// DocumentAdopter resolves which document each source file IS and records it.
//
// Optional, and separate from BlockStoreStamper, because a stamp is a cache
// hint and this is not: a document's key is half of every decision's identity.
// A store that cannot answer it leaves decisions keyed on the path they were
// made at, which is what happened before any of this existed.
type DocumentAdopter interface {
	// AdoptDocuments resolves the read against what the project knows and
	// returns each path's durable key.
	AdoptDocuments(ctx context.Context, current []reconcile.DocUnit) (map[string]string, error)
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
	stamper BlockStoreStamper,
	files []ResolvedFile,
) (ExtractStats, error) {
	var stats ExtractStats
	if pctx == nil {
		return stats, errors.New("project: extract to block store: nil project context")
	}
	if stamper == nil {
		return stats, errors.New("project: extract to block store: nil stamper: an unstamped store " +
			"reads as drifted forever, so extraction refuses rather than write one")
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
	// What each document held, for identity resolution below. Gathered here
	// because this is the one pass that reads every source file: asking again
	// afterwards would re-read the whole project to answer a question about
	// names.
	docs := make([]reconcile.DocUnit, 0, len(files))
	for _, rf := range files {
		if rf.Format == "" {
			stats.Skipped = append(stats.Skipped, ExtractSkip{
				Path:   rf.Relative,
				Reason: "no format detected (plugin may not be installed)",
			})
			continue
		}
		// The format configuration the recipe declares for THIS item — its own
		// format.config over the project defaults — applied the same way a run
		// configures the reader, because block numbering must match the CLI's
		// and an item's extraction rules decide which leaves become blocks at
		// all.
		cfg := pctx.FormatConfigFor(rf.Format, rf.Item)
		blocks, _, rerr := ReadSourceBlocks(ctx, reg, rf.Format, rf.Path, pctx.SourceLocale, "", cfg)
		if rerr != nil {
			stats.Skipped = append(stats.Skipped, ExtractSkip{Path: rf.Relative, Reason: rerr.Error()})
			continue
		}
		collection := CollectionLabel(rf.Collection)
		doc := reconcile.DocUnit{Path: rf.Relative, Content: make([]string, 0, len(blocks))}
		for _, b := range blocks {
			// Key the block globally-unique per (source file, in-file id) so
			// blocks from different files/collections don't collide in the
			// hash-keyed store.
			kb := &kbf.Block{
				ID:           b.ID,
				Hash:         BlockStoreHash(rf.Relative, b.ID, b.SourceText()),
				Translatable: b.Translatable,
				Source:       b.Source,
			}
			// The source file the block came from. It is what makes a stored
			// block placeable — an occurrence names the document it was found
			// in, and a unit-state record finds its block by (document, id) —
			// and it is not recoverable from the block itself, because the
			// store is keyed by content and holds no document rows.
			kb.Properties.File = rf.Relative
			if perr := sess.PutBlock(collection, kb); perr != nil {
				_ = sess.Rollback()
				return stats, fmt.Errorf("write block from %q: %w", rf.Relative, perr)
			}
			doc.Content = append(doc.Content, model.ComputeContentHash(b.SourceText()))
			stats.Blocks++
		}
		docs = append(docs, doc)
		stats.Files++

		// The drift stamp is not optional bookkeeping: DetectStoreDrift treats a
		// file with NO stamp as unconditionally Changed, so a dropped stamp means
		// this file reads as drifted on every future invocation — re-extracted and
		// re-translated forever, with any "N drifted" summary permanently
		// over-reporting. The whole-sidecar write below really is best-effort (it
		// self-heals on the next run); a per-file hole is not, because the same
		// stat/hash fails again every time. And stats.Files++ above has already
		// counted the file as extracted, so silence here makes the stats and the
		// stamp map disagree about the same file.
		stamp, serr := sourceStampFor(rf.Path)
		if serr != nil {
			_ = sess.Rollback()
			return stats, fmt.Errorf("stamp source %q for drift detection: %w. Without a stamp "+
				"This file would be re-extracted on every run and always report as drifted", rf.Relative, serr)
		}
		stamps[rf.Relative] = stamp
	}

	if err := sess.Commit(); err != nil {
		return stats, fmt.Errorf("commit extraction: %w", err)
	}

	// Stamp the store with the version that wrote it plus the per-source drift
	// stamps. Best-effort: a failed write only means the next drift check reads
	// the store as stale/drifted and re-extracts. The blocks are already
	// committed, so failing the whole extraction over a cache hint would throw
	// away good work.
	//
	// Reported rather than silent, though. "It self-heals on the next run"
	// holds only if the next write succeeds; when the cause is permanent — a
	// read-only state directory, a full disk, a changed owner — every run
	// re-extracts and re-translates the entire project, and the sole symptom is
	// that kapi is inexplicably slow and re-does finished work forever. That is
	// precisely the failure nobody diagnoses, because nothing ever said it
	// happened.
	// Which document each file IS, resolved against what the project already
	// knows and recorded before anything reads a decision back. Unlike the
	// stamps below, a failure here is reported rather than swallowed: a run
	// that could not resolve identity files its decisions against paths, and a
	// rename after that orphans them.
	if adopter, ok := stamper.(DocumentAdopter); ok {
		if _, aerr := adopter.AdoptDocuments(ctx, docs); aerr != nil {
			stats.Warnings = append(stats.Warnings, fmt.Sprintf(
				"could not resolve document identity: %v. Decisions will be filed against file paths, "+
					"so renaming a file will detach the approvals inside it", aerr))
		}
	}

	if err := stamper.StampBlockStoreVersion(ctx); err != nil {
		stats.Warnings = append(stats.Warnings, fmt.Sprintf(
			"could not record the block-store version stamp: %v. The store will read as stale and re-extract on every run", err))
	}
	if err := stamper.SaveSourceStamps(ctx, stamps); err != nil {
		stats.Warnings = append(stats.Warnings, fmt.Sprintf(
			"could not record the source drift stamps: %v. Every source file will read as drifted and re-extract on every run", err))
	}
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
