package tools

import (
	"fmt"
	"strings"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/tool"
)

// Zero-width characters that can be removed.
const (
	zeroWidthSpace      = '\u200B'
	zeroWidthNonJoiner  = '\u200C'
	zeroWidthJoiner     = '\u200D'
	zeroWidthNoBreak    = '\uFEFF' // BOM / zero-width no-break space
)

// WhitespaceCorrectConfig holds configuration for the whitespace correction tool.
type WhitespaceCorrectConfig struct {
	TargetLocale          model.LocaleID // Required
	NormalizeSpaces       bool           // Collapse multiple spaces to single (default: true)
	TrimLeading           bool           // Remove leading whitespace (default: false)
	TrimTrailing          bool           // Remove trailing whitespace (default: false)
	MatchSourceWhitespace bool           // Copy source leading/trailing whitespace to target (default: true)
	RemoveZeroWidthChars  bool           // Remove zero-width spaces/joiners (default: true)
}

// ToolName returns the tool name this config applies to.
func (c *WhitespaceCorrectConfig) ToolName() string { return "whitespace-correct" }

// Reset restores default values.
func (c *WhitespaceCorrectConfig) Reset() {
	c.TargetLocale = ""
	c.NormalizeSpaces = true
	c.TrimLeading = false
	c.TrimTrailing = false
	c.MatchSourceWhitespace = true
	c.RemoveZeroWidthChars = true
}

// Validate checks configuration validity.
func (c *WhitespaceCorrectConfig) Validate() error {
	if c.TargetLocale.IsEmpty() {
		return fmt.Errorf("whitespace-correct: TargetLocale is required")
	}
	return nil
}

// NewWhitespaceCorrectTool creates a tool that normalizes and fixes whitespace
// issues in target translations.
func NewWhitespaceCorrectTool(cfg *WhitespaceCorrectConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "whitespace-correct",
		ToolDescription: "Normalizes and fixes whitespace issues in translations",
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

		conf := t.Cfg.(*WhitespaceCorrectConfig)
		if conf.TargetLocale.IsEmpty() || !block.HasTarget(conf.TargetLocale) {
			return part, nil
		}

		targetText := block.TargetText(conf.TargetLocale)

		if conf.RemoveZeroWidthChars {
			targetText = removeZeroWidthChars(targetText)
		}

		if conf.NormalizeSpaces {
			targetText = normalizeSpaces(targetText)
		}

		if conf.TrimLeading {
			targetText = strings.TrimLeft(targetText, " \t\n\r")
		}

		if conf.TrimTrailing {
			targetText = strings.TrimRight(targetText, " \t\n\r")
		}

		if conf.MatchSourceWhitespace {
			sourceText := block.SourceText()
			targetText = matchSourceWhitespace(sourceText, targetText)
		}

		block.SetTargetText(conf.TargetLocale, targetText)
		return part, nil
	}
	return t
}

// removeZeroWidthChars strips zero-width Unicode characters from text.
func removeZeroWidthChars(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	for _, r := range text {
		switch r {
		case zeroWidthSpace, zeroWidthNonJoiner, zeroWidthJoiner, zeroWidthNoBreak:
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// normalizeSpaces collapses runs of multiple spaces into a single space.
func normalizeSpaces(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	prevSpace := false
	for _, r := range text {
		if r == ' ' {
			if prevSpace {
				continue
			}
			prevSpace = true
		} else {
			prevSpace = false
		}
		b.WriteRune(r)
	}
	return b.String()
}

// matchSourceWhitespace copies the leading and trailing whitespace pattern
// from source to target text.
func matchSourceWhitespace(source, target string) string {
	// Extract leading whitespace from source.
	sourceLeading := ""
	for _, r := range source {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			sourceLeading += string(r)
		} else {
			break
		}
	}

	// Extract trailing whitespace from source.
	sourceTrailing := ""
	runes := []rune(source)
	for i := len(runes) - 1; i >= 0; i-- {
		r := runes[i]
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			sourceTrailing = string(r) + sourceTrailing
		} else {
			break
		}
	}

	// Strip existing leading/trailing whitespace from target and reapply source's.
	trimmed := strings.TrimRight(strings.TrimLeft(target, " \t\n\r"), " \t\n\r")
	return sourceLeading + trimmed + sourceTrailing
}
