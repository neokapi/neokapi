package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/set"
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
	TargetLocale model.LocaleID `json:"targetLocale,omitempty" schema:"-"`

	// --- General checks ---
	CheckLeadingWhitespace        bool   `json:"checkLeadingWhitespace,omitempty"         schema:"title=Check Leading Whitespace,description=Check for leading whitespace mismatches between source and target,default=true,group=general"`
	CheckTrailingWhitespace       bool   `json:"checkTrailingWhitespace,omitempty"         schema:"title=Check Trailing Whitespace,description=Check for trailing whitespace mismatches between source and target,default=true,group=general"`
	CheckEmptyTarget              bool   `json:"checkEmptyTarget,omitempty"                schema:"title=Warn on Empty Target,description=Check for empty target when source has content,default=true,group=general"`
	CheckEmptySource              bool   `json:"checkEmptySource,omitempty"                schema:"title=Warn on Non-Empty Target with Empty Source,description=Check for non-empty target when source is empty,default=true,group=general"`
	CheckTargetSameAsSource       bool   `json:"checkTargetSameAsSource,omitempty"         schema:"title=Warn on Target Same as Source,description=Check when target text is identical to source text,default=true,group=general"`
	TargetSameAsSourceWithCodes   bool   `json:"targetSameAsSourceWithCodes,omitempty"     schema:"title=Include Codes in Comparison,description=Include inline codes when comparing source and target for identity,default=true,group=general"`
	TargetSameAsSourceWithNumbers bool   `json:"targetSameAsSourceWithNumbers,omitempty"   schema:"title=Include Number-Only Segments,description=Include number-only segments in same-as-source comparison,default=true,group=general"`
	CheckDoubleSpaces             bool   `json:"checkDoubleSpaces,omitempty"               schema:"title=Check Double Spaces,description=Check for double spaces in target text,default=true,group=general"`
	CheckDoubledWord              bool   `json:"checkDoubledWord,omitempty"                schema:"title=Warn on Doubled Words,description=Check for consecutive repeated words in target text,default=true,group=general"`
	DoubledWordExceptions         string `json:"doubledWordExceptions,omitempty"           schema:"title=Doubled Word Exceptions,description=Semicolon-separated list of words allowed to repeat (e.g. sie;vous;nous),group=general"`
	CheckTerminology              bool   `json:"checkTerminology,omitempty"                schema:"title=Verify Terminology,description=Enable terminology checks"`
	CheckSpanConstraints          bool   `json:"checkSpanConstraints,omitempty"            schema:"title=Check Span Constraints,description=Check non-deletable and non-cloneable span constraint violations,default=true,group=general"`

	// --- Inline code checks ---
	CheckCodeDifference bool `json:"checkCodeDifference,omitempty" schema:"title=Check Code Differences,description=Verify that target segments have the same inline codes as source segments,default=true,group=inlineCodes"`
	StrictCodeOrder     bool `json:"strictCodeOrder,omitempty"     schema:"title=Enforce Strict Code Order,description=Flag differences when codes appear in a different order between source and target,group=inlineCodes"`

	// --- Pattern checks ---
	CheckPatterns bool        `json:"checkPatterns,omitempty" schema:"title=Check Patterns,description=Verify that source patterns have expected corresponding content in the target,default=true,group=patterns"`
	Patterns      []QAPattern `json:"patterns,omitempty"      schema:"-"`

	// --- Character checks ---
	CheckCorruptedCharacters bool `json:"checkCorruptedCharacters,omitempty" schema:"title=Check Corrupted Characters,description=Check for patterns indicating encoding corruption (e.g. UTF-8 opened as ISO-8859-1),default=true,group=characters"`

	// --- Length checks ---
	CheckMaxCharLength         bool `json:"checkMaxCharLength,omitempty"       schema:"title=Check Maximum Length Ratio,description=Flag targets longer than a percentage of source character length,default=true,group=length"`
	MaxCharLengthBreak         int  `json:"maxCharLengthBreak,omitempty"       schema:"title=Short/Long Threshold (Max),description=Character count above which text is considered long for the maximum length check,default=20,group=length"`
	MaxCharLengthAbove         int  `json:"maxCharLengthAbove,omitempty"       schema:"title=Percentage for Long Text (Max),description=Maximum allowed percentage of source length for long text,default=200,group=length"`
	MaxCharLengthBelow         int  `json:"maxCharLengthBelow,omitempty"       schema:"title=Percentage for Short Text (Max),description=Maximum allowed percentage of source length for short text,default=350,group=length"`
	CheckMinCharLength         bool `json:"checkMinCharLength,omitempty"       schema:"title=Check Minimum Length Ratio,description=Flag targets shorter than a percentage of source character length,default=true,group=length"`
	MinCharLengthBreak         int  `json:"minCharLengthBreak,omitempty"       schema:"title=Short/Long Threshold (Min),description=Character count above which text is considered long for the minimum length check,default=20,group=length"`
	MinCharLengthAbove         int  `json:"minCharLengthAbove,omitempty"       schema:"title=Percentage for Long Text (Min),description=Minimum required percentage of source length for long text,default=45,group=length"`
	MinCharLengthBelow         int  `json:"minCharLengthBelow,omitempty"       schema:"title=Percentage for Short Text (Min),description=Minimum required percentage of source length for short text,default=30,group=length"`
	CheckAbsoluteMaxCharLength bool `json:"checkAbsoluteMaxCharLength,omitempty" schema:"title=Check Absolute Maximum Length,description=Flag target segments that exceed an absolute character count limit,group=length"`
	AbsoluteMaxCharLength      int  `json:"absoluteMaxCharLength,omitempty"      schema:"title=Absolute Maximum Characters,description=Maximum number of characters allowed in any target segment,default=255,group=length"`
}

