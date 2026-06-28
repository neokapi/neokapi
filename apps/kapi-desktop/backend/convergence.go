package backend

import (
	"context"
	"errors"
	"fmt"

	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/project"
)

// convergenceCLI lazily builds the shared cli.App used to derive convergence
// reports, so the format/tool registries register once rather than per request.
func (a *App) convergenceCLI() *cli.App {
	a.convergenceMu.Lock()
	defer a.convergenceMu.Unlock()
	if a.convergence == nil {
		c := &cli.App{}
		c.InitRegistries()
		a.convergence = c
	}
	return a.convergence
}

// GetConvergence returns the derived convergence report for a project tab: the
// per-(collection, locale) target coverage and ship-gate standing, the source
// authoring readiness, and the review queue. It is the same file-based
// derivation `kapi status`, `kapi status --review`, and `kapi verify --ship`
// report, computed in-process so the desktop and the CLI agree to the unit.
//
// A project with no recipe path yet (unsaved) returns an empty report rather
// than an error, so the panel renders a "nothing tracked yet" state.
func (a *App) GetConvergence(tabID string) (*cli.ConvergenceReport, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	if op.Project == nil || op.Path == "" {
		return &cli.ConvergenceReport{}, nil
	}
	src := string(op.Project.Defaults.SourceLanguage)
	return a.convergenceCLI().ProjectConvergence(context.Background(), op.Path, src)
}

// BringUpToDate runs the project's default flow (defaults.flow) over all of its
// content across every target language — one convergence pass that materializes
// the localized files. It returns once the run is launched; progress streams
// through the same run-event channel as any flow, and the frontend refreshes
// GetConvergence when the run completes. Whatever the pass can't carry to the
// ship gate stays in the review queue (the desktop's "parked" surface).
func (a *App) BringUpToDate(tabID string) error {
	op := a.getOpenProject(tabID)
	if op == nil {
		return fmt.Errorf("project tab %q not found", tabID)
	}
	if op.Project == nil {
		return errors.New("project has no recipe loaded")
	}

	flowName := op.Project.Defaults.Flow
	if flowName == "" {
		return errors.New("no default flow configured — set defaults.flow in the project, or run a flow from the Flows page")
	}

	pctx := project.NewProjectContext(op.Project, op.Path)
	resolved, err := pctx.ResolveContent(a.formatReg)
	if err != nil {
		return fmt.Errorf("resolve content: %w", err)
	}
	if len(resolved) == 0 {
		return errors.New("no content matched the project's content patterns")
	}
	paths := make([]string, 0, len(resolved))
	for _, rf := range resolved {
		paths = append(paths, rf.Path)
	}

	targets := make([]string, 0, len(op.Project.Defaults.TargetLanguages))
	for _, loc := range op.Project.Defaults.TargetLanguages {
		targets = append(targets, string(loc))
	}
	if len(targets) == 0 {
		return errors.New("no target languages configured (defaults.target_languages)")
	}

	return a.RunFlow(tabID, flowName, paths, targets)
}

// ApproveReviewItem promotes one review-queue unit to `reviewed`: it records the
// unit's current source→target translation as an approved correction in the
// project's committed .klftm (the same write `kapi apply` makes). After it
// returns, GetConvergence shows the unit reviewed and it leaves the queue. The
// unit is addressed by (locale, file, key) as listed in the review queue.
func (a *App) ApproveReviewItem(tabID, locale, file, key string) error {
	op := a.getOpenProject(tabID)
	if op == nil {
		return fmt.Errorf("project tab %q not found", tabID)
	}
	if op.Project == nil || op.Path == "" {
		return errors.New("project has no recipe loaded")
	}
	src := string(op.Project.Defaults.SourceLanguage)
	_, err := a.convergenceCLI().ApproveReviewUnit(context.Background(), op.Path, src, locale, file, key, "reviewed")
	return err
}
