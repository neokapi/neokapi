package tools

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/encoding/ianaindex"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// Default length-ratio thresholds for the qa length checks. These mirror the
// Okapi Framework bridge length-checker defaults.
const (
	DefaultLengthBreak      = 20  // character count dividing "short" from "long" text
	DefaultMaxPctLongText   = 200 // max target/source % for long text
	DefaultMaxPctShortText  = 350 // max target/source % for short text
	DefaultMinPctLongText   = 45  // min target/source % for long text
	DefaultMinPctShortText  = 30  // min target/source % for short text
	DefaultAbsoluteMaxChars = 255 // absolute max character count
)

// RuleCheckConfig holds configuration for the rule-based check tool.
type RuleCheckConfig struct {
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
	CheckPatterns bool           `json:"checkPatterns,omitempty" schema:"title=Check Patterns,description=Verify that source patterns have expected corresponding content in the target,default=true,group=patterns"`
	Patterns      []CheckPattern `json:"patterns,omitempty"      schema:"-"`
	// CheckPlaceholders verifies interpolation placeholders the same way the
	// placeholder-check tool does — including reading an ICU plural or select as
	// the message it is, which no regex pattern can express. It is off by
	// default and switched on by the callers that gate on integrity, the way
	// Patterns is.
	CheckPlaceholders bool `json:"checkPlaceholders,omitempty" schema:"-"`

	// --- Character checks ---
	CheckCorruptedCharacters bool   `json:"checkCorruptedCharacters,omitempty" schema:"title=Check Corrupted Characters,description=Check for patterns indicating encoding corruption (mojibake, replacement chars, stray control chars),default=true,group=characters"`
	ForbiddenChars           string `json:"forbiddenChars,omitempty"           schema:"title=Forbidden Characters,description=Characters that must not appear in target text (e.g. {}[]),group=characters"`
	RequiredChars            string `json:"requiredChars,omitempty"            schema:"title=Required Characters,description=Characters that must appear in target if present in source (e.g. punctuation),group=characters"`
	CheckCharset             bool   `json:"checkCharset,omitempty"             schema:"title=Check Against Charset Encoding,description=Warn if a target character is not included in the specified character set encoding,group=characters"`
	Charset                  string `json:"charset,omitempty"                  schema:"title=Character Set Encoding,description=Name of the character set encoding to check against (e.g. ISO-8859-1),default=ISO-8859-1,group=characters"`

	// --- Consistency checks (cross-block, stateful within a run) ---
	CheckTargetInconsistency bool `json:"checkTargetInconsistency,omitempty" schema:"title=Check Target Inconsistency,description=Flag when the same source text is translated differently across the run,group=consistency"`
	CheckSourceInconsistency bool `json:"checkSourceInconsistency,omitempty" schema:"title=Check Source Inconsistency,description=Flag when different source texts share the same translation,group=consistency"`
	ConsistencyCaseSensitive bool `json:"consistencyCaseSensitive,omitempty" schema:"title=Case-Sensitive Consistency,description=Whether consistency comparison is case-sensitive,default=true,group=consistency"`

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
	CheckMaxWords              bool `json:"checkMaxWords,omitempty"              schema:"title=Check Maximum Word Count,description=Flag target segments that exceed an absolute word count limit,group=length"`
	MaxWords                   int  `json:"maxWords,omitempty"                   schema:"title=Maximum Words,description=Maximum number of words allowed in any target segment,default=0,group=length"`
}

// CheckPattern defines a source/target regex pattern pair for pattern-based checks.
// With Forbidden set, Source is instead a pattern that must NOT match the target
// text (Target is ignored) — the forbidden-pattern rule family.
type CheckPattern struct {
	Enabled     bool   `json:"enabled"`
	Source      string `json:"source"`
	Target      string `json:"target"`
	FromSource  bool   `json:"fromSource"`
	Forbidden   bool   `json:"forbidden,omitempty"`
	Description string `json:"description"`
}

// ToolName returns the tool name this config applies to.
func (c *RuleCheckConfig) ToolName() string { return "qa" }

// Reset restores default values.
func (c *RuleCheckConfig) Reset() {
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
	c.ForbiddenChars = ""
	c.RequiredChars = ""
	c.CheckCharset = false
	c.Charset = "ISO-8859-1"

	// Consistency
	c.CheckTargetInconsistency = false
	c.CheckSourceInconsistency = false
	c.ConsistencyCaseSensitive = true

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
	c.CheckMaxWords = false
	c.MaxWords = 0
}