// QAPattern defines a source/target regex pattern pair for pattern-based QA checks.
type QAPattern struct {
	Enabled     bool   `json:"enabled"`
	Source      string `json:"source"`
	Target      string `json:"target"`
	FromSource  bool   `json:"fromSource"`
	Description string `json:"description"`
}

// ToolName returns the tool name this config applies to.
func (c *QACheckConfig) ToolName() string { return "qa-check" }

// Reset restores default values.
func (c *QACheckConfig) Reset() {
	c.TargetLocale = ""

	// General
	c.CheckLeadingWhitespace = true
	c.CheckTrailingWhitespace = true
	c.CheckEmptyTarget = true
	c.CheckEmptySource = true
	c.CheckTargetSameAsSource = true
	c.TargetSameAsSourceWithCodes = true
	c.TargetSameAsSourceWithNumbers = true
	c.CheckDoubleSpaces = true
	c.CheckDoubledWord = true
	c.DoubledWordExceptions = "sie;vous;nous"
	c.CheckTerminology = false
	c.CheckSpanConstraints = true

	// Inline codes
	c.CheckCodeDifference = true
	c.StrictCodeOrder = false

	// Patterns
	c.CheckPatterns = true
	c.Patterns = nil

	// Characters
	c.CheckCorruptedCharacters = true

	// Length
	c.CheckMaxCharLength = true
	c.MaxCharLengthBreak = DefaultLengthBreak
	c.MaxCharLengthAbove = DefaultMaxPctLongText
	c.MaxCharLengthBelow = DefaultMaxPctShortText
	c.CheckMinCharLength = true
	c.MinCharLengthBreak = DefaultLengthBreak
	c.MinCharLengthAbove = DefaultMinPctLongText
	c.MinCharLengthBelow = DefaultMinPctShortText
	c.CheckAbsoluteMaxCharLength = false
	c.AbsoluteMaxCharLength = DefaultAbsoluteMaxChars
}

// Validate checks configuration validity.
func (c *QACheckConfig) Validate() error {
	if c.TargetLocale.IsEmpty() {
		return errors.New("qa-check: TargetLocale is required")
	}
	return nil
}

// NewQACheckConfig creates a QACheckConfig with all standard checks enabled.
func NewQACheckConfig(targetLocale model.LocaleID) *QACheckConfig {
	cfg := &QACheckConfig{TargetLocale: targetLocale}
	cfg.Reset()
	cfg.TargetLocale = targetLocale
	return cfg
}

