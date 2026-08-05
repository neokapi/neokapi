package host

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/kpz"
)

// workspaceExt is the file extension for a .kpz project snapshot.
const workspaceExt = ".kpz"

// A .kpz now carries the FULL project recipe (core/project.KapiProject), one
// source of truth for intent (AD-025 §6). The helpers below centralize the
// few fields the kpz workspace flow reads — source locale, target locales,
// and the kpz output layout (a kpz-owned recipe Extras key) — so callers don't
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

// recipeOut returns the kpz workspace output layout from the recipe's Extras.
func recipeOut(r *project.KapiProject) string {
	return kpz.RecipeWorkspaceMeta(r).Out
}

// isWorkspacePackage reports whether a .kpz is an ad-hoc workspace (created by
// `extract -o work.kpz`) rather than a whole-project snapshot (created by
// `pack`). Both carry a full recipe, so the distinction rides in the recipe's
// kpz workspace meta.
func isWorkspacePackage(pkg *kpz.Package) bool {
	if pkg == nil || pkg.Recipe == nil {
		return false
	}
	return kpz.RecipeWorkspaceMeta(pkg.Recipe).Workspace
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
// .kpz workspace: schema version + source/target locales + the kpz output
// layout (carried in Extras under the "kpz" key). This is the full-recipe
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
	_ = kpz.SetRecipeWorkspaceMeta(r, kpz.WorkspaceMeta{Out: out, Workspace: true})
	return r
}

// newInterchangeRecipe synthesizes the minimal recipe a bilingual interchange
// .kpz carries: schema version + the source→target locale pair (AD-025 §7).
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
// project's block cache, inside the project store. Returns nil
// when there is no project context or the store can't be opened — callers
// fall back to the ephemeral in-memory store, so a failure here never
// breaks a run, it only forgoes overlay caching.
//
// Wiring this into a project run is what makes re-running a flow skip
// already-done per-block work (SessionTools hydrate from the cached
// overlays) — the resume story for projects, with no extra CLI surface.
func (a *App) openProjectBlockStore(ctx context.Context) blockstore.Store {
	if a.ProjectContext == nil || a.ProjectContext.ProjectDir == "" {
		return nil
	}
	layout, err := project.ResolveLayout(a.ProjectContext.ProjectDir)
	if err != nil {
		return nil
	}
	db, err := a.ProjectDB(ctx, layout.Root)
	if err != nil {
		return nil
	}
	// Autocommit sessions: flow runs open one session per file-run, and a
	// convergence pass runs locales concurrently — run-long transactions on
	// one database would deadlock (see sqlitestore.NewAutocommit).
	// Overlays are idempotent per key, so per-write durability is the
	// intended semantics here.
	return db.BlocksAutocommit()
}

// LoadWorkspace reads and validates a .kpz package from disk. It is the single
// ingest point for every packaged workspace this binary opens — merge, unpack,
// info, and the shadow cache all come through here — which is why the recipe is
// made inert here rather than at each caller.
//
// kpz.Unmarshal has already refused any package naming a path outside the
// project. What remains is intent: a recipe travels with the content, and a
// package arrives from outside, so the exec-class steps and non-local output
// layouts it may declare are stripped before anything can run them. What was
// removed is reported, because silently dropping a step the author meant would
// look like kapi ignoring the recipe.
func LoadWorkspace(path string) (*kpz.Package, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read snapshot %q: %w", filepath.Base(path), err)
	}
	pkg, err := kpz.Unmarshal(data)
	if err != nil {
		return nil, fmt.Errorf("parse snapshot %q: %w", filepath.Base(path), err)
	}
	if pkg.Recipe != nil {
		sanitized, removed := kpz.SanitizeRecipe(pkg.Recipe)
		pkg.Recipe = sanitized
		for _, r := range removed {
			fmt.Fprintf(os.Stderr, "Warning: %s: ignoring %s carried by the package\n", filepath.Base(path), r)
		}
	}
	return pkg, nil
}

// saveWorkspace writes a package to disk atomically (temp + rename) so a
// crash never leaves a half-written .kpz.
func saveWorkspace(pkg *kpz.Package, path string) error {
	data, err := pkg.Marshal()
	if err != nil {
		return fmt.Errorf("encode snapshot: %w", err)
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".kapi-kpz-*")
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
func copyContentToFile(c kpz.Content, dst string) error {
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

func kpzToStoreOverlays(in []kpz.OverlayDoc) []blockstore.Overlay {
	out := make([]blockstore.Overlay, len(in))
	for i, o := range in {
		out[i] = blockstore.Overlay{Kind: o.Kind, BlockHash: o.BlockHash, Payload: []byte(o.Payload)}
	}
	return out
}

func storeToKpzOverlays(in []blockstore.Overlay) []kpz.OverlayDoc {
	out := make([]kpz.OverlayDoc, len(in))
	for i, o := range in {
		out[i] = kpz.OverlayDoc{Kind: o.Kind, BlockHash: o.BlockHash, Payload: json.RawMessage(o.Payload)}
	}
	return out
}
