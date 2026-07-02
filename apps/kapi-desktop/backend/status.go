package backend

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/blockstore/sqlitestore"
	"github.com/neokapi/neokapi/core/klf"
	"github.com/neokapi/neokapi/core/project"
)

// CollectionStatus is the JSON-serialisable summary the UI renders on the
// project status panel. Coverage is per-target-locale: for each locale the
// value is the count of translatable blocks in the collection that have a
// committed `targets/<locale>` overlay in the project block store. BlockCount
// is the total number of translatable blocks extracted for the collection.
type CollectionStatus struct {
	Name            string         `json:"name"`
	BlockCount      int            `json:"blockCount"`
	Coverage        map[string]int `json:"coverage"`
	TargetLanguages []string       `json:"targetLanguages"`
}

// ProjectStatus bundles the per-collection summaries.
//
// HasData reports whether the project's block store exists and has been
// populated (i.e. extraction has run at least once). When false, Collections
// still lists the declared collections and their target languages, but
// BlockCount/Coverage are zero — the frontend renders a "no data yet, run
// extract" state rather than an error.
type ProjectStatus struct {
	ProjectPath string             `json:"projectPath"`
	ProjectName string             `json:"projectName"`
	HasData     bool               `json:"hasData"`
	Collections []CollectionStatus `json:"collections"`
}

// GetProjectStatus returns the current per-collection coverage for a project
// tab, computed from the project's persistent block store
// (`.kapi/cache/blocks.db`). It reuses the same store keys the CLI uses —
// blocks are addressed by their ID and translated targets live under
// `targets/<locale>` overlays (the keys `kapi run` / `kapi merge` write and
// read) — so the metric here is the same translated-vs-total measure, not a
// parallel one.
//
// If the block store does not exist yet (the project has never been
// extracted), the returned status has HasData=false and zeroed coverage; this
// is a well-defined "no data yet" state, not an error.
func (a *App) GetProjectStatus(tabID string) (*ProjectStatus, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}

	out := &ProjectStatus{ProjectPath: op.Path}
	if op.Project == nil {
		return out, nil
	}
	out.ProjectName = op.Project.Name

	// Declared collections + target languages — the shell the frontend draws
	// even before any extraction has happened.
	collTargets := make(map[string][]string)
	collOrder := make([]string, 0, len(op.Project.Content))
	for _, coll := range op.Project.Content {
		label := collectionLabel(coll.Name)
		targets := make([]string, 0, len(coll.TargetLanguages))
		for _, loc := range coll.TargetLanguages {
			targets = append(targets, string(loc))
		}
		// Fall back to project defaults when the collection declares none.
		if len(targets) == 0 {
			for _, loc := range op.Project.Defaults.TargetLanguages {
				targets = append(targets, string(loc))
			}
		}
		if _, seen := collTargets[label]; !seen {
			collOrder = append(collOrder, label)
		}
		collTargets[label] = targets
	}

	// No block store → "no data yet" shell with zeroed coverage.
	storePath, ok := a.projectBlockStorePath(op)
	if !ok {
		out.Collections = buildEmptyCollections(collOrder, collTargets)
		return out, nil
	}
	if info, serr := os.Stat(storePath); serr != nil || info.IsDir() {
		out.Collections = buildEmptyCollections(collOrder, collTargets)
		return out, nil
	}

	store, err := a.projectBlockStore(op)
	if err != nil {
		return nil, fmt.Errorf("open project block store: %w", err)
	}

	ctx := context.Background()
	sess, err := store.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("open block store session: %w", err)
	}
	defer sess.Close()

	out.HasData = true
	out.Collections = make([]CollectionStatus, 0, len(collOrder))
	for _, label := range collOrder {
		targets := collTargets[label]

		// Total translatable blocks for this collection.
		blockIDs := make([]string, 0)
		t := true
		for b, err := range sess.Blocks(blockstore.BlockFilter{Collection: label, Translatable: &t}) {
			if err != nil {
				return nil, fmt.Errorf("read blocks for %q: %w", label, err)
			}
			id := b.ID
			if id == "" {
				id = b.Hash
			}
			blockIDs = append(blockIDs, id)
		}

		coverage := make(map[string]int, len(targets))
		for _, loc := range targets {
			kind := "targets/" + loc
			n := 0
			for _, id := range blockIDs {
				if _, err := sess.GetOverlay(kind, id); err == nil {
					n++
				}
			}
			coverage[loc] = n
		}

		out.Collections = append(out.Collections, CollectionStatus{
			Name:            label,
			BlockCount:      len(blockIDs),
			Coverage:        coverage,
			TargetLanguages: targets,
		})
	}

	return out, nil
}

