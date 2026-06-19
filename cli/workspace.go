package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/blockstore/sqlitestore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/klz"
)

// WorkspaceExt is the file extension for a .klz project snapshot.
const WorkspaceExt = ".klz"

// A .klz now carries the FULL project recipe (core/project.KapiProject), one
// source of truth for intent (AD-025 §6). The helpers below centralize the
// few fields the klz workspace flow reads — source locale, target locales,
// and the klz output layout (a klz-owned recipe Extras key) — so callers don't
// reach into the recipe shape directly.

// recipeSourceLang returns the recipe's source language, or "" when unset.
func recipeSourceLang(r *project.KapiProject) string {
	if r == nil {
		return ""
	}
	return string(r.Defaults.SourceLanguage)
}

// recipeTargetLangs returns the recipe's target locales as strings, in order.
func recipeTargetLangs(r *project.KapiProject) []string {
	if r == nil {
		return nil
	}
	out := make([]string, 0, len(r.Defaults.TargetLanguages))
	for _, l := range r.Defaults.TargetLanguages {
		out = append(out, string(l))
	}
	return out
}

// recipeOut returns the klz workspace output layout from the recipe's Extras.
func recipeOut(r *project.KapiProject) string {
	return klz.RecipeWorkspaceMeta(r).Out
}

// isWorkspacePackage reports whether a .klz is an ad-hoc workspace (created by
// `extract -o work.klz`) rather than a whole-project snapshot (created by
// `pack`). Both carry a full recipe, so the distinction rides in the recipe's
// klz workspace meta.
func isWorkspacePackage(pkg *klz.Package) bool {
	if pkg == nil || pkg.Recipe == nil {
		return false
	}
	return klz.RecipeWorkspaceMeta(pkg.Recipe).Workspace
}

// recipeAddTargetLang appends a target locale to the recipe if absent,
// preserving first-seen order. Initializes the recipe shape as needed.
func recipeAddTargetLang(r *project.KapiProject, locale string) {
	if r == nil || locale == "" {
		return
	}
	loc := model.LocaleID(locale)
	if slices.Contains(r.Defaults.TargetLanguages, loc) {
		return
	}
	r.Defaults.TargetLanguages = append(r.Defaults.TargetLanguages, loc)
}

// newWorkspaceRecipe synthesizes a minimal KapiProject recipe for an ad-hoc
// .klz workspace: schema version + source/target locales + the klz output
// layout (carried in Extras under the "klz" key). This is the full-recipe
// slot a .kapi file uses; an ad-hoc extract fills only these fields.
func newWorkspaceRecipe(sourceLang string, targetLangs []string, out string) *project.KapiProject {
	r := &project.KapiProject{
		Version: project.CurrentVersion,
		Defaults: project.Defaults{
			SourceLanguage: model.LocaleID(sourceLang),
		},
	}
	for _, tl := range targetLangs {
		recipeAddTargetLang(r, tl)
	}
	// Mark this as an ad-hoc workspace recipe (vs a project snapshot), so
	// `unpack` rebuilds the shadow cache rather than a .kapi/ state dir.
	_ = klz.SetRecipeWorkspaceMeta(r, klz.WorkspaceMeta{Out: out, Workspace: true})
	return r
}

// newInterchangeRecipe synthesizes the minimal recipe a bilingual interchange
// .klz carries: schema version + the source→target locale pair (AD-025 §7).
func newInterchangeRecipe(sourceLang, targetLang string) *project.KapiProject {
	r := &project.KapiProject{
		Version: project.CurrentVersion,
		Defaults: project.Defaults{
			SourceLanguage: model.LocaleID(sourceLang),
		},
	}
	recipeAddTargetLang(r, targetLang)
	return r
}

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

// copyContentToFile streams a parcel member's Content into a file on disk,
// never buffering the whole member in memory.
func copyContentToFile(c klz.Content, dst string) error {
	rc, err := c.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, rc); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
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
