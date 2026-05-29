package tools

import (
	"errors"
	"fmt"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// Default length-ratio thresholds shared between length-check and qa-check.
// These mirror the Okapi Framework bridge length-checker defaults.
const (
	DefaultLengthBreak      = 20  // character count dividing "short" from "long" text
	DefaultMaxPctLongText   = 200 // max target/source % for long text
	DefaultMaxPctShortText  = 350 // max target/source % for short text
	DefaultMinPctLongText   = 45  // min target/source % for long text
	DefaultMinPctShortText  = 30  // min target/source % for short text
	DefaultAbsoluteMaxChars = 255 // absolute max character count
)

// LengthCheckConfig holds configuration for the length check tool.
type LengthCheckConfig struct {
	TargetLocale model.LocaleID `json:"targetLocale,omitempty" schema:"-"`

	// Absolute limits (simple mode).
	MaxChars int `json:"maxChars,omitempty" schema:"title=Maximum Characters,description=Absolute maximum character count for target text (0 = disabled),default=0,min=0"`
	MaxWords int `json:"maxWords,omitempty" schema:"title=Maximum Words,description=Maximum word count for target text (0 = disabled),default=0,min=0"`

	// Percentage-based limits (simple mode).
	MaxPercentage float64 `json:"maxPercentage,omitempty" schema:"title=Maximum Length Percentage,description=Maximum target/source length ratio as percentage (0 = disabled),default=0,min=0"`
	MinPercentage float64 `json:"minPercentage,omitempty" schema:"title=Minimum Length Percentage,description=Minimum target/source length ratio as percentage (0 = disabled),default=0,min=0"`

	// Long/short threshold model (mirrors bridge length-checker).
	CheckMaxCharLength bool `json:"checkMaxCharLength,omitempty" schema:"title=Check Maximum Length Ratio,description=Warn if target exceeds a percentage of source character length,default=true"`
	MaxCharLengthBreak int  `json:"maxCharLengthBreak,omitempty" schema:"title=Short/Long Threshold (Max),description=Character count threshold between short and long text for max check,default=20,min=0"`
	MaxCharLengthAbove int  `json:"maxCharLengthAbove,omitempty" schema:"title=Percentage for Long Text (Max),description=Max percentage of source length allowed for long text,default=200,min=0"`
	MaxCharLengthBelow int  `json:"maxCharLengthBelow,omitempty" schema:"title=Percentage for Short Text (Max),description=Max percentage of source length allowed for short text,default=350,min=0"`

	CheckMinCharLength bool `json:"checkMinCharLength,omitempty" schema:"title=Check Minimum Length Ratio,description=Warn if target is shorter than a percentage of source character length,default=true"`
	MinCharLengthBreak int  `json:"minCharLengthBreak,omitempty" schema:"title=Short/Long Threshold (Min),description=Character count threshold between short and long text for min check,default=20,min=0"`
	MinCharLengthAbove int  `json:"minCharLengthAbove,omitempty" schema:"title=Percentage for Long Text (Min),description=Min percentage of source length allowed for long text,default=45,min=0"`
	MinCharLengthBelow int  `json:"minCharLengthBelow,omitempty" schema:"title=Percentage for Short Text (Min),description=Min percentage of source length allowed for short text,default=30,min=0"`
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
	c.CheckMaxCharLength = true
	c.MaxCharLengthBreak = DefaultLengthBreak
	c.MaxCharLengthAbove = DefaultMaxPctLongText
	c.MaxCharLengthBelow = DefaultMaxPctShortText
	c.CheckMinCharLength = true
	c.MinCharLengthBreak = DefaultLengthBreak
	c.MinCharLengthAbove = DefaultMinPctLongText
	c.MinCharLengthBelow = DefaultMinPctShortText
}

// Validate checks configuration validity.
func (c *LengthCheckConfig) Validate() error {
	if c.TargetLocale.IsEmpty() {
		return errors.New("length-check: TargetLocale is required")
	}
	if c.MaxChars < 0 {
		return errors.New("length-check: MaxChars must be non-negative")
	}
	if c.MaxWords < 0 {
		return errors.New("length-check: MaxWords must be non-negative")
	}
	if c.MaxPercentage < 0 {
		return errors.New("length-check: MaxPercentage must be non-negative")
	}
	if c.MinPercentage < 0 {
		return errors.New("length-check: MinPercentage must be non-negative")
	}
	return nil
}

// LengthCheckSchema returns the auto-generated schema for the length-check tool.
func LengthCheckSchema() *schema.ComponentSchema {
	return schema.FromStruct(&LengthCheckConfig{}, schema.ToolMeta{
		ID:          "length-check",
		Category:    schema.CategoryQuality,
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
// recording violations as core/check.Finding under the unified quality.findings
// annotation (check.Annotate), accumulating with any other checker's findings.
func NewLengthCheckTool(cfg *LengthCheckConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "length-check",
		ToolDescription: "Verifies translation length constraints (chars, words, ratio)",
		Cfg:             cfg,
	}
	t.Annotate = func(v tool.BlockView) error {
		if !v.Translatable() {
			return nil
		}

		conf := t.Cfg.(*LengthCheckConfig)

		if !v.HasTarget(conf.TargetLocale) {
			return nil
		}

		targetText := v.TargetText(conf.TargetLocale)
		sourceText := v.SourceText()

		var findings []check.Finding

		// Check max character count.
		if conf.MaxChars > 0 {
			charCount := len([]rune(targetText))
			if charCount > conf.MaxChars {
				findings = append(findings, check.Finding{
					Category: "max-chars-exceeded",
					Severity: check.SeverityMajor,
					Message:  fmt.Sprintf("Target has %d characters, exceeds maximum of %d", charCount, conf.MaxChars),
				})
			}
		}

		// Check max word count.
		if conf.MaxWords > 0 {
			wordCount := countWords(targetText)
			if wordCount > conf.MaxWords {
				findings = append(findings, check.Finding{
					Category: "max-words-exceeded",
					Severity: check.SeverityMajor,
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
				findings = append(findings, check.Finding{
					Category: "max-percentage-exceeded",
					Severity: check.SeverityMinor,
					Message:  fmt.Sprintf("Target is %.0f%% of source length, exceeds maximum of %.0f%%", ratio, conf.MaxPercentage),
				})
			}

			if conf.MinPercentage > 0 && ratio < conf.MinPercentage {
				findings = append(findings, check.Finding{
					Category: "min-percentage-exceeded",
					Severity: check.SeverityMinor,
					Message:  fmt.Sprintf("Target is %.0f%% of source length, below minimum of %.0f%%", ratio, conf.MinPercentage),
				})
			}

			// Long/short threshold checks: use different percentage limits
			// depending on whether the source text is "long" or "short".
			if conf.CheckMaxCharLength {
				var maxPct int
				if sourceLen > conf.MaxCharLengthBreak {
					maxPct = conf.MaxCharLengthAbove
				} else {
					maxPct = conf.MaxCharLengthBelow
				}
				if maxPct > 0 && ratio > float64(maxPct) {
					findings = append(findings, check.Finding{
						Category: "max-char-length-exceeded",
						Severity: check.SeverityMinor,
						Message:  fmt.Sprintf("Target is %.0f%% of source length, exceeds %d%% threshold for %s text", ratio, maxPct, longOrShort(sourceLen, conf.MaxCharLengthBreak)),
					})
				}
			}

			if conf.CheckMinCharLength {
				var minPct int
				if sourceLen > conf.MinCharLengthBreak {
					minPct = conf.MinCharLengthAbove
				} else {
					minPct = conf.MinCharLengthBelow
				}
				if minPct > 0 && ratio < float64(minPct) {
					findings = append(findings, check.Finding{
						Category: "min-char-length-exceeded",
						Severity: check.SeverityMinor,
						Message:  fmt.Sprintf("Target is %.0f%% of source length, below %d%% threshold for %s text", ratio, minPct, longOrShort(sourceLen, conf.MinCharLengthBreak)),
					})
				}
			}
		}

		check.Annotate(v, "length-check", findings)

		return nil
	}
	return t
}

// longOrShort returns "long" or "short" depending on whether the given length
// exceeds the breakpoint threshold.
func longOrShort(length, breakpoint int) string {
	if length > breakpoint {
		return "long"
	}
	return "short"
}