// Validate checks configuration validity.
func (c *RuleCheckConfig) Validate() error {
	if c.TargetLocale.IsEmpty() {
		return errors.New("qa: TargetLocale is required")
	}
	return c.validatePatterns()
}

// validatePatterns rejects check patterns whose regexes do not compile.
//
// This is the whole point of surfacing them: a pattern that fails to compile
// can never match, so a typo'd rule is indistinguishable from a rule that
// legitimately found nothing. The person who wrote it gets silence either way,
// and reads the silence as "my content is clean". Naming the bad expression at
// construction is the only moment the mistake is still attributable.
//
// Split out from Validate so the config factory can enforce it without also
// newly enforcing TargetLocale, which the factory legitimately leaves empty
// when it is building the tool for schema/listing purposes rather than a run.
func (c *RuleCheckConfig) validatePatterns() error {
	for i, p := range c.Patterns {
		if !p.Enabled {
			continue
		}
		label := strconv.Itoa(i)
		if p.Description != "" {
			label = fmt.Sprintf("%d (%s)", i, p.Description)
		}
		if p.Source == "" {
			return fmt.Errorf("qa: pattern %s has no source expression", label)
		}
		if _, err := regexp.Compile(p.Source); err != nil {
			return fmt.Errorf("qa: pattern %s source %q is not a valid regular expression: %w", label, p.Source, err)
		}
		if !p.Forbidden && p.Target != "" && p.Target != "<same>" {
			if _, err := regexp.Compile(p.Target); err != nil {
				return fmt.Errorf("qa: pattern %s target %q is not a valid regular expression: %w", label, p.Target, err)
			}
		}
	}
	return nil
}

// NewRuleCheckConfig creates a RuleCheckConfig with all standard checks enabled.
func NewRuleCheckConfig(targetLocale model.LocaleID) *RuleCheckConfig {
	cfg := &RuleCheckConfig{TargetLocale: targetLocale}
	cfg.Reset()
	cfg.TargetLocale = targetLocale
	return cfg
}

// NewRuleCheckFromConfig creates a qa tool from a config map.
//
// This is the path a recipe's `qa:` step config travels, so it is the one
// place a hand-written regex can arrive — and therefore the one place a
// hand-written mistake can still be reported to the person who made it.
func NewRuleCheckFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	cfg := NewRuleCheckConfig(model.LocaleID(targetLang))
	if err := schema.ApplyConfig(config, cfg); err != nil {
		return nil, fmt.Errorf("qa config: %w", err)
	}
	if targetLang != "" {
		cfg.TargetLocale = model.LocaleID(targetLang)
	}
	if err := cfg.validatePatterns(); err != nil {
		return nil, fmt.Errorf("qa config: %w", err)
	}
	return NewRuleCheckTool(cfg), nil
}

// checkRouteHandler holds the config reference and provides methods for each check category.
type checkRouteHandler struct {
	tool *tool.BaseTool
	// patterns are the config's check patterns with their source/target regexes
	// compiled once at construction, instead of per block.
	patterns []compiledCheckPattern
	// Consistency state (used only when the consistency checks are enabled):
	// normalized source → set of normalized targets seen, and the reverse.
	sourceToTargets map[string]map[string]bool
	targetToSources map[string]map[string]bool
}

// compiledCheckPattern is a CheckPattern with its regexes precompiled. tgtRe is nil
// for "same-match" patterns (empty or "<same>" target).
type compiledCheckPattern struct {
	pat   CheckPattern
	srcRe *regexp.Regexp
	tgtRe *regexp.Regexp
}

