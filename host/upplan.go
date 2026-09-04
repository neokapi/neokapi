package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/host/config"
	"github.com/neokapi/neokapi/host/output"
	"github.com/neokapi/neokapi/memory"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// UpPlanScope is the planned work for one (collection, locale) scope: the units
// a convergence pass would spend work on, split by why they are work, how many of
// them an exact content-memory hit would cover, and what remains for AI
// translation with a rough token estimate.
//
// The three work axes partition the scope's work, so MissingTarget + Stale +
// Unanswered always equals MemoryExact + Drafts + AIRemaining + OutOfReach:
// every unit the plan counts is recycled, served from a stored draft, drafted,
// or beyond what the configured flow can do. A unit that is none of the four is
// not work: it holds a target, no decision of its has moved, and the corpus
// answers its source, so the pass fills it from the project's own record at no
// cost.
type UpPlanScope struct {
	Locale     string `json:"locale,omitempty"`
	Collection string `json:"collection,omitempty"`
	// MissingTarget is the count of translatable units with no committed
	// target for the locale.
	MissingTarget int `json:"missingTarget"`
	// MemoryExact is the count of counted units covered by an exact-hash
	// content-memory hit (the cheap leverage estimate — fuzzy leverage is not
	// counted).
	MemoryExact int `json:"tmExact"`
	// Drafts is the count of counted units the pass serves from the project
	// block store instead of a provider. The drafting step already holds a
	// translation of the unit's current source, made under the configuration it
	// would send now and the governing context in force, so it reuses that
	// answer and calls nobody (blockstore.TargetOverlay.ReusableFor). A parked
	// locale's whole draft set reads this way on the run after the one that
	// drafted it. They carry no token estimate.
	//
	// The question is put to a producer built the way the run builds one
	// (tool.StoredTargetReuser), so the plan and the run answer it from one
	// function. Where no producer can be built, a unit stays under AIRemaining:
	// the plan is an upper bound on what a run can spend.
	Drafts int `json:"drafts,omitempty"`
	// AIRemaining is the count of counted units left for AI translation after
	// content-memory leverage and stored drafts: the provider calls a run
	// makes.
	AIRemaining int `json:"aiRemaining"`
	// Stale is the count of units that HAVE a committed target whose decision
	// was recorded against source wording that has since changed. The run
	// re-drafts them — recycle against the new wording, AI for the remainder —
	// so they are PRICED into MemoryExact/AIRemaining/TokenEstimate exactly as a
	// unit with no target is. They are reported on their own axis as well, and
	// not folded into MissingTarget, because the two ask different things of the
	// reader: missing work finishes when the loop finishes it, stale work also
	// owes a review, and the tokens are being spent on a unit a person had
	// already decided.
	Stale int `json:"stale,omitempty"`
	// Unanswered is the count of units that HAVE a committed target which the
	// project content memory does not answer: no decision of theirs has moved,
	// but the corpus holds no exact answer for their source, so `recycle` cannot
	// fill them and the pass drafts them again over what is on disk.
	//
	// It is its own axis because it is its own fact. `stale` means a decision's
	// basis moved — it drives `blocked: stale`, the review worklist and shipping —
	// and a produced unit the record never paired is none of that. Folding the two
	// would put the plan and the run summary at odds, since the summary's re-draft
	// count is the decision-based coverage tally.
	Unanswered int `json:"unanswered,omitempty"`
	// OutOfReach is the count of counted units the configured flow has no step
	// that could produce: the corpus does not answer them and the flow drafts
	// nothing, so a run would leave them exactly as they are.
	//
	// They are neither leverage nor AI work, and pricing them was the whole
	// defect: a recipe whose flow is exact-match content-memory reuse and
	// nothing else was quoted a provider bill for every unit the memory missed,
	// then made zero provider calls (#1866).
	OutOfReach int `json:"outOfReach,omitempty"`
	// UnreadTargets is the count of produced units the plan declines to judge:
	// their committed translation has not been read into the project store yet,
	// so the corpus is unfinished and its silence about them means "not asked",
	// not "the pass will draft this". A run reads them first — the seed phase
	// absorbs the committed record before the pass — which is why this is a
	// disclosure and not an axis of work: pricing them would quote a provider
	// call for every translation the run is about to recycle, and quoting zero
	// would promise a free run.
	UnreadTargets int `json:"unreadTargets,omitempty"`
	// TokenEstimate approximates the input tokens for the remaining AI work:
	// source characters / 4 (a common chars-per-token heuristic — the
	// providers expose no tokenizer here, so this is an estimate, not a
	// quote).
	TokenEstimate int `json:"tokenEstimate"`
}

