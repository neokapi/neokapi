package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// Chars check property keys stored on Block.Properties.
const (
	PropCharsCheckPassed = "chars-check-passed" // "true" or "false"
	PropCharsCheckIssues = "chars-check-issues" // JSON array of issues
)

// CharsCheckConfig holds configuration for the character check tool.
type CharsCheckConfig struct {
	TargetLocale   model.LocaleID `schema:"description=Target locale for processing"` // Required
	ForbiddenChars string         `schema:"description=Characters that should not appear in target text (e.g. {}[])"` // Characters that should not appear (e.g., "{}[]")
	RequiredChars  string         `schema:"description=Characters that must appear in target if present in source (e.g. punctuation)"` // Characters that must appear if in source (e.g., punctuation)
	CheckCorrupted bool           `schema:"description=Check for common corruption patterns such as mojibake,default=true"` // Check for common corruption patterns (default: true)
}

// ToolName returns the tool name this config applies to.
func (c *CharsCheckConfig) ToolName() string { return "chars-check" }

// Reset restores default values.
func (c *CharsCheckConfig) Reset() {
	c.TargetLocale = ""
	c.ForbiddenChars = ""
	c.RequiredChars = ""
	c.CheckCorrupted = true
}

// Validate checks configuration validity.
func (c *CharsCheckConfig) Validate() error {
	if c.TargetLocale.IsEmpty() {
		return fmt.Errorf("chars-check: TargetLocale is required")
	}
	return nil
}

// NewCharsCheckConfig creates a CharsCheckConfig with corruption checking enabled.
func NewCharsCheckConfig(targetLocale model.LocaleID) *CharsCheckConfig {
	return &CharsCheckConfig{
		TargetLocale:   targetLocale,
		CheckCorrupted: true,
	}
}

// mojibakePatterns are common sequences that indicate UTF-8 decoded as Latin-1.
var mojibakePatterns = []string{
	"\u00c3\u00a4", // Ã¤ (ä mojibake)
	"\u00c3\u00b6", // Ã¶ (ö mojibake)
	"\u00c3\u00bc", // Ã¼ (ü mojibake)
	"\u00c3\u00a9", // Ã© (é mojibake)
	"\u00c3\u00a8", // Ã¨ (è mojibake)
	"\u00c3\u00ab", // Ã« (ë mojibake)
	"\u00c3\u00af", // Ã¯ (ï mojibake)
	"\u00c3\u00b1", // Ã± (ñ mojibake)
	"\u00c3\u0089", // Ã‰ (É mojibake)
	"\u00c3\u0096", // Ã– (Ö mojibake)
	"\u00c3\u009c", // Ãœ (Ü mojibake)
}

// NewCharsCheckTool creates a tool that checks for invalid or unexpected characters
// in translations. It detects forbidden characters, missing required characters,
// and common corruption patterns (mojibake, replacement chars, control chars).
func NewCharsCheckTool(cfg *CharsCheckConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "chars-check",
		ToolDescription: "Checks for invalid or unexpected characters in translations",
		Cfg:             cfg,
	}
	t.HandleBlockFn = func(part *model.Part) (*model.Part, error) {
		block, ok := part.Resource.(*model.Block)
		if !ok {
			return part, nil
		}
		if !block.Translatable {
			return part, nil
		}

		conf := t.Cfg.(*CharsCheckConfig)

		if block.Properties == nil {
			block.Properties = make(map[string]string)
		}

		if !block.HasTarget(conf.TargetLocale) {
			return part, nil
		}

		targetText := block.TargetText(conf.TargetLocale)
		sourceText := block.SourceText()

		var issues []QAIssue

		// Check forbidden characters.
		if conf.ForbiddenChars != "" {
			for _, ch := range conf.ForbiddenChars {
				if strings.ContainsRune(targetText, ch) {
					issues = append(issues, QAIssue{
						Type:     "forbidden-char",
						Severity: QASeverityError,
						Message:  fmt.Sprintf("Target contains forbidden character %q (U+%04X)", ch, ch),
					})
				}
			}
		}

		// Check required characters (characters present in source must also appear in target).
		if conf.RequiredChars != "" {
			for _, ch := range conf.RequiredChars {
				if strings.ContainsRune(sourceText, ch) && !strings.ContainsRune(targetText, ch) {
					issues = append(issues, QAIssue{
						Type:     "required-char-missing",
						Severity: QASeverityWarning,
						Message:  fmt.Sprintf("Source contains %q (U+%04X) but target does not", ch, ch),
					})
				}
			}
		}

		// Check corruption patterns.
		if conf.CheckCorrupted {
			issues = append(issues, checkCorruption(targetText)...)
		}

		storeCharsCheckIssues(block, issues)

		return part, nil
	}
	return t
}

// checkCorruption detects common text corruption patterns.
func checkCorruption(text string) []QAIssue {
	var issues []QAIssue

	// Check for mojibake patterns.
	for _, pattern := range mojibakePatterns {
		if strings.Contains(text, pattern) {
			issues = append(issues, QAIssue{
				Type:     "mojibake",
				Severity: QASeverityError,
				Message:  fmt.Sprintf("Possible mojibake detected: %q (UTF-8 decoded as Latin-1)", pattern),
			})
			break // Report mojibake once, not per pattern.
		}
	}

	// Check for Unicode replacement character U+FFFD.
	if strings.ContainsRune(text, '\uFFFD') {
		issues = append(issues, QAIssue{
			Type:     "replacement-char",
			Severity: QASeverityError,
			Message:  "Target contains Unicode replacement character U+FFFD",
		})
	}

	// Check for control characters (U+0000-U+001F except \t, \n, \r).
	for _, r := range text {
		if r <= 0x1F && r != '\t' && r != '\n' && r != '\r' {
			issues = append(issues, QAIssue{
				Type:     "control-char",
				Severity: QASeverityError,
				Message:  fmt.Sprintf("Target contains control character U+%04X", r),
			})
			break // Report once.
		}
	}

	return issues
}

// storeCharsCheckIssues writes character check findings to Block.Properties.
func storeCharsCheckIssues(block *model.Block, issues []QAIssue) {
	if block.Properties == nil {
		block.Properties = make(map[string]string)
	}

	if len(issues) == 0 {
		block.Properties[PropCharsCheckPassed] = "true"
		block.Properties[PropCharsCheckIssues] = "[]"
		return
	}

	block.Properties[PropCharsCheckPassed] = "false"
	data, err := json.Marshal(issues)
	if err != nil {
		block.Properties[PropCharsCheckIssues] = "[]"
		return
	}
	block.Properties[PropCharsCheckIssues] = string(data)
}
