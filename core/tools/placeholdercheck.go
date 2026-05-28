package tools

import (
	"errors"
	"fmt"
	"regexp"
	"sort"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
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
func NewPlaceholderCheckFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	cfg := NewPlaceholderCheckConfig(model.LocaleID(targetLang))
	if err := schema.ApplyConfig(config, cfg); err != nil {
		return nil, fmt.Errorf("placeholder-check config: %w", err)
	}
	if targetLang != "" {
		cfg.TargetLocale = model.LocaleID(targetLang)
	}
	return NewPlaceholderCheckTool(cfg), nil
}

// NewPlaceholderCheckTool creates a checker that verifies every interpolation
// placeholder in the source survives, by count, into the target: a dropped
// `{count}` or unbalanced `<0>` is the kind of break that crashes a localized
// app at runtime. Read-only (Annotate) over core/check.
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
		if len(srcCounts) == 0 && !conf.FlagExtra {
			return nil
		}
		tgtCounts := placeholderCounts(target)

		var findings []check.Finding
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
