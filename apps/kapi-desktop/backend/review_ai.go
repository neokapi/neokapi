package backend

// AI in review (issue #1077, phase 3): per-unit AI actions (fix findings,
// retranslate with instruction, explain) and the batch AI pre-review. Provider
// calls happen ONLY inside these explicitly-invoked methods — queue listing
// and unit loading never touch a provider. All provider configuration resolves
// exactly like the flow runner's: the shared ai.provider/ai.model defaults plus
// the desktop credential store, so "the model that runs your flows" is the
// model that reviews. Autonomous approvals record the honest identity
// "ai/<model-id>" via host.ApplyReviewDecisionAs.

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/host"
	appconfig "github.com/neokapi/neokapi/host/config"
	"github.com/neokapi/neokapi/host/credentials"
)

// Review AI actions.
const (
	ReviewAIFixFindings = "fix-findings"
	ReviewAIRetranslate = "retranslate"
	ReviewAIExplain     = "explain"
)

// ReviewAIActionResult is the outcome of one per-unit AI action. Fix/retranslate
// return a PROPOSED target only — nothing is written until the reviewer accepts
// the diff (which routes through UpdateReviewTarget); explain returns text.
type ReviewAIActionResult struct {
	ProposedTarget string `json:"proposed_target,omitempty"`
	Explanation    string `json:"explanation,omitempty"`
	// Exchanges are the LLM calls this action made: the messages sent, the
	// schema constraining the output, the reply and the token usage. A reviewer
	// asked to accept a proposed translation can see what the model was told
	// before deciding, rather than judging an answer whose question is hidden.
	Exchanges []AIActivityEntry `json:"exchanges,omitempty"`
}

