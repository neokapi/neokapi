package tools

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/set"
	"github.com/neokapi/neokapi/core/tool"
)

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
		Category:    schema.CategoryQuality,
		DisplayName: "QA Check",
		Description: "Run rule-based quality checks on translations",
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
	// patterns are the config's QA patterns with their source/target regexes
	// compiled once at construction, instead of per block.
	patterns []compiledQAPattern
}

// compiledQAPattern is a QAPattern with its regexes precompiled. tgtRe is nil
// for "same-match" patterns (empty or "<same>" target).
type compiledQAPattern struct {
	pat   QAPattern
	srcRe *regexp.Regexp
	tgtRe *regexp.Regexp
}

// compileQAPatterns precompiles the enabled patterns, dropping any with an
// invalid (or missing) source regex, or an invalid target regex — matching the
// old per-block code's "continue past bad patterns" behavior.
func compileQAPatterns(patterns []QAPattern) []compiledQAPattern {
	out := make([]compiledQAPattern, 0, len(patterns))
	for _, p := range patterns {
		if !p.Enabled || p.Source == "" {
			continue
		}
		srcRe, err := regexp.Compile(p.Source)
		if err != nil {
			continue
		}
		cp := compiledQAPattern{pat: p, srcRe: srcRe}
		if p.Target != "" && p.Target != "<same>" {
			tgtRe, err := regexp.Compile(p.Target)
			if err != nil {
				continue
			}
			cp.tgtRe = tgtRe
		}
		out = append(out, cp)
	}
	return out
}

// checkTextIssues runs text-level checks: empty, whitespace, doubled words, same-as-source, corrupted chars.
func (h *qaCheckHandler) checkTextIssues(conf *QACheckConfig, sourceText, targetText string) []check.Finding {
	var findings []check.Finding

	// Check: empty target (target segments exist but text is empty).
	if conf.CheckEmptyTarget && targetText == "" && sourceText != "" {
		findings = append(findings, check.Finding{
			Category: "empty-target",
			Severity: check.SeverityMajor,
			Message:  "Target is empty but source has content",
		})
	}

	// Check: empty source (non-empty target but empty source).
	if conf.CheckEmptySource && sourceText == "" && targetText != "" {
		findings = append(findings, check.Finding{
			Category: "empty-source",
			Severity: check.SeverityMinor,
			Message:  "Target is not empty but source is empty",
		})
	}

	// Check: leading whitespace mismatch.
	if conf.CheckLeadingWhitespace && targetText != "" {
		srcLeading := leadingWhitespace(sourceText)
		tgtLeading := leadingWhitespace(targetText)
		if srcLeading != tgtLeading {
			findings = append(findings, check.Finding{
				Category: "leading-whitespace",
				Severity: check.SeverityMinor,
				Message:  "Leading whitespace differs between source and target",
			})
		}
	}

	// Check: trailing whitespace mismatch.
	if conf.CheckTrailingWhitespace && targetText != "" {
		srcTrailing := trailingWhitespace(sourceText)
		tgtTrailing := trailingWhitespace(targetText)
		if srcTrailing != tgtTrailing {
			findings = append(findings, check.Finding{
				Category: "trailing-whitespace",
				Severity: check.SeverityMinor,
				Message:  "Trailing whitespace differs between source and target",
			})
		}
	}

	// Check: double spaces in target.
	if conf.CheckDoubleSpaces && strings.Contains(targetText, "  ") {
		findings = append(findings, check.Finding{
			Category: "double-spaces",
			Severity: check.SeverityMinor,
			Message:  "Target contains double spaces",
		})
	}

	// Check: doubled words in target.
	if conf.CheckDoubledWord && targetText != "" {
		if word := findDoubledWord(targetText, conf.DoubledWordExceptions); word != "" {
			findings = append(findings, check.Finding{
				Category:     "doubled-word",
				Severity:     check.SeverityMinor,
				Message:      fmt.Sprintf("Target contains doubled word: %q", word),
				OriginalText: word,
			})
		}
	}

	// Check: target same as source.
	if conf.CheckTargetSameAsSource && targetText != "" && sourceText != "" && targetText == sourceText {
		if containsWordChar(sourceText) {
			if conf.TargetSameAsSourceWithNumbers || !isNumberOnly(sourceText) {
				findings = append(findings, check.Finding{
					Category: "target-same-as-source",
					Severity: check.SeverityMinor,
					Message:  "Target is identical to source",
				})
			}
		}
	}

	// Check: corrupted characters.
	if conf.CheckCorruptedCharacters && targetText != "" {
		if hasCorruptedCharacters(targetText) {
			findings = append(findings, check.Finding{
				Category: "corrupted-characters",
				Severity: check.SeverityMinor,
				Message:  "Target may contain corrupted characters (encoding issue)",
			})
		}
	}

	return findings
}

