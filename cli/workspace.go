package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/blockstore/sqlitestore"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/klz"
)

// WorkspaceExt is the file extension for a .klz project snapshot.
const WorkspaceExt = ".klz"

// openProjectBlockStore opens (creating dirs as needed) the active
// project's persistent block store at .kapi/cache/blocks.db. Returns nil
// when there is no project context or the store can't be opened — callers
// fall back to the ephemeral in-memory store, so a failure here never
// breaks a run, it only forgoes overlay caching.
//
// Wiring this into a project run is what makes re-running a flow skip
// already-done per-block work (SessionTools hydrate from the cached
// overlays) — the resume story for projects, with no extra CLI surface.
func (a *App) openProjectBlockStore() blockstore.Store {
	if a.projectContext == nil || a.projectContext.ProjectDir == "" {
		return nil
	}
	layout, err := project.ResolveLayout(a.projectContext.ProjectDir)
	if err != nil {
		return nil
	}
	if err := project.EnsureLayout(layout); err != nil {
		return nil
	}
	store, err := sqlitestore.New(layout.BlockStorePath())
	if err != nil {
		return nil
	}
	return store
}

// loadWorkspace reads and validates a .klz package from disk.
func loadWorkspace(path string) (*klz.Package, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read snapshot %q: %w", filepath.Base(path), err)
	}
	pkg, err := klz.Unmarshal(data)
	if err != nil {
		return nil, fmt.Errorf("parse snapshot %q: %w", filepath.Base(path), err)
	}
	return pkg, nil
}

// saveWorkspace writes a package to disk atomically (temp + rename) so a
// crash never leaves a half-written .klz.
func saveWorkspace(pkg *klz.Package, path string) error {
	data, err := pkg.Marshal()
	if err != nil {
		return fmt.Errorf("encode snapshot: %w", err)
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".kapi-klz-*")
	if err != nil {
		return fmt.Errorf("write snapshot: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write snapshot: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write snapshot: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("finalize snapshot: %w", err)
	}
	return nil
}

func klzToStoreOverlays(in []klz.OverlayDoc) []blockstore.Overlay {
	out := make([]blockstore.Overlay, len(in))
	for i, o := range in {
		out[i] = blockstore.Overlay{Kind: o.Kind, BlockHash: o.BlockHash, Payload: []byte(o.Payload)}
	}
	return out
}

func storeToKlzOverlays(in []blockstore.Overlay) []klz.OverlayDoc {
	out := make([]klz.OverlayDoc, len(in))
	for i, o := range in {
		out[i] = klz.OverlayDoc{Kind: o.Kind, BlockHash: o.BlockHash, Payload: json.RawMessage(o.Payload)}
	}
	return out
}