// UpPlanOutput is the structured result of `kapi up --plan`: the dry-run work
// plan per (collection, locale), with totals. No provider calls are made and
// nothing is written.
type UpPlanOutput struct {
	Flow string `json:"flow,omitempty"`
	// FlowDrafts reports whether the flow contains any step that produces a
	// target other than by content-memory recycling. False means the flow
	// recycles and nothing else, so every unit the corpus does not answer is
	// out of reach of this run.
	FlowDrafts bool `json:"flowDrafts"`
	// FlowCallsProvider reports whether producing a target under this flow
	// reaches an AI or machine-translation provider. False means the run makes
	// no provider calls and spends nothing, whatever a default provider is
	// configured to.
	FlowCallsProvider bool          `json:"flowCallsProvider"`
	Scopes            []UpPlanScope `json:"scopes"`
	Totals            UpPlanScope   `json:"totals"`
	// Provider is the AI provider a converge run would use (the shared
	// ai.provider default), when one is configured.
	Provider string `json:"provider,omitempty"`
	// Subscription is true when that provider bills a personal subscription
	// (claude-code): the token estimate is scale, not a metered API cost.
	Subscription bool `json:"subscription,omitempty"`
	// Monolingual reports a project that resolves no target locale: there is no
	// per-language work to price. An empty Scopes list otherwise means the work
	// is already done — or, with UnreadTargets set, that the plan has not been
	// able to judge it yet. The three must not read the same.
	Monolingual bool `json:"monolingual,omitempty"`
	// Note documents the estimation method for agents reading the JSON.
	Note string `json:"note"`
}

// upPlanNote is the estimation-method disclosure carried in the output.
const upPlanNote = "content-memory leverage counts exact-hash hits only; token estimate is source chars / 4 for the remaining units (no tokenizer, no provider calls)."

// upPlanSubscriptionNote replaces the metered-cost framing when the resolved
// provider bills a personal subscription instead of per-token API usage.
const upPlanSubscriptionNote = "AI work runs on your Claude subscription, so the token estimate is scale rather than a metered cost. Content-memory leverage counts exact-hash hits only; token estimate is source chars / 4 (no tokenizer, no provider calls)."

