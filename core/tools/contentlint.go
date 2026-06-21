package tools

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// ContentLintConfig holds configuration for the content-lint tool. The checker
// has no tunable behavior today; the struct exists so it follows the standard
// tool config contract (and can grow toggles without an API break).
type ContentLintConfig struct{}

// ToolName returns the tool name this config applies to.
func (c *ContentLintConfig) ToolName() string { return "content-lint" }

// Reset restores default values.
func (c *ContentLintConfig) Reset() {}

// Validate checks configuration validity.
func (c *ContentLintConfig) Validate() error { return nil }

// ContentLintSchema returns the auto-generated schema for the content-lint tool.
func ContentLintSchema() *schema.ComponentSchema {
	return schema.FromStruct(&ContentLintConfig{}, schema.ToolMeta{
		ID:          "content-lint",
		Category:    schema.CategoryTextProcessing,
		DisplayName: "Content Lint",
		Description: "Flag text-hygiene issues in source content",
	})
}

// NewContentLintFromConfig creates a content-lint tool from a config map.
func NewContentLintFromConfig(config map[string]any, _ string) (tool.Tool, error) {
	var cfg ContentLintConfig
	if err := schema.ApplyConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("content-lint config: %w", err)
	}
	return NewContentLintTool(&cfg), nil
}

// NewContentLintTool creates a generic, source-side content-hygiene checker. It
// inspects a single text (the source) with no target or locale comparison and
// records issues as core/check.Finding under the unified quality.findings
// annotation (check.Annotate), where they accumulate alongside other checkers.
func NewContentLintTool(cfg *ContentLintConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "content-lint",
		ToolDescription: "Flags text-hygiene issues (empty, double spaces, doubled words, stray whitespace, control chars) in source content",
		Cfg:             cfg,
	}
	t.Annotate = func(v tool.BlockView) error {
		if !v.Translatable() {
			return nil
		}
		check.Annotate(v, "content-lint", contentLintFindings(v.SourceText()))
		return nil
	}
	return t
}

// contentLintFindings runs the single-text hygiene heuristics over text and
// returns one Finding per issue. Empty/whitespace-only content is the single
// major issue (and short-circuits the rest); the remaining nits are minor.
func contentLintFindings(text string) []check.Finding {
	if strings.TrimSpace(text) == "" {
		return []check.Finding{{
			Category: "empty",
			Severity: check.SeverityMajor,
			Message:  "Content is empty or whitespace-only",
		}}
	}

	var findings []check.Finding

	if leadingWhitespace(text) != "" {
		findings = append(findings, check.Finding{
			Category: "leading-whitespace",
			Severity: check.SeverityMinor,
			Message:  "Content has leading whitespace",
		})
	}

	if trailingWhitespace(text) != "" {
		findings = append(findings, check.Finding{
			Category: "trailing-whitespace",
			Severity: check.SeverityMinor,
			Message:  "Content has trailing whitespace",
		})
	}

	if strings.Contains(text, "  ") {
		findings = append(findings, check.Finding{
			Category: "double-spaces",
			Severity: check.SeverityMinor,
			Message:  "Content contains consecutive spaces",
		})
	}

	if word := findDoubledWord(text, ""); word != "" {
		findings = append(findings, check.Finding{
			Category:     "doubled-word",
			Severity:     check.SeverityMinor,
			Message:      fmt.Sprintf("Content contains a doubled word: %q", word),
			OriginalText: word,
		})
	}

	if r, ok := firstControlChar(text); ok {
		findings = append(findings, check.Finding{
			Category: "control-char",
			Severity: check.SeverityMinor,
			Message:  fmt.Sprintf("Content contains a stray control character (U+%04X)", r),
		})
	}

	return findings
}

// firstControlChar returns the first non-whitespace control character in s.
// Tab, newline, and carriage return are treated as ordinary whitespace, so only
// stray control codes (e.g. NUL, BEL, ESC) are reported.
func firstControlChar(s string) (rune, bool) {
	for _, r := range s {
		switch r {
		case '\t', '\n', '\r':
			continue
		}
		if unicode.IsControl(r) {
			return r, true
		}
	}
	return 0, false
}
