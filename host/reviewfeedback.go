package host

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/review"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/tool"
)

// Review feedback: a unit a reviewer rejected is drafted again, and the next
// draft is told what was rejected and why. The feedback is derived per unit
// from the decision entries the state ledger keeps, keyed by the pairing each
// judged. A rejection counts while its basis is the unit's current source, so
// a rewritten source starts the unit afresh.
//
// The rejected wording is read from the target file while the file still
// holds that draft (its hash matches). An older rejection keeps its reasons
// and note after its draft is replaced.

// reviewFeedbackTools are the steps a run hands review feedback to: the
// drafting step, which tells the model and stamps the draft, and recycling,
// which leaves a rejected unit for the drafter.
var reviewFeedbackTools = map[string]bool{"translate": true, "recycle": true}

// reviewFeedbackFor is the review feedback for the units of inputs, the source
// files one chain is built for, in locale.
func (a *App) reviewFeedbackFor(ctx context.Context, proj *project.KapiProject, root, locale string, inputs []string) (tool.ReviewFeedback, error) {
	if proj == nil || root == "" || locale == "" || len(inputs) == 0 {
		return nil, nil
	}
	// A read: a project with no store has no decisions, and a plan or a status
	// must not create one.
	st, err := a.OpenProjectState(withExistingStoresOnly(ctx), root)
	if errors.Is(err, errNoProjectStore) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	want := make(map[string]bool, len(inputs))
	for _, in := range inputs {
		if abs, aerr := filepath.Abs(in); aerr == nil {
			want[abs] = true
		}
	}
	units, err := a.UnitsFromProject(proj, root, locale)
	if err != nil {
		return nil, err
	}
	docs := a.documentIndexOrEmpty(ctx, root)
	sources := newSourceBlockCache(a, ctx)
	sourceLocale := model.LocaleID(ResolveSourceLocale("", proj.Defaults.SourceLanguage))
	out := tool.ReviewFeedback{}
	for _, u := range units {
		if u.Locale != locale || !want[u.SourcePath] {
			continue
		}
		entries, err := st.EntriesInScope(ctx, docs.Scope(root, u.SourcePath))
		if err != nil {
			return nil, err
		}
		rejected := rejectionsByUnit(entries, model.LocaleID(locale))
		if len(rejected) == 0 {
			continue
		}
		blocks, berr := sources.forUnit(u)
		if berr != nil {
			continue // an unreadable source is reported by the run itself
		}
		targets := a.targetTexts(ctx, u)
		rel := blockstore.SourceNamespace(root, u.SourcePath)
		for _, b := range blocks {
			if !b.Translatable {
				continue
			}
			key := blockKey(b)
			read := state.ReadSource(b, sourceLocale)
			fb, ok := unitFeedback(rejected[key], read.HoldsBasis, state.SourceHash(b.SourceText()), targets[key])
			if ok {
				out[tool.FeedbackUnitKey(rel, key)] = fb
			}
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// rejectionsByUnit groups the rejections a document's history holds for
// locale by unit key, most recent first.
func rejectionsByUnit(entries []state.Entry, locale model.LocaleID) map[string][]state.Entry {
	out := map[string][]state.Entry{}
	for _, e := range entries {
		if e.Revoked || e.State.Variant.Locale != locale || e.State.Decision.ReviewState != ReviewDecisionRejected {
			continue
		}
		out[e.State.Unit] = append(out[e.State.Unit], e)
	}
	return out
}

// unitFeedback is one unit's feedback: the rejections of drafts of the source
// the unit holds now (holdsBasis, the grading coverage uses), one per rejected
// draft, the latest tool.MaxRejectionsSent of them oldest first, and Held once
// tool.MaxRejections drafts were rejected. A record written before revisions
// names its source by sourceHash instead. current is the translation the
// target file holds for the unit.
func unitFeedback(entries []state.Entry, holdsBasis func(string) bool, sourceHash, current string) (tool.UnitReviewFeedback, bool) {
	var picked []state.Entry
	seen := map[string]bool{}
	for _, e := range entries {
		if e.State.Basis != "" {
			if !holdsBasis(e.State.Basis) {
				continue
			}
		} else if e.State.ContentHash == "" || e.State.ContentHash != sourceHash {
			continue
		}
		draft := e.State.TargetHash
		if draft == "" {
			draft = e.State.Revision
		}
		if seen[draft] {
			continue
		}
		seen[draft] = true
		picked = append(picked, e)
	}
	if len(picked) == 0 {
		return tool.UnitReviewFeedback{}, false
	}
	fb := tool.UnitReviewFeedback{Held: len(picked) >= tool.MaxRejections}
	if len(picked) > tool.MaxRejectionsSent {
		picked = picked[:tool.MaxRejectionsSent]
	}
	currentHash := ""
	if current != "" {
		currentHash = targetHash(current)
	}
	for _, e := range slices.Backward(picked) {
		r := tool.ReviewRejection{
			TargetHash: e.State.TargetHash,
			Reasons:    e.State.Decision.Reasons,
			Note:       e.State.Decision.Note,
		}
		if currentHash != "" && e.State.TargetHash == currentHash {
			r.Text = current
		}
		fb.Rejections = append(fb.Rejections, r)
	}
	return fb, true
}

// targetTexts reads the translation the unit's target file holds, by unit
// key; empty when the file is missing or unreadable.
func (a *App) targetTexts(ctx context.Context, u VerifyUnit) map[string]string {
	blocks, missing, err := a.bilingualBlocks(ctx, u)
	if err != nil || missing {
		return nil
	}
	loc := model.LocaleID(u.Locale)
	out := make(map[string]string, len(blocks))
	for _, b := range blocks {
		if t := b.TargetText(loc); t != "" {
			out[blockKey(b)] = t
		}
	}
	return out
}

// applyReviewFeedback hands the chain's review feedback to a drafting or
// recycle step under tool.ReviewFeedbackKey. A machine translation engine
// takes no instructions: it is still handed the feedback, which it stamps on
// each draft and never sends, and the run says so once.
func (a *App) applyReviewFeedback(toolName string, config map[string]any, cmd Command) map[string]any {
	if len(a.flowReviewFeedback) == 0 || !reviewFeedbackTools[toolName] {
		return config
	}
	out := make(map[string]any, len(config)+1)
	maps.Copy(out, config)
	out[tool.ReviewFeedbackKey] = a.flowReviewFeedback
	if toolName == "translate" && !a.reviewFeedbackMTNoted && aitools.IsMTProvider(stringConfig(config, "provider")) && cmd != nil {
		a.reviewFeedbackMTNoted = true
		fmt.Fprintf(cmd.ErrOrStderr(), "Note: machine translation takes no instructions, so the reviewers' reasons and notes for %s are not sent to it; those units are drafted again without them.\n",
			pluralUnit(len(a.flowReviewFeedback), "rejected unit", "rejected units"))
	}
	return out
}

func stringConfig(config map[string]any, key string) string {
	s, _ := config[key].(string)
	return s
}

// reviewRejections is what a unit's next draft is told about its rejected
// drafts, for the review context: a reviewer sees what the model is told.
func (a *App) reviewRejections(ctx context.Context, req ReviewContextRequest, b *model.Block) []review.Rejection {
	if req.SourcePath == "" || req.Locale == "" {
		return nil
	}
	proj, err := project.LoadWithOptions(filepath.Join(req.Root, project.RecipeFileName), project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return nil
	}
	source := req.SourcePath
	if !filepath.IsAbs(source) {
		source = filepath.Join(req.Root, source)
	}
	fb, err := a.reviewFeedbackFor(ctx, proj, req.Root, req.Locale, []string{source})
	if err != nil {
		return nil
	}
	unit := fb[tool.FeedbackUnitKey(blockstore.SourceNamespace(req.Root, source), blockKey(b))]
	out := make([]review.Rejection, 0, len(unit.Rejections))
	for _, r := range unit.Rejections {
		rr := review.Rejection{Text: r.Text, Note: r.Note}
		for _, code := range r.Reasons {
			rr.Reasons = append(rr.Reasons, tool.RejectionReasonPhrase(code))
		}
		out = append(out, rr)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
