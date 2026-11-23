package host

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/project"
)

// A convergence pass drafts; only delivery puts a file where something reads it.
//
// Coverage grades each unit's target from a file, so a pass writes its output
// before the next pass measures the locale. Under `materialize: on-converge` a
// locale short of its gate gets no files (#1936). So a pass writes into a
// run-local draft tree under `work/` in the checkout's state directory, one subtree per locale, and
// delivery happens once, at the end of the run. A parked locale's drafts stay
// in the workspace home (host/workhome.go).

// convergeDraftsDirName is the run-local draft tree, inside the project's
// derived state (never version control, never a build input).
const convergeDraftsDirName = "drafts"

// deliveryIsGated reports whether this run owns delivery under a gate: the
// recipe's `materialize: on-converge`, or an explicit `--materialize`. Those are
// the runs that promise a locale short of its ship gate gets no files, so those
// are the runs whose passes draft rather than deliver.
//
// `materialize: manual` promises the opposite, that delivery is a separate
// `kapi merge`, and claims nothing about a gate, so its passes keep writing
// where the recipe points.
func deliveryIsGated(proj *project.KapiProject, opts ConvergeOptions) bool {
	return opts.materialize || proj.Defaults.ResolvedMaterialize() == project.MaterializeOnConverge
}

// beginConvergeDrafts opens the draft tree for a convergence run over the
// project rooted at root, clearing anything a previous run left behind, and
// returns a function that removes it again.
//
// A leftover tree is not merely untidy: coverage prefers a draft over the
// delivered file, so a draft from an abandoned run would grade a locale on
// output nobody kept.
func (a *App) beginConvergeDrafts(projectPath, root string) (func(), error) {
	layout, err := project.LayoutFor(projectPath)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(layout.WorkDir(), convergeDraftsDirName)
	if err := os.RemoveAll(dir); err != nil {
		return nil, fmt.Errorf("clear the convergence draft tree: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create the convergence draft tree: %w", err)
	}
	a.convergeDraftDir = dir
	a.convergeDraftRoot = root
	a.convergeDeliveries = newDraftDeliveries()
	return a.endConvergeDrafts, nil
}

// endConvergeDrafts closes the draft tree: nothing is graded against a draft
// after it, and the tree is gone. Safe to call more than once, which is what
// lets the run end the drafts as soon as delivery is done, so the standing it
// reports is read off the delivered tree, while the caller's defer still holds
// the guarantee on every error path.
func (a *App) endConvergeDrafts() {
	dir := a.convergeDraftDir
	a.convergeDraftDir = ""
	a.convergeDraftRoot = ""
	a.convergeDeliveries = nil
	if dir != "" {
		_ = os.RemoveAll(dir)
	}
}

// draftPathFor maps a resolved target path into one locale's draft subtree. It
// returns (path, false) unchanged when no convergence run is drafting, when the
// locale is unknown, or when the target lies outside the project root, a
// destination the recipe's gates do not reach is not one this can hold back.
func (a *App) draftPathFor(locale, targetPath string) (string, bool) {
	if a.convergeDraftDir == "" || a.convergeDraftRoot == "" || locale == "" {
		return targetPath, false
	}
	rel, err := filepath.Rel(a.convergeDraftRoot, targetPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return targetPath, false
	}
	return filepath.Join(a.convergeDraftDir, locale, rel), true
}

// inConvergeDraftTree reports whether path lies in the draft tree of the
// convergence run in progress.
func (a *App) inConvergeDraftTree(path string) bool {
	if a.convergeDraftDir == "" {
		return false
	}
	rel, err := filepath.Rel(a.convergeDraftDir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// draftedTargetPath is draftPathFor for a reader: the draft when one exists,
// otherwise the delivered file. Coverage asks it, so a pass grades its own
// output while a locale with nothing drafted yet is still graded on what was
// last delivered.
//
// This is the LOOP's subject, and only the loop's: it answers "did this pass
// move the locale", which is what decides whether another pass runs and whether
// the locale is delivered. It is not the subject of what the run reports,
// a parked locale's draft is discarded, so a closing table derived from it
// describes a tree that no longer exists and that `kapi status`, run a second
// later, contradicts. The run ends the drafts before it reports (#2024).
func (a *App) draftedTargetPath(locale, targetPath string) string {
	drafted, ok := a.draftPathFor(locale, targetPath)
	if !ok {
		return targetPath
	}
	if _, err := os.Stat(drafted); err != nil {
		return targetPath
	}
	return drafted
}
