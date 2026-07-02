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
		cov, err := a.computeShipCoverage(ctx, proj, root, units, nil)
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

// Review decisions a human (or agent) can record for a review-queue unit. They
// map onto the target ladder: approved → reviewed, signed-off → signed-off, and
// rejected → draft (the unit re-enters the work queue, with an optional note
// explaining why).
const (
	ReviewDecisionApproved  = "approved"
	ReviewDecisionRejected  = "rejected"
	ReviewDecisionSignedOff = "signed-off"
)

// ReviewUnitRef addresses one review-queue unit by (file, key, locale) exactly
// as the queue lists it: file is the target's display path (relative to the
// project root), key the block's stable unit key, locale the target locale.
type ReviewUnitRef struct {
	File   string
	Key    string
	Locale string
}

// decisionStatus maps a review decision to the target-ladder status it records.
func decisionStatus(decision string) (model.TargetStatus, error) {
	switch decision {
	case ReviewDecisionApproved:
		return model.TargetStatusReviewed, nil
	case ReviewDecisionSignedOff:
		return model.TargetStatusSignedOff, nil
	case ReviewDecisionRejected:
		return model.TargetStatusDraft, nil
	default:
		return "", fmt.Errorf("unknown review decision %q: want %s, %s, or %s",
			decision, ReviewDecisionApproved, ReviewDecisionRejected, ReviewDecisionSignedOff)
	}
}

// ApproveReviewUnit promotes one review-queue unit by recording its review
// decision in the project STATE store (core/state). It is the approval-only
// veneer over ApplyReviewDecision, kept for callers that speak the ladder
// vocabulary: reviewState is "reviewed" (the default approval) or "signed-off"
// (the final sign-off, the top rung). An empty string means reviewed.
func (a *App) ApproveReviewUnit(ctx context.Context, projectPath, sourceLang, locale, file, key, reviewState string) (bool, error) {
	decision := ReviewDecisionApproved
	if reviewState == string(model.TargetStatusSignedOff) {
		decision = ReviewDecisionSignedOff
	}
	return a.ApplyReviewDecision(ctx, projectPath, sourceLang, ReviewUnitRef{File: file, Key: key, Locale: locale}, decision, "")
}

// ApplyReviewDecision records a review decision for one review-queue unit in the
// project STATE store (core/state) — the authoritative carrier of workflow state,
// keyed by unit identity + locale and bound to the content hash of the
// translation it judges, so a later edit invalidates a stale decision. The unit
// is addressed by (file, key, locale) as listed in the review queue; the method
// re-reads the exact target text before recording. The decision is exported to
// the committed state artifact (defaults.state) — distinct from the `.klftm`,
// which stays the recycle corpus.
//
// decision is one of ReviewDecisionApproved (→ reviewed), ReviewDecisionSignedOff
// (→ signed-off), or ReviewDecisionRejected (→ draft: the unit drops out of the
// review queue and re-enters the work queue, carrying note as the reviewer's
// reason). An edit to the translation after any decision makes it stale, so the
// unit re-derives from its content (a rejected unit re-enters review once it is
// retranslated).
//
// It returns changed=false (no error) when the unit is already at this decision
// for this exact translation, so an embedder can treat a redundant click as a
// no-op.
func (a *App) ApplyReviewDecision(ctx context.Context, projectPath, sourceLang string, ref ReviewUnitRef, decision, note string) (bool, error) {
	a.InitRegistries()
	if ctx == nil {
		ctx = context.Background()
	}
	status, err := decisionStatus(decision)
	if err != nil {
		return false, err
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

	units, err := a.unitsFromProject(proj, root, ref.Locale)
	if err != nil {
		return false, fmt.Errorf("resolve content: %w", err)
	}

	for _, u := range units {
		if u.locale != ref.Locale || u.displayPath != ref.File {
			continue
		}
		blocks, missing, berr := a.bilingualBlocks(ctx, u)
		if berr != nil {
			if errors.Is(berr, errTargetUnreadable) {
				continue // unreadable target (e.g. a compiled .mo) — not decidable per unit
			}
			return false, berr
		}
		if missing {
			continue
		}
		loc := model.LocaleID(ref.Locale)
		for _, b := range blocks {
			if !b.Translatable || blockKey(b) != ref.Key {
				continue
			}
			target := b.TargetText(loc)
			if status != model.TargetStatusDraft && strings.TrimSpace(target) == "" {
				return false, fmt.Errorf("unit %s has no %s translation to approve", ref.Key, ref.Locale)
			}
			return a.recordDecisionState(proj, root, blockKey(b), loc, target, status, decision, note)
		}
	}
	return false, fmt.Errorf("review unit %q (%s) not found in %s", ref.Key, ref.Locale, ref.File)
}

// recordDecisionState records a unit's review decision in the project state store
// — the authoritative carrier of workflow state — keyed by unit identity + locale,
// bound to the content hash of the translation it judges so a later edit drops
// the decision (stale). The decision is transient until Export persists it to the
// committed state artifact (the export sink). The TM (.klftm) is no longer
// touched here: it is the recycle corpus, not the state carrier.
func (a *App) recordDecisionState(proj *project.KapiProject, root, unit string, locale model.LocaleID, target string, status model.TargetStatus, decision, note string) (bool, error) {
	st, err := openProjectState(proj, root)
	if err != nil {
		return false, err
	}
	k := state.Key{Unit: unit, Variant: model.Variant(locale)}
	th := targetHash(target)
	if prev, ok := st.Get(k); ok && prev.Status == status && prev.TargetHash == th && prev.Decision.Note == note {
		return false, nil // already at this decision for this exact translation
	}
	now := nowRFC3339()
	st.Put(state.UnitState{
		Unit:       unit,
		Variant:    model.Variant(locale),
		Status:     status,
		TargetHash: th,
		Decision:   state.Decision{ReviewState: decision, At: now, Note: note},
		Updated:    now,
	})
	if err := st.Export(); err != nil {
		return false, err
	}
	return true, nil
}
