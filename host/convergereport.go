package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/state"
)

// ProjectConvergence computes the convergence report for a project recipe,
// deriving everything from the working tree (content × target files) and the
// committed .memory.json corrections — the same file-based derivation the CLI status
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
	ctx = ctxOrBackground(ctx)

	proj, err := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return nil, fmt.Errorf("load project: %w", err)
	}
	root := filepath.Dir(projectPath)

	a.SourceLang = ResolveSourceLocale(sourceLang, proj.Defaults.SourceLanguage)

	units, err := a.UnitsFromProject(proj, root, "")
	if err != nil {
		return nil, fmt.Errorf("resolve content: %w", err)
	}

	// The tool builders speak cobra (flag lookups, project discovery); this
	// embedded read drives the bound checks through a synthetic command bound to
	// THIS recipe, so terms resolution binds to this project rather than walking
	// up from the process cwd.
	cmd := NewEnvCommand(ctx, "project-convergence")
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	AddProjectFlag(cmd)
	if err := cmd.Flags().Set(projectFlagName, projectPath); err != nil {
		return nil, err
	}

	var report *ConvergenceReport
	cacheErr := a.withParseCache(root, func() error {
		// This report carries a ship verdict to the desktop, so it asks the
		// guardrail question every verdict-publishing surface asks (#2024).
		excl, err := a.computeLoopCheckExclusions(ctx, cmd, proj, root, units)
		if err != nil {
			return fmt.Errorf("run project checks: %w", err)
		}
		cov, err := a.ComputeShipCoverage(ctx, proj, root, units, excl)
		if err != nil {
			return fmt.Errorf("compute coverage: %w", err)
		}
		src, err := a.computeSourceReadiness(ctx, proj, root, units)
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
		return "", fmt.Errorf("unknown review outcome %q: want %s, %s, or %s",
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
// re-reads the exact target text before recording. `kapi commit` writes it on
// into the committed record under `.kapi/state/` — distinct from the
// `.memory.json`, which stays the recycle corpus.
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
	ctx = ctxOrBackground(ctx)
	status, err := decisionStatus(decision)
	if err != nil {
		return false, err
	}
	proj, err := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return false, fmt.Errorf("load project: %w", err)
	}
	root := filepath.Dir(projectPath)
	a.SourceLang = ResolveSourceLocale(sourceLang, proj.Defaults.SourceLanguage)

	units, err := a.UnitsFromProject(proj, root, ref.Locale)
	if err != nil {
		return false, fmt.Errorf("resolve content: %w", err)
	}

	for _, u := range units {
		if u.Locale != ref.Locale || u.DisplayPath != ref.File {
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
			// What governs the unit where the decider is deciding it: the voice
			// guidance and term rules in force at the file's governance point
			// for this locale, folded by the one function every producer
			// stamps with. It is recorded beside the decision because the
			// decision is the claim it supports (this answer stands under this
			// context), and because for most delivered formats the record is
			// the only carrier the answer has.
			governing, gerr := a.governingFingerprintFor(ctx, projectPath, proj, root, u)
			if gerr != nil {
				return false, gerr
			}
			// The decision's document is its SOURCE file, resolved to the
			// durable key the project holds for it: the identity namespace the
			// unit key lives in, and the half of the decision's identity that
			// tells one page's `p` from another's. The review queue's display
			// path is the target file, which no other party names anything by.
			return a.recordDecisionState(ctx, proj, root, a.documentIndexOrEmpty(ctx, root).Scope(root, u.SourcePath), blockKey(b), loc,
				decidedContent{source: b.SourceText(), target: target}, governing, status, decision, note, by)
		}
	}
	return false, fmt.Errorf("review unit %q (%s) not found in %s", ref.Key, ref.Locale, ref.File)
}

// governingFingerprintFor resolves the governing context in force at a unit's
// governance point for its locale, as the fingerprint a producer running there
// now would stamp (coreprofile.GovernanceContext, through the same resolver the
// staleness gate compares against). Empty for an ungoverned project.
//
// The resolvers bind to a project through a Command, so an embedded caller
// (the desktop, an MCP tool) is given a synthetic one carrying this recipe:
// resolving through the working directory instead would read whichever
// project the process happens to sit in.
func (a *App) governingFingerprintFor(ctx context.Context, projectPath string, proj *project.KapiProject, root string, u VerifyUnit) (string, error) {
	cmd := NewEnvCommand(ctx, "record-decision")
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	AddProjectFlag(cmd)
	if err := cmd.Flags().Set(projectFlagName, projectPath); err != nil {
		return "", err
	}
	fps, err := newContextFingerprints(a, cmd, proj, root)
	if err != nil {
		return "", fmt.Errorf("resolve the governing context: %w", err)
	}
	defer fps.close()
	g, err := fps.at(a.unitGovernancePoint(root, u), u.Locale)
	if err != nil {
		return "", fmt.Errorf("resolve the governing context: %w", err)
	}
	return g.fingerprint, nil
}

// decidedContent is the pairing a decision is about: the source wording in front
// of the decider and the translation of it they judged. Both are hashed into the
// record, because an approval that bound only the translation survived its source
// being rewritten — the reviewer's blessing outliving the sentence it blessed.
type decidedContent struct {
	source string
	target string
}

// recordDecisionState records a unit's review decision in the project state store
// — the authoritative carrier of workflow state — keyed by (document, unit,
// locale), and bound to BOTH halves of what it blessed: the content hash of the translation
// it judges (targetHash) and the basis, the content hash of the source it judged it
// against (contentHash). A later edit to either drops the decision (stale), derived
// on read. The decision is transient until Export persists it to the committed state
// artifact (the export sink). The content memory (.memory.json) is no longer
// touched here: it is the recycle corpus, not the state carrier. Advisory fields
// already on the unit's record (origin, source status, a fresh AI pre-review
// annotation) survive the decision write.
//
// governing is the fingerprint of the context the decision is made under, and
// is part of what the record says: the same verdict on the same pairing under a
// moved context is a new decision, recorded again.
func (a *App) recordDecisionState(ctx context.Context, proj *project.KapiProject, root, file, unit string, locale model.LocaleID, content decidedContent, governing string, status model.TargetStatus, decision, note, by string) (bool, error) {
	st, err := a.OpenProjectState(ctx, root)
	if err != nil {
		return false, err
	}
	k := state.Key{Scope: file, Unit: unit, Variant: model.Variant(locale)}
	th := targetHash(content.target)
	ch := state.SourceHash(content.source)
	prev, hadPrev := st.Get(ctx, k)
	if hadPrev && prev.Status == status && prev.TargetHash == th && prev.ContentHash == ch &&
		prev.Decision.Note == note && prev.Decision.By == by && prev.GoverningFingerprint == governing {
		return false, nil // already at this decision for this exact pairing, under this context
	}
	now := nowRFC3339()
	next := state.UnitState{
		Unit:                 unit,
		Variant:              model.Variant(locale),
		Status:               status,
		TargetHash:           th,
		ContentHash:          ch,
		GoverningFingerprint: governing,
		Decision:             state.Decision{ReviewState: decision, By: by, At: now, Note: note},
		Updated:              now,
		// The document the unit was decided in — half of the record's identity,
		// and what lets a decision travel the sync protocol scoped to the item
		// whose identity namespace the unit key lives in. Until the reconcile
		// resolver is wired into the review path, the source path IS the
		// document key; when resolved keys land, the working store's document
		// map translates key → path and this field keeps meaning "which
		// document".
		Scope: file,
	}
	if hadPrev {
		// Advisory state rides along; only the decision itself is replaced.
		next.Origin = prev.Origin
		next.SourceStatus = prev.SourceStatus
		next.ContextHash = prev.ContextHash
		if prev.AIReview.Fresh(th) {
			next.AIReview = prev.AIReview
		}
	}
	if err := st.Put(ctx, next); err != nil {
		return false, err
	}
	// Staged, not committed. A decision is durable the moment it lands in the
	// working store; making it part of the project's committed record is a
	// separate, deliberate act — see `kapi commit`.
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
	ctx = ctxOrBackground(ctx)
	if len(reviews) == 0 {
		return 0, nil
	}
	proj, err := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return 0, fmt.Errorf("load project: %w", err)
	}
	root := filepath.Dir(projectPath)
	a.SourceLang = ResolveSourceLocale(sourceLang, proj.Defaults.SourceLanguage)

	units, err := a.UnitsFromProject(proj, root, locale)
	if err != nil {
		return 0, fmt.Errorf("resolve content: %w", err)
	}

	st, err := a.OpenProjectState(ctx, root)
	if err != nil {
		return 0, err
	}

	recorded := 0
	loc := model.LocaleID(locale)
	docs := a.documentIndexOrEmpty(ctx, root)
	for _, u := range units {
		if u.Locale != locale || u.DisplayPath != file {
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
		scope := docs.Scope(root, u.SourcePath)
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
			k := state.Key{Scope: scope, Unit: blockKey(b), Variant: model.Variant(loc)}
			us, _ := st.Get(ctx, k)
			us.Unit = blockKey(b)
			us.Variant = model.Variant(loc)
			us.Scope = scope
			r := rev
			us.AIReview = &r
			us.Updated = rev.At
			if err := st.Put(ctx, us); err != nil {
				return recorded, err
			}
			recorded++
		}
	}
	// Staged only; `kapi commit` writes the project's committed record.
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
	// Language is the language this unit belongs to, repeating Locale: the
	// target locale for a translation, the project's source language for a
	// source unit.
	Language string `json:"language,omitempty"`
	// IsSource marks a unit in the project's source language. Its Target is
	// empty, its Status is a rung of the authoring ladder, and its decision is
	// recorded under the source locale variant.
	IsSource bool `json:"is_source,omitempty"`
	// Status is the unit's effective ladder state (draft|translated|reviewed|
	// signed-off), with a fresh state-store decision applied over the presence
	// baseline. For a source unit it is the settled authoring rung
	// (authored|checked|approved).
	Status string `json:"status"`
	// ReviewState/Note/By carry the last recorded decision when it still judges
	// the current pairing — the translation it blessed, of the source it blessed
	// it for.
	ReviewState string `json:"review_state,omitempty"`
	Note        string `json:"note,omitempty"`
	By          string `json:"by,omitempty"`
	// Stale reports that a decision exists but was recorded against source
	// wording that has since changed, so the unit is back in the queue: the
	// reviewer blessed a rendering of a sentence the project no longer has.
	Stale bool `json:"stale,omitempty"`
	// AIScore/AIModel/AIFindings surface a fresh AI pre-review annotation.
	AIScore    *int                    `json:"ai_score,omitempty"`
	AIModel    string                  `json:"ai_model,omitempty"`
	AIFindings []state.AIReviewFinding `json:"ai_findings,omitempty"`
	// Context is everything that makes the decision judgeable: the point the
	// content sits at, the blocks around it, what it said before, what the
	// checks found, and who decided last. Present only when the caller asked
	// for it (ReviewUnitOptions.WithContext) — assembling it opens the voice
	// store, the terms store and the content memory, which a queue listing
	// thousands of units must not pay per row.
	Context *ReviewContext `json:"context,omitempty"`
}