// FormatText renders the plan as a table.
func (o UpPlanOutput) FormatText(w io.Writer) error {
	if o.Monolingual {
		fmt.Fprintln(w, "No target languages configured: `kapi up` reconciles the source. It seeds the committed context, re-extracts the working tree and refreshes the occurrence graph. Nothing is translated and no provider is called.")
		return nil
	}
	if len(o.Scopes) == 0 {
		if o.Totals.UnreadTargets > 0 {
			fmt.Fprintf(w, "Not priced yet: this project's store has not read its committed translations, so "+
				"what a run would recycle, and what it would draft, is not known. `kapi up` reads them "+
				"before the pass; %d produced unit(s) wait on that.\n", o.Totals.UnreadTargets)
			return nil
		}
		fmt.Fprintln(w, "Nothing to do: every unit has a committed target the project's content memory answers.")
		return nil
	}
	fmt.Fprintf(w, "Plan for flow %q (dry run: nothing written, no provider calls):\n\n", o.Flow)
	t := output.NewTable(w).Accent(0).
		Headers("scope", "missing", "stale", "unanswered", "content memory exact",
			"stored drafts", "drafted by flow", "out of reach", "~tokens")
	for _, s := range o.Scopes {
		scope := s.Locale
		if s.Collection != "" {
			scope = s.Locale + "/" + s.Collection
		}
		t.Rowf(scope, s.MissingTarget, s.Stale, s.Unanswered, s.MemoryExact,
			s.Drafts, s.AIRemaining, s.OutOfReach, s.TokenEstimate)
	}
	t.Rowf("total", o.Totals.MissingTarget, o.Totals.Stale, o.Totals.Unanswered,
		o.Totals.MemoryExact, o.Totals.Drafts, o.Totals.AIRemaining, o.Totals.OutOfReach, o.Totals.TokenEstimate)
	t.Render()
	if o.Totals.Stale > 0 {
		fmt.Fprintf(w, "\n  %d unit(s) stale: their source changed since the translation was decided. "+
			"They are re-drafted against the current source (priced above) and return to review "+
			"un-approved. See `kapi status --review`.\n", o.Totals.Stale)
	}
	if o.Totals.Unanswered > 0 {
		fmt.Fprintf(w, "\n  %d unit(s) unanswered: they hold a target the project's content memory does not "+
			"account for, so the pass drafts them (priced above) and the draft replaces what is on disk.\n",
			o.Totals.Unanswered)
	}
	if o.Totals.Drafts > 0 {
		fmt.Fprintf(w, "\n  %d unit(s) served from stored drafts: the project store already holds a translation "+
			"of their current source, made under the configuration and governing context this run would use, "+
			"so the pass reuses it and calls no provider for them.\n", o.Totals.Drafts)
	}
	if o.Totals.UnreadTargets > 0 {
		fmt.Fprintf(w, "\n  %d produced unit(s) are not priced: the project store has not read their committed "+
			"translations yet, and a run reads them before it drafts anything.\n", o.Totals.UnreadTargets)
	}
	if o.Totals.OutOfReach > 0 {
		fmt.Fprintf(w, "\n  %d unit(s) out of reach of flow %q: it has no step that would produce them, "+
			"so this run leaves them as they are. They are not priced. Add a drafting step, or "+
			"converge them at a venue whose flow has one.\n", o.Totals.OutOfReach, o.Flow)
	}
	// The flow decides whether there is a bill at all, so it is read before the
	// configured default: naming a provider for a flow that never calls one is
	// how a run with no credentials was quoted 487k tokens (#1866).
	if !o.FlowCallsProvider {
		if o.Totals.AIRemaining > 0 {
			fmt.Fprintf(w, "\n  Flow %q produces its drafts locally: no provider is called and nothing is spent.\n", o.Flow)
		} else {
			fmt.Fprintf(w, "\n  Flow %q calls no provider, so this run spends nothing.\n", o.Flow)
		}
		fmt.Fprintf(w, "\n%s\n", o.Note)
		return nil
	}
	// Always name the provider a run would resolve. A plan exists to answer
	// "what will this do", and which provider does the work is part of that —
	// it is also the fastest way to see that a configured default is being
	// picked up, which is exactly what silently failed before.
	switch {
	case o.Subscription:
		fmt.Fprintf(w, "\n  AI provider: %s, running on your Claude subscription (no per-token API cost).\n", o.Provider)
	case o.Provider != "":
		fmt.Fprintf(w, "\n  AI provider: %s\n", o.Provider)
	default:
		fmt.Fprintf(w, "\n  AI provider: none configured. Run `kapi models setup`, or `kapi models default <model>`.\n")
	}
	fmt.Fprintf(w, "\n%s\n", o.Note)
	return nil
}

