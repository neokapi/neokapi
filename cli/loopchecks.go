package cli

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	coretools "github.com/neokapi/neokapi/core/tools"
	"github.com/spf13/cobra"
)

// checkExclusions is the coverage-exclusion set fed by check findings (#1078
// C2/G4): the units that are produced but failing guardrails. A unit in the set
// does not count toward the `translated` rung when computeShipCoverage
// evaluates ship gates — it reads at `draft` (produced, not shippable) until
// the finding is fixed, so a locale cannot clear its gate on the back of
// translations that fail the project's bound checks.
type checkExclusions struct {
	// failing keys are exclusionKey(sourcePath, blockKey, locale).
	failing map[string]bool
	// byLocale counts the failing units per locale, for reporting.
	byLocale map[string]int
}

func exclusionKey(sourcePath, blockKey, locale string) string {
	return sourcePath + "\x00" + blockKey + "\x00" + locale
}

// excluded reports whether the block's unit fails the loop checks for the locale.
func (e *checkExclusions) excluded(sourcePath string, b *model.Block, locale string) bool {
	if e == nil || e.failing == nil {
		return false
	}
	return e.failing[exclusionKey(sourcePath, blockKey(b), locale)]
}

// failingForLocale returns the count of failing units for a locale (0 when nil).
func (e *checkExclusions) failingForLocale(locale string) int {
	if e == nil {
		return 0
	}
	return e.byLocale[locale]
}

// totalFailing returns the count of failing units across every locale (0 when nil).
func (e *checkExclusions) totalFailing() int {
	if e == nil {
		return 0
	}
	total := 0
	for _, n := range e.byLocale {
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
func (a *App) computeLoopCheckExclusions(ctx context.Context, cmd *cobra.Command, units []verifyUnit) (*checkExclusions, error) {
	excl := &checkExclusions{failing: map[string]bool{}, byLocale: map[string]int{}}

	// Glossary per locale, resolved once (opens the termbase).
	glossaryByLocale := map[string][]coretools.GlossaryEntry{}
	glossaryFor := func(locale string) ([]coretools.GlossaryEntry, error) {
		if g, ok := glossaryByLocale[locale]; ok {
			return g, nil
		}
		g, err := a.resolveProjectGlossary(cmd, locale)
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

		glossary, gerr := glossaryFor(u.locale)
		if gerr != nil {
			return nil, gerr
		}
		var termTool blockProcessor
		if len(glossary) > 0 {
			termTool = coretools.NewTermCheckTool(&coretools.TermCheckConfig{
				Glossary:     glossary,
				TargetLocale: model.LocaleID(u.locale),
			})
		}

		qaCfg := coretools.NewQACheckConfig(model.LocaleID(u.locale))
		qaCfg.Patterns = append(qaCfg.Patterns, defaultPlaceholderPatterns()...)
		qa := coretools.NewQACheckTool(qaCfg)

		for _, b := range blocks {
			if !b.Translatable {
				continue
			}
			// Only produced units are checkable: an empty target is pending
			// work, not a guardrail failure.
			if strings.TrimSpace(b.TargetText(model.LocaleID(u.locale))) == "" {
				continue
			}
			runCheckTool(ctx, qa, b)
			fails := slices.ContainsFunc(check.Findings(tool.NewBlockView(b)), qaFindingFails)
			if !fails && termTool != nil {
				runCheckTool(ctx, termTool, b)
				if b.Properties[coretools.PropTermCheckPassed] == "false" {
					fails = true
				}
			}
			if fails {
				key := exclusionKey(u.sourcePath, blockKey(b), u.locale)
				if !excl.failing[key] {
					excl.failing[key] = true
					excl.byLocale[u.locale]++
				}
			}
		}
	}
	return excl, nil
}

// demoteFailing maps a unit state through the exclusion: a state at or above
// `translated` reads as `draft` — produced, but failing guardrails, so it does
// not climb the ladder past the produced rung. States below `translated` pass
// through unchanged.
func demoteFailing(state string) string {
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