// ReviewUnitOptions says how much of the review model to assemble.
type ReviewUnitOptions struct {
	// WithContext assembles ReviewUnitInfo.Context.
	WithContext bool
	// Window is how many blocks either side the neighbourhood carries; zero
	// means DefaultReviewWindow.
	Window int
}

// ReviewUnit resolves one review-queue unit by (file, key, locale) — exactly as
// `kapi status --review` lists it — and returns its full text and recorded
// state. It is the read leg agents pair with ApplyReviewDecisionAs.
func (a *App) ReviewUnit(ctx context.Context, projectPath, sourceLang string, ref ReviewUnitRef) (*ReviewUnitInfo, error) {
	return a.ReviewUnitWithOptions(ctx, projectPath, sourceLang, ref, ReviewUnitOptions{})
}

// ReviewUnitWithContext is ReviewUnit with the full review model attached: the
// point, the neighbourhood, the history, the judgement and the provenance. It
// is what a client rendering a review surface reads.
func (a *App) ReviewUnitWithContext(ctx context.Context, projectPath, sourceLang string, ref ReviewUnitRef) (*ReviewUnitInfo, error) {
	return a.ReviewUnitWithOptions(ctx, projectPath, sourceLang, ref, ReviewUnitOptions{WithContext: true})
}