// runUpPlan is `kapi up --plan`: a read-only dry run of the convergence work.
// For every (collection, locale) scope it counts the units missing a target,
// estimates content-memory leverage by exact-hash lookup against the project content memory, and
// prices the remainder as AI work with a chars/4 token estimate. It makes no
// provider calls and writes nothing — not even the block store.
func (a *App) runUpPlan(cmd Command, proj *project.KapiProject, projectPath string) error {
	ctx := cmd.Context()
	ctx = ctxOrBackground(ctx)

	// The plan is priced in the language the run would work in, which is the
	// recipe's unless --source-lang names another (host/sourcelang.go).
	a.ResolveSourceLang(proj.Defaults.SourceLanguage)

	plan, err := a.computeProjectPlan(ctx, proj, projectPath)
	if err != nil {
		return err
	}
	return output.Print(cmd, plan)
}

// computeProjectPlan resolves the project's units, binds the content memory a
// run would recycle from as a read-only leverage source, and derives the dry-run
// work plan. Shared by `kapi up --plan` and the exported UpPlan the desktop
// binds to.
func (a *App) computeProjectPlan(ctx context.Context, proj *project.KapiProject, projectPath string) (UpPlanOutput, error) {
	root := filepath.Dir(projectPath)
	units, err := a.UnitsFromProject(proj, root, "")
	if err != nil {
		return UpPlanOutput{}, fmt.Errorf("resolve content: %w", err)
	}

	// The plan IS a number, so a content memory that cannot be read is a fault,
	// not a degradation: with mem nil every unit reports zero leverage, and the
	// plan quotes the token and cost estimate for translating from scratch a
	// project the memory already largely covers. The user approves a far larger —
	// and more expensive — run than reality warrants, off a figure that looks
	// authoritative. A wrong number here costs real money.
	//
	// The os.Stat guard below draws the distinction this family needs and it is
	// load-bearing, not decoration: past the stat the store exists, so a failure
	// to open it can only mean it exists and cannot be read — exactly the case
	// that must not read as "no memory". An ABSENT store is a different fact: it
	// says nothing about the corpus, only that nothing has projected it yet, and
	// the branch below reads the committed bundles instead.
	//
	// It stats the store rather than opening it and asking, because opening
	// CREATES it: the handle runs every subsystem's migrations at open. A plan
	// that left a `.kapi/work/store.db` behind would be a dry run with a side effect,
	// and the next `up` would find a store it did not write.
	// The decisions the project already holds are read under the same guard, and
	// for the same reason: a unit whose basis no longer matches its source is
	// work the plan owes the reader. Read out of the store on its own axis, not
	// inside the memory branch — an injected MemoryBackend says where leverage
	// comes from, and says nothing about whether the project has decisions.
	// An absent store yields an empty index: no decisions, so nothing is stale,
	// and no artifact has been absorbed, so the corpus has not finished being
	// taught and a produced unit is not judged by it.
	basis := upPlanBasis{memory: a.MemoryBackend, root: root, projectPath: projectPath}
	layout, lerr := project.LayoutFor(projectPath)
	if lerr != nil {
		return UpPlanOutput{}, fmt.Errorf("resolve project layout for %s: %w", projectPath, lerr)
	}
	if _, statErr := os.Stat(layout.StorePath()); statErr == nil {
		db, derr := a.ProjectDB(ctx, layout.Root)
		if derr != nil {
			return UpPlanOutput{}, fmt.Errorf("open project store at %s: %w. The plan's "+
				"leverage, token and cost figures are computed against it, so they would understate "+
				"the work and overstate the spend; fix or remove the store before planning",
				layout.StorePath(), derr)
		}
		if basis.memory == nil {
			if m := db.Memory(); m != nil {
				basis.memory = m
			}
		}
		if idx, rerr := a.loadReviewedCorrections(ctx, proj, layout.Root); rerr == nil {
			basis.reviewed = idx
		}
		basis.settled = a.recordSettlement(ctx, db, proj, projectPath, layout.Root)
		basis.docs = a.documentIndexOrEmpty(ctx, layout.Root)
		// The stored drafts a pass would serve instead of calling a provider
		// live in the same store, and only there: past the stat above the store
		// exists, so asking it creates nothing.
		basis.store = a.storedTargetStore(ctx, layout.Root)
	} else if basis.memory == nil {
		// No store on disk — a fresh checkout, which is exactly the leg a
		// pull-request CI job runs. The corpus a run recycles from is still
		// there: the committed bundles under `.kapi/memory/`, which the run
		// seeds into the store before it converges. Reading only the store here
		// priced from scratch the work git already carries reviewed wording for.
		//
		// So the bundles are compiled into a corpus that exists only for this
		// call and is discarded with it. The plan reads what the run will read,
		// and the checkout is left as git wrote it — the promise the stat above
		// exists to keep.
		mem, release, merr := a.CommittedMemoryView(ctx, proj, layout)
		if merr != nil {
			return UpPlanOutput{}, merr
		}
		defer release()
		basis.memory = mem
	}

	plan, err := a.computeUpPlan(ctx, basis, proj, units)
	if err != nil {
		return plan, err
	}
	plan.Monolingual = !proj.DeclaresTargetLanguages()
	a.applyPlanProvider(&plan)
	return plan, nil
}

