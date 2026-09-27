package project

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/profile"
)

// termRulesKey is the config key every governed tool reads its term rules
// under, in a flow step and in a tool preset alike.
const termRulesKey = "term_rules"

// DeclaredTermRules collects the term rules the recipe declares under
// `term_rules:` (profile.RecipeTermRules). root is the recipe's directory, which
// a `flows_dir:` is relative to; "" reads the inline flows alone.
//
// Flows are read in name order, the inline ones first, and a file in
// `flows_dir:` that an inline flow of the same name shadows is skipped, as a run
// skips it. A flow file that does not load contributes nothing: `kapi flows`
// reports it, and no run can use its rules. A rule declared twice with the same
// content is kept once; two different rules for one term are both kept, since
// each holds the step that declares it.
func (p *KapiProject) DeclaredTermRules(root string) (profile.RecipeTermRules, error) {
	var d profile.RecipeTermRules
	if p == nil {
		return d, nil
	}
	seen := map[string]bool{}
	add := func(where string, v any) error {
		rules, err := decodeTermRules(v)
		if err != nil {
			return fmt.Errorf("%s.%s: %w", where, termRulesKey, err)
		}
		d.All = profile.AppendNewTermRules(d.All, seen, rules)
		return nil
	}

	for _, name := range slices.Sorted(maps.Keys(p.Flows)) {
		if err := addStepRules("flows."+name, p.Flows[name], add); err != nil {
			return profile.RecipeTermRules{}, err
		}
	}
	if root != "" {
		for _, df := range ListDirFlows(p.FlowsDirIn(root)) {
			if df.Err != nil || p.Flow(df.Name) != nil {
				continue
			}
			if err := addStepRules(p.FlowsDir+"/"+df.Name+".yaml", df.Spec, add); err != nil {
				return profile.RecipeTermRules{}, err
			}
		}
	}
	for _, tool := range slices.Sorted(maps.Keys(p.Defaults.Tools)) {
		if v, ok := p.Defaults.Tools[tool][termRulesKey]; ok {
			if err := add("defaults.tools."+tool, v); err != nil {
				return profile.RecipeTermRules{}, err
			}
		}
	}
	for _, loc := range slices.Sorted(maps.Keys(p.Defaults.Locales)) {
		tools := p.Defaults.Locales[loc].Tools
		locSeen := map[string]bool{}
		var rules []profile.TermRule
		for _, tool := range slices.Sorted(maps.Keys(tools)) {
			v, ok := tools[tool][termRulesKey]
			if !ok {
				continue
			}
			list, err := decodeTermRules(v)
			if err != nil {
				return profile.RecipeTermRules{}, fmt.Errorf("defaults.locales.%s.tools.%s.%s: %w", loc, tool, termRulesKey, err)
			}
			rules = profile.AppendNewTermRules(rules, locSeen, list)
		}
		if len(rules) > 0 {
			if d.ByLocale == nil {
				d.ByLocale = map[string][]profile.TermRule{}
			}
			d.ByLocale[loc] = rules
		}
	}
	return d, nil
}

// addStepRules hands each step's `term_rules:` in spec to add, parallel
// branches included, naming the step for an error.
func addStepRules(where string, spec *flow.StepsSpec, add func(string, any) error) error {
	if spec == nil {
		return nil
	}
	var walk func(prefix string, steps []flow.FlowStep) error
	walk = func(prefix string, steps []flow.FlowStep) error {
		for i, st := range steps {
			at := fmt.Sprintf("%s[%d]", prefix, i)
			if v, ok := st.Config[termRulesKey]; ok {
				if err := add(at+".config", v); err != nil {
					return err
				}
			}
			if err := walk(at+".parallel", st.Parallel); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(where+".steps", spec.Steps)
}

// decodeTermRules reads a `term_rules:` value in the shape every governed tool
// decodes it in: a list of rules with the fields the voice profile reference
// documents.
func decodeTermRules(v any) ([]profile.TermRule, error) {
	if v == nil {
		return nil, nil
	}
	if rules, ok := v.([]profile.TermRule); ok {
		return rules, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var rules []profile.TermRule
	if err := json.Unmarshal(b, &rules); err != nil {
		return nil, fmt.Errorf("not a list of term rules: %w", err)
	}
	return rules, nil
}
