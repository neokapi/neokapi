package tools

import (
	"encoding/json"
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// Length check property keys stored on Block.Properties.
const (
	PropLengthCheckPassed = "length-check-passed" // "true" or "false"
	PropLengthCheckIssues = "length-check-issues" // JSON array of issues
)

// LengthCheckConfig holds configuration for the length check tool.
type LengthCheckConfig struct {
	TargetLocale  model.LocaleID `json:"targetLocale,omitempty"  schema:"-"`
	MaxChars      int            `json:"maxChars,omitempty"      schema:"description=Maximum character count for target text (0 = disabled),default=0,min=0"`
	MaxWords      int            `json:"maxWords,omitempty"      schema:"description=Maximum word count for target text (0 = disabled),default=0,min=0"`
	MaxPercentage float64        `json:"maxPercentage,omitempty" schema:"description=Maximum target/source length ratio as percentage (0 = disabled),default=0,min=0"`
	MinPercentage float64        `json:"minPercentage,omitempty" schema:"description=Minimum target/source length ratio as percentage (0 = disabled),default=0,min=0"`
}

// ToolName returns the tool name this config applies to.
func (c *LengthCheckConfig) ToolName() string { return "length-check" }

// Reset restores default values.
func (c *LengthCheckConfig) Reset() {
	c.TargetLocale = ""
	c.MaxChars = 0
	c.MaxWords = 0
	c.MaxPercentage = 0
	c.MinPercentage = 0
}

// Validate checks configuration validity.
func (c *LengthCheckConfig) Validate() error {
	if c.TargetLocale.IsEmpty() {
		return fmt.Errorf("length-check: TargetLocale is required")
	}
	if c.MaxChars < 0 {
		return fmt.Errorf("length-check: MaxChars must be non-negative")
	}
	if c.MaxWords < 0 {
		return fmt.Errorf("length-check: MaxWords must be non-negative")
	}
	if c.MaxPercentage < 0 {
		return fmt.Errorf("length-check: MaxPercentage must be non-negative")
	}
	if c.MinPercentage < 0 {
		return fmt.Errorf("length-check: MinPercentage must be non-negative")
	}
	return nil
}

// LengthCheckSchema returns the auto-generated schema for the length-check tool.
func LengthCheckSchema() *schema.ComponentSchema {
	return schema.FromStruct(&LengthCheckConfig{}, schema.ToolMeta{
		ID:          "length-check",
		Category:    schema.CategoryValidate,
		DisplayName: "Length Check",
		Description: "Validate string length against configured limits",
		Inputs:      []string{schema.PartTypeBlock},
		Requires:    []string{schema.RequiresTargetLanguage},
	})
}

// NewLengthCheckFromConfig creates a length-check tool from a config map.
func NewLengthCheckFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	var cfg LengthCheckConfig
	if err := schema.ApplyConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("length-check config: %w", err)
	}
	if targetLang != "" {
		cfg.TargetLocale = model.LocaleID(targetLang)
	}
	return NewLengthCheckTool(&cfg), nil
}

// NewLengthCheckTool creates a tool that verifies translation length constraints.
// It checks character count, word count, and source/target length ratios,
// storing findings in Block.Properties as a JSON array of QAIssue.
func NewLengthCheckTool(cfg *LengthCheckConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "length-check",
		ToolDescription: "Verifies translation length constraints (chars, words, ratio)",
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

		conf := t.Cfg.(*LengthCheckConfig)

		if block.Properties == nil {
			block.Properties = make(map[string]string)
		}

		if !block.HasTarget(conf.TargetLocale) {
			return part, nil
		}

		targetText := block.TargetText(conf.TargetLocale)
		sourceText := block.SourceText()

		var issues []QAIssue

		// Check max character count.
		if conf.MaxChars > 0 {
			charCount := len([]rune(targetText))
			if charCount > conf.MaxChars {
				issues = append(issues, QAIssue{
					Type:     "max-chars-exceeded",
					Severity: QASeverityError,
					Message:  fmt.Sprintf("Target has %d characters, exceeds maximum of %d", charCount, conf.MaxChars),
				})
			}
		}

		// Check max word count.
		if conf.MaxWords > 0 {
			wordCount := countWords(targetText)
			if wordCount > conf.MaxWords {
				issues = append(issues, QAIssue{
					Type:     "max-words-exceeded",
					Severity: QASeverityError,
					Message:  fmt.Sprintf("Target has %d words, exceeds maximum of %d", wordCount, conf.MaxWords),
				})
			}
		}

		// Check percentage-based constraints (only when source is non-empty).
		if sourceText != "" {
			sourceLen := len([]rune(sourceText))
			targetLen := len([]rune(targetText))
			ratio := float64(targetLen) / float64(sourceLen) * 100.0

			if conf.MaxPercentage > 0 && ratio > conf.MaxPercentage {
				issues = append(issues, QAIssue{
					Type:     "max-percentage-exceeded",
					Severity: QASeverityWarning,
					Message:  fmt.Sprintf("Target is %.0f%% of source length, exceeds maximum of %.0f%%", ratio, conf.MaxPercentage),
				})
			}

			if conf.MinPercentage > 0 && ratio < conf.MinPercentage {
				issues = append(issues, QAIssue{
					Type:     "min-percentage-exceeded",
					Severity: QASeverityWarning,
					Message:  fmt.Sprintf("Target is %.0f%% of source length, below minimum of %.0f%%", ratio, conf.MinPercentage),
				})
			}
		}

		storeLengthCheckIssues(block, issues)

		return part, nil
	}
	return t
}

// storeLengthCheckIssues writes length check findings to Block.Properties.
func storeLengthCheckIssues(block *model.Block, issues []QAIssue) {
	if block.Properties == nil {
		block.Properties = make(map[string]string)
	}

	if len(issues) == 0 {
		block.Properties[PropLengthCheckPassed] = "true"
		block.Properties[PropLengthCheckIssues] = "[]"
		return
	}

	block.Properties[PropLengthCheckPassed] = "false"
	data, err := json.Marshal(issues)
	if err != nil {
		block.Properties[PropLengthCheckIssues] = "[]"
		return
	}
	block.Properties[PropLengthCheckIssues] = string(data)
}
