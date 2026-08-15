package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/config"
	"github.com/neokapi/neokapi/host/output"
	"github.com/neokapi/neokapi/memory"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// UpPlanScope is the planned work for one (collection, locale) scope: how many
// units have no target yet, how many of those an exact content-memory hit would cover, and
// what remains for AI translation with a rough token estimate.
type UpPlanScope struct {
	Locale     string `json:"locale,omitempty"`
	Collection string `json:"collection,omitempty"`
	// MissingTarget is the count of translatable units with no committed
	// target for the locale.
	MissingTarget int `json:"missingTarget"`
	// MemoryExact is the count of missing units covered by an exact-hash content-memory hit
	// (the cheap leverage estimate — fuzzy leverage is not counted).
	MemoryExact int `json:"tmExact"`
	// AIRemaining is the count of missing units left for AI translation
	// after content-memory leverage.
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
	Flow   string        `json:"flow,omitempty"`
	Scopes []UpPlanScope `json:"scopes"`
	Totals UpPlanScope   `json:"totals"`
	// Provider is the AI provider a converge run would use (the shared
	// ai.provider default), when one is configured.
	Provider string `json:"provider,omitempty"`
	// Subscription is true when that provider bills a personal subscription
	// (claude-code): the token estimate is scale, not a metered API cost.
	Subscription bool `json:"subscription,omitempty"`
	// Monolingual reports a project that resolves no target locale: there is no
	// per-language work to price. An empty Scopes list otherwise means the work
	// is already done, and the two must not read the same.
	Monolingual bool `json:"monolingual,omitempty"`
	// Note documents the estimation method for agents reading the JSON.
	Note string `json:"note"`
}

// upPlanNote is the estimation-method disclosure carried in the output.
const upPlanNote = "content-memory leverage counts exact-hash hits only; token estimate is source chars / 4 for the remaining units (no tokenizer, no provider calls)."

// upPlanSubscriptionNote replaces the metered-cost framing when the resolved
// provider bills a personal subscription instead of per-token API usage.
const upPlanSubscriptionNote = "AI work runs on your Claude subscription — the token estimate is scale, not a metered cost. Content-memory leverage counts exact-hash hits only; token estimate is source chars / 4 (no tokenizer, no provider calls)."

