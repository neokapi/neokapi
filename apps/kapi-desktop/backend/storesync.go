package backend

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/core/project"
)

// StoreSyncResult reports one SyncProjectStore call.
type StoreSyncResult struct {
	// Extracted is true when the block cache had drifted from the working tree
	// and was rebuilt. False means it was current, or that a run holds the
	// project and extracts on its own.
	Extracted bool `json:"extracted"`
	// Files and Blocks count what the rebuild read.
	Files  int `json:"files"`
	Blocks int `json:"blocks"`
	// Skipped lists files the rebuild could not read, with the reason.
	Skipped []ExtractSkip `json:"skipped,omitempty"`
}

// SyncProjectStore keeps the project's block cache current with its source
// files. The frontend calls it when a project tab opens and after the file
// watcher reports a change. It extracts only when the cache is missing, was
// written by another kapi version, or no longer matches the files, so a call
// on a current project costs a stat of each source file.
//
// Extraction is local and calls no provider, so the app does it without
// asking: the cache is how the app reads the project, and a person has nothing
// to decide about it. While a convergence run holds the project the call
// returns without extracting, because the run extracts before each pass.
func (a *App) SyncProjectStore(tabID string) (*StoreSyncResult, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	root, ok := projectRoot(op)
	if op.Project == nil || !ok {
		return &StoreSyncResult{}, nil
	}
	if a.runActive() {
		return &StoreSyncResult{}, nil
	}

	// One sync at a time: the watcher and a tab switch can ask together, and
	// the second caller then finds the cache the first one wrote.
	a.storeSyncMu.Lock()
	defer a.storeSyncMu.Unlock()

	if err := a.checkProjectOnDisk(op); err != nil {
		return nil, err
	}
	pctx := project.NewProjectContext(op.Project, op.Path)
	resolved, err := pctx.ResolveContent(a.formatReg)
	if err != nil {
		return nil, fmt.Errorf("resolve content: %w", err)
	}
	db, err := a.projectStore(op)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	if !db.DetectStoreDrift(ctx, resolved).Any() {
		return &StoreSyncResult{}, nil
	}

	a.emitEvent("project:store-syncing", map[string]any{"tabID": tabID})
	stats, err := a.hostEngine().ExtractToProjectStore(ctx, a.formatReg, root, pctx, resolved)
	if err != nil {
		return nil, fmt.Errorf("read the project's files: %w", err)
	}
	out := &StoreSyncResult{Extracted: true, Files: stats.Files, Blocks: stats.Blocks}
	for _, s := range stats.Skipped {
		out.Skipped = append(out.Skipped, ExtractSkip{Path: s.Path, Reason: s.Reason})
	}
	a.emitEvent("project:extracted", map[string]any{
		"tabID":  tabID,
		"files":  out.Files,
		"blocks": out.Blocks,
	})
	return out, nil
}

// runActive reports whether a flow or convergence run is in progress.
func (a *App) runActive() bool {
	if a.runState == nil {
		return false
	}
	a.runState.mu.Lock()
	defer a.runState.mu.Unlock()
	return a.runState.running
}
