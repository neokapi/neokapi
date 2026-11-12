package backend

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/host"
)

// CollectionStatus is the JSON-serialisable summary the UI renders on the
// project status panel. Coverage is per-target-locale: for each locale the
// value is the count of translatable blocks in the collection that have a
// committed `targets/<locale>` overlay in the project block store. BlockCount
// is the total number of translatable blocks extracted for the collection.
type CollectionStatus struct {
	Name       string `json:"name"`
	BlockCount int    `json:"blockCount"`
	// Coverage is the count of units carrying a translation, per target locale,
	// derived from the working tree.
	Coverage map[string]int `json:"coverage"`
	// Units is how many units that count is out of, per target locale, from the
	// same derivation. It is the denominator a percentage must use: BlockCount
	// answers a different question (how much this project has extracted) and a
	// ratio mixing the two belongs to neither.
	Units           map[string]int `json:"units"`
	TargetLanguages []string       `json:"targetLanguages"`
}

// ProjectStatus bundles the per-collection summaries.
//
// HasData reports whether the project's block store exists and has been
// populated (i.e. extraction has run at least once). When false, Collections
// still lists the declared collections, their target languages and the
// working tree's coverage, but BlockCount is zero, and the frontend renders a
// "no data yet, run extract" state rather than an error.
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
	// ContextFiles names the context files this checkout carries when the
	// project's store has never held its context, and the command that reads
	// them. nil the rest of the time, which is every project someone has
	// already read in.
	ContextFiles *ContextFilesNoticeDTO `json:"contextFiles,omitempty"`
}

// GetProjectStatus returns the current per-collection status for a project tab,
// on the two axes `kapi status` reports.
//
// Coverage is the CLI's own derivation (host.ProjectCoverageTally over
// working-tree reads), so a target committed beside its source counts here
// exactly as it counts at the terminal. Extracted totals come from the block
// cache, which the store holds and the tree does not.
//
// If the project has never been extracted, the returned status has
// HasData=false and zero block counts beside the working tree's coverage; this
// is a well-defined "no data yet" state, not an error. It is a row question, not a file one: the store file
// exists from the first command that touches any subsystem, so its presence
// stopped being evidence that anything was extracted.
func (a *App) GetProjectStatus(tabID string) (*ProjectStatus, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}

	out := &ProjectStatus{ProjectPath: op.Path}
	out.ContextFiles = a.contextFilesNotice(op.Path)
	if op.Project == nil {
		return out, nil
	}
	out.ProjectName = op.Project.Name

	// Declared collections and the languages each writes: the shell the
	// frontend draws even before any extraction has happened. Keyed by the
	// recipe collection name; bare entries share the "" bucket (displayed as
	// "(unnamed)").
	collOrder, collTargets := collectionTargets(op.Project)

	// Coverage comes from the working tree, through the derivation `kapi
	// status` counts from, whether or not the block store has read the
	// project. A target file committed beside its source is translated
	// content before any run has carried it into the store.
	var tally *convergence.CoverageTally
	if op.onDisk() {
		t, terr := a.workingTreeCoverage(host.WithExistingStoresOnly(context.Background()), op)
		if terr != nil {
			return nil, terr
		}
		tally = t
	}

	// Nothing extracted: the "no data yet" shell with zero block counts. Asked before
	// the store is opened where the project has none, so drawing the shell for a
	// project nobody has run anything on does not create one.
	ctx := context.Background()
	db, ok := a.existingProjectStore(op)
	if !ok {
		out.Collections = buildCollections(collOrder, collTargets, nil, tally)
		return out, nil
	}
	if extracted, err := db.HasBlocks(ctx); err != nil || !extracted {
		out.Collections = buildCollections(collOrder, collTargets, nil, tally)
		return out, nil
	}

	// Autocommit, not the session-transactional store: this is a read beside
	// whatever else the desktop is doing, and a session that held the write
	// permit for the length of a status query would stall every writer.
	store := db.BlocksAutocommit()
	if store == nil {
		out.Collections = buildCollections(collOrder, collTargets, nil, tally)
		return out, nil
	}
	sess, err := store.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("open block store session: %w", err)
	}
	defer sess.Close()

	out.HasData = true
	// A block cache whose schema stamp is missing or doesn't match the running
	// kapi was produced under different extraction semantics — its counts can be
	// silently wrong, so flag it for re-extraction.
	out.Stale = db.BlockStoreStale(ctx)

	scopes := make([]convergence.BlockStoreScope, 0, len(collOrder))
	for _, name := range collOrder {
		scopes = append(scopes, convergence.BlockStoreScope{
			Collection: name,
			Label:      collectionLabel(name),
			Locales:    collTargets[name],
		})
	}
	_, totals, err := convergence.TallyBlockStore(sess, scopes)
	if err != nil {
		return nil, fmt.Errorf("read block store coverage: %w", err)
	}

	// The extracted totals are the store's: they answer how much content this
	// project has read in, which the store holds and the tree does not.
	out.Collections = buildCollections(collOrder, collTargets, totals, tally)
	return out, nil
}