// checkLengthIssues runs length-related checks: max ratio, min ratio, absolute max.
func (h *qaCheckHandler) checkLengthIssues(conf *QACheckConfig, sourceText, targetText string) []check.Finding {
	var findings []check.Finding

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
				findings = append(findings, check.Finding{
					Category: "max-length",
					Severity: check.SeverityMinor,
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
				findings = append(findings, check.Finding{
					Category: "min-length",
					Severity: check.SeverityMinor,
					Message:  fmt.Sprintf("Target is %d%% of source length (min required: %d%%)", pct, minPct),
				})
			}
		}
	}

	// Check: absolute maximum character length.
	if conf.CheckAbsoluteMaxCharLength && targetText != "" {
		tgtLen := len([]rune(targetText))
		if tgtLen > conf.AbsoluteMaxCharLength {
			findings = append(findings, check.Finding{
				Category: "absolute-max-length",
				Severity: check.SeverityMinor,
				Message:  fmt.Sprintf("Target has %d characters (max allowed: %d)", tgtLen, conf.AbsoluteMaxCharLength),
			})
		}
	}

	return findings
}

// checkPatternAndCodeIssues runs pattern verification and inline code/span constraint checks.
func (h *qaCheckHandler) checkPatternAndCodeIssues(conf *QACheckConfig, v tool.BlockView, sourceText, targetText string) []check.Finding {
	var findings []check.Finding

	// Check: pattern verification.
	if conf.CheckPatterns && len(h.patterns) > 0 {
		findings = append(findings, h.checkPatterns(sourceText, targetText)...)
	}

	// Check: inline code differences.
	if conf.CheckCodeDifference {
		sourceRuns := v.SourceRuns()
		if runsHaveInline(sourceRuns) && v.HasTarget(conf.TargetLocale) {
			targetRuns := v.TargetRuns(conf.TargetLocale)
			findings = append(findings, checkCodeDifferencesRuns(sourceRuns, targetRuns, conf.StrictCodeOrder)...)
		}
	}

	// Check: run constraint violations.
	if conf.CheckSpanConstraints {
		sourceRuns := v.SourceRuns()
		if runsHaveInline(sourceRuns) && v.HasTarget(conf.TargetLocale) {
			targetRuns := v.TargetRuns(conf.TargetLocale)
			findings = append(findings, checkRunConstraints(sourceRuns, targetRuns)...)
		}
	}

	return findings
}

// NewQACheckTool creates a rule-based QA check tool.
// It examines source and target text for common translation quality issues
// and records them as core/check.Finding under the unified quality.findings
// annotation (check.Annotate), where they accumulate alongside any other
// checker's findings on the same block.
func NewQACheckTool(cfg *QACheckConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "qa-check",
		ToolDescription: "Performs rule-based quality checks on translations",
		Cfg:             cfg,
	}
	h := &qaCheckHandler{tool: t, patterns: compileQAPatterns(cfg.Patterns)}

	t.Annotate = func(v tool.BlockView) error {
		if !v.Translatable() {
			return nil
		}

		conf := t.Cfg.(*QACheckConfig)

		sourceText := v.SourceText()

		// If there is no target, check if empty target is an issue.
		if !v.HasTarget(conf.TargetLocale) {
			if conf.CheckEmptyTarget && sourceText != "" {
				check.Annotate(v, "qa-check", []check.Finding{{
					Category: "empty-target",
					Severity: check.SeverityMajor,
					Message:  "Target is empty but source has content",
				}})
			}
			return nil
		}

		targetText := v.TargetText(conf.TargetLocale)

		var findings []check.Finding
		findings = append(findings, h.checkTextIssues(conf, sourceText, targetText)...)
		findings = append(findings, h.checkLengthIssues(conf, sourceText, targetText)...)
		findings = append(findings, h.checkPatternAndCodeIssues(conf, v, sourceText, targetText)...)

		check.Annotate(v, "qa-check", findings)

		return nil
	}
	return t
}

// runFingerprint returns a string key for matching runs: "type|kind".
// Kind is one of ph / pcOpen / pcClose / sub, mirroring the old
// Span fingerprint ("type|SpanType") shape.
func runFingerprint(r model.Run) (key string, ok bool) {
	switch {
	case r.Ph != nil:
		return r.Ph.Type + "|ph", true
	case r.PcOpen != nil:
		return r.PcOpen.Type + "|pcOpen", true
	case r.PcClose != nil:
		return r.PcClose.Type + "|pcClose", true
	case r.Sub != nil:
		return "sub|sub", true
	}
	return "", false
}

