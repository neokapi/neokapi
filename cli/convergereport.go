package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/model"
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

	var report *ConvergenceReport
	cacheErr := a.withParseCache(root, func() error {
		cov, err := a.computeShipCoverage(ctx, proj, root, units)
		if err != nil {
			return fmt.Errorf("compute coverage: %w", err)
		}
		src, err := a.computeSourceReadiness(ctx, proj, units)
		if err != nil {
			return fmt.Errorf("compute source readiness: %w", err)
		}
		review, err := a.computeReviewQueue(ctx, proj, root, units)
		if err != nil {
			return fmt.Errorf("compute review queue: %w", err)
		}
		report = &ConvergenceReport{Project: proj.Name, Locales: cov, Review: review}
		if src.Total > 0 {
			report.Source = &src
		}
		return nil
	})
	if cacheErr != nil {
		return nil, cacheErr
	}
	return report, nil
}

// ApproveReviewUnit promotes one review-queue unit to `reviewed` by recording its
// current source→target translation as an approved correction in the project's
// committed .klftm — exactly what `kapi apply` (a tm correction) does, the same
// review record `kapi status --review` derives from. The unit is addressed by
// (locale, file, key) as listed in the review queue: the method re-reads the
// exact source and target text (not the truncated previews) before writing, so
// the approved pair is precise.
//
// It returns approved=false (no error) when the pair is already an approved
// correction, so an embedder can treat a redundant click as a no-op. Binding the
// .klftm source on first use writes `defaults.tm_source` into the recipe, the
// same one-time effect as the CLI apply path.
// reviewState is "reviewed" (the default approval) or "signed-off" (the final
// sign-off, the top rung). An empty string means reviewed.
func (a *App) ApproveReviewUnit(ctx context.Context, projectPath, sourceLang, locale, file, key, reviewState string) (bool, error) {
	a.InitRegistries()
	if ctx == nil {
		ctx = context.Background()
	}
	proj, err := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return false, fmt.Errorf("load project: %w", err)
	}
	root := filepath.Dir(projectPath)
	if sourceLang == "" {
		sourceLang = string(proj.Defaults.SourceLanguage)
	}
	if sourceLang == "" {
		sourceLang = "en"
	}
	a.SourceLang = sourceLang

	units, err := a.unitsFromProject(proj, root, locale)
	if err != nil {
		return false, fmt.Errorf("resolve content: %w", err)
	}

	for _, u := range units {
		if u.locale != locale || u.displayPath != file {
			continue
		}
		blocks, missing, berr := a.bilingualBlocks(ctx, u)
		if berr != nil {
			if errors.Is(berr, errTargetUnreadable) {
				continue // unreadable target (e.g. a compiled .mo) — not approvable per unit
			}
			return false, berr
		}
		if missing {
			continue
		}
		loc := model.LocaleID(locale)
		for _, b := range blocks {
			if !b.Translatable || blockKey(b) != key {
				continue
			}
			source := b.SourceText()
			target := b.TargetText(loc)
			if strings.TrimSpace(target) == "" {
				return false, fmt.Errorf("unit %s has no %s translation to approve", key, locale)
			}
			return a.recordApprovedCorrection(ctx, projectPath, root, source, target, sourceLang, locale, reviewState)
		}
	}
	return false, fmt.Errorf("review unit %q (%s) not found in %s", key, locale, file)
}

// recordApprovedCorrection writes one approved source→target pair into the
// project's committed .klftm and recompiles the cache — the shared core of the
// `kapi apply` tm path, reused so approval and apply write the corpus one way.
func (a *App) recordApprovedCorrection(ctx context.Context, projectPath, root, source, target, sourceLang, targetLang, reviewState string) (bool, error) {
	srcPath, err := a.ensureTMSourceBinding(projectPath, root)
	if err != nil {
		return false, err
	}
	entries, err := loadKLFTMEntries(srcPath)
	if err != nil {
		return false, err
	}
	entries, changed := upsertTMPair(entries, source, target, model.LocaleID(sourceLang), model.LocaleID(targetLang), reviewState)
	if !changed {
		return false, nil // already an approved correction
	}
	if err := writeKLFTM(srcPath, entries); err != nil {
		return false, err
	}
	if err := a.compileTMSource(ctx, root, srcPath); err != nil {
		return false, err
	}
	return true, nil
}
