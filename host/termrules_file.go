package host

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// TermRulesFlag is the `kapi exec` flag naming a file of ad-hoc term rules,
// read by LoadTermRulesFile.
const TermRulesFlag = "term-rules"

// TermRulesFlagUsage is the help line for TermRulesFlag.
const TermRulesFlagUsage = "YAML or JSON file of term rules to check against instead of the project's terms store: a `term_rules:` list of {term, replacement}, the shape the MCP tool takes"

// LoadTermRulesFile reads a file of term rules in the shape the MCP term-check
// tool takes: a `term_rules:` list of coreprofile.TermRule, in YAML or JSON. A
// bare list is read as the same thing.
//
// The rules are the file's alone. A caller that names one is driving the tool
// with ad-hoc input, as an MCP call does, and the project's terms store is
// left out of the run.
func LoadTermRulesFile(path string) ([]coreprofile.TermRule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read term rules: %w", err)
	}
	var doc struct {
		TermRules []coreprofile.TermRule `yaml:"term_rules" json:"term_rules"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		var list []coreprofile.TermRule
		if lerr := yaml.Unmarshal(data, &list); lerr != nil {
			return nil, fmt.Errorf("read term rules %s: expected a `term_rules:` list of {term, replacement}: %w", path, err)
		}
		doc.TermRules = list
	}
	if len(doc.TermRules) == 0 {
		return nil, fmt.Errorf("read term rules %s: the file names no rule under `term_rules:`", path)
	}
	for i, r := range doc.TermRules {
		if r.Term == "" {
			return nil, fmt.Errorf("read term rules %s: rule %d has no term", path, i+1)
		}
	}
	return doc.TermRules, nil
}
