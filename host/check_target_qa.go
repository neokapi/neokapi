package host

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	coretools "github.com/neokapi/neokapi/core/tools"
)

// collectTargetQADiagnostics holds a translation checked against its source
// (`kapi check SOURCE --target TARGET`, check_file with a target) to the rules
// of the ship gate's checks gate (verifyChecksGate) that say a unit is not
// translated: an empty target, and a target identical to its source. Each
// fails as it does there, and the identity of a target the project settles
// (its terms keep the string, or its record approves the unit) is not
// reported, so the one-off check and the gate give one answer. The codes a
// target drops are the placeholder family's (collectBilingualDiagnostics).
// The commit check runs none of these: a translation an agent writes may be
// its source's text where the language keeps it.
func (a *App) collectTargetQADiagnostics(ctx context.Context, cmd Command, blocks []*model.Block, sourcePath string, loc model.LocaleID, execution *checkExecution) ([]check.Diagnostic, error) {
	start := time.Now()
	identical := a.targetIdentityRule(ctx, cmd)
	cfg := coretools.NewRuleCheckConfig(loc)
	checker := coretools.NewRuleCheckTool(cfg)
	var diags []check.Diagnostic
	for _, b := range blocks {
		// The findings this family adds follow the ones the block holds.
		seen := len(FindingsFromBlock(b, false))
		if err := RunCheckTool(ctx, checker, b); err != nil {
			return nil, fmt.Errorf("target checks %s (%s): %w", DisplayName(sourcePath), loc, err)
		}
		found := FindingsFromBlock(b, false)
		for _, f := range found[min(seen, len(found)):] {
			if !untranslatedCategories[f.Category] || identical.suppresses(f, sourcePath, b, string(loc)) {
				continue
			}
			f.Fails = checkFindingFails(f)
			if f.Suggestion == "" {
				f.Suggestion = checkFindingSuggestion(f)
			}
			diags = append(diags, check.DiagnosticFrom(f, "qa", check.Location{File: DisplayName(sourcePath), Block: blockKey(b)}))
		}
	}
	if err := execution.probed("checks.target", sourcePath, len(diags), start, true, func() (check.CanaryOutcome, error) {
		canaries, uncheckable := coretools.RuleCheckCanaries(cfg)
		return probeTool(ctx, checker, canaries, uncheckable)
	}); err != nil {
		return nil, fmt.Errorf("target checks %s (%s): %w", DisplayName(sourcePath), loc, err)
	}
	return diags, nil
}

// untranslatedCategories are the rule-check findings that say a unit is not
// translated.
var untranslatedCategories = map[string]bool{"empty-target": true, checkCategoryTargetSameAsSource: true}

// targetIdentityRule is the project's identicalTargetRule when the command
// names a project, and nil (which settles nothing) outside one or when the
// project does not load.
func (a *App) targetIdentityRule(ctx context.Context, cmd Command) *identicalTargetRule {
	recipe, err := ResolveProjectPath(cmd)
	if err != nil || recipe == "" {
		return nil
	}
	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return nil
	}
	rule, err := a.newIdenticalTargetRule(ctx, cmd, proj, filepath.Dir(recipe))
	if err != nil {
		return nil
	}
	return rule
}
