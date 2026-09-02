package host

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/tool"
	coretools "github.com/neokapi/neokapi/core/tools"
)

// CheckExclusions is the check-findings set (#1078 C2/G4): the units that are
// produced but failing guardrails. A unit in the set keeps the rung it holds
// when ComputeShipCoverage tallies it, and is recorded as a failing check on
// its scope — which withholds that scope's ship and verified verdicts until the
// finding is fixed, so a locale cannot clear its gate on the back of
// translations that fail the project's bound checks.
type CheckExclusions struct {
	// failing keys are ExclusionKey(sourcePath, blockKey, locale).
	Failing map[string]bool
	// ByLocale counts the failing units per locale.
	ByLocale map[string]int
}

func ExclusionKey(sourcePath, blockKey, locale string) string {
	return sourcePath + "\x00" + blockKey + "\x00" + locale
}

// excluded reports whether the block's unit fails the loop checks for the locale.
func (e *CheckExclusions) excluded(sourcePath string, b *model.Block, locale string) bool {
	if e == nil || e.Failing == nil {
		return false
	}
	return e.Failing[ExclusionKey(sourcePath, blockKey(b), locale)]
}

// totalFailing returns the count of failing units across every locale (0 when nil).
func (e *CheckExclusions) totalFailing() int {
	if e == nil {
		return 0
	}
	total := 0
	for _, n := range e.ByLocale {
		total += n
	}
	return total
}

// computeLoopCheckExclusions runs the project's bound target-side checks over
// the produced units — the same engines `kapi check --ship` gates on: the
// rule-based checkset (placeholder/tag integrity, plus the default placeholder
// patterns) always, and the terminology check when the project binds a terms
// store. A unit whose findings fail the ship predicate (any critical/major
// finding, or an integrity category like pattern-mismatch) enters the set.
//
// Cost: checks only run over units whose target exists and is readable — an
// untranslated unit has nothing to check — and they are annotate-only over
// blocks the coverage pass reads anyway (the parse cache absorbs the re-read).
// That bound is what lets every verdict-publishing surface afford to run them,
// which is what keeps their answers the same (#2024).
func (a *App) computeLoopCheckExclusions(ctx context.Context, cmd Command, proj *project.KapiProject, root string, units []VerifyUnit) (*CheckExclusions, error) {
	excl := &CheckExclusions{Failing: map[string]bool{}, ByLocale: map[string]int{}}

	// The same rule `kapi check`'s checks gate applies to a target identical to its
	// source. This set feeds the ship gate, so a question the two surfaces answer
	// differently is a unit that passes the check and is held back by the
	// coverage, in one project, on one run.
	identical, err := a.newIdenticalTargetRule(ctx, cmd, proj, root)
	if err != nil {
		return nil, err
	}

	// Term rules per locale, resolved once (opens the terms store).
	preferredTermsByLocale := map[string][]coreprofile.TermRule{}
	termRulesFor := func(locale string) ([]coreprofile.TermRule, error) {
		if g, ok := preferredTermsByLocale[locale]; ok {
			return g, nil
		}
		g, err := a.ResolveTermRules(cmd, locale)
		if err != nil {
			return nil, err
		}
		preferredTermsByLocale[locale] = g
		return g, nil
	}

	for _, u := range units {
		blocks, missing, berr := a.bilingualBlocks(ctx, u)
		if berr != nil {
			if errors.Is(berr, errTargetUnreadable) {
				continue // unmeasurable target (e.g. a compiled .mo) — can't check
			}
			return nil, berr
		}
		if missing {
			continue // untranslated — there is no translation to check
		}

		rules, gerr := termRulesFor(u.Locale)
		if gerr != nil {
			return nil, gerr
		}
		var termTool BlockProcessor
		if len(rules) > 0 {
			termTool = coretools.NewTermCheckTool(&coretools.TermCheckConfig{
				TermRules:    rules,
				TargetLocale: model.LocaleID(u.Locale),
			})
		}

		qaCfg := coretools.NewQACheckConfig(model.LocaleID(u.Locale))
		qaCfg.CheckPlaceholders = true
		qa := coretools.NewQACheckTool(qaCfg)

		for _, b := range blocks {
			if !b.Translatable {
				continue
			}
			// Only produced units are checkable: an empty target is pending
			// work, not a guardrail failure. Presence is the shared run-aware
			// predicate — under TargetText() a target whose only run is an inline
			// code read as empty, so the guardrails silently never saw it and it
			// reached the ship gate unchecked.
			if !model.RunsHaveContent(b.TargetRuns(model.LocaleID(u.Locale))) {
				continue
			}
			// A checker that could not run must not read as a checker that
			// passed: this exclusion set feeds the ship gate, so swallowing the
			// error would readmit a failing unit as shippable — "the operation
			// failed and the system reports success" applied to the gate itself.
			if err := RunCheckTool(ctx, qa, b); err != nil {
				return nil, fmt.Errorf("qa check %s (%s): %w", u.DisplayPath, u.Locale, err)
			}
			fails := slices.ContainsFunc(check.Findings(tool.NewBlockViewWithContext(ctx, b)), func(f check.Finding) bool {
				return !identical.suppresses(f, u.SourcePath, b, u.Locale) && qaFindingFails(f)
			})
			if !fails && termTool != nil {
				if err := RunCheckTool(ctx, termTool, b); err != nil {
					return nil, fmt.Errorf("terminology check %s (%s): %w", u.DisplayPath, u.Locale, err)
				}
				if b.Properties[coretools.PropTermCheckPassed] == "false" {
					fails = true
				}
			}
			if fails {
				key := ExclusionKey(u.SourcePath, blockKey(b), u.Locale)
				if !excl.Failing[key] {
					excl.Failing[key] = true
					excl.ByLocale[u.Locale]++
				}
			}
		}
	}
	return excl, nil
}
