package tools

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/text/encoding/ianaindex"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// CharsCheckConfig holds configuration for the character check tool.
type CharsCheckConfig struct {
	TargetLocale   model.LocaleID `json:"targetLocale,omitempty"      schema:"-"`
	ForbiddenChars string         `json:"forbiddenChars,omitempty"    schema:"title=Forbidden Characters,description=Characters that should not appear in target text (e.g. {}[])"`
	RequiredChars  string         `json:"requiredChars,omitempty"     schema:"title=Required Characters,description=Characters that must appear in target if present in source (e.g. punctuation)"`
	CheckCorrupted bool           `json:"checkCorrupted,omitempty"    schema:"title=Check Corrupted Characters,description=Check for common corruption patterns such as mojibake,default=true"`
	CheckCharset   bool           `json:"checkCharset,omitempty"      schema:"title=Check Against Charset Encoding,description=Warn if a character is not included in the specified character set encoding"`
	Charset        string         `json:"charset,omitempty"           schema:"title=Character Set Encoding,description=Name of the character set encoding to check against (e.g. ISO-8859-1),default=ISO-8859-1"`
}

// ToolName returns the tool name this config applies to.
func (c *CharsCheckConfig) ToolName() string { return "chars-check" }

// Reset restores default values.
func (c *CharsCheckConfig) Reset() {
	c.TargetLocale = ""
	c.ForbiddenChars = ""
	c.RequiredChars = ""
	c.CheckCorrupted = true
	c.CheckCharset = false
	c.Charset = "ISO-8859-1"
}

// Validate checks configuration validity.
func (c *CharsCheckConfig) Validate() error {
	if c.TargetLocale.IsEmpty() {
		return errors.New("chars-check: TargetLocale is required")
	}
	return nil
}

// NewCharsCheckConfig creates a CharsCheckConfig with corruption checking enabled.
func NewCharsCheckConfig(targetLocale model.LocaleID) *CharsCheckConfig {
	return &CharsCheckConfig{
		TargetLocale:   targetLocale,
		CheckCorrupted: true,
		CheckCharset:   false,
		Charset:        "ISO-8859-1",
	}
}

// CharsCheckSchema returns the auto-generated schema for the chars-check tool.
func CharsCheckSchema() *schema.ComponentSchema {
	return schema.FromStruct(NewCharsCheckConfig(""), schema.ToolMeta{
		ID:          "chars-check",
		Category:    schema.CategoryQuality,
		DisplayName: "Chars Check",
		Description: "Check for invalid or unexpected Unicode characters",
		Inputs:      []string{schema.PartTypeBlock},
		Requires:    []string{schema.RequiresTargetLanguage},
	})
}

// NewCharsCheckFromConfig creates a chars-check tool from a config map.
func NewCharsCheckFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	cfg := NewCharsCheckConfig(model.LocaleID(targetLang))
	if err := schema.ApplyConfig(config, cfg); err != nil {
		return nil, fmt.Errorf("chars-check config: %w", err)
	}
	if targetLang != "" {
		cfg.TargetLocale = model.LocaleID(targetLang)
	}
	return NewCharsCheckTool(cfg), nil
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
	t.Annotate = func(v tool.BlockView) error {
		if !v.Translatable() {
			return nil
		}

		conf := t.Cfg.(*CharsCheckConfig)

		if !v.HasTarget(conf.TargetLocale) {
			return nil
		}

		targetText := v.TargetText(conf.TargetLocale)
		sourceText := v.SourceText()

		var findings []check.Finding

		// Check forbidden characters.
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

		// Check required characters (characters present in source must also appear in target).
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

		// Check characters against charset encoding.
		if conf.CheckCharset && conf.Charset != "" {
			findings = append(findings, checkCharset(targetText, conf.Charset)...)
		}

		// Check corruption patterns.
		if conf.CheckCorrupted {
			findings = append(findings, checkCorruption(targetText)...)
		}

		check.Annotate(v, "chars-check", findings)

		return nil
	}
	return t
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

// checkCorruption detects common text corruption patterns.
func checkCorruption(text string) []check.Finding {
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
	if strings.ContainsRune(text, '\uFFFD') {
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