// buildEmptyCollections returns the declared collections with zeroed coverage,
// used for the "no data yet" state before any extraction has run.
func buildEmptyCollections(order []string, targets map[string][]string) []CollectionStatus {
	out := make([]CollectionStatus, 0, len(order))
	for _, label := range order {
		locs := targets[label]
		coverage := make(map[string]int, len(locs))
		for _, loc := range locs {
			coverage[loc] = 0
		}
		out = append(out, CollectionStatus{
			Name:            label,
			BlockCount:      0,
			Coverage:        coverage,
			TargetLanguages: locs,
		})
	}
	return out
}

// projectBlockStorePath resolves the project's `.kapi/cache/blocks.db` path
// from its recipe location. Returns false when the project has no on-disk path.
func (a *App) projectBlockStorePath(op *openProject) (string, bool) {
	if op == nil || op.Path == "" {
		return "", false
	}
	layout, err := project.LayoutFor(op.Path)
	if err != nil {
		return "", false
	}
	return layout.BlockStorePath(), true
}

// projectBlockStore returns the project's block store, opening it once and
// caching it on the openProject for reuse. Opening (and migrating) a fresh
// SQLite pool on every call let concurrent operations collide on blocks.db with
// "database is locked"; a single shared pool lets SQLite/WAL serialize access.
// The store is closed in CloseProject. Concurrent callers within the process
// share the one pool; other processes (e.g. the kapi CLI) open their own pool
// and coordinate via WAL.
func (a *App) projectBlockStore(op *openProject) (blockstore.Store, error) {
	storePath, ok := a.projectBlockStorePath(op)
	if !ok {
		return nil, errors.New("project has no block store path")
	}
	op.blockStoreMu.Lock()
	defer op.blockStoreMu.Unlock()
	if op.blockStore != nil {
		return op.blockStore, nil
	}
	store, err := sqlitestore.New(storePath)
	if err != nil {
		return nil, err
	}
	op.blockStore = store
	return store, nil
}

// ExtractResult summarises one extraction request from the UI.
type ExtractResult struct {
	// Files is the number of source files successfully extracted.
	Files int `json:"files"`
	// Blocks is the total number of translatable blocks written to the store.
	Blocks int `json:"blocks"`
	// Skipped lists files that could not be extracted (no reader, read error)
	// with a short reason. Extraction is best-effort: an unreadable file (e.g.
	// a format whose plugin is not installed) is skipped, not fatal.
	Skipped []ExtractSkip `json:"skipped,omitempty"`
	// Log is a human-readable summary the frontend can show.
	Log string `json:"log"`
}