// compileCheckPatterns precompiles the enabled patterns, dropping any with an
// invalid (or missing) source regex, or an invalid target regex — matching the
// old per-block code's "continue past bad patterns" behavior. Forbidden
// patterns only need the source regex (matched against the target text).
//
// The skip is a backstop, not the report. Every configurable path reaches this
// function through NewRuleCheckFromConfig, which runs validatePatterns first and
// refuses to build the tool at all when a regex does not compile — so a typo in
// a recipe now names itself instead of producing a rule that quietly never
// fires. What is left here catches an in-process caller that assembled a
// RuleCheckConfig in Go and skipped Validate; dropping the pattern keeps that
// caller running rather than panicking on its behalf.
func compileCheckPatterns(patterns []CheckPattern) []compiledCheckPattern {
	out := make([]compiledCheckPattern, 0, len(patterns))
	for _, p := range patterns {
		if !p.Enabled || p.Source == "" {
			continue
		}
		srcRe, err := regexp.Compile(p.Source)
		if err != nil {
			continue
		}
		cp := compiledCheckPattern{pat: p, srcRe: srcRe}
		if !p.Forbidden && p.Target != "" && p.Target != "<same>" {
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

// checkTextIssues runs the content-*shape* checks: emptiness, whitespace at the
// boundaries, adjacency, repetition, same-as-source, corrupted characters.
//
// srcShape / tgtShape are check.HygieneText flattenings, where each inline-code
// run stands as one sentinel rune. Every predicate below asks where the content
// begins and ends, or what sits next to what, and a plain SourceText/TargetText
// answers those wrongly because it drops inline code: `[ph][text " each"]` reads
// as leading whitespace, `[text "a "][ph][text " b"]` as a double space, and a
// placeholder-only target as empty. sourceText / targetText stay available for
// the judgements that are genuinely about characters (see corruptionFindings,
// which must not see a sentinel).
func (h *checkRouteHandler) checkTextIssues(conf *RuleCheckConfig, v tool.BlockView, sourceText, targetText, srcShape, tgtShape string) []check.Finding {
	var findings []check.Finding

	// Check: empty target (target segments exist but text is empty).
	if conf.CheckEmptyTarget && tgtShape == "" && srcShape != "" {
		findings = append(findings, check.Finding{
			Category: "empty-target",
			Severity: check.SeverityMajor,
			Message:  "Target is empty but source has content",
		})
	}

	// Check: empty source (non-empty target but empty source).
	if conf.CheckEmptySource && srcShape == "" && tgtShape != "" {
		findings = append(findings, check.Finding{
			Category: "empty-source",
			Severity: check.SeverityMinor,
			Message:  "Target is not empty but source is empty",
		})
	}

	// Check: leading whitespace mismatch.
	if conf.CheckLeadingWhitespace && tgtShape != "" {
		srcLeading := check.LeadingWhitespace(srcShape)
		tgtLeading := check.LeadingWhitespace(tgtShape)
		if srcLeading != tgtLeading {
			findings = append(findings, check.Finding{
				Category: "leading-whitespace",
				Severity: check.SeverityMinor,
				Message:  "Leading whitespace differs between source and target",
			})
		}
	}

	// Check: trailing whitespace mismatch.
	if conf.CheckTrailingWhitespace && tgtShape != "" {
		srcTrailing := check.TrailingWhitespace(srcShape)
		tgtTrailing := check.TrailingWhitespace(tgtShape)
		if srcTrailing != tgtTrailing {
			findings = append(findings, check.Finding{
				Category: "trailing-whitespace",
				Severity: check.SeverityMinor,
				Message:  "Trailing whitespace differs between source and target",
			})
		}
	}

	// Check: double spaces in target.
	if conf.CheckDoubleSpaces && check.DoubleSpaces(tgtShape) {
		findings = append(findings, check.Finding{
			Category: "double-spaces",
			Severity: check.SeverityMinor,
			Message:  "Target contains double spaces",
		})
	}

	// Check: doubled words in target.
	if conf.CheckDoubledWord && tgtShape != "" {
		if word := check.DoubledWord(tgtShape, conf.DoubledWordExceptions); word != "" {
			findings = append(findings, check.Finding{
				Category:     "doubled-word",
				Severity:     check.SeverityMinor,
				Message:      fmt.Sprintf("Target contains doubled word: %q", word),
				OriginalText: word,
			})
		}
	}

	// Check: target same as source. The comparison is on the shapes, so a target
	// that merely moved a placeholder is not "identical"; the word/number
	// heuristics below stay on the plain text, which is what they are about.
	// Whether the *codes* count too is TargetSameAsSourceWithCodes — see
	// sameAsSource.
	if conf.CheckTargetSameAsSource && tgtShape != "" && srcShape != "" &&
		sameAsSource(conf, srcShape, tgtShape, v.SourceRuns(), v.TargetRuns(conf.TargetLocale)) {
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

	// Check: corrupted characters (mojibake, replacement chars, control chars).
	if conf.CheckCorruptedCharacters && targetText != "" {
		findings = append(findings, corruptionFindings(targetText)...)
	}

	return findings
}

// sameAsSource decides whether a target counts as identical to its source for
// the `target-same-as-source` rule, honouring TargetSameAsSourceWithCodes.
//
// The shapes settle the text and where the codes sit: they are check.HygieneText
// flattenings, so each inline code stands as one sentinel and a target that
// merely moved a placeholder is already not identical. That is Okapi's
// IGNORE_CODE comparison — "ignore difference in codes; markers are still
// considered" — and it is the `withCodes = false` reading.
//
// With TargetSameAsSourceWithCodes set (the default, and Okapi's), *which* codes
// they are counts too, so the target must additionally carry exactly the source's
// inline codes — model.DiffRunCodes, the same predicate every stage that commits
// a machine-selected target keys off. This is Okapi's CODE_DATA_ONLY: text equal
// AND codes equal. A target with the source's words but a different placeholder
// is then not "identical to source", which is right — swapping {name} for {count}
// is a change, and reporting it as untranslated points a reviewer at the wrong
// defect (the code difference is separately reported as `missing-code` /
// `extra-code`).
//
// The knob was declared, documented and defaulted true but read by nothing, so
// every comparison silently took the `false` branch (#1463). It is Okapi's
// targetSameAsSourceWithCodes, and this is what it means.
func sameAsSource(conf *RuleCheckConfig, srcShape, tgtShape string, sourceRuns, targetRuns []model.Run) bool {
	if srcShape != tgtShape {
		return false
	}
	if !conf.TargetSameAsSourceWithCodes {
		return true
	}
	return model.DiffRunCodes(sourceRuns, targetRuns).Balanced()
}

// checkCharacterIssues runs the character rules ported from the retired
// chars-check fragment: forbidden characters, required characters, and
// charset-encodability.
func (h *checkRouteHandler) checkCharacterIssues(conf *RuleCheckConfig, sourceText, targetText string) []check.Finding {
	var findings []check.Finding

	// Check: forbidden characters in target.
	if conf.ForbiddenChars != "" {
		for _, ch := range conf.ForbiddenChars {
			if strings.ContainsRune(targetText, ch) {
				findings = append(findings, check.Finding{
					Category:     "forbidden-char",
					Severity:     check.SeverityMajor,
					Message:      fmt.Sprintf("Target contains forbidden character %q (U+%04X)", ch, ch),
					OriginalText: string(ch),
				})
			}
		}
	}

	// Check: required characters (characters present in source must also appear in target).
	if conf.RequiredChars != "" {
		for _, ch := range conf.RequiredChars {
			if strings.ContainsRune(sourceText, ch) && !strings.ContainsRune(targetText, ch) {
				findings = append(findings, check.Finding{
					Category:     "required-char-missing",
					Severity:     check.SeverityMinor,
					Message:      fmt.Sprintf("Source contains %q (U+%04X) but target does not", ch, ch),
					OriginalText: string(ch),
				})
			}
		}
	}

	// Check: characters against charset encoding.
	if conf.CheckCharset && conf.Charset != "" {
		findings = append(findings, checkCharset(targetText, conf.Charset)...)
	}

	return findings
}

// checkConsistencyIssues runs the cross-block consistency checks ported from
// the retired inconsistency-check fragment. It records the source↔target
// mapping for every block it sees and flags the current block when the same
// source has been translated differently (target inconsistency) or different
// sources share the same translation (source inconsistency).
func (h *checkRouteHandler) checkConsistencyIssues(conf *RuleCheckConfig, sourceText, targetText string) []check.Finding {
	normSource := strings.TrimSpace(sourceText)
	normTarget := strings.TrimSpace(targetText)
	if !conf.ConsistencyCaseSensitive {
		normSource = strings.ToLower(normSource)
		normTarget = strings.ToLower(normTarget)
	}

	// Record source -> target mapping.
	if h.sourceToTargets[normSource] == nil {
		h.sourceToTargets[normSource] = make(map[string]bool)
	}
	h.sourceToTargets[normSource][normTarget] = true

	// Record target -> source mapping (only if needed).
	if conf.CheckSourceInconsistency {
		if h.targetToSources[normTarget] == nil {
			h.targetToSources[normTarget] = make(map[string]bool)
		}
		h.targetToSources[normTarget][normSource] = true
	}

	// Target inconsistency: same source, different targets.
	if conf.CheckTargetInconsistency && len(h.sourceToTargets[normSource]) > 1 {
		alternatives := alternativesExcluding(h.sourceToTargets[normSource], normTarget)
		return []check.Finding{{
			Category: "inconsistency",
			Severity: check.SeverityMajor,
			Message:  "Source has more than one translation; also seen as: " + strings.Join(alternatives, ", "),
		}}
	}

	// Source inconsistency: different sources, same target.
	if conf.CheckSourceInconsistency && len(h.targetToSources[normTarget]) > 1 {
		alternatives := alternativesExcluding(h.targetToSources[normTarget], normSource)
		return []check.Finding{{
			Category: "inconsistency",
			Severity: check.SeverityMajor,
			Message:  "Different sources share this translation; also from: " + strings.Join(alternatives, ", "),
		}}
	}

	return nil
}

// alternativesExcluding returns all keys in the set except the excluded one.
func alternativesExcluding(set map[string]bool, exclude string) []string {
	var result []string
	for k := range set {
		if k != exclude {
			result = append(result, k)
		}
	}
	return result
}

// checkLengthIssues runs length-related checks: max ratio, min ratio, absolute max.
func (h *checkRouteHandler) checkLengthIssues(conf *RuleCheckConfig, sourceText, targetText string) []check.Finding {
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

	// Check: absolute maximum word count.
	if conf.CheckMaxWords && conf.MaxWords > 0 && targetText != "" {
		wordCount := model.CountWords(targetText)
		if wordCount > conf.MaxWords {
			findings = append(findings, check.Finding{
				Category: "max-words",
				Severity: check.SeverityMinor,
				Message:  fmt.Sprintf("Target has %d words (max allowed: %d)", wordCount, conf.MaxWords),
			})
		}
	}

	return findings
}

// checkPatternAndCodeIssues runs pattern verification and inline code/span constraint checks.
func (h *checkRouteHandler) checkPatternAndCodeIssues(conf *RuleCheckConfig, v tool.BlockView, sourceText, targetText string) []check.Finding {
	var findings []check.Finding

	// Check: pattern verification.
	if conf.CheckPatterns && len(h.patterns) > 0 {
		findings = append(findings, h.checkPatterns(sourceText, targetText)...)
	}

	// Check: placeholder integrity.
	if conf.CheckPlaceholders {
		findings = append(findings, placeholderFindings(conf.TargetLocale, sourceText, targetText)...)
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

// NewRuleCheckTool creates the rule-based check tool.
// It examines source and target text for common translation quality issues
// and records them as core/check.Finding under the unified quality.findings
// annotation (check.Annotate), where they accumulate alongside any other
// checker's findings on the same block.
func NewRuleCheckTool(cfg *RuleCheckConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "qa",
		ToolDescription: "Performs rule-based quality checks on translations",
		Cfg:             cfg,
	}
	h := &checkRouteHandler{
		tool:            t,
		patterns:        compileCheckPatterns(cfg.Patterns),
		sourceToTargets: make(map[string]map[string]bool),
		targetToSources: make(map[string]map[string]bool),
	}

	t.Annotate = func(v tool.BlockView) error {
		if !v.Translatable() {
			return nil
		}

		conf := t.Cfg.(*RuleCheckConfig)

		sourceText := v.SourceText()
		// The shape flattening keeps inline code visible as a sentinel; every
		// boundary/adjacency/emptiness predicate reads it instead of the plain
		// text (check.HygieneText).
		srcShape := check.HygieneText(v.SourceRuns())

		// If there is no target, check if empty target is an issue.
		if !v.HasTarget(conf.TargetLocale) {
			if conf.CheckEmptyTarget && srcShape != "" {
				check.Annotate(v, "qa", []check.Finding{{
					Category: "empty-target",
					Severity: check.SeverityMajor,
					Message:  "Target is empty but source has content",
				}})
			}
			return nil
		}

		targetText := v.TargetText(conf.TargetLocale)
		tgtShape := check.HygieneText(v.TargetRuns(conf.TargetLocale))

		var findings []check.Finding
		findings = append(findings, h.checkTextIssues(conf, v, sourceText, targetText, srcShape, tgtShape)...)
		findings = append(findings, h.checkCharacterIssues(conf, sourceText, targetText)...)
		findings = append(findings, h.checkLengthIssues(conf, sourceText, targetText)...)
		findings = append(findings, h.checkPatternAndCodeIssues(conf, v, sourceText, targetText)...)
		if conf.CheckTargetInconsistency || conf.CheckSourceInconsistency {
			findings = append(findings, h.checkConsistencyIssues(conf, sourceText, targetText)...)
		}

		check.Annotate(v, "qa", findings)

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
// same scope rules as the rest of the checks.
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
// by the finding message formatters.
func splitFingerprint(key string) (kind, typ string) {
	for i := len(key) - 1; i >= 0; i-- {
		if key[i] == '|' {
			return mapKindToSpanName(key[i+1:]), key[:i]
		}
	}
	return "", key
}

// mapKindToSpanName renders a Run kind back to the human-friendly
// SpanType name the check messages used to print ("Opening" / "Closing"
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

// corruptionFindings detects common text corruption patterns: UTF-8 mojibake,
// the Unicode replacement character, and stray control characters. Ported from
// the retired chars-check fragment.
func corruptionFindings(text string) []check.Finding {
	var findings []check.Finding

	// Check for mojibake patterns.
	for _, pattern := range mojibakePatterns {
		if strings.Contains(text, pattern) {
			findings = append(findings, check.Finding{
				Category:     "mojibake",
				Severity:     check.SeverityMajor,
				Message:      fmt.Sprintf("Possible mojibake detected: %q (UTF-8 decoded as Latin-1)", pattern),
				OriginalText: pattern,
			})
			break // Report mojibake once, not per pattern.
		}
	}

	// Check for Unicode replacement character U+FFFD.
	if strings.ContainsRune(text, unicode.ReplacementChar) {
		findings = append(findings, check.Finding{
			Category: "replacement-char",
			Severity: check.SeverityMajor,
			Message:  "Target contains Unicode replacement character U+FFFD",
		})
	}

	// Check for control characters (U+0000-U+001F except \t, \n, \r).
	for _, r := range text {
		if r <= 0x1F && r != '\t' && r != '\n' && r != '\r' {
			findings = append(findings, check.Finding{
				Category: "control-char",
				Severity: check.SeverityMajor,
				Message:  fmt.Sprintf("Target contains control character U+%04X", r),
			})
			break // Report once.
		}
	}

	return findings
}

// checkCharset verifies that all characters in text can be encoded in the named charset.
func checkCharset(text, charsetName string) []check.Finding {
	enc, err := ianaindex.IANA.Encoding(charsetName)
	if err != nil || enc == nil {
		return []check.Finding{{
			Category: "charset-lookup-error",
			Severity: check.SeverityMinor,
			Message:  fmt.Sprintf("Unknown character set encoding %q", charsetName),
		}}
	}
	encoder := enc.NewEncoder()
	for _, r := range text {
		_, err := encoder.Bytes([]byte(string(r)))
		if err != nil {
			return []check.Finding{{
				Category:     "charset-violation",
				Severity:     check.SeverityMinor,
				Message:      fmt.Sprintf("Character %q (U+%04X) cannot be encoded in %s", r, r, charsetName),
				OriginalText: string(r),
			}}
		}
	}
	return nil
}

// checkPatterns verifies source/target pattern pairs using precompiled regexes.
func (h *checkRouteHandler) checkPatterns(sourceText, targetText string) []check.Finding {
	var findings []check.Finding
	for _, cp := range h.patterns {
		p := cp.pat

		// Forbidden patterns check the target directly: the pattern must not
		// appear there at all (ported from pattern-check's MustNotMatch rules).
		if p.Forbidden {
			if m := cp.srcRe.FindString(targetText); m != "" {
				desc := p.Description
				if desc == "" {
					desc = fmt.Sprintf("Forbidden pattern %q found in target", p.Source)
				}
				findings = append(findings, check.Finding{
					Category:     "forbidden-pattern",
					Severity:     check.SeverityMajor,
					Message:      desc,
					OriginalText: m,
				})
			}
			continue
		}

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
