package backend

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/blockstore/sqlitestore"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
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
	ProjectPath string `json:"projectPath"`
	ProjectName string `json:"projectName"`
	HasData     bool   `json:"hasData"`
	// Stale reports that the block store exists but was written under
	// different extraction semantics (core/project's block-store schema
	// version) than the running binary, so its counts may be wrong (e.g. a
	// store extracted before the `**`-glob fix shows too few blocks). The UI
	// should offer a Re-extract rather than trusting the numbers. It is always
	// false for the "no data yet" shells (no store ⇒ nothing to be stale about).
	Stale       bool               `json:"stale"`
	Collections []CollectionStatus `json:"collections"`
}

// GetProjectStatus returns the current per-collection coverage for a project
// tab, computed from the project's persistent block store
// (`.kapi/cache/blocks.db`) through the shared coverage engine
// (convergence.TallyBlockStore + CoverageTally) — the same tally the CLI's
// `kapi status` feeds from working-tree file reads, so the desktop and the
// CLI count with one rung semantics. Blocks are addressed by their ID and
// translated targets live under `targets/<locale>` overlays (the keys
// `kapi run` / `kapi merge` write and read).
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
	// even before any extraction has happened. Keyed by the recipe collection
	// name; bare entries share the "" bucket (displayed as "(unnamed)").
	collTargets := make(map[string][]string)
	collOrder := make([]string, 0, len(op.Project.Content))
	for _, coll := range op.Project.Content {
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
		if _, seen := collTargets[coll.Name]; !seen {
			collOrder = append(collOrder, coll.Name)
		}
		collTargets[coll.Name] = targets
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
	// A store whose schema stamp is missing or doesn't match the running kapi
	// was produced under different extraction semantics — its counts can be
	// silently wrong, so flag it for re-extraction.
	out.Stale = blockStoreStale(storePath)

	scopes := make([]convergence.BlockStoreScope, 0, len(collOrder))
	for _, name := range collOrder {
		scopes = append(scopes, convergence.BlockStoreScope{
			Collection: name,
			Label:      collectionLabel(name),
			Locales:    collTargets[name],
		})
	}
	tally, totals, err := convergence.TallyBlockStore(sess, scopes)
	if err != nil {
		return nil, fmt.Errorf("read block store coverage: %w", err)
	}

	ladder := gate.TargetLadder()
	out.Collections = make([]CollectionStatus, 0, len(collOrder))
	for _, name := range collOrder {
		targets := collTargets[name]
		coverage := make(map[string]int, len(targets))
		for _, loc := range targets {
			n := 0
			if cov, ok := tally.Coverage(convergence.Scope{Collection: name, Locale: loc}); ok {
				n = cov.AtLeastCount(ladder, string(model.TargetStatusTranslated))
			}
			coverage[loc] = n
		}
		out.Collections = append(out.Collections, CollectionStatus{
			Name:            collectionLabel(name),
			BlockCount:      totals[name],
			Coverage:        coverage,
			TargetLanguages: targets,
		})
	}

	return out, nil
}

// buildEmptyCollections returns the declared collections with zeroed coverage,
// used for the "no data yet" state before any extraction has run. order and
// targets are keyed by recipe collection name; the DTO carries the display
// label.
func buildEmptyCollections(order []string, targets map[string][]string) []CollectionStatus {
	out := make([]CollectionStatus, 0, len(order))
	for _, name := range order {
		locs := targets[name]
		coverage := make(map[string]int, len(locs))
		for _, loc := range locs {
			coverage[loc] = 0
		}
		out = append(out, CollectionStatus{
			Name:            collectionLabel(name),
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

// blockStoreVersionStampPath is the sidecar file recording the extraction
// schema version that last wrote the block store, e.g.
// `.kapi/cache/blocks.db.kapiversion`. Thin alias over the shared core
// implementation (core/project owns the stamp mechanism so the CLI's
// `kapi up` drift check and the desktop agree).
func blockStoreVersionStampPath(storePath string) string {
	return project.BlockStoreVersionStampPath(storePath)
}

// blockStoreStale reports whether the block store at storePath was written
// under different extraction semantics than the running binary. Callers
// invoke this only once the store is known to exist. Delegates to the shared
// core implementation.
func blockStoreStale(storePath string) bool {
	return project.BlockStoreStale(storePath)
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
// It is a thin binding over the shared core extract-into-store path
// (project.ExtractToBlockStore) — the same implementation the CLI's `kapi up`
// uses when it auto-extracts on source drift — so the desktop and CLI read,
// number, and key blocks identically.
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

	stats, err := project.ExtractToBlockStore(context.Background(), a.formatReg, pctx, store, storePath, resolved)
	if err != nil {
		return nil, err
	}

	result := &ExtractResult{Files: stats.Files, Blocks: stats.Blocks}
	for _, s := range stats.Skipped {
		result.Skipped = append(result.Skipped, ExtractSkip{Path: s.Path, Reason: s.Reason})
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

// collectionLabel maps a collection name to its block-store label; shared
// definition lives in core/project so store writers and readers agree.
func collectionLabel(name string) string {
	return project.CollectionLabel(name)
}
