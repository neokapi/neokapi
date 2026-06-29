package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/state"
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

// ApproveReviewUnit promotes one review-queue unit by recording its review
// decision in the project STATE store (core/state) — the authoritative carrier of
// workflow state, keyed by unit identity + locale and bound to the content hash of
// the translation it blesses, so a later edit invalidates a stale approval. The
// unit is addressed by (locale, file, key) as listed in the review queue; the
// method re-reads the exact target text before recording. The decision is exported
// to the committed state artifact (defaults.state) — distinct from the `.klftm`,
// which stays the recycle corpus.
//
// It returns approved=false (no error) when the unit is already at this review
// state for this exact translation, so an embedder can treat a redundant click as
// a no-op. reviewState is "reviewed" (the default approval) or "signed-off" (the
// final sign-off, the top rung). An empty string means reviewed.
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
			target := b.TargetText(loc)
			if strings.TrimSpace(target) == "" {
				return false, fmt.Errorf("unit %s has no %s translation to approve", key, locale)
			}
			return a.recordApprovedState(proj, root, blockKey(b), loc, target, reviewState)
		}
	}
	return false, fmt.Errorf("review unit %q (%s) not found in %s", key, locale, file)
}

// recordApprovedState records a unit's review decision in the project state store
// — the authoritative carrier of workflow state — keyed by unit identity + locale,
// bound to the content hash of the translation it blesses so a later edit drops
// the unit back down the ladder. The decision is transient until Export persists
// it to the committed state artifact (the export sink). The TM (.klftm) is no
// longer touched here: it is the recycle corpus, not the state carrier.
func (a *App) recordApprovedState(proj *project.KapiProject, root, unit string, locale model.LocaleID, target, reviewState string) (bool, error) {
	st, err := openProjectState(proj, root)
	if err != nil {
		return false, err
	}
	status := model.TargetStatusReviewed
	if reviewState == string(model.TargetStatusSignedOff) {
		status = model.TargetStatusSignedOff
	}
	k := state.Key{Unit: unit, Variant: model.Variant(locale)}
	th := targetHash(target)
	if prev, ok := st.Get(k); ok && prev.Status == status && prev.TargetHash == th {
		return false, nil // already at this review state for this exact translation
	}
	now := nowRFC3339()
	st.Put(state.UnitState{
		Unit:       unit,
		Variant:    model.Variant(locale),
		Status:     status,
		TargetHash: th,
		Decision:   state.Decision{ReviewState: firstNonEmpty(reviewState, "approved"), At: now},
		Updated:    now,
	})
	if err := st.Export(); err != nil {
		return false, err
	}
	return true, nil
}
