package cli

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/neokapi/neokapi/core/project"
)

// ConvergenceReport is the full derived convergence picture for a project: the
// per-(collection,locale) target coverage and ship-gate standing, the source
// authoring readiness, and the review queue. It is exactly what `kapi status`,
// `kapi status --review`, and `kapi verify --ship` derive — one shape, so an
// embedding surface (the Kapi Desktop project view) shows the same numbers the
// CLI does rather than a parallel computation.
type ConvergenceReport struct {
	Project string           `json:"project,omitempty"`
	Source  *SourceCoverage  `json:"source,omitempty"`
	Locales []LocaleCoverage `json:"locales"`
	Review  []ReviewItem     `json:"review"`
}

// ProjectConvergence computes the convergence report for a project recipe,
// deriving everything from the working tree (content × target files) and the
// committed .klftm corrections — the same file-based derivation the CLI status
// and verify commands use, so an in-process caller (the desktop) agrees with
// `kapi status` to the unit. sourceLang overrides the project's source language
// when non-empty.
//
// It is read-only and self-contained: it initialises the registries, loads the
// recipe, resolves content units, and rolls up coverage, source readiness, and
// the review queue. State is derived on every call (nothing is cached), so the
// report is always current with the files on disk.
func (a *App) ProjectConvergence(ctx context.Context, projectPath, sourceLang string) (*ConvergenceReport, error) {
	a.InitRegistries()
	if ctx == nil {
		ctx = context.Background()
	}

	proj, err := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return nil, fmt.Errorf("load project: %w", err)
	}
	root := filepath.Dir(projectPath)

	if sourceLang == "" {
		sourceLang = string(proj.Defaults.SourceLanguage)
	}
	if sourceLang == "" {
		sourceLang = "en"
	}
	a.SourceLang = sourceLang

	units, err := a.unitsFromProject(proj, root, "")
	if err != nil {
		return nil, fmt.Errorf("resolve content: %w", err)
	}

	cov, err := a.computeShipCoverage(ctx, proj, root, units)
	if err != nil {
		return nil, fmt.Errorf("compute coverage: %w", err)
	}
	src, err := a.computeSourceReadiness(ctx, proj, units)
	if err != nil {
		return nil, fmt.Errorf("compute source readiness: %w", err)
	}
	review, err := a.computeReviewQueue(ctx, proj, root, units)
	if err != nil {
		return nil, fmt.Errorf("compute review queue: %w", err)
	}

	report := &ConvergenceReport{Project: proj.Name, Locales: cov, Review: review}
	if src.Total > 0 {
		report.Source = &src
	}
	return report, nil
}