// applyPlanProvider annotates a plan with the AI provider a converge run would
// resolve (the shared ai.provider app default). When that provider bills a
// personal subscription (claude-code), the plan swaps the metered-cost framing
// for "runs on your Claude subscription" in both text and JSON.
//
// A flow that reaches no provider is left unannotated: which provider the app
// would resolve is not a fact about a run that will never call one.
func (a *App) applyPlanProvider(plan *UpPlanOutput) {
	if !plan.FlowCallsProvider {
		return
	}
	def := config.ResolveAIDefault(a.Config)
	if !def.Configured() {
		return
	}
	prov := def.Provider
	plan.Provider = prov
	if info, ok := aiprovider.ProviderInfoFor(aiprovider.ProviderID(prov)); ok && info.Subscription {
		plan.Subscription = true
		plan.Note = upPlanSubscriptionNote
	}
}

// UpPlan computes the dry-run convergence plan `kapi up --plan` reports — per
// (collection, locale): units missing a target, exact content-memory leverage, the
// remaining AI work, and a chars/4 token estimate — for an embedding caller
// (the desktop's pre-flight dialog). It is read-only and self-contained: no
// provider calls, nothing written, state derived from the working tree on
// every call. sourceLang overrides the project's source language when
// non-empty.
func (a *App) UpPlan(ctx context.Context, projectPath, sourceLang string) (*UpPlanOutput, error) {
	a.InitRegistries()
	ctx = ctxOrBackground(ctx)
	proj, err := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return nil, fmt.Errorf("load project: %w", err)
	}
	// Bounded to this call: the embedding caller's App outlives it and may plan a
	// different project next, which must not inherit this recipe's language.
	defer a.scopeSourceLang()()
	a.SourceLang = ResolveSourceLocale(sourceLang, proj.Defaults.SourceLanguage)

	plan, err := a.computeProjectPlan(ctx, proj, projectPath)
	if err != nil {
		return nil, err
	}
	return &plan, nil
}

// upPlanFlow is what the plan needs to know about the flow a run would actually
// execute: which of the ways a unit can be produced this flow contains, and
// whether any of them costs a provider call.
//
// It is read out of the tool registry rather than matched on tool names, so a
// plugin's producer is classified by what it declares — a step that produces a
// target and reads the content memory is leverage, one that produces a target
// and calls out is spend — and a flow assembled from tools this binary has
// never heard of is planned as honestly as the built-in one.
type upPlanFlow struct {
	// Name is the name the flow is registered as, and Spec its steps as the
	// pass executes them, from the same resolution the run makes
	// (convergeFlowSpec).
	Name string
	Spec *flow.StepsSpec
	// Label is how the run reports the flow.
	Label string
	// Recycles is true when a step fills a target from the content memory.
	Recycles bool
	// Drafts is true when a step produces a target some other way — an AI or
	// machine-translation provider, or a local transform.
	Drafts bool
	// CallsProvider is true when one of those drafting steps reaches a provider,
	// which is the only case a token estimate prices. A flow that drafts
	// locally (pseudo-translation, say) does the work and bills nothing.
	CallsProvider bool
}

