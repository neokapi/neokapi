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
	return a.ApplyReviewDecisionAs(ctx, projectPath, sourceLang, ref, decision, note, "")
}

// ApplyReviewDecisionAs is ApplyReviewDecision with an explicit decider
// identity. by is recorded as the decision's Decision.By: empty for a plain
// human decision (the single-player default — unchanged behavior),
// "ai/<model>" for an autonomous AI approval (pre-review auto-approve), or
// "agent/<client>" for an MCP agent acting on a person's behalf. Gate
// evaluation treats only "ai/…" decisions specially (core/gate approver
// classes).
func (a *App) ApplyReviewDecisionAs(ctx context.Context, projectPath, sourceLang string, ref ReviewUnitRef, decision, note, by string) (bool, error) {
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
			return a.recordDecisionState(proj, root, blockKey(b), loc, target, status, decision, note, by)
		}
	}
	return false, fmt.Errorf("review unit %q (%s) not found in %s", ref.Key, ref.Locale, ref.File)
}

// recordDecisionState records a unit's review decision in the project state store
// — the authoritative carrier of workflow state — keyed by unit identity + locale,
// bound to the content hash of the translation it judges so a later edit drops
// the decision (stale). The decision is transient until Export persists it to the
// committed state artifact (the export sink). The TM (.klftm) is no longer
// touched here: it is the recycle corpus, not the state carrier. Advisory fields
// already on the unit's record (origin, source status, a fresh AI pre-review
// annotation) survive the decision write.
func (a *App) recordDecisionState(proj *project.KapiProject, root, unit string, locale model.LocaleID, target string, status model.TargetStatus, decision, note, by string) (bool, error) {
	st, err := openProjectState(proj, root)
	if err != nil {
		return false, err
	}
	k := state.Key{Unit: unit, Variant: model.Variant(locale)}
	th := targetHash(target)
	prev, hadPrev := st.Get(k)
	if hadPrev && prev.Status == status && prev.TargetHash == th && prev.Decision.Note == note && prev.Decision.By == by {
		return false, nil // already at this decision for this exact translation
	}
	now := nowRFC3339()
	next := state.UnitState{
		Unit:       unit,
		Variant:    model.Variant(locale),
		Status:     status,
		TargetHash: th,
		Decision:   state.Decision{ReviewState: decision, By: by, At: now, Note: note},
		Updated:    now,
	}
	if hadPrev {
		// Advisory state rides along; only the decision itself is replaced.
		next.Origin = prev.Origin
		next.SourceStatus = prev.SourceStatus
		if prev.AIReview.Fresh(th) {
			next.AIReview = prev.AIReview
		}
	}
	st.Put(next)
	if err := st.Export(); err != nil {
		return false, err
	}
	return true, nil
}

// RecordAIReviews stores advisory AI pre-review annotations for units of one
// (file, locale) review scope in the project state store: for each unit key in
// reviews, the annotation is bound to the content hash of the unit's CURRENT
// translation (re-read from the target file), so a later edit invalidates it.
// Annotations never move a unit on the ladder — any existing decision, origin,
// and status ride along untouched. It returns the number of units annotated;
// unit keys that no longer resolve are skipped (content moved on), not errors.
func (a *App) RecordAIReviews(ctx context.Context, projectPath, sourceLang, locale, file string, reviews map[string]state.AIReview) (int, error) {
	a.InitRegistries()
	if ctx == nil {
		ctx = context.Background()
	}
	if len(reviews) == 0 {
		return 0, nil
	}
	proj, err := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return 0, fmt.Errorf("load project: %w", err)
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
		return 0, fmt.Errorf("resolve content: %w", err)
	}

	st, err := openProjectState(proj, root)
	if err != nil {
		return 0, err
	}

	recorded := 0
	loc := model.LocaleID(locale)
	for _, u := range units {
		if u.locale != locale || u.displayPath != file {
			continue
		}
		blocks, missing, berr := a.bilingualBlocks(ctx, u)
		if berr != nil {
			if errors.Is(berr, errTargetUnreadable) {
				continue
			}
			return recorded, berr
		}
		if missing {
			continue
		}
		for _, b := range blocks {
			if !b.Translatable {
				continue
			}
			rev, ok := reviews[blockKey(b)]
			if !ok {
				continue
			}
			rev.TargetHash = targetHash(b.TargetText(loc))
			if rev.At == "" {
				rev.At = nowRFC3339()
			}
			k := state.Key{Unit: blockKey(b), Variant: model.Variant(loc)}
			us, _ := st.Get(k)
			us.Unit = blockKey(b)
			us.Variant = model.Variant(loc)
			r := rev
			us.AIReview = &r
			us.Updated = rev.At
			st.Put(us)
			recorded++
		}
	}
	if recorded > 0 {
		if err := st.Export(); err != nil {
			return recorded, err
		}
	}
	return recorded, nil
}