// ReviewUnitWithOptions is the one implementation the two forms above name.
func (a *App) ReviewUnitWithOptions(ctx context.Context, projectPath, sourceLang string, ref ReviewUnitRef, opts ReviewUnitOptions) (*ReviewUnitInfo, error) {
	a.InitRegistries()
	ctx = ctxOrBackground(ctx)
	proj, err := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return nil, fmt.Errorf("load project: %w", err)
	}
	root := filepath.Dir(projectPath)
	a.SourceLang = ResolveSourceLocale(sourceLang, proj.Defaults.SourceLanguage)

	// A unit in the source language is the author's own wording, read from the
	// source file and settled on the authoring ladder. It reaches this method
	// from the same queue as a translation, so it is answered here rather than
	// through a second entry point.
	if ref.Locale == a.SourceLang {
		return a.reviewSourceUnit(ctx, proj, root, projectPath, ref, opts)
	}

	units, err := a.UnitsFromProject(proj, root, ref.Locale)
	if err != nil {
		return nil, fmt.Errorf("resolve content: %w", err)
	}

	loc := model.LocaleID(ref.Locale)
	for _, u := range units {
		if u.Locale != ref.Locale || u.DisplayPath != ref.File {
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
				Language:   ref.Locale,
				File:       ref.File,
				Key:        ref.Key,
				Collection: u.Collection,
				Source:     b.SourceText(),
				Target:     b.TargetText(loc),
				Status:     unitState(b, ref.Locale),
			}
			st, serr := a.OpenProjectState(ctx, root)
			if serr != nil {
				return nil, serr
			}
			k := state.Key{Scope: a.documentIndexOrEmpty(ctx, root).Scope(root, u.SourcePath), Unit: ref.Key, Variant: model.Variant(loc)}
			var record *state.UnitState
			if us, found := st.Get(ctx, k); found {
				record = &us
				th := targetHash(info.Target)
				ch := state.SourceHash(info.Source)
				info.Stale = us.SourceStale(ch)
				if us.Fresh(th, ch) {
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
			if opts.WithContext {
				// The blocks are already in document order and already
				// overlaid with the locale, so the neighbourhood costs one
				// index rather than a second read.
				cmd := NewEnvCommand(ctx, "review")
				AddProjectFlag(cmd)
				AddResourceFlags(cmd)
				if ferr := cmd.Flags().Set("project", projectPath); ferr != nil {
					return nil, fmt.Errorf("bind project: %w", ferr)
				}
				info.Context = a.AssembleReviewContext(ctx, ReviewContextRequest{
					Cmd:        cmd,
					Root:       root,
					SourcePath: u.SourcePath,
					Collection: u.Collection,
					Locale:     ref.Locale,
					SourceLang: a.SourceLang,
					Blocks:     blocks,
					Key:        ref.Key,
					Window:     opts.Window,
					Memory:     a.ReviewMemory(ctx, root),
					Unit:       record,
				})
				if info.Context != nil {
					info.Context.Provenance.Stale = info.Stale
				}
			}
			return info, nil
		}
	}
	return nil, fmt.Errorf("review unit %q (%s) not found in %s", ref.Key, ref.Locale, ref.File)
}

// DecisionScope is a document's ADDRESS: its SOURCE file, relative to the
// project root, in slash form. It is what a decision is scoped by where the
// project holds no durable key for the document — a fresh checkout, a build
// with no store — and it is the fallback DocumentIndex.Scope falls back to.
//
// Reach for DocumentIndex.Scope rather than this. The address is not the
// identity: renaming a file changes it, and a decision keyed on it is detached
// by a rename, silently. This stays exported because the fallback has to be one
// definition — every party that records a decision and every party that reads
// one back has to name an unresolved document the same way.
func DecisionScope(root, sourcePath string) string {
	return filepath.ToSlash(relToRoot(root, sourcePath))
}

// relToRoot renders a unit path relative to the project root when possible —
// the shape the connector names items by. An outside-the-root or already-
// relative path is kept verbatim.
func relToRoot(root, p string) string {
	if p == "" || !filepath.IsAbs(p) {
		return p
	}
	if rel, err := filepath.Rel(root, p); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return p
}
