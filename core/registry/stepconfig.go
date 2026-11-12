package registry

import (
	"fmt"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/schema"
)

// CheckModeField is the `qa` tool's mode selector. A qa config chooses its
// backend with this key alone: a provider in the config never selects one.
const CheckModeField = "mode"

// The `qa` tool's mode values. They double as the tool group's member names.
const (
	CheckModeRules = "rules"
	CheckModeAI    = "ai"
)

// CheckMode is one value the `qa` tool's mode selector takes.
type CheckMode struct {
	// Value is what a config writes under `mode`.
	Value string
	// Label names the mode on a selector.
	Label string
	// Runs is what the mode runs, as a message names it.
	Runs string
}

// CheckModes lists the values `mode` takes on a qa step, in the order a message
// or a selector offers them.
var CheckModes = []CheckMode{
	{Value: CheckModeRules, Label: "Deterministic rules", Runs: "the rule checks"},
	{Value: CheckModeAI, Label: "AI review", Runs: "the AI review"},
}

// retiredCheckKeys are keys a qa config may still carry that no code reads. The
// rule checks decode a step's config case-insensitively, so a key matches in
// any letter case; check_terminology is the recipe-style spelling.
var retiredCheckKeys = []string{"checkTerminology", "check_terminology"}

// StepConfigError reports the first key in a tool's config that a recipe or a
// flow file may not carry, or nil. at names the step or preset, such as
// `flows.review.steps[1]` or `defaults.tools.qa`, and keys is the key path of
// the config map itself, such as `flows.review.steps[1].config`. A key is named
// as the config spells it.
//
// Every tool's config keys are snake_case, so a camelCase key is refused with
// its snake_case spelling: `flows.translate.steps[1].config:
// fillTargetThreshold is now fill_target_threshold`. Beyond that, only the qa
// tool has such keys. A qa config refuses checkTerminology, which nothing
// reads, and a provider without a mode, because mode alone selects the AI
// review.
func StepConfigError(tool ToolID, config map[string]any, at, keys string) error {
	names := make([]string, 0, len(config))
	for k := range config {
		names = append(names, k)
	}
	slices.Sort(names)
	if tool != "qa" {
		return camelCaseKeyError(names, keys)
	}
	for _, k := range names {
		if slices.ContainsFunc(retiredCheckKeys, func(r string) bool { return strings.EqualFold(k, r) }) {
			return fmt.Errorf("%s.%s is retired and nothing reads it. Remove the key", keys, k)
		}
	}
	if err := camelCaseKeyError(names, keys); err != nil {
		return err
	}

	mode, hasMode := config[CheckModeField]
	if !hasMode {
		if provider, ok := config["provider"]; ok {
			step := at
			if !strings.HasSuffix(at, ".qa") {
				step += " (qa)"
			}
			return fmt.Errorf("%s sets provider: %v without mode, and a qa config that names a provider requires mode. Write %s",
				step, provider, checkModeChoices())
		}
		return nil
	}
	if s, ok := mode.(string); !ok || !slices.ContainsFunc(CheckModes, func(m CheckMode) bool { return m.Value == s }) {
		return fmt.Errorf("%s.%s is %v, a value qa does not take. Write %s", keys, CheckModeField, mode, checkModeChoices())
	}
	return nil
}

// checkModeChoices spells the mode values for a message:
// `mode: rules for the rule checks, or mode: ai for the AI review`.
func checkModeChoices() string {
	parts := make([]string, len(CheckModes))
	for i, m := range CheckModes {
		parts[i] = fmt.Sprintf("%s: %s for %s", CheckModeField, m.Value, m.Runs)
	}
	return strings.Join(parts, ", or ")
}

// camelCaseKeyError reports the first of a step config's keys, in sorted
// order, that is not snake_case, naming the key to write instead. keys is the
// key path of the config map.
func camelCaseKeyError(names []string, keys string) error {
	for _, k := range names {
		if schema.IsSnakeCase(k) {
			continue
		}
		return fmt.Errorf("%s: %s is now %s", keys, k, schema.SnakeCase(k))
	}
	return nil
}