// workingTreeCoverage derives the project's coverage the way `kapi status`
// does, from the files in the tree rather than from the block store.
func (a *App) workingTreeCoverage(ctx context.Context, op *openProject) (*convergence.CoverageTally, error) {
	root := filepath.Dir(op.Path)
	engine := a.hostEngine()
	units, err := engine.UnitsFromProject(op.Project, root, "")
	if err != nil {
		return nil, fmt.Errorf("resolve verify blocks: %w", err)
	}
	tally, err := engine.ProjectCoverageTally(ctx, op.Project, root, units, nil)
	if err != nil {
		return nil, fmt.Errorf("compute working-tree coverage: %w", err)
	}
	return tally, nil
}

// collectionTargets lists the recipe's collections in order with the target
// languages each writes: the union of its items' languages
// (ContentItem.ResolvedTargetLanguages, canonical BCP-47), over the items
// with a target path. These are the languages `kapi status` reports a
// collection's units in; a collection whose items write no target lists none.
func collectionTargets(proj *project.KapiProject) ([]string, map[string][]string) {
	targets := make(map[string][]string)
	order := make([]string, 0, len(proj.Collections))
	for i := range proj.Collections {
		coll := &proj.Collections[i]
		if _, seen := targets[coll.Name]; !seen {
			order = append(order, coll.Name)
			targets[coll.Name] = []string{}
		}
		for _, item := range coll.EffectiveItems() {
			if item.Target == "" {
				continue
			}
			for _, loc := range item.ResolvedTargetLanguages(nil, proj.Defaults) {
				if !slices.Contains(targets[coll.Name], string(loc)) {
					targets[coll.Name] = append(targets[coll.Name], string(loc))
				}
			}
		}
	}
	return order, targets
}

// buildCollections returns the status of each collection: its extracted block
// count from totals (zero before any extraction) and, per target language, the
// units carrying a translation and the units counted, from the working-tree
// tally (zero where tally is nil or holds no such scope). order and targets
// are keyed by recipe collection name; the DTO carries the display label.
func buildCollections(order []string, targets map[string][]string, totals map[string]int, tally *convergence.CoverageTally) []CollectionStatus {
	ladder := gate.TargetLadder()
	out := make([]CollectionStatus, 0, len(order))
	for _, name := range order {
		locs := targets[name]
		coverage := make(map[string]int, len(locs))
		units := make(map[string]int, len(locs))
		for _, loc := range locs {
			coverage[loc], units[loc] = 0, 0
			if tally == nil {
				continue
			}
			if cov, ok := tally.Coverage(convergence.Scope{Collection: name, Locale: loc}); ok {
				coverage[loc] = cov.AtLeastCount(ladder, string(model.TargetStatusTranslated))
				units[loc] = cov.Total
			}
		}
		out = append(out, CollectionStatus{
			Name:            collectionLabel(name),
			BlockCount:      totals[name],
			Coverage:        coverage,
			Units:           units,
			TargetLanguages: locs,
		})
	}
	return out
}

// existingProjectStore returns the tab's project store only when the project
// already has one, so a read-only surface never brings a store into being just
// by rendering. Opening is what creates the file; a status panel or a plan
// preview that created one would leave the next real command a store it did not
// write.
func (a *App) existingProjectStore(op *openProject) (*projectdb.DB, bool) {
	root, ok := projectRoot(op)
	if !ok {
		return nil, false
	}
	info, err := os.Stat(a.hostEngine().ProjectStorePath(root))
	if err != nil || info.IsDir() {
		return nil, false
	}
	db, err := a.projectStore(op)
	if err != nil {
		a.logger.Printf("open project store for %s: %v", root, err)
		return nil, false
	}
	return db, true
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

// RunExtract extracts the open project's declared content into the block cache
// in the project's store, the same cache that `kapi run` / `kapi merge` read and
// write. After it runs, GetProjectStatus coverage reflects the extracted content
// (every block becomes part of the per-collection denominator; targets remain at
// zero until a translate flow runs and commits `targets/<locale>` overlays).
//
// It is a thin binding over the shared host extract-into-store path
// (host.App.ExtractToProjectStore), the same implementation the CLI's `kapi up`
// uses when it auto-extracts on source drift, so the desktop and CLI read,
// number and key blocks identically and both leave the context graph rebuilt
// over them.
func (a *App) RunExtract(tabID string) (*ExtractResult, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	if op.Project == nil {
		return nil, errors.New("project has no recipe loaded")
	}

	pctx := project.NewProjectContext(op.Project, op.Path)
	resolved, err := pctx.ResolveContent(a.formatReg)
	if err != nil {
		return nil, fmt.Errorf("resolve content: %w", err)
	}
	if len(resolved) == 0 {
		return &ExtractResult{Log: "No source files matched the project's content patterns.\n"}, nil
	}

	root, ok := projectRoot(op)
	if !ok {
		return nil, errors.New("project has no file path; save it first")
	}
	// The shared host path: the block set is rebuilt and then the context graph
	// projected from it, so the usage counts the explorer reads describe this
	// extraction rather than the previous one.
	stats, err := a.hostEngine().ExtractToProjectStore(context.Background(), a.formatReg, root, pctx, resolved)
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
	for _, w := range stats.Warnings {
		result.Log += "Warning: " + w + "\n"
	}

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
