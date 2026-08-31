package backend

import (
	"context"
	"errors"
	"fmt"
	"github.com/neokapi/neokapi/core/format"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/state"
	coretools "github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
)

// ReviewUnitDetail is the full picture of one review-queue unit for the Review
// page: the rendered source and target text, the unit's check findings (the
// registered checkers run on demand for just this block), the recorded review
// state and provenance from the project state store, and the best content-memory match when
// the project content memory is open. It is read-derived — nothing here is cached.
type ReviewUnitDetail struct {
	Locale     string `json:"locale"`
	File       string `json:"file"`
	Key        string `json:"key"`
	Collection string `json:"collection,omitempty"`
	// Source and Target are the unit's rendered text (v1: plain text, not runs).
	Source string `json:"source"`
	Target string `json:"target"`
	// SourceLocale is the project's source language. The reviewer renders the
	// source and target panes in their own writing direction, so both locales
	// have to reach the UI — a right-to-left source (or target) otherwise
	// inherits the app's direction and reads scrambled.
	SourceLocale string `json:"source_locale,omitempty"`
	// Status is the unit's effective ladder state (draft|translated|reviewed|
	// signed-off), with a fresh state-store decision applied over the presence
	// baseline.
	Status string `json:"status"`
	// ReviewState/Note carry the last recorded decision when it still judges
	// the current translation (approved | rejected | signed-off).
	ReviewState string `json:"review_state,omitempty"`
	Note        string `json:"note,omitempty"`
	// Origin is the target's provenance when known (from the state store).
	Origin *model.Origin `json:"origin,omitempty"`
	// MemoryScore is the best content-memory match percent for the source (0 = none found or
	// no project content memory open).
	MemoryScore int `json:"tm_score,omitempty"`
	// Findings are the unit's current check findings (placeholder integrity,
	// do-not-translate, voice vocabulary — the same checkers the Checks panel
	// runs, scoped to this one block).
	Findings []DesktopFinding `json:"findings"`
	// AIReviewScore/AIReviewModel surface a fresh AI pre-review annotation from
	// the state store (read-derived; loading a unit never calls a provider).
	AIReviewScore *int   `json:"ai_review_score,omitempty"`
	AIReviewModel string `json:"ai_review_model,omitempty"`
	// Editable reports whether the target is a single plain-text run, the only
	// shape UpdateReviewTarget can rewrite safely.
	Editable bool `json:"editable"`
}

// expandReviewTargetPath mirrors host.expandTargetTemplate: {lang} → locale and
// "*" → the source basename without extension, rooted at the project directory.
// The review queue's File field is produced by that exact expansion (relative
// to the root), so the desktop must expand identically to find the unit again.
func expandReviewTargetPath(tmpl, sourceRel, locale, root string) string {
	out := strings.ReplaceAll(tmpl, "{lang}", locale)
	base := format.Stem(sourceRel)
	out = strings.ReplaceAll(out, "*", base)
	if !filepath.IsAbs(out) {
		out = filepath.Join(root, out)
	}
	return out
}

// findReviewSource locates the resolved content file whose expanded target path
// (for the locale) matches a review item's File, returning the source file and
// the absolute target path.
func (a *App) findReviewSource(op *openProject, locale, file string) (project.ResolvedFile, string, error) {
	pctx := project.NewProjectContext(op.Project, op.Path)
	resolved, err := pctx.ResolveContent(a.formatReg)
	if err != nil {
		return project.ResolvedFile{}, "", fmt.Errorf("resolve content: %w", err)
	}
	root := filepath.Dir(op.Path)
	for _, rf := range resolved {
		if rf.Item == nil || rf.Item.Target == "" {
			continue
		}
		tgt := expandReviewTargetPath(rf.Item.Target, rf.Relative, locale, root)
		rel, rerr := filepath.Rel(root, tgt)
		if rerr != nil {
			rel = tgt
		}
		if rel == file || tgt == file {
			return rf, tgt, nil
		}
	}
	return project.ResolvedFile{}, "", fmt.Errorf("no content file resolves to target %q (%s)", file, locale)
}

