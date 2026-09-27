package profile

import (
	"encoding/json"
	"fmt"
)

// RecipeTermRules are the term rules a recipe declares under `term_rules:`,
// wherever it declares them: on a flow step, inline under `flows:` or in a
// file in `flows_dir:`, in a project-wide tool preset (`defaults.tools`), or in
// a per-language one (`defaults.locales.<lang>.tools`). core/project reads
// them from a recipe (KapiProject.DeclaredTermRules). The terms gate and
// `kapi status` count them beside the terms bound where content sits, so a
// rule that tells the model what to write also holds the translation to it.
//
// A rule names no language of its own. One declared on a flow step or in a
// project-wide preset applies to every target language (All); one in a
// per-language preset applies to that language alone (ByLocale).
type RecipeTermRules struct {
	All      []TermRule            `json:"all,omitempty"`
	ByLocale map[string][]TermRule `json:"locales,omitempty"`
}

// Empty reports whether the recipe declares no term rule anywhere.
func (d RecipeTermRules) Empty() bool {
	if len(d.All) > 0 {
		return false
	}
	for _, rules := range d.ByLocale {
		if len(rules) > 0 {
			return false
		}
	}
	return true
}

// For returns the rules the recipe declares for a target language: the ones
// every language takes, then the language's own, each rule once.
func (d RecipeTermRules) For(locale string) []TermRule {
	var out []TermRule
	seen := map[string]bool{}
	for _, list := range [][]TermRule{d.All, d.ByLocale[locale]} {
		out = AppendNewTermRules(out, seen, list)
	}
	return out
}

// Encode renders the rules as the compact JSON a venue stores and compares.
// Equal declarations encode to equal strings, and a recipe with no rules
// encodes to "".
func (d RecipeTermRules) Encode() (string, error) {
	if d.Empty() {
		return "", nil
	}
	b, err := json.Marshal(d)
	if err != nil {
		return "", fmt.Errorf("encode term rules: %w", err)
	}
	return string(b), nil
}

// DecodeRecipeTermRules reads what Encode wrote. "" decodes to no rules.
func DecodeRecipeTermRules(s string) (RecipeTermRules, error) {
	var d RecipeTermRules
	if s == "" {
		return d, nil
	}
	if err := json.Unmarshal([]byte(s), &d); err != nil {
		return RecipeTermRules{}, fmt.Errorf("decode term rules: %w", err)
	}
	return d, nil
}

// Covers reports whether every rule in other is also in d for the same
// languages, unchanged. A recipe whose rules cover the ones a venue holds adds
// rules and removes none, so it holds translations to more than before.
func (d RecipeTermRules) Covers(other RecipeTermRules) bool {
	if !termRulesCover(d.All, other.All) {
		return false
	}
	for loc, rules := range other.ByLocale {
		// A language's own rule is also covered by the same rule declared for
		// every language.
		if !termRulesCover(append(append([]TermRule{}, d.All...), d.ByLocale[loc]...), rules) {
			return false
		}
	}
	return true
}

func termRulesCover(have, want []TermRule) bool {
	keys := map[string]bool{}
	for _, r := range have {
		keys[termRuleKey(r)] = true
	}
	for _, r := range want {
		if !keys[termRuleKey(r)] {
			return false
		}
	}
	return true
}

// AppendNewTermRules appends each rule in list that seen does not already
// hold, and records it there. A rule with no term says nothing and is skipped.
// Two declarations of one rule are one, and two different rules for one term
// stay two.
func AppendNewTermRules(out []TermRule, seen map[string]bool, list []TermRule) []TermRule {
	for _, r := range list {
		if r.Term == "" {
			continue
		}
		k := termRuleKey(r)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, r)
	}
	return out
}

// termRuleKey identifies a rule by its whole content.
func termRuleKey(r TermRule) string {
	b, _ := json.Marshal(r)
	return string(b)
}

// WithDeclaredTermRules adds the rules a recipe declares for a language to the
// rules the terms bound where content sits give for it. A stored rule for a
// term the recipe also has a rule for is left out, so the recipe's rule holds,
// as it does for the step that declares it. Every surface that holds a
// translation to both sources merges them here.
func WithDeclaredTermRules(stored, declared []TermRule) []TermRule {
	if len(declared) == 0 {
		return stored
	}
	own := make(map[string]bool, len(declared))
	for _, r := range declared {
		own[r.Term] = true
	}
	out := make([]TermRule, 0, len(stored)+len(declared))
	for _, r := range stored {
		if !own[r.Term] {
			out = append(out, r)
		}
	}
	return append(out, declared...)
}
