package host

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	coretools "github.com/neokapi/neokapi/core/tools"
)

// CheckExclusions is the coverage-exclusion set fed by check findings (#1078
// C2/G4): the units that are produced but failing guardrails. A unit in the set
// does not count toward the `translated` rung when ComputeShipCoverage
// evaluates ship gates — it reads at `draft` (produced, not shippable) until
// the finding is fixed, so a locale cannot clear its gate on the back of
// translations that fail the project's bound checks.
type CheckExclusions struct {
	// failing keys are ExclusionKey(sourcePath, blockKey, locale).
	Failing map[string]bool
	// byLocale counts the failing units per locale, for reporting.
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

// failingForLocale returns the count of failing units for a locale (0 when nil).
func (e *CheckExclusions) failingForLocale(locale string) int {
	if e == nil {
		return 0
	}
	return e.ByLocale[locale]
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
// the produced units — the same engines `kapi check --ship` gates on: the QA
// checkset (placeholder/tag integrity, plus the default placeholder patterns)
// always, and the terminology check when the project binds a termbase. A unit
// whose findings fail the ship predicate (any critical/major finding, or an
// integrity category like pattern-mismatch) enters the exclusion set.
//
// Cost: checks only run over units whose target exists and is readable — a
// missing target is already below every rung, so there is nothing to demote —
// and they are annotate-only over blocks the coverage pass reads anyway (the
// parse cache absorbs the re-read).
func (a *App) computeLoopCheckExclusions(ctx context.Context, cmd Command, units []VerifyUnit) (*CheckExclusions, error) {
	excl := &CheckExclusions{Failing: map[string]bool{}, ByLocale: map[string]int{}}

	// Glossary per locale, resolved once (opens the termbase).
	glossaryByLocale := map[string][]coretools.GlossaryEntry{}
	glossaryFor := func(locale string) ([]coretools.GlossaryEntry, error) { //nolint:contextcheck // ctx flows via the Command (CmdContext), not a detached context
		if g, ok := glossaryByLocale[locale]; ok {
			return g, nil
		}
		g, err := a.ResolveProjectGlossary(cmd, locale)
		if err != nil {
			return nil, err
		}
		glossaryByLocale[locale] = g
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
			continue // untranslated — below every rung already, nothing to demote
		}

		glossary, gerr := glossaryFor(u.Locale)
		if gerr != nil {
			return nil, gerr
		}
		var termTool BlockProcessor
		if len(glossary) > 0 {
			termTool = coretools.NewTermCheckTool(&coretools.TermCheckConfig{
				Glossary:     glossary,
				TargetLocale: model.LocaleID(u.Locale),
			})
		}

		qaCfg := coretools.NewQACheckConfig(model.LocaleID(u.Locale))
		qaCfg.Patterns = append(qaCfg.Patterns, defaultPlaceholderPatterns()...)
		qa := coretools.NewQACheckTool(qaCfg)

		for _, b := range blocks {
			if !b.Translatable {
				continue
			}
			// Only produced units are checkable: an empty target is pending
			// work, not a guardrail failure.
			if strings.TrimSpace(b.TargetText(model.LocaleID(u.Locale))) == "" {
				continue
			}
			// A checker that could not run must not read as a checker that
			// passed: this exclusion set feeds the ship gate, so swallowing the
			// error would readmit a failing unit as shippable — "the operation
			// failed and the system reports success" applied to the gate itself.
			if err := RunCheckTool(ctx, qa, b); err != nil {
				return nil, fmt.Errorf("qa check %s (%s): %w", u.DisplayPath, u.Locale, err)
			}
			fails := slices.ContainsFunc(check.Findings(tool.NewBlockViewWithContext(ctx, b)), qaFindingFails)
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

// DemoteFailing maps a unit state through the exclusion: a state at or above
// `translated` reads as `draft` — produced, but failing guardrails, so it does
// not climb the ladder past the produced rung. States below `translated` pass
// through unchanged.
func DemoteFailing(state string) string {
	ladder := gate.TargetLadder()
	rank := func(s string) int {
		for i, l := range ladder {
			if l == s {
				return i
			}
		}
		return -1
	}
	if rank(state) >= rank(string(model.TargetStatusTranslated)) {
		return string(model.TargetStatusDraft)
	}
	return state
}