// resolveUpPlanFlow resolves the flow `kapi up` would run for the project and
// reads its production capabilities off the tool registry. It resolves the flow
// exactly as RunDefaultFlowConverge does, including its errors: a plan that
// quotes figures for a run that cannot start is the same fault as a plan that
// quotes figures the run would not produce.
func (a *App) resolveUpPlanFlow(proj *project.KapiProject) (upPlanFlow, error) {
	cf, err := convergeFlowSpec(proj)
	if err != nil {
		return upPlanFlow{Label: cf.label}, err
	}
	out := upPlanFlow{Name: cf.name, Spec: cf.spec, Label: cf.label}

	for _, step := range cf.spec.Steps {
		s := a.ToolReg.Schema(registry.ToolID(step.Tool))
		if !toolProducesTarget(s) {
			continue
		}
		if ToolRequires(s, schema.RequiresMemory) {
			out.Recycles = true
			continue
		}
		out.Drafts = true
		if toolCallsProvider(s) {
			out.CallsProvider = true
		}
	}
	return out, nil
}

// toolProducesTarget reports whether the tool writes target-side content — the
// question that decides whether a step can close a pending unit at all.
func toolProducesTarget(s *schema.ComponentSchema) bool {
	if s == nil || s.ToolMeta == nil {
		return false
	}
	for _, p := range s.ToolMeta.Produces {
		if p.Type == schema.PortTarget && p.Side == model.SideTarget {
			return true
		}
	}
	return false
}

// toolCallsProvider reports whether running the tool reaches an external
// provider — the side effect a token estimate is an estimate of.
func toolCallsProvider(s *schema.ComponentSchema) bool {
	if s == nil || s.ToolMeta == nil {
		return false
	}
	return slices.Contains(s.ToolMeta.SideEffects, schema.SideEffectAPICall)
}

// upPlanBasis is what a plan is derived against besides the units themselves:
// the corpus a run would recycle from, the decisions it would read, and the
// target artifacts the record absorber has already had its say about.
type upPlanBasis struct {
	memory   memory.ContentMemory
	reviewed reviewedIndex
	// settled keys absolute target paths; see App.recordSettlement.
	settled map[string]bool
	// root is the project root a unit's source path is resolved against to name
	// the document its decision was recorded in.
	root string
	// projectPath is the recipe the plan is for, which the drafting producers
	// are built against when the store holds drafts to ask about.
	projectPath string
	// docs turns that path into the document's durable key, so a decision made
	// before the file was renamed still answers for it.
	docs DocumentIndex
	// store is the project block store holding the `targets/<locale>` overlays a
	// drafting step serves instead of calling out, or nil when the project has
	// none yet: nothing is stored, so nothing is reused.
	store blockstore.Store
}