// reviewUnitBlocks reads a review unit's source file, overlays its target file
// for the locale, and returns the blocks keyed by their stable unit key — the
// same source-block+target-overlay pairing RunChecks measures.
func (a *App) reviewUnitBlocks(ctx context.Context, op *openProject, rf project.ResolvedFile, tgtPath, locale string) (map[string]*model.Block, error) {
	pctx := project.NewProjectContext(op.Project, op.Path)
	sourceLang := string(pctx.SourceLocale)
	// Source and target are two renderings of one item, so both are read under
	// the format and reader config the recipe declares for it.
	fmtCfg := pctx.FormatConfigFor(rf.Format, rf.Item)
	passBlocks, err := a.readBlocksForChecks(ctx, rf.Path, rf.Format, fmtCfg, sourceLang)
	if err != nil {
		return nil, err
	}
	if _, serr := os.Stat(tgtPath); serr != nil {
		return nil, fmt.Errorf("target file %q not found: %w", tgtPath, serr)
	}
	targetBlocks, err := a.readBlocksForChecks(ctx, tgtPath, rf.Format, fmtCfg, sourceLang)
	if err != nil {
		return nil, err
	}
	host.OverlayTargets(passBlocks, targetBlocks, model.LocaleID(locale))
	byKey := make(map[string]*model.Block, len(passBlocks))
	for _, b := range passBlocks {
		if b.Translatable {
			byKey[convergence.BlockKey(b)] = b
		}
	}
	return byKey, nil
}

// blockCheckFindings runs the registered content checkers over one block for a
// locale — voice vocabulary on the source when a profile is bound, placeholder
// integrity and do-not-translate on the target — the ChecksPanel checkset
// scoped to a single unit.
//
// A checker that cannot RUN becomes a `major` "checks did not complete" finding
// rather than being dropped. Dropping it made every caller fail unsafe: the
// review pane showed a clean unit, HasFindings read false, and — worst —
// review_ai's AutoApprove consults hasBlockingCheckFinding on this very slice, so
// a checker that errored would have auto-approved a translation whose placeholder
// integrity was never verified. A synthetic blocking finding is honest at the
// panel and fail-safe at the gate, and keeps the signature usable from the paths
// that legitimately continue past one bad unit.
func (a *App) blockCheckFindings(ctx context.Context, b *model.Block, locale model.LocaleID, profile *coreprofile.VoiceProfile, tb terms.Terminology, dntTerms []string) []DesktopFinding {
	findings := []DesktopFinding{}
	// The review surface addresses a unit, and carries its own scope in the
	// queue rather than on each finding, so the point stays unset here.
	var point ContextPointDTO
	fail := func(what string, err error) []DesktopFinding {
		return append(findings, DesktopFinding{
			Category: "check",
			Severity: string(check.SeverityMajor),
			Message:  fmt.Sprintf("checks did not complete: %s: %v", what, err),
			BlockID:  b.ID,
		})
	}

	if profile != nil {
		vocab := coretools.NewVoiceVocabCheckTool(profile, tb)
		if err := host.RunCheckTool(ctx, vocab, b); err != nil {
			return fail("voice vocabulary", err)
		}
		if ann, ok := model.AnnoAs[*coreprofile.VoiceAnnotation](b, "voice"); ok {
			for _, f := range ann.Findings {
				findings = append(findings, toDesktopFinding(f, b, "source", point))
			}
			b.DelAnno("voice")
		}
	}

	placeholder := coretools.NewPlaceholderCheckTool(coretools.NewPlaceholderCheckConfig(locale))
	if err := host.RunCheckTool(ctx, placeholder, b); err != nil {
		return fail("placeholder", err)
	}
	for _, f := range host.FindingsFromBlock(b, true) {
		findings = append(findings, toDesktopFinding(f, b, "target", point))
	}

	if len(dntTerms) > 0 {
		dntCfg := coretools.NewDNTCheckConfig(locale)
		dntCfg.Terms = dntTerms
		dnt := coretools.NewDNTCheckTool(dntCfg)
		if err := host.RunCheckTool(ctx, dnt, b); err != nil {
			return fail("do-not-translate", err)
		}
		for _, f := range host.FindingsFromBlock(b, true) {
			findings = append(findings, toDesktopFinding(f, b, "target", point))
		}
	}

	sortDesktopFindings(findings)
	return findings
}