// runConstraints returns (deletable, cloneable) for a run, reading
// the per-run RunConstraints when present and falling back to
// "inline codes mirror source structure" defaults otherwise. For
// PcClose runs, which don't carry their own Constraints per RFC
// 0001, we look up the matching PcOpen in the reference run list
// so the closing half inherits the opening half's constraints.
func runConstraints(r model.Run, reference []model.Run) (deletable, cloneable bool) {
	var c *model.RunConstraints
	switch {
	case r.Ph != nil:
		c = r.Ph.Constraints
	case r.PcOpen != nil:
		c = r.PcOpen.Constraints
	case r.PcClose != nil:
		// Find the matching PcOpen by ID in the same runs scope so
		// the pair shares constraint metadata.
		if paired := findPcOpen(reference, r.PcClose.ID); paired != nil {
			c = paired.Constraints
		}
	}
	if c == nil {
		return false, false
	}
	return c.Deletable, c.Cloneable
}

// findPcOpen walks `runs` looking for a PcOpen with the given id.
// Recurses into plural / select forms so the search respects the
// same scope rules as the rest of the QA checks.
func findPcOpen(runs []model.Run, id string) *model.PcOpenRun {
	for _, r := range runs {
		if r.PcOpen != nil && r.PcOpen.ID == id {
			return r.PcOpen
		}
		if r.Plural != nil {
			for _, form := range r.Plural.Forms {
				if p := findPcOpen(form, id); p != nil {
					return p
				}
			}
		}
		if r.Select != nil {
			for _, form := range r.Select.Cases {
				if p := findPcOpen(form, id); p != nil {
					return p
				}
			}
		}
	}
	return nil
}

// checkRunConstraints compares source and target inline-code counts
// by (type, kind) fingerprint and reports violations where a
// non-deletable code is missing from the target or a non-cloneable
// code is duplicated. Direct Run-native port of checkSpanConstraints.
func checkRunConstraints(source, target []model.Run) []check.Finding {
	sourceCounts, sourceRuns := inlineCodeFingerprints(source)
	targetCounts, _ := inlineCodeFingerprints(target)

	var findings []check.Finding

	// Non-deletable missing from target.
	for key, srcCount := range sourceCounts {
		tgtCount := targetCounts[key]
		if tgtCount >= srcCount {
			continue
		}
		r := sourceRuns[key]
		deletable, _ := runConstraints(r, source)
		if deletable {
			continue
		}
		kind, typ := splitFingerprint(key)
		missing := srcCount - tgtCount
		findings = append(findings, check.Finding{
			Category: "non-deletable-span-missing",
			Severity: check.SeverityMajor,
			Message:  fmt.Sprintf("Non-deletable %s span %q is missing from target (%d missing)", kind, typ, missing),
		})
	}

	// Non-cloneable duplicated in target.
	for key, tgtCount := range targetCounts {
		srcCount := sourceCounts[key]
		if tgtCount <= srcCount {
			continue
		}
		r, ok := sourceRuns[key]
		if !ok {
			continue
		}
		_, cloneable := runConstraints(r, source)
		if cloneable {
			continue
		}
		kind, typ := splitFingerprint(key)
		extra := tgtCount - srcCount
		findings = append(findings, check.Finding{
			Category: "non-cloneable-span-duplicated",
			Severity: check.SeverityMajor,
			Message:  fmt.Sprintf("Non-cloneable %s span %q was duplicated in target (%d extra)", kind, typ, extra),
		})
	}

	return findings
}

// inlineCodeFingerprints counts inline-code runs by fingerprint and
// also returns the exemplar run for each fingerprint (used to look
// up constraints).
func inlineCodeFingerprints(runs []model.Run) (map[string]int, map[string]model.Run) {
	counts := make(map[string]int)
	exemplars := make(map[string]model.Run)
	var walk func(rs []model.Run)
	walk = func(rs []model.Run) {
		for _, r := range rs {
			if key, ok := runFingerprint(r); ok {
				counts[key]++
				if _, seen := exemplars[key]; !seen {
					exemplars[key] = r
				}
			}
			if r.Plural != nil {
				for _, form := range r.Plural.Forms {
					walk(form)
				}
			}
			if r.Select != nil {
				for _, form := range r.Select.Cases {
					walk(form)
				}
			}
		}
	}
	walk(runs)
	return counts, exemplars
}

// splitFingerprint decomposes "type|kind" into its two halves. Used
// by the QA issue message formatters.
func splitFingerprint(key string) (kind, typ string) {
	for i := len(key) - 1; i >= 0; i-- {
		if key[i] == '|' {
			return mapKindToSpanName(key[i+1:]), key[:i]
		}
	}
	return "", key
}