// FormatText renders the plan as a table.
func (o UpPlanOutput) FormatText(w io.Writer) error {
	if o.Monolingual {
		fmt.Fprintln(w, "No target languages configured: `kapi up` reconciles the source — it seeds the committed context, re-extracts the working tree and refreshes the occurrence graph. Nothing is translated and no provider is called.")
		return nil
	}
	if len(o.Scopes) == 0 {
		fmt.Fprintln(w, "Nothing to do: every unit has a committed target for the current source.")
		return nil
	}
	fmt.Fprintf(w, "Plan for flow %q (dry run — nothing written, no provider calls):\n\n", o.Flow)
	t := output.NewTable(w).Accent(0).
		Headers("scope", "missing", "stale", "content memory exact", "AI work", "~tokens")
	for _, s := range o.Scopes {
		scope := s.Locale
		if s.Collection != "" {
			scope = s.Locale + "/" + s.Collection
		}
		t.Rowf(scope, s.MissingTarget, s.Stale, s.MemoryExact, s.AIRemaining, s.TokenEstimate)
	}
	t.Rowf("total", o.Totals.MissingTarget, o.Totals.Stale, o.Totals.MemoryExact, o.Totals.AIRemaining, o.Totals.TokenEstimate)
	t.Render()
	if o.Totals.Stale > 0 {
		fmt.Fprintf(w, "\n  %d unit(s) stale: their source changed since the translation was decided. "+
			"They are re-drafted against the current source (priced above) and return to review "+
			"un-approved — `kapi status --review`.\n", o.Totals.Stale)
	}
	// Always name the provider a run would resolve. A plan exists to answer
	// "what will this do", and which provider does the work is part of that —
	// it is also the fastest way to see that a configured default is being
	// picked up, which is exactly what silently failed before.
	switch {
	case o.Subscription:
		fmt.Fprintf(w, "\n  AI provider: %s — runs on your Claude subscription (no per-token API cost).\n", o.Provider)
	case o.Provider != "":
		fmt.Fprintf(w, "\n  AI provider: %s\n", o.Provider)
	default:
		fmt.Fprintf(w, "\n  AI provider: none configured — `kapi models setup`, or `kapi models default <model>`.\n")
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

	// Source language: an explicit --source-lang wins; otherwise the project's
	// source_language (the flag's static default would shadow it).
	if !cmd.Flags().Changed("source-lang") && proj.Defaults.SourceLanguage != "" {
		a.SourceLang = string(proj.Defaults.SourceLanguage)
	}
	if a.SourceLang == "" {
		a.SourceLang = "en"
	}

	plan, err := a.computeProjectPlan(ctx, proj, projectPath)
	if err != nil {
		return err
	}
	return output.Print(cmd, plan)
}

// computeProjectPlan resolves the project's units, opens the project content
// memory as a read-only leverage source (only an existing store — a plan must
// not create files), and derives the dry-run work plan. Shared by `kapi up --plan` and
// the exported UpPlan the desktop binds to.
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
	// load-bearing, not decoration: a plan must not create the project store, so
	// an ABSENT store is the legitimate no-leverage case and stays silent. Past
	// the stat the store exists, so a failure to open it can only mean it exists
	// and cannot be read — exactly the case that must not read as "no memory".
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
	// An absent store yields an empty index: no decisions, so nothing is stale.
	var mem memory.ContentMemory
	reviewed := reviewedIndex{}
	if a.MemoryBackend != nil {
		mem = a.MemoryBackend
	}
	layout, lerr := project.LayoutFor(projectPath)
	if lerr != nil {
		return UpPlanOutput{}, fmt.Errorf("resolve project layout for %s: %w", projectPath, lerr)
	}
	if _, statErr := os.Stat(layout.StorePath()); statErr == nil {
		db, derr := a.ProjectDB(ctx, layout.Root)
		if derr != nil {
			return UpPlanOutput{}, fmt.Errorf("open project store at %s: %w — the plan's "+
				"leverage, token and cost figures are computed against it, so they would understate "+
				"the work and overstate the spend; fix or remove the store before planning",
				layout.StorePath(), derr)
		}
		if mem == nil {
			if m := db.Memory(); m != nil {
				mem = m
			}
		}
		if idx, rerr := a.loadReviewedCorrections(ctx, proj, layout.Root); rerr == nil {
			reviewed = idx
		}
	}

	plan, err := a.computeUpPlan(ctx, mem, reviewed, proj, units)
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
func (a *App) applyPlanProvider(plan *UpPlanOutput) {
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
	if sourceLang == "" {
		sourceLang = string(proj.Defaults.SourceLanguage)
	}
	if sourceLang == "" {
		sourceLang = "en"
	}
	a.SourceLang = sourceLang

	plan, err := a.computeProjectPlan(ctx, proj, projectPath)
	if err != nil {
		return nil, err
	}
	return &plan, nil
}

// computeUpPlan derives the per-scope work plan from the verify units.
func (a *App) computeUpPlan(ctx context.Context, tm memory.ContentMemory, reviewed reviewedIndex, proj *project.KapiProject, units []VerifyUnit) (UpPlanOutput, error) {
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

	source := model.LocaleID(a.SourceLang)
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
		for _, b := range blocks {
			if !b.Translatable {
				continue
			}
			// "Already produced" is the shared run-aware presence question
			// (model.RunsHaveContent). Under TargetText() a target whose only run
			// is an inline code flattened to "" and counted as missing, so every
			// `kapi up` pass re-planned and re-translated a unit that was already
			// done — billed AI work on content that needed none.
			if !missing && model.RunsHaveContent(b.TargetRuns(target)) {
				// Produced — but "a target exists" is not "the target translates
				// this source". A unit whose decision blessed wording that has
				// since been rewritten is drift the plan owes the reader, and
				// reporting only presence is what let an edited sentence read as
				// converged while its translation said something else.
				//
				// It is also WORK, and priced below with everything else. A plan
				// that named the drift but quoted nothing for it described a run
				// that would go on to spend tokens the reader had not been shown
				// — the one thing a plan exists to prevent.
				if reviewed.basisFor(b, u.Locale) != basisStale {
					continue
				}
				s.Stale++
			} else {
				s.MissingTarget++
			}
			if planMemoryExactHit(ctx, tm, b, source, target) {
				s.MemoryExact++
				continue
			}
			s.AIRemaining++
			s.TokenEstimate += EstimateTokens(b.SourceText())
		}
	}

	// A recipe with no defaults.flow converges through the built-in default
	// (#1078 G6) — label the plan the same way the run reports it.
	flowLabel := proj.Defaults.Flow
	if flowLabel == "" {
		flowLabel = BuiltinDefaultFlowLabel
	}
	out := UpPlanOutput{Flow: flowLabel, Note: upPlanNote}
	for _, s := range scopes {
		if s.MissingTarget == 0 && s.Stale == 0 {
			continue
		}
		out.Scopes = append(out.Scopes, *s)
		out.Totals.MissingTarget += s.MissingTarget
		out.Totals.Stale += s.Stale
		out.Totals.MemoryExact += s.MemoryExact
		out.Totals.AIRemaining += s.AIRemaining
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