// GetReviewUnit returns the full review picture for one queue unit, addressed by
// (locale, file, key) exactly as the review queue lists it: rendered source and
// target text, the unit's current check findings (checkers run on demand for
// this block), the recorded decision + provenance from the project state store,
// and the best content-memory match percent when the project content memory is open.
func (a *App) GetReviewUnit(tabID, locale, file, key string) (*ReviewUnitDetail, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	if op.Project == nil || op.Path == "" {
		return nil, errors.New("project has no recipe loaded")
	}

	loc, err := canonicalLocale(locale)
	if err != nil {
		return nil, err
	}
	locale = string(loc)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rf, tgtPath, err := a.findReviewSource(op, locale, file)
	if err != nil {
		return nil, err
	}
	byKey, err := a.reviewUnitBlocks(ctx, op, rf, tgtPath, locale)
	if err != nil {
		return nil, err
	}
	b, ok := byKey[key]
	if !ok {
		return nil, fmt.Errorf("unit %q not found in %s", key, file)
	}

	sourceLang := string(project.NewProjectContext(op.Project, op.Path).SourceLocale)
	targetText := b.TargetText(loc)

	detail := &ReviewUnitDetail{
		Locale:       locale,
		File:         file,
		Key:          key,
		Collection:   rf.Collection,
		Source:       b.SourceText(),
		Target:       targetText,
		SourceLocale: sourceLang,
		Status:       string(model.TargetStatusTranslated),
		Editable:     targetEditable(b, loc),
	}
	if strings.TrimSpace(targetText) == "" {
		detail.Status = ""
	}

	// Findings — the ChecksPanel checkset scoped to this block, judged by the
	// voice governing the point this unit's source file sits at.
	points := a.newPointResolver(op, false)
	profile := points.at(ctx, rf.Collection, rf.Relative)
	dntTerms := a.resolveProjectDNTTerms(ctx, op, sourceLang)
	detail.Findings = a.blockCheckFindings(ctx, b, loc, profile,
		points.termsAt(ctx, rf.Collection, rf.Relative), dntTerms)

	// Recorded decision + provenance from the project state store, when the
	// decision still judges the current translation.
	// The working store is a schema of the project's one store, so this is the
	// engine's handle, not a second opener — and closing it is not ours to do.
	root := filepath.Dir(op.Path)
	if st, serr := a.hostEngine().OpenProjectState(ctx, root); serr == nil {
		scope := a.hostEngine().DocumentScope(ctx, root, rf.Path)
		if us, found := st.Get(ctx, state.Key{Scope: scope, Unit: key, Variant: model.Variant(loc)}); found {
			th := project.HashBytes([]byte(strings.TrimSpace(targetText)))
			if !us.Stale(th) {
				if us.Status != "" {
					detail.Status = string(us.Status)
				}
				detail.ReviewState = us.Decision.ReviewState
				detail.Note = us.Decision.Note
			}
			if us.Origin.Kind != "" {
				o := us.Origin
				detail.Origin = &o
			}
			// Fresh AI pre-review annotation → CONTEXT row ("AI review: 92 (model)").
			if us.AIReview.Fresh(th) {
				score := us.AIReview.Score
				detail.AIReviewScore = &score
				detail.AIReviewModel = us.AIReview.Model
			}
		}
	}
	// Block-side provenance wins when the format carries it.
	if t := b.Target(loc); t != nil && t.Origin.Kind != "" {
		o := t.Origin
		detail.Origin = &o
	}

	// Best content-memory match, when the project content memory is already open — an existing lookup,
	// not a new content memory API.
	if op.memoryHandle != "" {
		if tm, tok := a.memoryHandles.Get(op.memoryHandle); tok && tm != nil {
			lookup := &model.Block{ID: "review-lookup", Translatable: true, Source: b.SourceRuns()}
			matches, lerr := tm.Lookup(ctx, lookup, model.LocaleID(sourceLang), loc,
				memory.LookupOptions{MinScore: 0.5, MaxResults: 1})
			if lerr == nil && len(matches) > 0 {
				detail.MemoryScore = int(math.Round(matches[0].Score * 100))
			}
		}
	}

	return detail, nil
}