// ExtractSkip records one file that extraction could not process.
type ExtractSkip struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// RunExtract extracts the open project's declared content into the project's
// persistent block store (`.kapi/cache/blocks.db`), the same store that
// `kapi run` / `kapi merge` read and write. After it runs, GetProjectStatus
// coverage reflects the extracted content (every block becomes part of the
// per-collection denominator; targets remain at zero until a translate flow
// runs and commits `targets/<locale>` overlays).
//
// Extraction resolves the project's content patterns against the filesystem
// (reusing project.ProjectContext.ResolveContent), reads each source file with
// the detected format reader, and PutBlocks every block under its collection
// name. It is best-effort per file: a file with no reader or a read error is
// recorded in Skipped rather than failing the whole run.
func (a *App) RunExtract(tabID string) (*ExtractResult, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	if op.Project == nil {
		return nil, errors.New("project has no recipe loaded")
	}

	storePath, ok := a.projectBlockStorePath(op)
	if !ok {
		return nil, errors.New("project has no file path; save it before extracting")
	}

	pctx := project.NewProjectContext(op.Project, op.Path)
	resolved, err := pctx.ResolveContent(a.formatReg)
	if err != nil {
		return nil, fmt.Errorf("resolve content: %w", err)
	}
	if len(resolved) == 0 {
		return &ExtractResult{Log: "No source files matched the project's content patterns.\n"}, nil
	}

	// Ensure the cache dir (.kapi/cache/) exists — sqlitestore.New does not
	// create parent directories.
	if err := os.MkdirAll(filepath.Dir(storePath), 0o755); err != nil {
		return nil, fmt.Errorf("create cache dir: %w", err)
	}

	store, err := a.projectBlockStore(op)
	if err != nil {
		return nil, fmt.Errorf("open project block store: %w", err)
	}

	ctx := context.Background()
	sess, err := store.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("open block store session: %w", err)
	}

	// Blocks are a pure cache re-derived from source on every extract. Clear the
	// prior set first so stale rows can't linger under the content-unique keys
	// (re-extraction is a full rebuild; target overlays live in a separate table
	// and are preserved).
	if purger, ok := sess.(blockstore.BlockPurger); ok {
		if perr := purger.DeleteBlocks(); perr != nil {
			_ = sess.Rollback()
			return nil, fmt.Errorf("clear block store: %w", perr)
		}
	}

	result := &ExtractResult{}
	for _, rf := range resolved {
		if rf.Format == "" {
			result.Skipped = append(result.Skipped, ExtractSkip{
				Path:   rf.Relative,
				Reason: "no format detected (plugin may not be installed)",
			})
			continue
		}
		// Per-format project defaults, applied the same way a run configures the
		// reader — block numbering must match the CLI's.
		var cfg map[string]any
		if fd, ok := pctx.FormatDefaults[rf.Format]; ok {
			cfg = fd.Config
		}
		// Shared source-reading path (core/project), identical to the CLI.
		blocks, _, rerr := project.ReadSourceBlocks(
			ctx, a.formatReg, rf.Format, rf.Path, pctx.SourceLocale, "", cfg,
		)
		if rerr != nil {
			result.Skipped = append(result.Skipped, ExtractSkip{Path: rf.Relative, Reason: rerr.Error()})
			continue
		}
		collection := collectionLabel(rf.Collection)
		for _, b := range blocks {
			// Key the block globally-unique per (source file, in-file id) so
			// blocks from different files/collections don't collide in the
			// hash-keyed store (issue: "Website 0 blocks").
			kb := &klf.Block{
				ID:           b.ID,
				Hash:         project.BlockStoreHash(rf.Relative, b.ID, b.SourceText()),
				Translatable: b.Translatable,
				Source:       b.Source,
			}
			if perr := sess.PutBlock(collection, kb); perr != nil {
				_ = sess.Rollback()
				return nil, fmt.Errorf("write block from %q: %w", rf.Relative, perr)
			}
			result.Blocks++
		}
		result.Files++
	}

	if err := sess.Commit(); err != nil {
		return nil, fmt.Errorf("commit extraction: %w", err)
	}

	result.Log = fmt.Sprintf("Extracted %d block(s) from %d file(s).", result.Blocks, result.Files)
	if len(result.Skipped) > 0 {
		result.Log += fmt.Sprintf(" Skipped %d file(s).", len(result.Skipped))
	}
	result.Log += "\n"

	a.emitEvent("project:extracted", map[string]any{
		"tabID":  tabID,
		"files":  result.Files,
		"blocks": result.Blocks,
	})
	return result, nil
}

func collectionLabel(name string) string {
	if name == "" {
		return "(unnamed)"
	}
	return name
}
