package tools

import (
	"errors"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// FullWidthMode controls the conversion direction.
type FullWidthMode string

const (
	FullWidthToHalf FullWidthMode = "to-half" // Convert full-width → half-width
	FullWidthToFull FullWidthMode = "to-full" // Convert half-width → full-width
)

// FullWidthConvertConfig holds configuration for the full-width conversion tool.
type FullWidthConvertConfig struct {
	Mode            FullWidthMode  `schema:"title=Conversion Mode,description=Conversion direction between half-width and full-width characters,enum=to-half|to-full,default=to-half"`                       // Conversion direction (default: "to-half")
	ApplySource     bool           `schema:"title=Apply to Source,description=Apply to source text"`                                                                                                         // Apply to source (default: false)
	ApplyTarget     bool           `schema:"title=Apply to Target,description=Apply to target text,default=true"`                                                                                            // Apply to target (default: true)
	TargetLocale    model.LocaleID `schema:"title=Target Locale,description=Target locale for processing,showIfSet=ApplyTarget"`                                                                             // Target locale to process (required when ApplyTarget)
	IncludeSLA      bool           `schema:"title=Include Squared Latin Abbreviations,description=Also convert Squared Latin Abbreviations from the CJK Compatibility block to non-CJK character sequences"` // Include Squared Latin Abbreviations
	IncludeLLS      bool           `schema:"title=Include Letter-Like Symbols,description=Also convert characters from the Letter-Like Symbols block to character sequences"`                                // Include Letter-Like Symbols
	IncludeKatakana bool           `schema:"title=Include Katakana,description=Also convert Japanese Katakana and associated punctuation to half-width forms"`                                               // Include Katakana
}

// ToolName returns the tool name this config applies to.
func (c *FullWidthConvertConfig) ToolName() string { return "fullwidth-convert" }

// Reset restores default values.
func (c *FullWidthConvertConfig) Reset() {
	c.Mode = FullWidthToHalf
	c.ApplySource = false
	c.ApplyTarget = true
	c.TargetLocale = ""
	c.IncludeSLA = false
	c.IncludeLLS = false
	c.IncludeKatakana = false
}

// Validate checks configuration validity.
func (c *FullWidthConvertConfig) Validate() error {
	switch c.Mode {
	case FullWidthToHalf, FullWidthToFull:
	default:
		return fmt.Errorf("fullwidth-convert: invalid Mode %q (use to-half or to-full)", c.Mode)
	}
	if c.ApplyTarget && c.TargetLocale.IsEmpty() {
		return errors.New("fullwidth-convert: TargetLocale required when ApplyTarget is true")
	}
	return nil
}

// NewFullWidthConvertTool creates a tool that converts between half-width and
// full-width characters. This is essential for CJK localization where full-width
// Latin characters and punctuation are commonly used.
func NewFullWidthConvertTool(cfg *FullWidthConvertConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "fullwidth-convert",
		ToolDescription: "Converts between half-width and full-width characters",
		Cfg:             cfg,
	}
	// Transform: fullwidth-convert may rewrite source and/or target.
	t.Transform = func(v tool.SourceView) error {
		if !v.Translatable() {
			return nil
		}

		conf := t.Cfg.(*FullWidthConvertConfig)

		if conf.ApplySource {
			v.SetSourceText(convertFullWidth(v.SourceText(), conf.Mode))
		}

		if conf.ApplyTarget && !conf.TargetLocale.IsEmpty() && v.HasTarget(conf.TargetLocale) {
			v.SetTargetText(conf.TargetLocale, convertFullWidth(v.TargetText(conf.TargetLocale), conf.Mode))
		}

		return nil
	}
	return t
}

// convertFullWidth converts text between half-width and full-width characters.
//
// Mapping rules:
//   - ASCII 0x21–0x7E maps to full-width 0xFF01–0xFF5E (offset 0xFEE0)
//   - Space (0x20) maps to ideographic space (0x3000)
func convertFullWidth(text string, mode FullWidthMode) string {
	var b strings.Builder
	b.Grow(len(text))

	for _, r := range text {
		switch mode {
		case FullWidthToFull:
			// Half-width ASCII printable → full-width
			if r >= 0x21 && r <= 0x7E {
				b.WriteRune(r + 0xFEE0)
			} else if r == 0x20 {
				b.WriteRune(0x3000) // space → ideographic space
			} else {
				b.WriteRune(r)
			}
		case FullWidthToHalf:
			// Full-width → half-width ASCII printable
			if r >= 0xFF01 && r <= 0xFF5E {
				b.WriteRune(r - 0xFEE0)
			} else if r == 0x3000 {
				b.WriteRune(0x20) // ideographic space → space
			} else {
				b.WriteRune(r)
			}
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