// computeUpPlan derives the per-scope work plan from the verify units, against
// the flow a run would execute: what that flow can produce is what the plan
// counts as work, and what it cannot is reported rather than priced.
func (a *App) computeUpPlan(ctx context.Context, basis upPlanBasis, proj *project.KapiProject, units []VerifyUnit) (UpPlanOutput, error) {
	fl, ferr := a.resolveUpPlanFlow(proj)
	if ferr != nil {
		return UpPlanOutput{}, ferr
	}
	reuse := a.newPlanReuse(ctx, basis, proj, fl)
	defer reuse.close()

	type scopeKey struct{ Locale, Collection string }
	scopes := map[scopeKey]*UpPlanScope{}
	scopeFor := func(k scopeKey) *UpPlanScope {
		if s, ok := scopes[k]; ok {
			return s
		}
		s := &UpPlanScope{Locale: k.Locale, Collection: k.Collection}
		scopes[k] = s
		return s
	}

	source := model.LocaleID(a.SourceLocale())
	for _, u := range units {
		blocks, missing, berr := a.bilingualBlocks(ctx, u)
		if berr != nil {
			if errors.Is(berr, errTargetUnreadable) {
				continue // unmeasurable target — coverage counts it by presence
			}
			return UpPlanOutput{}, berr
		}
		if missing {
			// No target file yet: every translatable source unit is pending.
			srcs, serr := a.readSource(ctx, u)
			if serr != nil {
				return UpPlanOutput{}, serr
			}
			blocks = srcs
		}
		s := scopeFor(scopeKey{Locale: u.Locale, Collection: u.Collection})
		target := model.LocaleID(u.Locale)
		// Whether the record absorber has already read this artifact at the bytes
		// on disk, which decides whether the corpus's silence about its units is
		// final (see App.recordSettlement).
		settled := basis.settled[u.TargetPath]
		delivered := !missing && targetDelivered(u.TargetPath)
		scope := basis.docs.Scope(basis.root, u.SourcePath)
		for _, b := range blocks {
			if !b.Translatable {
				continue
			}
			// "Already produced" is the shared run-aware presence question
			// (model.RunsHaveContent). Under TargetText() a target whose only run
			// is an inline code flattened to "" and counted as missing, so every
			// `kapi up` pass re-planned and re-translated a unit that was already
			// done — billed AI work on content that needed none.
			produced := !missing && model.RunsHaveContent(b.TargetRuns(target))
			// A produced unit whose corpus answer the absorber has not settled yet
			// cannot be judged: the seed phase runs before the pass and may pair
			// this very target with its source. Asking now would price a provider
			// call for every translation the run is about to recycle — so it is
			// reported as unread rather than guessed at either way.
			//
			// That holds for a delivered file. A produced unit with no file on
			// disk is a parked locale's draft, read out of the block store: there
			// is no committed translation for the absorber to read, so the
			// corpus's silence about it is final, and the reuse question below is
			// what prices it.
			if produced && delivered && !settled && basis.reviewed.basisFor(scope, b, u.Locale) != basisStale {
				s.UnreadTargets++
				continue
			}
			// The pass is driven by the corpus, not by the target files: a recycle
			// step fills what the content memory answers exactly and a drafting
			// step produces the remainder (skipMatched). So this is the question
			// that decides what a unit costs, and it is put to every unit the plan
			// counts. It is a fact about the corpus alone — what the configured
			// flow does with the answer is decided below.
			answered := planMemoryExactHit(ctx, basis.memory, b, source, target)
			switch {
			case !produced:
				s.MissingTarget++
			case basis.reviewed.basisFor(scope, b, u.Locale) == basisStale:
				// A unit whose decision blessed wording that has since been
				// rewritten is drift the plan owes the reader, and reporting only
				// presence is what let an edited sentence read as converged while
				// its translation said something else.
				s.Stale++
			case answered:
				// Produced, current, and the corpus answers it: `recycle` fills the
				// unit from the project's own record and the AI step skips it.
				// Nothing to do, so nothing to price.
				continue
			default:
				// Produced, and the record does not pair it with its source — a
				// rewritten source, an identical pair no approval stands behind, a
				// pair refused for asymmetric codes. `recycle` has nothing to fill
				// it with, so the pass drafts it and the draft replaces what is on
				// disk. Counting it by target presence quoted zero for exactly this
				// work.
				s.Unanswered++
			}
			// Leverage is only leverage for a flow that recycles. A flow with no
			// recycle step reaches the corpus's answer in no way at all, so
			// counting the hit would credit this run with work another flow
			// would do.
			if answered && fl.Recycles {
				s.MemoryExact++
				continue
			}
			// Nothing in this flow can produce the unit: it has no step that
			// writes a target other than by recycling, and recycling is not on
			// offer here. A run would leave the unit as it is, so it is reported
			// as out of reach rather than quoted.
			if !fl.Drafts {
				s.OutOfReach++
				continue
			}
			// The drafting step asks the block store before it calls out, and
			// serves a stored translation of this exact source made under its
			// current configuration and context (#2356). That is the reuse
			// question put to a producer built as the run builds one, so what it
			// says yes to costs nothing here, and nothing at run time.
			reused, rerr := reuse.reuses(ctx, u, b)
			if rerr != nil {
				return UpPlanOutput{}, rerr
			}
			if reused {
				s.Drafts++
				continue
			}
			s.AIRemaining++
			// Only a provider call costs tokens. A flow that drafts locally
			// produces the same units and bills nothing, and quoting it a token
			// figure would invite approval of a spend that cannot happen.
			if fl.CallsProvider {
				s.TokenEstimate += EstimateTokens(b.SourceText())
			}
		}
	}

	out := UpPlanOutput{Flow: fl.Label, FlowDrafts: fl.Drafts,
		FlowCallsProvider: fl.CallsProvider, Note: upPlanNote}
	for _, s := range scopes {
		// Unread targets are totalled from every scope, priced or not: a scope
		// with no work to show still owes the reader the reason it has nothing
		// to say about the units it holds.
		out.Totals.UnreadTargets += s.UnreadTargets
		if s.MissingTarget == 0 && s.Stale == 0 && s.Unanswered == 0 {
			continue
		}
		out.Scopes = append(out.Scopes, *s)
		out.Totals.MissingTarget += s.MissingTarget
		out.Totals.Stale += s.Stale
		out.Totals.Unanswered += s.Unanswered
		out.Totals.MemoryExact += s.MemoryExact
		out.Totals.Drafts += s.Drafts
		out.Totals.AIRemaining += s.AIRemaining
		out.Totals.OutOfReach += s.OutOfReach
		out.Totals.TokenEstimate += s.TokenEstimate
	}
	sort.Slice(out.Scopes, func(i, j int) bool {
		if out.Scopes[i].Locale != out.Scopes[j].Locale {
			return out.Scopes[i].Locale < out.Scopes[j].Locale
		}
		return out.Scopes[i].Collection < out.Scopes[j].Collection
	})
	return out, nil
}