// ReviewAIAction runs one AI action for a review-queue unit, addressed by
// (locale, file, key) exactly as the queue lists it:
//
//   - fix-findings: a one-block translate invocation whose instruction carries
//     the unit's current check findings (and any fresh AI-review findings) as
//     constraints — the model produces a corrected translation.
//   - retranslate: a one-block translate invocation with the reviewer's
//     instruction verbatim.
//   - explain: a one-block run of the ai review tool; the structured result
//     ({score, findings}) is rendered as explanation text. Read-only.
//
// The provider is the SAME one the desktop flow runner resolves (shared
// ai.provider/ai.model defaults + credential store); with none configured a
// friendly error tells the user to configure one in Settings.
func (a *App) ReviewAIAction(tabID, locale, file, key, action, instruction string) (*ReviewAIActionResult, error) {
	canon, lerr := requireLocale(locale)
	if lerr != nil {
		return nil, lerr
	}
	locale = string(canon)
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	if op.Project == nil || op.Path == "" {
		return nil, errors.New("project has no recipe loaded")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	rf, tgtPath, err := a.findReviewSource(op, locale, file)
	if err != nil {
		return nil, err
	}
	_, byKey, err := a.reviewUnitBlocks(ctx, op, rf, tgtPath, locale)
	if err != nil {
		return nil, err
	}
	b, ok := byKey[key]
	if !ok {
		return nil, fmt.Errorf("unit %q not found in %s", key, file)
	}

	loc := model.LocaleID(locale)
	sourceLang := string(project.NewProjectContext(op.Project, op.Path).SourceLocale)
	root := filepath.Dir(op.Path)
	scope := a.hostEngine().DocumentScope(ctx, root, rf.Path)

	// Everything below runs under a scope, so the session's activity log can
	// attribute each call to the unit it was about. The mark is taken before the
	// call so this action can collect exactly its own exchanges afterwards.
	ctx = withAIScope(ctx, AIActivityScope{
		Surface: "review", Action: action, Locale: locale, File: file, Key: key,
	})
	mark := a.aiActivity.lastID()

	var res *ReviewAIActionResult
	switch action {
	case ReviewAIExplain:
		res, err = a.reviewAIExplain(ctx, b, sourceLang, locale)
	case ReviewAIFixFindings:
		findings := a.currentUnitFindings(ctx, op, scope, b, loc, sourceLang, key, rf.Collection, rf.Relative)
		instruction = fixFindingsInstruction(b.TargetText(loc), findings, instruction)
		res, err = a.reviewAIPropose(ctx, b, sourceLang, locale, instruction)
	case ReviewAIRetranslate:
		if strings.TrimSpace(instruction) == "" {
			return nil, errors.New("retranslate needs an instruction — tell the model what to change")
		}
		res, err = a.reviewAIPropose(ctx, b, sourceLang, locale, instruction)
	default:
		return nil, fmt.Errorf("unknown review AI action %q: want %s, %s, or %s",
			action, ReviewAIFixFindings, ReviewAIRetranslate, ReviewAIExplain)
	}
	if err != nil {
		return nil, err
	}
	res.Exchanges = a.aiActivity.since(mark)
	return res, nil
}

// reviewAIPropose runs a one-block translate invocation and returns the
// produced target as a proposal. The block is a fresh read-side copy — nothing
// touches the target file.
func (a *App) reviewAIPropose(ctx context.Context, b *model.Block, sourceLang, locale, instruction string) (*ReviewAIActionResult, error) {
	cfg := map[string]any{
		"sourceLocale": sourceLang,
		"instruction":  instruction,
		// One block, one call: the single-block path renders the instruction
		// via TranslateRequest.Directives.
		"batchSize": 1,
	}
	tl, err := a.newReviewAITool("translate", cfg, locale)
	if err != nil {
		return nil, err
	}
	if err := runReviewAITool(ctx, tl, b); err != nil {
		return nil, fmt.Errorf("ai action: %w", err)
	}
	proposed := b.TargetText(model.LocaleID(locale))
	if strings.TrimSpace(proposed) == "" {
		return nil, errors.New("the model returned an empty translation — try a more specific instruction")
	}
	return &ReviewAIActionResult{ProposedTarget: proposed}, nil
}

// reviewAIExplain runs the ai review tool over the block and renders its
// structured result as explanation text (score + findings). Read-only: the
// review lands in block properties on an in-memory copy.
func (a *App) reviewAIExplain(ctx context.Context, b *model.Block, sourceLang, locale string) (*ReviewAIActionResult, error) {
	tl, err := a.newReviewAITool("review", map[string]any{"sourceLocale": sourceLang}, locale)
	if err != nil {
		return nil, err
	}
	if err := runReviewAITool(ctx, tl, b); err != nil {
		return nil, fmt.Errorf("ai explain: %w", err)
	}
	payload := b.Properties["review"]
	if payload == "" {
		return nil, errors.New("the model returned no review for this unit")
	}
	if res, perr := aitools.ParseReviewResult(payload); perr == nil {
		return &ReviewAIActionResult{Explanation: renderReviewExplanation(res)}, nil
	}
	// Lenient fallback: prose-style model output is the explanation.
	return &ReviewAIActionResult{Explanation: payload}, nil
}

// renderReviewExplanation formats a structured review result as reviewer-facing
// text.
func renderReviewExplanation(res *aitools.ReviewResult) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Score: %d/100\n", res.Score)
	if len(res.Findings) == 0 {
		sb.WriteString("No issues found.")
		return sb.String()
	}
	for _, f := range res.Findings {
		fmt.Fprintf(&sb, "• [%s] %s", f.Severity, f.Message)
		if f.Suggestion != "" {
			fmt.Fprintf(&sb, " — suggestion: %s", f.Suggestion)
		}
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// currentUnitFindings gathers the unit's current findings for the fix-findings
// instruction: the deterministic check findings (the ChecksPanel checkset) plus
// any fresh AI-review findings recorded in the state store.
func (a *App) currentUnitFindings(ctx context.Context, op *openProject, scope string, b *model.Block, loc model.LocaleID, sourceLang, key, collection, relPath string) []string {
	var lines []string
	// The findings a fix is asked to address are the ones this unit's own
	// point produces, so the voice is resolved there.
	points := a.newPointResolver(op, false)
	profile := points.at(ctx, collection, relPath)
	dntTerms := a.resolveProjectDNTTerms(ctx, op, sourceLang)
	for _, f := range a.blockCheckFindings(ctx, b, sourceLang, loc, profile,
		points.termsAt(ctx, collection, relPath), dntTerms) {
		line := fmt.Sprintf("[%s] %s", f.Severity, f.Message)
		if f.Suggestion != "" {
			line += " (suggestion: " + f.Suggestion + ")"
		}
		lines = append(lines, line)
	}
	if rev := a.freshAIReview(ctx, op, scope, key, loc, b.TargetText(loc)); rev != nil {
		for _, f := range rev.Findings {
			line := fmt.Sprintf("[%s] %s", f.Severity, f.Message)
			if f.Suggestion != "" {
				line += " (suggestion: " + f.Suggestion + ")"
			}
			lines = append(lines, line)
		}
	}
	return lines
}

// fixFindingsInstruction builds the translate instruction for fix-findings:
// the current translation, the findings to resolve, and any extra reviewer
// guidance.
func fixFindingsInstruction(currentTarget string, findings []string, extra string) string {
	var sb strings.Builder
	sb.WriteString("Produce a corrected translation. The current translation is:\n")
	sb.WriteString(currentTarget)
	if len(findings) > 0 {
		sb.WriteString("\n\nIt has these review findings; resolve every one of them while preserving the meaning of the source:\n")
		for _, f := range findings {
			sb.WriteString("- " + f + "\n")
		}
	} else {
		sb.WriteString("\n\nImprove its accuracy and fluency while preserving the meaning of the source.\n")
	}
	if strings.TrimSpace(extra) != "" {
		sb.WriteString("\nAdditional guidance: " + extra)
	}
	return sb.String()
}

// freshAIReview returns the unit's AI pre-review annotation from the state
// store when it still judges the given translation, else nil.
func (a *App) freshAIReview(ctx context.Context, op *openProject, scope, key string, loc model.LocaleID, targetText string) *state.AIReview {
	// One handle per project, memoized on the engine: this is called per unit
	// while rendering a review, and under the four-file layout each call opened
	// and closed a database of its own. Now there is nothing to close — the
	// working store is a schema of the project's store, and the engine owns it.
	root := filepath.Dir(op.Path)
	st, err := a.hostEngine().OpenProjectState(ctx, root)
	if err != nil {
		return nil
	}

	us, found := st.Get(ctx, state.Key{Scope: scope, Unit: key, Variant: model.Variant(loc)})
	if !found {
		return nil
	}
	th := project.HashBytes([]byte(strings.TrimSpace(targetText)))
	if !us.AIReview.Fresh(th) {
		return nil
	}
	return us.AIReview
}

// newReviewAITool builds the AI tool an action uses through the same registry
// path the flow runner takes (shared AI defaults + credential resolution).
// Tests inject aiToolFactory with a MockProvider-backed tool.
func (a *App) newReviewAITool(name string, cfg map[string]any, targetLang string) (tool.Tool, error) {
	if a.aiToolFactory != nil {
		return a.aiToolFactory(name, cfg, targetLang)
	}
	tl, err := a.toolReg.NewToolWithConfig(registry.ToolID(name), cfg, targetLang)
	if err != nil {
		return nil, friendlyAIProviderError(err)
	}
	return tl, nil
}

// friendlyAIProviderError rewrites credential-resolution failures into the
// message the review UI surfaces; other errors pass through.
func friendlyAIProviderError(err error) error {
	var amb *credentials.AmbiguousCredentialError
	if errors.As(err, &amb) {
		return fmt.Errorf("multiple AI credentials are configured (%s) — set a default in Settings → AI Models",
			strings.Join(amb.Candidates, ", "))
	}
	if strings.Contains(err.Error(), "no saved credentials") {
		return errors.New("no AI provider is configured — add one in Settings → AI Models, then try again")
	}
	return err
}

// runReviewAITool drives one block through a tool's Process, draining the
// output — the same one-block invocation the checks panel uses.
func runReviewAITool(ctx context.Context, tl tool.Tool, b *model.Block) error {
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 4)
	in <- &model.Part{Type: model.PartBlock, Resource: b}
	close(in)
	errc := make(chan error, 1)
	go func() {
		defer close(out)
		errc <- tl.Process(ctx, in, out)
	}()
	for range out { //nolint:revive // drain
	}
	return <-errc
}

// --- AI pre-review -----------------------------------------------------------

// PreReviewScope narrows the pre-review to one content collection (empty = the
// whole queue for the locale).
type PreReviewScope struct {
	Collection string `json:"collection,omitempty"`
}

// PreReviewPolicy decides what the pre-review may do. Annotate-only (the
// default: AutoApprove false) stores score + findings so the queue can show
// them; with AutoApprove, units scoring at least MinScore AND free of
// critical/major deterministic-check findings are approved with the identity
// "ai/<model-id>".
type PreReviewPolicy struct {
	AutoApprove bool `json:"autoApprove"`
	MinScore    int  `json:"minScore"`
}

// PreReviewResult summarizes a pre-review run: N auto-approved · M left.
type PreReviewResult struct {
	// Model is the reviewer model id (what "ai/<model-id>" identities carry).
	Model string `json:"model"`
	// Reviewed counts units annotated with a score.
	Reviewed int `json:"reviewed"`
	// AutoApproved counts units approved under the policy.
	AutoApproved int `json:"auto_approved"`
	// Remaining counts reviewed units still awaiting a human decision.
	Remaining int `json:"remaining"`
	// Skipped counts units whose model response carried no usable score.
	Skipped int `json:"skipped,omitempty"`
}

// RunAIPreReview runs the ai review tool over the pending review queue for a
// locale (batch, explicitly invoked — never during queue listing). Every unit
// gets an advisory annotation (score + findings) in the project state store,
// bound to the translation it judged; with policy.AutoApprove, clean
// high-scoring units are approved through host.ApplyReviewDecisionAs with the
// honest identity "ai/<model-id>". Human-required gates are unaffected by
// those approvals (core/gate approver classes).
func (a *App) RunAIPreReview(tabID, locale string, scope PreReviewScope, policy PreReviewPolicy) (*PreReviewResult, error) {
	canon, lerr := canonicalLocale(locale)
	if lerr != nil {
		return nil, lerr
	}
	locale = string(canon)
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	if op.Project == nil || op.Path == "" {
		return nil, errors.New("project has no recipe loaded")
	}

	rep, err := a.GetConvergence(tabID)
	if err != nil {
		return nil, err
	}
	var pending []host.ReviewQueueItem
	for _, it := range rep.Review {
		if locale != "" && it.Locale != locale {
			continue
		}
		if scope.Collection != "" && it.Collection != scope.Collection {
			continue
		}
		pending = append(pending, it)
	}
	modelID, err := a.reviewerModelID()
	if err != nil {
		return nil, err
	}
	res := &PreReviewResult{Model: modelID}
	if len(pending) == 0 {
		return res, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	sourceLang := string(project.NewProjectContext(op.Project, op.Path).SourceLocale)
	points := a.newPointResolver(op, false)
	dntTerms := a.resolveProjectDNTTerms(ctx, op, sourceLang)
	src := string(op.Project.Defaults.SourceLanguage)

	// Group by (file, locale) so each pair is read and overlaid once, and the
	// annotations for a file land in one state write.
	type scopeKey struct{ file, locale string }
	groups := map[scopeKey][]host.ReviewQueueItem{}
	order := []scopeKey{}
	for _, it := range pending {
		k := scopeKey{file: it.File, locale: it.Locale}
		if _, seen := groups[k]; !seen {
			order = append(order, k)
		}
		groups[k] = append(groups[k], it)
	}

	done := 0
	total := len(pending)
	for _, k := range order {
		items := groups[k]
		rf, tgtPath, ferr := a.findReviewSource(op, k.locale, k.file)
		if ferr != nil {
			res.Skipped += len(items)
			done += len(items)
			continue
		}
		_, byKey, berr := a.reviewUnitBlocks(ctx, op, rf, tgtPath, k.locale)
		if berr != nil {
			res.Skipped += len(items)
			done += len(items)
			continue
		}
		tl, terr := a.newReviewAITool("review", map[string]any{"sourceLocale": sourceLang}, k.locale)
		if terr != nil {
			return nil, terr
		}
		// A proposal is steered by the voice and vocabulary governing this
		// file's point.
		profile := points.at(ctx, rf.Collection, rf.Relative)
		tb := points.termsAt(ctx, rf.Collection, rf.Relative)

		annotations := map[string]state.AIReview{}
		type approval struct {
			item  host.ReviewQueueItem
			score int
		}
		var approvals []approval

		for _, it := range items {
			done++
			a.emitEvent("prereview:progress", map[string]any{
				"done": done, "total": total, "file": it.File, "key": it.Key,
			})
			b, found := byKey[it.Key]
			if !found {
				res.Skipped++
				continue
			}
			unitCtx := withAIScope(ctx, AIActivityScope{
				Surface: "pre-review", Locale: it.Locale, File: it.File, Key: it.Key,
			})
			if rerr := runReviewAITool(unitCtx, tl, b); rerr != nil {
				return nil, fmt.Errorf("ai pre-review %s:%s: %w", it.File, it.Key, rerr)
			}
			scoreStr, hasScore := b.Properties["review-score"]
			if !hasScore {
				res.Skipped++ // prose fallback — no usable score, nothing stored
				continue
			}
			score, serr := strconv.Atoi(scoreStr)
			if serr != nil {
				res.Skipped++
				continue
			}
			rev := state.AIReview{Score: score, Model: modelID}
			if parsed, perr := aitools.ParseReviewResult(b.Properties["review"]); perr == nil {
				for _, f := range parsed.Findings {
					rev.Findings = append(rev.Findings, state.AIReviewFinding{
						Severity: f.Severity, Message: f.Message, Suggestion: f.Suggestion,
					})
				}
			}
			annotations[it.Key] = rev
			res.Reviewed++

			if policy.AutoApprove && score >= policy.MinScore &&
				!hasBlockingCheckFinding(a.blockCheckFindings(ctx, b, sourceLang, model.LocaleID(k.locale), profile, tb, dntTerms)) {
				approvals = append(approvals, approval{item: it, score: score})
			}
		}

		if len(annotations) > 0 {
			if _, aerr := a.hostEngine().RecordAIReviews(ctx, op.Path, src, k.locale, k.file, annotations); aerr != nil {
				return nil, fmt.Errorf("record ai reviews: %w", aerr)
			}
		}
		// Approvals AFTER annotations, so the decision write carries the fresh
		// annotation along (recordDecisionState preserves it).
		for _, ap := range approvals {
			if _, derr := a.hostEngine().ApplyReviewDecisionAs(ctx, op.Path, src,
				host.ReviewUnitRef{File: ap.item.File, Key: ap.item.Key, Locale: ap.item.Locale},
				host.ReviewDecisionApproved, "", state.AIIdentityPrefix+modelID); derr != nil {
				return nil, fmt.Errorf("auto-approve %s:%s: %w", ap.item.File, ap.item.Key, derr)
			}
			res.AutoApproved++
		}
	}
	res.Remaining = res.Reviewed - res.AutoApproved
	return res, nil
}

// hasBlockingCheckFinding reports whether any deterministic-check finding is
// critical or major — the classes that veto an auto-approval regardless of the
// model's score.
func hasBlockingCheckFinding(findings []DesktopFinding) bool {
	for _, f := range findings {
		if f.Severity == "critical" || f.Severity == "major" {
			return true
		}
	}
	return false
}

// reviewerModelID resolves the model identity a pre-review records
// ("ai/<model-id>"): the shared ai.model default, the credential's model, or —
// with neither — the provider id. In tests (aiToolFactory injected) the
// credential store is not consulted.
func (a *App) reviewerModelID() (string, error) {
	cfg := map[string]any{}
	cfg = appconfig.ApplyAIToolDefaults(a.aiConfig, "review", []string{schema.RequiresCredentials}, cfg)
	if a.aiToolFactory == nil {
		resolved, err := credentials.ResolveCredentials(a.credentials, "review", []string{schema.RequiresCredentials}, cfg)
		if err != nil {
			return "", friendlyAIProviderError(err)
		}
		cfg = resolved
	}
	if m, _ := cfg["model"].(string); m != "" {
		return m, nil
	}
	if p, _ := cfg["provider"].(string); p != "" {
		return p, nil
	}
	return "mock", nil
}
