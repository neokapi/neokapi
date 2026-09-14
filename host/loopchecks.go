package host

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
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
	// TermsGoverned records, per unit, whether terms govern it: whether any
	// concept bound at the unit's point answers for its locale
	// (terms.RulesFromConcepts). Keys are TermsKey(sourcePath, locale). A unit
	// absent from the map was not resolved.
	TermsGoverned map[string]bool
}

// TermsKey keys a unit in CheckExclusions.TermsGoverned.
func TermsKey(sourcePath, locale string) string {
	return sourcePath + "\x00" + locale
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

// termsGovern reports whether terms govern the unit. It is false for a nil set,
// which means the checks did not run and nothing was resolved.
func (e *CheckExclusions) termsGovern(u VerifyUnit) bool {
	return e != nil && e.TermsGoverned[TermsKey(u.SourcePath, u.Locale)]
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
// patterns) always, and the terminology check over each unit terms govern at
// its point. A unit whose findings fail the ship predicate (any critical/major
// finding, or an integrity category like pattern-mismatch) enters the set.
//
// Cost: checks only run over units whose target exists and is readable — an
// untranslated unit has nothing to check — and they are annotate-only over
// blocks the coverage pass reads anyway (the parse cache absorbs the re-read).
// That bound is what lets every verdict-publishing surface afford to run them,
// which is what keeps their answers the same (#2024).
func (a *App) computeLoopCheckExclusions(ctx context.Context, cmd Command, proj *project.KapiProject, root string, units []VerifyUnit) (*CheckExclusions, error) {
	return a.loopCheckExclusions(ctx, cmd, proj, root, units, nil)
}

// loopCheckExclusions is computeLoopCheckExclusions for a check over the
// project's declared content: a unit whose source no installed reader opens is
// skipped and recorded in unread. With unread nil such a unit fails the run.
func (a *App) loopCheckExclusions(ctx context.Context, cmd Command, proj *project.KapiProject, root string, units []VerifyUnit, unread *UnreadSet) (*CheckExclusions, error) {
	excl := &CheckExclusions{Failing: map[string]bool{}, ByLocale: map[string]int{}, TermsGoverned: map[string]bool{}}

	// The same rule `kapi check`'s checks gate applies to a target identical to its
	// source. This set feeds the ship gate, so a question the two surfaces answer
	// differently is a unit that passes the check and is held back by the
	// coverage, in one project, on one run.
	identical, err := a.newIdenticalTargetRule(ctx, cmd, proj, root)
	if err != nil {
		return nil, err
	}

	// The term rules at each unit's point, the resolution the terminology gate
	// uses, so a profile's terms hold that profile's content here too.
	termRules := a.newUnitTermRules(cmd, proj, root)

	for _, u := range units {
		// Resolved before the target is read, so a unit's governance is known
		// when its target is missing or cannot be read back.
		rules, gerr := termRules.forUnit(u)
		if gerr != nil {
			return nil, gerr
		}
		excl.TermsGoverned[TermsKey(u.SourcePath, u.Locale)] = len(rules) > 0

		blocks, missing, berr := a.bilingualBlocks(ctx, u)
		if berr != nil {
			if errors.Is(berr, errTargetUnreadable) {
				continue // unmeasurable target (e.g. a compiled .mo) — can't check
			}
			if unread.skipUnit(nil, berr, root, u) {
				continue
			}
			return nil, berr
		}
		if missing {
			continue // untranslated — there is no translation to check
		}

		// term-check holds the do-not-translate rules as well, so one decision
		// answers for every rule the terms bind here.
		var termTool BlockProcessor
		if len(rules) > 0 {
			termTool = coretools.NewTermCheckTool(&coretools.TermCheckConfig{
				TermRules:    rules,
				SourceLocale: model.LocaleID(a.SourceLocale()),
				TargetLocale: model.LocaleID(u.Locale),
			})
		}

		checkCfg := coretools.NewRuleCheckConfig(model.LocaleID(u.Locale))
		checkCfg.CheckPlaceholders = true
		checker := coretools.NewRuleCheckTool(checkCfg)

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
			if err := RunCheckTool(ctx, checker, b); err != nil {
				return nil, fmt.Errorf("check %s (%s): %w", u.DisplayPath, u.Locale, err)
			}
			fails := slices.ContainsFunc(check.Findings(tool.NewBlockViewWithContext(ctx, b)), func(f check.Finding) bool {
				return !identical.suppresses(f, u.SourcePath, b, u.Locale) && checkFindingFails(f)
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