// ReviewUnitInfo is the CLI-layer picture of one review-queue unit — the
// agent-facing counterpart of the desktop's richer detail view: full source and
// target text, the effective ladder state, the recorded decision (with its
// identity), and any fresh AI pre-review annotation. Derived on demand from the
// content files and the project state store; nothing is cached.
type ReviewUnitInfo struct {
	Locale     string `json:"locale"`
	File       string `json:"file"`
	Key        string `json:"key"`
	Collection string `json:"collection,omitempty"`
	Source     string `json:"source"`
	Target     string `json:"target"`
	// Status is the unit's effective ladder state (draft|translated|reviewed|
	// signed-off), with a fresh state-store decision applied over the presence
	// baseline.
	Status string `json:"status"`
	// ReviewState/Note/By carry the last recorded decision when it still judges
	// the current translation.
	ReviewState string `json:"review_state,omitempty"`
	Note        string `json:"note,omitempty"`
	By          string `json:"by,omitempty"`
	// AIScore/AIModel/AIFindings surface a fresh AI pre-review annotation.
	AIScore    *int                    `json:"ai_score,omitempty"`
	AIModel    string                  `json:"ai_model,omitempty"`
	AIFindings []state.AIReviewFinding `json:"ai_findings,omitempty"`
}

// ReviewUnit resolves one review-queue unit by (file, key, locale) — exactly as
// `kapi status --review` lists it — and returns its full text and recorded
// state. It is the read leg agents pair with ApplyReviewDecisionAs.
func (a *App) ReviewUnit(ctx context.Context, projectPath, sourceLang string, ref ReviewUnitRef) (*ReviewUnitInfo, error) {
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

	units, err := a.unitsFromProject(proj, root, ref.Locale)
	if err != nil {
		return nil, fmt.Errorf("resolve content: %w", err)
	}

	loc := model.LocaleID(ref.Locale)
	for _, u := range units {
		if u.locale != ref.Locale || u.displayPath != ref.File {
			continue
		}
		blocks, missing, berr := a.bilingualBlocks(ctx, u)
		if berr != nil {
			if errors.Is(berr, errTargetUnreadable) {
				continue
			}
			return nil, berr
		}
		if missing {
			continue
		}
		for _, b := range blocks {
			if !b.Translatable || blockKey(b) != ref.Key {
				continue
			}
			info := &ReviewUnitInfo{
				Locale:     ref.Locale,
				File:       ref.File,
				Key:        ref.Key,
				Collection: u.collection,
				Source:     b.SourceText(),
				Target:     b.TargetText(loc),
				Status:     unitState(b, ref.Locale),
			}
			st, serr := openProjectState(proj, root)
			if serr != nil {
				return nil, serr
			}
			if us, found := st.Get(state.Key{Unit: ref.Key, Variant: model.Variant(loc)}); found {
				th := targetHash(info.Target)
				if !us.Stale(th) {
					if us.Status != "" {
						info.Status = string(us.Status)
					}
					info.ReviewState = us.Decision.ReviewState
					info.Note = us.Decision.Note
					info.By = us.Decision.By
				}
				if us.AIReview.Fresh(th) {
					score := us.AIReview.Score
					info.AIScore = &score
					info.AIModel = us.AIReview.Model
					info.AIFindings = us.AIReview.Findings
				}
			}
			return info, nil
		}
	}
	return nil, fmt.Errorf("review unit %q (%s) not found in %s", ref.Key, ref.Locale, ref.File)
}
