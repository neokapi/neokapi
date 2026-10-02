package host

import (
	"fmt"
	"slices"
	"strings"

	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
)

// The terms gate holds a translation to two sources of term rules: the terms
// bound where its content sits (the project's own store, or a profile's
// `termstore:`), and the `term_rules:` the recipe declares, on a flow step or
// in a tool preset. The step that drafts is handed the recipe's list, so the
// gate holds the draft to the same list the model was told.
//
// gateTermRules is the one resolution. The ship terminology gate (`kapi check
// --ship`, `--gate terms`), the loop checks behind `kapi status`, `ship.json`
// and `kapi up`, and `kapi check` with a target language all ask it, so they
// agree about which languages terms govern and which rules a target breaks.

// declaredTermRules reads the term rules the recipe declares, none outside a
// project.
func declaredTermRules(proj *project.KapiProject, root string) (coreprofile.RecipeTermRules, error) {
	if proj == nil {
		return coreprofile.RecipeTermRules{}, nil
	}
	declared, err := proj.DeclaredTermRules(root)
	if err != nil {
		return coreprofile.RecipeTermRules{}, fmt.Errorf("read the recipe's term_rules: %w", err)
	}
	return declared, nil
}

// gateTermRules returns the rules a translation into locale of content written
// in source is held to at a point: the rules the terms bound there give for the
// language pair, and the rules the recipe declares for the language. Where both
// have a rule for one term, the recipe's holds, as it does for the step that
// declares it.
func (a *App) gateTermRules(cmd Command, declared coreprofile.RecipeTermRules, source, locale string, point project.GovernancePoint) ([]coreprofile.TermRule, error) {
	stored, err := a.resolveTermRules(cmd, source, locale, point)
	if err != nil {
		return nil, err
	}
	return coreprofile.WithDeclaredTermRules(stored, declared.For(locale)), nil
}

// GateTermRules is gateTermRules for one call, reading the recipe's rules from
// proj, whose recipe sits in root, for content in the App's source language.
// It is what an embedded surface asks when it checks one translation the way
// the gate does.
func (a *App) GateTermRules(cmd Command, proj *project.KapiProject, root, locale string, point project.GovernancePoint) ([]coreprofile.TermRule, error) {
	declared, err := declaredTermRules(proj, root)
	if err != nil {
		return nil, err
	}
	return a.gateTermRules(cmd, declared, a.SourceLocale(), locale, point)
}

// declaredTermRulesWhere names where the recipe declares term rules, for a
// message about what binds terms: "the recipe's term_rules", or, when every
// declared rule names its language, "the recipe's term_rules for de, nb". Empty
// when the recipe declares none.
func declaredTermRulesWhere(declared coreprofile.RecipeTermRules) string {
	if declared.Empty() {
		return ""
	}
	if len(declared.All) > 0 {
		return "the recipe's term_rules"
	}
	var langs []string
	for loc, rules := range declared.ByLocale {
		if len(rules) > 0 {
			langs = append(langs, loc)
		}
	}
	slices.Sort(langs)
	return "the recipe's term_rules for " + strings.Join(langs, ", ")
}