// targetDelivered reports whether a unit's target file is on disk. A produced
// unit without one was read out of the block store, which is what a parked
// locale's drafts look like between the run that drafted them and the review
// that delivers them.
func targetDelivered(targetPath string) bool {
	_, err := os.Stat(targetPath)
	return err == nil
}

// planMemoryExactHit reports whether the block's source has an unambiguous exact
// (score 1.0) content-memory hit with a non-empty target variant — the cheap "content memory exact"
// leverage counted by the plan.
func planMemoryExactHit(ctx context.Context, tm memory.ContentMemory, b *model.Block, source, target model.LocaleID) bool {
	if tm == nil {
		return false
	}
	matches, err := tm.Lookup(ctx, b, source, target, memory.LookupOptions{MinScore: 1.0, MaxResults: 1})
	if err != nil || len(matches) == 0 || matches[0].Ambiguous || matches[0].Score < 1.0 {
		return false
	}
	return matches[0].Entry.VariantText(target) != ""
}

// EstimateTokens approximates the token count of a text as chars/4 (rounded
// up) — the widely used chars-per-token rule of thumb. The AI providers here
// expose no tokenizer, so a real count is not available without a call.
func EstimateTokens(text string) int {
	chars := utf8.RuneCountInString(text)
	if chars == 0 {
		return 0
	}
	return (chars + 3) / 4
}
