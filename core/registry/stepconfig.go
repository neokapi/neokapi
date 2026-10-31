package registry

import (
	"fmt"
	"slices"
	"strings"
)

// QAModeField is the `qa` tool's mode selector. A qa config chooses its
// backend with this key alone: a provider in the config never selects one.
const QAModeField = "mode"

// The `qa` tool's mode values. They double as the tool group's member names.
const (
	QAModeRules = "rules"
	QAModeAI    = "ai"
)

// QAMode is one value the `qa` tool's mode selector takes.
type QAMode struct {
	// Value is what a config writes under `mode`.
	Value string
	// Label names the mode on a selector.
	Label string
	// Runs is what the mode runs, as a message names it.
	Runs string
}

// QAModes lists the values `mode` takes on a qa step, in the order a message
// or a selector offers them.
var QAModes = []QAMode{
	{Value: QAModeRules, Label: "Deterministic rules", Runs: "the rule checks"},
	{Value: QAModeAI, Label: "AI review", Runs: "the AI review"},
}

// retiredQAKeys are keys a qa config may still carry that no code reads. The
// rule checks decode a step's config case-insensitively, so a key matches in
// any letter case; check_terminology is the recipe-style spelling.
var retiredQAKeys = []string{"checkTerminology", "check_terminology"}

// StepConfigError reports the first key in a tool's config that a recipe or a
// flow file may not carry, or nil. at names the step or preset, such as
// `flows.review.steps[1]` or `defaults.tools.qa`, and keys is the key path of
// the config map itself, such as `flows.review.steps[1].config`. A key is named
// as the config spells it.
//
// Only the qa tool has such keys. A qa config refuses checkTerminology, which
// nothing reads, and a provider without a mode, because mode alone selects the
// AI review.
func StepConfigError(tool ToolID, config map[string]any, at, keys string) error {
	if tool != "qa" {
		return nil
	}
	names := make([]string, 0, len(config))
	for k := range config {
		names = append(names, k)
	}
	slices.Sort(names)
	for _, k := range names {
		if slices.ContainsFunc(retiredQAKeys, func(r string) bool { return strings.EqualFold(k, r) }) {
			return fmt.Errorf("%s.%s is retired and nothing reads it. Remove the key", keys, k)
		}
	}

	mode, hasMode := config[QAModeField]
	if !hasMode {
		if provider, ok := config["provider"]; ok {
			step := at
			if !strings.HasSuffix(at, ".qa") {
				step += " (qa)"
			}
			return fmt.Errorf("%s sets provider: %v without mode, and a qa step that names a provider requires mode. Write %s",
				step, provider, qaModeChoices())
		}
		return nil
	}
	if s, ok := mode.(string); !ok || !slices.ContainsFunc(QAModes, func(m QAMode) bool { return m.Value == s }) {
		return fmt.Errorf("%s.%s is %v, a value qa does not take. Write %s", keys, QAModeField, mode, qaModeChoices())
	}
	return nil
}

// qaModeChoices spells the mode values for a message:
// `mode: rules for the rule checks, or mode: ai for the AI review`.
func qaModeChoices() string {
	parts := make([]string, len(QAModes))
	for i, m := range QAModes {
		parts[i] = fmt.Sprintf("%s: %s for %s", QAModeField, m.Value, m.Runs)
	}
	return strings.Join(parts, ", or ")
}