// QACheckSchema returns the auto-generated schema for the qa-check tool.
func QACheckSchema() *schema.ComponentSchema {
	return schema.FromStruct(NewQACheckConfig(""), schema.ToolMeta{
		ID:          "qa-check",
		Category:    schema.CategoryValidate,
		DisplayName: "QA Check",
		Description: "Run rule-based quality checks on translations",
		Inputs:      []string{schema.PartTypeBlock},
		Requires:    []string{schema.RequiresTargetLanguage},
	})
}

// NewQACheckFromConfig creates a qa-check tool from a config map.
func NewQACheckFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	cfg := NewQACheckConfig(model.LocaleID(targetLang))
	if err := schema.ApplyConfig(config, cfg); err != nil {
		return nil, fmt.Errorf("qa-check config: %w", err)
	}
	if targetLang != "" {
		cfg.TargetLocale = model.LocaleID(targetLang)
	}
	return NewQACheckTool(cfg), nil
}

// qaCheckHandler holds the config reference and provides methods for each check category.
type qaCheckHandler struct {
	tool *tool.BaseTool
}

// checkTextIssues runs text-level checks: empty, whitespace, doubled words, same-as-source, corrupted chars.
func (h *qaCheckHandler) checkTextIssues(conf *QACheckConfig, sourceText, targetText string) []QAIssue {
	var issues []QAIssue

	// Check: empty target (target segments exist but text is empty).
	if conf.CheckEmptyTarget && targetText == "" && sourceText != "" {
		issues = append(issues, QAIssue{
			Type:     "empty-target",
			Severity: QASeverityError,
			Message:  "Target is empty but source has content",
		})
	}

	// Check: empty source (non-empty target but empty source).
	if conf.CheckEmptySource && sourceText == "" && targetText != "" {
		issues = append(issues, QAIssue{
			Type:     "empty-source",
			Severity: QASeverityWarning,
			Message:  "Target is not empty but source is empty",
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

	// Check: doubled words in target.
	if conf.CheckDoubledWord && targetText != "" {
		if word := findDoubledWord(targetText, conf.DoubledWordExceptions); word != "" {
			issues = append(issues, QAIssue{
				Type:     "doubled-word",
				Severity: QASeverityWarning,
				Message:  fmt.Sprintf("Target contains doubled word: %q", word),
			})
		}
	}

	// Check: target same as source.
	if conf.CheckTargetSameAsSource && targetText != "" && sourceText != "" && targetText == sourceText {
		if containsWordChar(sourceText) {
			if conf.TargetSameAsSourceWithNumbers || !isNumberOnly(sourceText) {
				issues = append(issues, QAIssue{
					Type:     "target-same-as-source",
					Severity: QASeverityWarning,
					Message:  "Target is identical to source",
				})
			}
		}
	}

	// Check: corrupted characters.
	if conf.CheckCorruptedCharacters && targetText != "" {
		if hasCorruptedCharacters(targetText) {
			issues = append(issues, QAIssue{
				Type:     "corrupted-characters",
				Severity: QASeverityWarning,
				Message:  "Target may contain corrupted characters (encoding issue)",
			})
		}
	}

	return issues
}

// checkLengthIssues runs length-related checks: max ratio, min ratio, absolute max.
func (h *qaCheckHandler) checkLengthIssues(conf *QACheckConfig, sourceText, targetText string) []QAIssue {
	var issues []QAIssue

	// Check: maximum character length ratio.
	if conf.CheckMaxCharLength && targetText != "" && sourceText != "" {
		srcLen := len([]rune(sourceText))
		tgtLen := len([]rune(targetText))
		if srcLen > 0 {
			pct := (tgtLen * 100) / srcLen
			maxPct := conf.MaxCharLengthBelow
			if srcLen > conf.MaxCharLengthBreak {
				maxPct = conf.MaxCharLengthAbove
			}
			if pct > maxPct {
				issues = append(issues, QAIssue{
					Type:     "max-length",
					Severity: QASeverityWarning,
					Message:  fmt.Sprintf("Target is %d%% of source length (max allowed: %d%%)", pct, maxPct),
				})
			}
		}
	}

	// Check: minimum character length ratio.
	if conf.CheckMinCharLength && targetText != "" && sourceText != "" {
		srcLen := len([]rune(sourceText))
		tgtLen := len([]rune(targetText))
		if srcLen > 0 {
			pct := (tgtLen * 100) / srcLen
			minPct := conf.MinCharLengthBelow
			if srcLen > conf.MinCharLengthBreak {
				minPct = conf.MinCharLengthAbove
			}
			if pct < minPct {
				issues = append(issues, QAIssue{
					Type:     "min-length",
					Severity: QASeverityWarning,
					Message:  fmt.Sprintf("Target is %d%% of source length (min required: %d%%)", pct, minPct),
				})
			}
		}
	}

	// Check: absolute maximum character length.
	if conf.CheckAbsoluteMaxCharLength && targetText != "" {
		tgtLen := len([]rune(targetText))
		if tgtLen > conf.AbsoluteMaxCharLength {
			issues = append(issues, QAIssue{
				Type:     "absolute-max-length",
				Severity: QASeverityWarning,
				Message:  fmt.Sprintf("Target has %d characters (max allowed: %d)", tgtLen, conf.AbsoluteMaxCharLength),
			})
		}
	}

	return issues
}

// checkPatternAndCodeIssues runs pattern verification and inline code/span constraint checks.
func (h *qaCheckHandler) checkPatternAndCodeIssues(conf *QACheckConfig, block *model.Block, sourceText, targetText string) []QAIssue {
	var issues []QAIssue

	// Check: pattern verification.
	if conf.CheckPatterns && len(conf.Patterns) > 0 {
		issues = append(issues, checkPatterns(sourceText, targetText, conf.Patterns)...)
	}

	// Check: inline code differences.
	if conf.CheckCodeDifference {
		sourceFrag := block.FirstFragment()
		if sourceFrag != nil && sourceFrag.HasSpans() && block.HasTarget(conf.TargetLocale) {
			targetSegs := block.Targets[conf.TargetLocale]
			if len(targetSegs) > 0 {
				targetFrag := targetSegs[0].Content
				issues = append(issues, checkCodeDifferences(sourceFrag, targetFrag, conf.StrictCodeOrder)...)
			}
		}
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

	return issues
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
	h := &qaCheckHandler{tool: t}

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
				storeQAIssues(block, []QAIssue{{
					Type:     "empty-target",
					Severity: QASeverityError,
					Message:  "Target is empty but source has content",
				}})
			}
			return part, nil
		}

		targetText := block.TargetText(conf.TargetLocale)

		var issues []QAIssue
		issues = append(issues, h.checkTextIssues(conf, sourceText, targetText)...)
		issues = append(issues, h.checkLengthIssues(conf, sourceText, targetText)...)
		issues = append(issues, h.checkPatternAndCodeIssues(conf, block, sourceText, targetText)...)

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

// findDoubledWord checks for consecutive repeated words in text.
// Returns the first doubled word found, or "" if none.
// Exceptions is a semicolon-separated list of words to allow.
func findDoubledWord(text, exceptions string) string {
	excSet := set.New[string]()
	if exceptions != "" {
		for _, w := range strings.Split(exceptions, ";") {
			w = strings.TrimSpace(w)
			if w != "" {
				excSet.Add(strings.ToLower(w))
			}
		}
	}
	words := strings.Fields(text)
	for i := 1; i < len(words); i++ {
		prev := strings.ToLower(words[i-1])
		curr := strings.ToLower(words[i])
		if prev == curr && !excSet.Contains(curr) {
			return words[i]
		}
	}
	return ""
}

// containsWordChar returns true if s contains at least one Unicode letter or digit.
func containsWordChar(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// isNumberOnly returns true if s contains only digits, whitespace, and punctuation (no letters).
func isNumberOnly(s string) bool {
	hasDigit := false
	for _, r := range s {
		if unicode.IsDigit(r) {
			hasDigit = true
		} else if unicode.IsLetter(r) {
			return false
		}
	}
	return hasDigit
}

// hasCorruptedCharacters checks for patterns that often indicate encoding corruption.
func hasCorruptedCharacters(s string) bool {
	// Check for common UTF-8 mojibake patterns: sequences like Ã¤ Ã¶ Ã¼ etc.
	// These appear when UTF-8 is misread as ISO-8859-1.
	for _, r := range s {
		if r == unicode.ReplacementChar {
			return true
		}
	}
	return false
}

// checkPatterns verifies source/target pattern pairs.
func checkPatterns(sourceText, targetText string, patterns []QAPattern) []QAIssue {
	var issues []QAIssue
	for _, p := range patterns {
		if !p.Enabled {
			continue
		}
		if p.Source == "" {
			continue
		}
		re, err := regexp.Compile(p.Source)
		if err != nil {
			continue
		}

		matches := re.FindAllString(sourceText, -1)
		if len(matches) == 0 {
			continue
		}

		// Check that target matches the target pattern.
		targetPattern := p.Target
		if targetPattern == "" || targetPattern == "<same>" {
			// Target should contain the same matches.
			for _, m := range matches {
				if !strings.Contains(targetText, m) {
					desc := p.Description
					if desc == "" {
						desc = fmt.Sprintf("Pattern %q found in source but not in target", m)
					}
					issues = append(issues, QAIssue{
						Type:     "pattern-mismatch",
						Severity: QASeverityWarning,
						Message:  desc,
					})
				}
			}
		} else {
			tgtRe, err := regexp.Compile(targetPattern)
			if err != nil {
				continue
			}
			tgtMatches := tgtRe.FindAllString(targetText, -1)
			if len(tgtMatches) != len(matches) {
				desc := p.Description
				if desc == "" {
					desc = fmt.Sprintf("Pattern count mismatch: %d in source, %d in target", len(matches), len(tgtMatches))
				}
				issues = append(issues, QAIssue{
					Type:     "pattern-mismatch",
					Severity: QASeverityWarning,
					Message:  desc,
				})
			}
		}
	}
	return issues
}

// checkCodeDifferences compares source and target inline codes by type.
func checkCodeDifferences(source, target *model.Fragment, strictOrder bool) []QAIssue {
	if source == nil || target == nil {
		return nil
	}

	sourceTypes := spanTypeList(source.Spans)
	targetTypes := spanTypeList(target.Spans)

	var issues []QAIssue

	// Check for missing and extra codes.
	sourceCounts := countStrings(sourceTypes)
	targetCounts := countStrings(targetTypes)

	for typ, srcCount := range sourceCounts {
		tgtCount := targetCounts[typ]
		if tgtCount < srcCount {
			issues = append(issues, QAIssue{
				Type:     "missing-code",
				Severity: QASeverityWarning,
				Message:  fmt.Sprintf("Inline code %q missing from target (%d in source, %d in target)", typ, srcCount, tgtCount),
			})
		}
	}
	for typ, tgtCount := range targetCounts {
		srcCount := sourceCounts[typ]
		if tgtCount > srcCount {
			issues = append(issues, QAIssue{
				Type:     "extra-code",
				Severity: QASeverityWarning,
				Message:  fmt.Sprintf("Extra inline code %q in target (%d in source, %d in target)", typ, srcCount, tgtCount),
			})
		}
	}

	// Strict order check.
	if strictOrder && len(issues) == 0 {
		minLen := len(sourceTypes)
		if len(targetTypes) < minLen {
			minLen = len(targetTypes)
		}
		for i := range minLen {
			if sourceTypes[i] != targetTypes[i] {
				issues = append(issues, QAIssue{
					Type:     "code-order",
					Severity: QASeverityWarning,
					Message:  "Inline code order differs between source and target",
				})
				break
			}
		}
	}

	return issues
}

// spanTypeList returns an ordered list of span Type strings.
func spanTypeList(spans []*model.Span) []string {
	types := make([]string, len(spans))
	for i, s := range spans {
		types[i] = s.Type
	}
	return types
}

// countStrings counts occurrences of each string.
func countStrings(ss []string) map[string]int {
	counts := make(map[string]int)
	for _, s := range ss {
		counts[s]++
	}
	return counts
}
