package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
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
	TargetLocale model.LocaleID `schema:"description=Target locale for processing"`

	// Individual check toggles; all default to true.
	CheckLeadingWhitespace  bool `schema:"description=Check for leading whitespace mismatches between source and target,default=true"`
	CheckTrailingWhitespace bool `schema:"description=Check for trailing whitespace mismatches between source and target,default=true"`
	CheckDoubleSpaces       bool `schema:"description=Check for double spaces in target text,default=true"`
	CheckEmptyTarget        bool `schema:"description=Check for empty target when source has content,default=true"`
	CheckTargetSameAsSource bool `schema:"description=Check when target text is identical to source text,default=true"`
	CheckTerminology        bool `schema:"description=Enable terminology checks"` // Placeholder for future terminology integration
	CheckSpanConstraints    bool `schema:"description=Check non-deletable and non-cloneable span constraint violations,default=true"` // Check non-deletable/non-cloneable span constraint violations
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
	c.CheckSpanConstraints = true
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
		CheckSpanConstraints:    true,
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

		// Check: span constraint violations.
		if conf.CheckSpanConstraints {
			sourceFrag := block.FirstFragment()
			if sourceFrag != nil && sourceFrag.HasSpans() && block.HasTarget(conf.TargetLocale) {
				targetSegs := block.Targets[conf.TargetLocale]
				if len(targetSegs) > 0 {
					targetFrag := targetSegs[0].Content
					issues = append(issues, checkSpanConstraints(sourceFrag, targetFrag)...)
				}
			}
		}

		storeQAIssues(block, issues)

		return part, nil
	}
	return t
}

// spanFingerprint returns a string key for matching spans: "type|spanType".
func spanFingerprint(s *model.Span) string {
	return s.Type + "|" + s.SpanType.String()
}

// checkSpanConstraints compares source and target span counts, reporting
// violations where non-deletable spans are missing or non-cloneable spans
// are duplicated.
func checkSpanConstraints(source, target *model.Fragment) []QAIssue {
	sourceCounts := spanFingerprints(source.Spans)
	targetCounts := spanFingerprints(target.Spans)

	// Build a map from fingerprint → Span (for constraint lookup).
	spanByKey := make(map[string]*model.Span)
	for _, s := range source.Spans {
		spanByKey[spanFingerprint(s)] = s
	}

	var issues []QAIssue

	// Non-deletable span missing from target.
	for key, srcCount := range sourceCounts {
		tgtCount := targetCounts[key]
		if tgtCount < srcCount {
			s := spanByKey[key]
			if s != nil && !s.Deletable {
				missing := srcCount - tgtCount
				issues = append(issues, QAIssue{
					Type:     "non-deletable-span-missing",
					Severity: QASeverityError,
					Message:  fmt.Sprintf("Non-deletable %s span %q is missing from target (%d missing)", s.SpanType, s.Type, missing),
				})
			}
		}
	}

	// Non-cloneable span duplicated in target.
	for key, tgtCount := range targetCounts {
		srcCount := sourceCounts[key]
		if tgtCount > srcCount {
			s := spanByKey[key]
			if s != nil && !s.Cloneable {
				extra := tgtCount - srcCount
				issues = append(issues, QAIssue{
					Type:     "non-cloneable-span-duplicated",
					Severity: QASeverityError,
					Message:  fmt.Sprintf("Non-cloneable %s span %q was duplicated in target (%d extra)", s.SpanType, s.Type, extra),
				})
			}
		}
	}

	return issues
}

// spanFingerprints counts spans by type|spanType fingerprint.
func spanFingerprints(spans []*model.Span) map[string]int {
	counts := make(map[string]int)
	for _, s := range spans {
		counts[spanFingerprint(s)]++
	}
	return counts
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
