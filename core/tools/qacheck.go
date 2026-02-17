package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/tool"
)

// QA check property keys stored on Block.Properties.
const (
	PropQAIssues = "qa-issues"
	PropQAPassed = "qa-passed"
)

// QAIssueSeverity indicates the severity of a QA issue.
type QAIssueSeverity string

const (
	QASeverityError   QAIssueSeverity = "error"
	QASeverityWarning QAIssueSeverity = "warning"
)

// QAIssue represents a single QA check finding.
type QAIssue struct {
	Type     string          `json:"type"`
	Severity QAIssueSeverity `json:"severity"`
	Message  string          `json:"message"`
}

// QACheckConfig holds configuration for the QA check tool.
type QACheckConfig struct {
	TargetLocale model.LocaleID

	// Individual check toggles; all default to true.
	CheckLeadingWhitespace  bool
	CheckTrailingWhitespace bool
	CheckDoubleSpaces       bool
	CheckEmptyTarget        bool
	CheckTargetSameAsSource bool
	CheckTerminology        bool // Placeholder for future terminology integration
}

// ToolName returns the tool name this config applies to.
func (c *QACheckConfig) ToolName() string { return "qa-check" }

// Reset restores default values.
func (c *QACheckConfig) Reset() {
	c.TargetLocale = ""
	c.CheckLeadingWhitespace = true
	c.CheckTrailingWhitespace = true
	c.CheckDoubleSpaces = true
	c.CheckEmptyTarget = true
	c.CheckTargetSameAsSource = true
	c.CheckTerminology = false
}

// Validate checks configuration validity.
func (c *QACheckConfig) Validate() error {
	if c.TargetLocale.IsEmpty() {
		return fmt.Errorf("qa-check: TargetLocale is required")
	}
	return nil
}

// NewQACheckConfig creates a QACheckConfig with all standard checks enabled.
func NewQACheckConfig(targetLocale model.LocaleID) *QACheckConfig {
	return &QACheckConfig{
		TargetLocale:            targetLocale,
		CheckLeadingWhitespace:  true,
		CheckTrailingWhitespace: true,
		CheckDoubleSpaces:       true,
		CheckEmptyTarget:        true,
		CheckTargetSameAsSource: true,
		CheckTerminology:        false,
	}
}

// NewQACheckTool creates a rule-based QA check tool.
// It examines source and target text for common translation quality issues
// and stores findings in Block.Properties["qa-issues"] as a JSON array.
func NewQACheckTool(cfg *QACheckConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "qa-check",
		ToolDescription: "Performs rule-based quality checks on translations",
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

		conf := t.Cfg.(*QACheckConfig)

		if block.Properties == nil {
			block.Properties = make(map[string]string)
		}

		sourceText := block.SourceText()

		// If there is no target, check if empty target is an issue.
		if !block.HasTarget(conf.TargetLocale) {
			if conf.CheckEmptyTarget && sourceText != "" {
				issues := []QAIssue{
					{
						Type:     "empty-target",
						Severity: QASeverityError,
						Message:  "Target is empty but source has content",
					},
				}
				storeQAIssues(block, issues)
			}
			return part, nil
		}

		targetText := block.TargetText(conf.TargetLocale)

		var issues []QAIssue

		// Check: empty target (target segments exist but text is empty).
		if conf.CheckEmptyTarget && targetText == "" && sourceText != "" {
			issues = append(issues, QAIssue{
				Type:     "empty-target",
				Severity: QASeverityError,
				Message:  "Target is empty but source has content",
			})
		}

		// Check: leading whitespace mismatch.
		if conf.CheckLeadingWhitespace && targetText != "" {
			srcLeading := leadingWhitespace(sourceText)
			tgtLeading := leadingWhitespace(targetText)
			if srcLeading != tgtLeading {
				issues = append(issues, QAIssue{
					Type:     "leading-whitespace",
					Severity: QASeverityWarning,
					Message:  "Leading whitespace differs between source and target",
				})
			}
		}

		// Check: trailing whitespace mismatch.
		if conf.CheckTrailingWhitespace && targetText != "" {
			srcTrailing := trailingWhitespace(sourceText)
			tgtTrailing := trailingWhitespace(targetText)
			if srcTrailing != tgtTrailing {
				issues = append(issues, QAIssue{
					Type:     "trailing-whitespace",
					Severity: QASeverityWarning,
					Message:  "Trailing whitespace differs between source and target",
				})
			}
		}

		// Check: double spaces in target.
		if conf.CheckDoubleSpaces && strings.Contains(targetText, "  ") {
			issues = append(issues, QAIssue{
				Type:     "double-spaces",
				Severity: QASeverityWarning,
				Message:  "Target contains double spaces",
			})
		}

		// Check: target same as source.
		if conf.CheckTargetSameAsSource && targetText != "" && sourceText != "" && targetText == sourceText {
			issues = append(issues, QAIssue{
				Type:     "target-same-as-source",
				Severity: QASeverityWarning,
				Message:  "Target is identical to source",
			})
		}

		storeQAIssues(block, issues)

		return part, nil
	}
	return t
}

// storeQAIssues writes QA findings to Block.Properties.
func storeQAIssues(block *model.Block, issues []QAIssue) {
	if block.Properties == nil {
		block.Properties = make(map[string]string)
	}

	if len(issues) == 0 {
		block.Properties[PropQAPassed] = "true"
		block.Properties[PropQAIssues] = "[]"
		return
	}

	block.Properties[PropQAPassed] = "false"
	data, err := json.Marshal(issues)
	if err != nil {
		block.Properties[PropQAIssues] = "[]"
		return
	}
	block.Properties[PropQAIssues] = string(data)
}

// leadingWhitespace returns the leading whitespace characters of a string.
func leadingWhitespace(s string) string {
	trimmed := strings.TrimLeft(s, " \t\n\r")
	return s[:len(s)-len(trimmed)]
}

// trailingWhitespace returns the trailing whitespace characters of a string.
func trailingWhitespace(s string) string {
	trimmed := strings.TrimRight(s, " \t\n\r")
	return s[len(trimmed):]
}