// targetEditable reports whether the block's target for the locale is a single
// plain-text run — the only shape UpdateReviewTarget can rewrite safely.
func targetEditable(b *model.Block, loc model.LocaleID) bool {
	t := b.Target(loc)
	if t == nil {
		return false
	}
	return isSinglePlainTextRun(t.Runs)
}

// GetReviewQueue returns the project's review queue (the same derivation
// GetConvergence reports) with each item enriched with hasFindings — whether
// the unit currently trips any registered checker — so the Review page can
// order findings-first and offer a "clean only" batch. Enrichment is
// best-effort: a file that cannot be measured leaves its items unmarked.
func (a *App) GetReviewQueue(tabID string) ([]host.ReviewItem, error) {
	rep, err := a.GetConvergence(tabID)
	if err != nil {
		return nil, err
	}
	items := rep.Review
	if len(items) == 0 {
		return []host.ReviewItem{}, nil
	}
	op := a.getOpenProject(tabID)
	if op == nil || op.Project == nil || op.Path == "" {
		return items, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	sourceLang := string(project.NewProjectContext(op.Project, op.Path).SourceLocale)
	points := a.newPointResolver(op, false)
	dntTerms := a.resolveProjectDNTTerms(ctx, op, sourceLang)

	// Group items by (file, locale) so each pair is read and overlaid once.
	type scope struct{ file, locale string }
	groups := map[scope][]int{}
	for i, it := range items {
		s := scope{file: it.File, locale: it.Locale}
		groups[s] = append(groups[s], i)
	}
	for s, idxs := range groups {
		rf, tgtPath, ferr := a.findReviewSource(op, s.locale, s.file)
		if ferr != nil {
			continue // best-effort: leave hasFindings unset
		}
		byKey, berr := a.reviewUnitBlocks(ctx, op, rf, tgtPath, s.locale)
		if berr != nil {
			continue
		}
		// Each file is judged by the voice and vocabulary governing its own point.
		profile := points.at(ctx, rf.Collection, rf.Relative)
		tb := points.termsAt(ctx, rf.Collection, rf.Relative)
		for _, i := range idxs {
			b, ok := byKey[items[i].Key]
			if !ok {
				continue
			}
			has := len(a.blockCheckFindings(ctx, b, model.LocaleID(s.locale), profile, tb, dntTerms)) > 0
			items[i].HasFindings = &has
		}
	}
	return items, nil
}

// RejectReviewItem sends one review-queue unit back to the work queue: it
// records a `rejected` decision (status draft) in the project state store, with
// the reviewer's note, through the same host.ApplyReviewDecision path the CLI
// uses. The unit leaves the review queue; retranslating it makes the rejection
// stale and it re-enters review.
func (a *App) RejectReviewItem(tabID, locale, file, key, note string) error {
	return a.applyReviewDecision(tabID, locale, file, key, host.ReviewDecisionRejected, note)
}

// SignOffReviewItem promotes one review-queue unit to `signed-off` — the top
// rung of the target ladder — through host.ApplyReviewDecision.
func (a *App) SignOffReviewItem(tabID, locale, file, key string) error {
	return a.applyReviewDecision(tabID, locale, file, key, host.ReviewDecisionSignedOff, "")
}

// applyReviewDecision routes every desktop review decision through the shared
// host.ApplyReviewDecision path, so the CLI and the desktop record identical
// state (no desktop-only state writes).
func (a *App) applyReviewDecision(tabID, locale, file, key, decision, note string) error {
	op := a.getOpenProject(tabID)
	if op == nil {
		return fmt.Errorf("project tab %q not found", tabID)
	}
	loc, lerr := canonicalLocale(locale)
	if lerr != nil {
		return lerr
	}
	locale = string(loc)
	if op.Project == nil || op.Path == "" {
		return errors.New("project has no recipe loaded")
	}
	src := string(op.Project.Defaults.SourceLanguage)
	_, err := a.hostEngine().ApplyReviewDecision(context.Background(), op.Path, src,
		host.ReviewUnitRef{File: file, Key: key, Locale: locale}, decision, note)
	return err
}

// UpdateReviewTarget writes an edited translation back into the unit's target
// file through the same block-rewrite path the Checks panel's "Apply fix" uses
// (format reader → mutate one block → format writer, atomically). The edit is
// only applied when the unit's target content is a single plain text run — a
// substring-level rewrite over runs carrying placeholders or paired codes could
// corrupt the markup, so that case is refused with a clear error.
//
// After the write, any prior hash-bound decision for the unit is stale by
// construction (decisions bind to the content hash of the text they judged), so
// a subsequent approval blesses the NEW text.
func (a *App) UpdateReviewTarget(tabID, locale, file, key, text string) error {
	op := a.getOpenProject(tabID)
	if op == nil {
		return fmt.Errorf("project tab %q not found", tabID)
	}
	if op.Project == nil || op.Path == "" {
		return errors.New("project has no recipe loaded")
	}
	if strings.TrimSpace(text) == "" {
		return errors.New("the edited translation is empty — reject the unit instead to send it back to draft")
	}
	loc, lerr := canonicalLocale(locale)
	if lerr != nil {
		return lerr
	}
	locale = string(loc)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rf, tgtPath, err := a.findReviewSource(op, locale, file)
	if err != nil {
		return err
	}
	if _, serr := os.Stat(tgtPath); serr != nil {
		return fmt.Errorf("target file %q not found: %w", tgtPath, serr)
	}

	pctx := project.NewProjectContext(op.Project, op.Path)
	sourceLang := string(pctx.SourceLocale)
	// A target is the translated rendering of its source item, so the rewrite
	// reads and writes it under the format the recipe binds that item to rather
	// than the one its extension suggests.
	fmtName := rf.Format
	if fmtName == "" {
		fmtName = pctx.DetectFormat(a.formatReg, tgtPath)
	}
	if fmtName == "" {
		return fmt.Errorf("could not detect a format for %q", filepath.Base(tgtPath))
	}

	// The target file is read monolingually, so its translated text lives in
	// each block's source runs — the same convention ApplyCheckFix relies on.
	applied := false
	var applyErr error
	transform := func(b *model.Block) {
		if convergence.BlockKey(b) != key {
			return
		}
		if !isSinglePlainTextRun(b.SourceRuns()) {
			applyErr = fmt.Errorf("manual edit needed (formatted content): unit %q has inline markup or multiple runs, so a plain-text rewrite could corrupt it", key)
			return
		}
		b.SetSourceText(text)
		applied = true
	}
	if err := a.rewriteFile(ctx, tgtPath, fmtName, sourceLang, pctx, rf.Item, transform); err != nil {
		return err
	}
	if applyErr != nil {
		return applyErr
	}
	if !applied {
		return fmt.Errorf("unit %q not found in %q", key, filepath.Base(tgtPath))
	}
	return nil
}