// mapKindToSpanName renders a Run kind back to the human-friendly
// SpanType name the QA messages used to print ("Opening" / "Closing"
// / "Placeholder") so migrating tests only need to care about the
// issue Type field, not the exact wording.
func mapKindToSpanName(kind string) string {
	switch kind {
	case "pcOpen":
		return "Opening"
	case "pcClose":
		return "Closing"
	case "ph":
		return "Placeholder"
	case "sub":
		return "Sub"
	}
	return kind
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
		for w := range strings.SplitSeq(exceptions, ";") {
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

// checkPatterns verifies source/target pattern pairs using precompiled regexes.
func (h *qaCheckHandler) checkPatterns(sourceText, targetText string) []check.Finding {
	var findings []check.Finding
	for _, cp := range h.patterns {
		p := cp.pat
		matches := cp.srcRe.FindAllString(sourceText, -1)
		if len(matches) == 0 {
			continue
		}

		// Check that target matches the target pattern.
		if cp.tgtRe == nil {
			// Target should contain the same matches.
			for _, m := range matches {
				if !strings.Contains(targetText, m) {
					desc := p.Description
					if desc == "" {
						desc = fmt.Sprintf("Pattern %q found in source but not in target", m)
					}
					findings = append(findings, check.Finding{
						Category:     "pattern-mismatch",
						Severity:     check.SeverityMinor,
						Message:      desc,
						OriginalText: m,
					})
				}
			}
		} else {
			tgtMatches := cp.tgtRe.FindAllString(targetText, -1)
			if len(tgtMatches) != len(matches) {
				desc := p.Description
				if desc == "" {
					desc = fmt.Sprintf("Pattern count mismatch: %d in source, %d in target", len(matches), len(tgtMatches))
				}
				findings = append(findings, check.Finding{
					Category: "pattern-mismatch",
					Severity: check.SeverityMinor,
					Message:  desc,
				})
			}
		}
	}
	return findings
}

// checkCodeDifferencesRuns compares source and target inline codes
// by type, walking Run sequences. Direct Run-native port of
// checkCodeDifferences.
func checkCodeDifferencesRuns(source, target []model.Run, strictOrder bool) []check.Finding {
	sourceTypes := inlineCodeTypes(source)
	targetTypes := inlineCodeTypes(target)

	var findings []check.Finding
	sourceCounts := countStrings(sourceTypes)
	targetCounts := countStrings(targetTypes)

	for typ, srcCount := range sourceCounts {
		tgtCount := targetCounts[typ]
		if tgtCount < srcCount {
			findings = append(findings, check.Finding{
				Category: "missing-code",
				Severity: check.SeverityMinor,
				Message:  fmt.Sprintf("Inline code %q missing from target (%d in source, %d in target)", typ, srcCount, tgtCount),
			})
		}
	}
	for typ, tgtCount := range targetCounts {
		srcCount := sourceCounts[typ]
		if tgtCount > srcCount {
			findings = append(findings, check.Finding{
				Category: "extra-code",
				Severity: check.SeverityMinor,
				Message:  fmt.Sprintf("Extra inline code %q in target (%d in source, %d in target)", typ, srcCount, tgtCount),
			})
		}
	}

	if strictOrder && len(findings) == 0 {
		minLen := min(len(targetTypes), len(sourceTypes))
		for i := range minLen {
			if sourceTypes[i] != targetTypes[i] {
				findings = append(findings, check.Finding{
					Category: "code-order",
					Severity: check.SeverityMinor,
					Message:  "Inline code order differs between source and target",
				})
				break
			}
		}
	}

	return findings
}

// inlineCodeTypes returns an ordered list of inline-code Type strings
// walking text-adjacent Ph / PcOpen / PcClose / Sub runs (skipping
// TextRuns but recursing through plural / select forms).
func inlineCodeTypes(runs []model.Run) []string {
	var types []string
	var walk func(rs []model.Run)
	walk = func(rs []model.Run) {
		for _, r := range rs {
			switch {
			case r.Ph != nil:
				types = append(types, r.Ph.Type)
			case r.PcOpen != nil:
				types = append(types, r.PcOpen.Type)
			case r.PcClose != nil:
				types = append(types, r.PcClose.Type)
			case r.Sub != nil:
				types = append(types, "sub")
			case r.Plural != nil:
				// Walk in a canonical plural-form order so the list
				// is deterministic across maps.
				for _, f := range []model.PluralForm{model.PluralZero, model.PluralOne, model.PluralTwo, model.PluralFew, model.PluralMany, model.PluralOther} {
					if form, ok := r.Plural.Forms[f]; ok {
						walk(form)
					}
				}
			case r.Select != nil:
				if form, ok := r.Select.Cases["other"]; ok {
					walk(form)
				}
				// Sort-stable iteration over the remaining keys.
				keys := make([]string, 0, len(r.Select.Cases))
				for k := range r.Select.Cases {
					if k != "other" {
						keys = append(keys, k)
					}
				}
				for _, k := range keys {
					walk(r.Select.Cases[k])
				}
			}
		}
	}
	walk(runs)
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
