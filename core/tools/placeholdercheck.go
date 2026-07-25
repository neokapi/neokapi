package tools

import (
	"errors"
	"fmt"
	"regexp"
	"sort"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// placeholderToken matches the common interpolation/placeholder styles seen in
// localization content, longest/most-specific forms first so "{{x}}" wins over
// "{x}". Covered: {{name}}, ${name}, %(name)s (Python), %1$s (positional),
// %s/%d/%@ (printf/ObjC), {name}/{0} (ICU/.NET), <0>…</0> (numbered tags).
var placeholderToken = regexp.MustCompile(
	`\{\{[^{}]+\}\}|\$\{[^{}]+\}|%\([^)]+\)[a-zA-Z]|%\d+\$[a-zA-Z]|%[sdifeEgGxXobpqv@%]|\{[^{}]+\}|</?[0-9]+>`)

// PlaceholderCheckConfig configures the placeholder/tag integrity checker.
type PlaceholderCheckConfig struct {
	TargetLocale model.LocaleID `json:"targetLocale,omitempty" schema:"-"`
	// FlagExtra also reports placeholders that appear in the target but not the
	// source (a major issue — usually a stray token), in addition to dropped
	// ones (always critical).
	FlagExtra bool `json:"flagExtra,omitempty" schema:"title=Flag extra placeholders,description=Report placeholders present in the target but absent from the source,default=true"`
}

// ToolName returns the tool name this config applies to.
func (c *PlaceholderCheckConfig) ToolName() string { return "placeholder-check" }

// Reset restores default values.
func (c *PlaceholderCheckConfig) Reset() {
	c.TargetLocale = ""
	c.FlagExtra = true
}

// Validate checks configuration validity.
func (c *PlaceholderCheckConfig) Validate() error {
	if c.TargetLocale.IsEmpty() {
		return errors.New("placeholder-check: TargetLocale is required")
	}
	return nil
}

// NewPlaceholderCheckConfig creates a PlaceholderCheckConfig for the locale.
func NewPlaceholderCheckConfig(targetLocale model.LocaleID) *PlaceholderCheckConfig {
	return &PlaceholderCheckConfig{TargetLocale: targetLocale, FlagExtra: true}
}

// NewPlaceholderCheckFromConfig creates a placeholder-check tool from a config map.
//
// Also written but never registered (#1476): from a flow the tool was pinned to
// an `en` target, so the check that exists to catch a dropped `{count}` in a
// translation never looked at one.
func NewPlaceholderCheckFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	cfg := NewPlaceholderCheckConfig(model.LocaleID(targetLang))
	if err := applyStepConfig("placeholder-check", config, cfg, targetLang, setPlaceholderCheckLocale); err != nil {
		return nil, err
	}
	return NewPlaceholderCheckTool(cfg), nil
}

func setPlaceholderCheckLocale(c *PlaceholderCheckConfig, loc model.LocaleID) { c.TargetLocale = loc }

// NewPlaceholderCheckTool creates a checker that verifies every interpolation
// placeholder in the source survives, by count, into the target: a dropped
// `{count}` or unbalanced `<0>` is the kind of break that crashes a localized
// app at runtime. Read-only (Annotate) over core/check.
//
// Two representations are checked, because a placeholder can arrive either way:
// literal tokens inside the text (`{count}`, `%s`, `<0>`), and inline-code Runs
// (a Ph / PcOpen / PcClose / Sub the reader lifted out of the text). The text
// projection contributes no characters for inline-code runs, so a text-only
// check is blind to a target that dropped them — which is exactly how a lossy
// recycled translation used to pass through the pipeline unnoticed.
func NewPlaceholderCheckTool(cfg *PlaceholderCheckConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "placeholder-check",
		ToolDescription: "Verifies interpolation placeholders and numbered tags are preserved in the target",
		Cfg:             cfg,
	}
	t.Annotate = func(v tool.BlockView) error {
		if !v.Translatable() {
			return nil
		}
		conf := t.Cfg.(*PlaceholderCheckConfig)
		if !v.HasTarget(conf.TargetLocale) {
			return nil
		}

		source := v.SourceText()
		target := v.TargetText(conf.TargetLocale)
		srcCounts := placeholderCounts(source)
		codeDiff := model.DiffRunCodes(v.SourceRuns(), v.TargetRuns(conf.TargetLocale))
		if len(srcCounts) == 0 && codeDiff.Balanced() && !conf.FlagExtra {
			return nil
		}
		tgtCounts := placeholderCounts(target)

		var findings []check.Finding
		for _, code := range codeDiff.MissingCodes() {
			findings = append(findings, check.Finding{
				Category:     "placeholder",
				Severity:     check.SeverityCritical,
				Message:      fmt.Sprintf("Inline code %s is missing from the %s target (dropped %d×)", code, conf.TargetLocale, codeDiff.Missing[code]),
				Suggestion:   fmt.Sprintf("Keep %s in the target", code),
				OriginalText: code,
			})
		}
		if conf.FlagExtra {
			for _, code := range codeDiff.ExtraCodes() {
				findings = append(findings, check.Finding{
					Category:     "placeholder",
					Severity:     check.SeverityMajor,
					Message:      fmt.Sprintf("Inline code %s appears in the %s target but not the source", code, conf.TargetLocale),
					OriginalText: code,
				})
			}
		}
		for _, tok := range sortedKeys(srcCounts) {
			if tgtCounts[tok] < srcCounts[tok] {
				findings = append(findings, check.Finding{
					Category:     "placeholder",
					Severity:     check.SeverityCritical,
					Message:      fmt.Sprintf("Placeholder %s is missing from the %s target (source %d×, target %d×)", tok, conf.TargetLocale, srcCounts[tok], tgtCounts[tok]),
					Suggestion:   fmt.Sprintf("Keep %s in the target", tok),
					OriginalText: tok,
				})
			}
		}
		if conf.FlagExtra {
			for _, tok := range sortedKeys(tgtCounts) {
				if srcCounts[tok] == 0 {
					findings = append(findings, check.Finding{
						Category:     "placeholder",
						Severity:     check.SeverityMajor,
						Message:      fmt.Sprintf("Placeholder %s appears in the %s target but not the source", tok, conf.TargetLocale),
						OriginalText: tok,
					})
				}
			}
		}

		check.Annotate(v, "placeholder-check", findings)
		return nil
	}
	return t
}

func placeholderCounts(s string) map[string]int {
	counts := map[string]int{}
	for _, m := range placeholderToken.FindAllString(s, -1) {
		counts[m]++
	}
	return counts
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
