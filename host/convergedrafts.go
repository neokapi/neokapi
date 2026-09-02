package host

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

// A convergence pass drafts; only delivery puts a file where something reads it.
//
// The loop's coverage is derived from files — it reads each unit's target and
// grades it — so a pass has to write its output somewhere before the pass after
// it can tell whether the locale moved. Writing it at the collection's own
// `target:` path made measuring and delivering the same act, and delivering is
// the one the ship gate exists to hold back: `materialize: on-converge` promises
// that a locale short of its gate does not get its files, and the parked
// locale's unreviewed draft was already sitting in the tree a site build globs
// before the gate was ever consulted (#1936). A gate documented to withhold and
// observably withholding nothing is worse than no gate.
//
// So under a policy that claims a gate, a pass writes into a run-local draft
// tree under `.kapi/work/`, one subtree per locale, and coverage grades the
// draft. Delivery happens once, at the end of the run, for the locales that
// cleared their gate: their drafts are promoted to the destination and the
// materialize pass writes whatever the project block store holds on top. A
// parked locale's draft is discarded with the tree — the work itself survives in
// the store, where `kapi merge` or `up --materialize` can still deliver it.

// convergeDraftsDirName is the run-local draft tree, inside the project's
// derived state (never version control, never a build input).
const convergeDraftsDirName = "drafts"

// deliveryIsGated reports whether this run owns delivery under a gate: the
// recipe's `materialize: on-converge`, or an explicit `--materialize`. Those are
// the runs that promise a locale short of its ship gate gets no files, so those
// are the runs whose passes draft rather than deliver.
//
// `materialize: manual` promises the opposite — that delivery is a separate
// `kapi merge` — and claims nothing about a gate, so its passes keep writing
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
	return a.endConvergeDrafts, nil
}

// endConvergeDrafts closes the draft tree: nothing is graded against a draft
// after it, and the tree is gone. Safe to call more than once, which is what
// lets the run end the drafts as soon as delivery is done — so the standing it
// reports is read off the delivered tree — while the caller's defer still holds
// the guarantee on every error path.
func (a *App) endConvergeDrafts() {
	dir := a.convergeDraftDir
	a.convergeDraftDir = ""
	a.convergeDraftRoot = ""
	if dir != "" {
		_ = os.RemoveAll(dir)
	}
}

// draftPathFor maps a resolved target path into one locale's draft subtree. It
// returns (path, false) unchanged when no convergence run is drafting, when the
// locale is unknown, or when the target lies outside the project root — a
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

// draftedTargetPath is draftPathFor for a reader: the draft when one exists,
// otherwise the delivered file. Coverage asks it, so a pass grades its own
// output while a locale with nothing drafted yet is still graded on what was
// last delivered.
//
// This is the LOOP's subject, and only the loop's: it answers "did this pass
// move the locale", which is what decides whether another pass runs and whether
// the locale is delivered. It is not the subject of what the run reports —
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

// deliverDrafts moves one locale's drafted files to the destinations the recipe
// resolved for them, and returns those destinations.
//
// This is the delivery the ship gate governs: it runs for a locale that cleared
// its gate and for no other, so a parked locale's files are genuinely absent
// rather than present and unblessed. It moves the run's own output rather than
// re-deriving it, because the pass is what produced the bytes — the block store
// is a second, independent record of the same work and is written on top
// afterwards, but it is not guaranteed to hold a target for every flow.
//
// The destinations are named rather than counted because they are also the run's
// answer to "which translations on disk did I write", which is what entitles it
// to record a basis for the units inside them (host/basisrecord.go).
func (a *App) deliverDrafts(locale model.LocaleID) ([]string, error) {
	if a.convergeDraftDir == "" || a.convergeDraftRoot == "" {
		return nil, nil
	}
	base := filepath.Join(a.convergeDraftDir, string(locale))
	if _, err := os.Stat(base); err != nil {
		return nil, nil
	}
	var delivered []string
	err := filepath.WalkDir(base, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(base, path)
		if rerr != nil {
			return rerr
		}
		dest := filepath.Join(a.convergeDraftRoot, rel)
		if merr := os.MkdirAll(filepath.Dir(dest), 0o755); merr != nil {
			return fmt.Errorf("deliver %s: %w", rel, merr)
		}
		if rnerr := os.Rename(path, dest); rnerr != nil {
			// A cross-device draft tree (a project whose state dir is elsewhere)
			// cannot be renamed into place; copy the bytes instead.
			body, readErr := os.ReadFile(path)
			if readErr != nil {
				return fmt.Errorf("deliver %s: %w", rel, rnerr)
			}
			if writeErr := os.WriteFile(dest, body, 0o644); writeErr != nil {
				return fmt.Errorf("deliver %s: %w", rel, writeErr)
			}
		}
		delivered = append(delivered, dest)
		return nil
	})
	return delivered, err
}
