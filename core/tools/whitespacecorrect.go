package tools

import (
	"errors"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// Zero-width characters that can be removed.
const (
	zeroWidthSpace     = '\u200B'
	zeroWidthNonJoiner = '\u200C'
	zeroWidthJoiner    = '\u200D'
	zeroWidthNoBreak   = '\uFEFF' // BOM / zero-width no-break space
)

// WhitespaceCorrectConfig holds configuration for the whitespace correction tool.
type WhitespaceCorrectConfig struct {
	TargetLocale          model.LocaleID `json:"targetLocale,omitempty"          schema:"title=Target Locale,description=Target locale for processing"`                                             // Required
	NormalizeSpaces       bool           `json:"normalizeSpaces,omitempty"       schema:"title=Normalize Spaces,description=Collapse multiple spaces to a single space,default=true"`               // Collapse multiple spaces to single (default: true)
	TrimLeading           bool           `json:"trimLeading,omitempty"           schema:"title=Trim Leading Whitespace,description=Remove leading whitespace from target text"`                     // Remove leading whitespace (default: false)
	TrimTrailing          bool           `json:"trimTrailing,omitempty"          schema:"title=Trim Trailing Whitespace,description=Remove trailing whitespace from target text"`                   // Remove trailing whitespace (default: false)
	MatchSourceWhitespace bool           `json:"matchSourceWhitespace,omitempty" schema:"title=Match Source Whitespace,description=Copy source leading/trailing whitespace to target,default=true"` // Copy source leading/trailing whitespace to target (default: true)
	RemoveZeroWidthChars  bool           `json:"removeZeroWidthChars,omitempty"  schema:"title=Remove Zero-Width Characters,description=Remove zero-width spaces and joiners,default=true"`         // Remove zero-width spaces/joiners (default: true)

	// Punctuation-specific correction toggles (CJK full-width/ASCII conversion).
	CorrectFullStop    bool `json:"correctFullStop,omitempty"    schema:"title=Full Stop,description=Correct whitespace after full stops (periods),default=true,group=punctuation"`
	CorrectComma       bool `json:"correctComma,omitempty"       schema:"title=Comma,description=Correct whitespace after commas,default=true,group=punctuation"`
	CorrectExclamation bool `json:"correctExclamation,omitempty" schema:"title=Exclamation Point,description=Correct whitespace after exclamation marks,default=true,group=punctuation"`
	CorrectQuestion    bool `json:"correctQuestion,omitempty"    schema:"title=Question Mark,description=Correct whitespace after question marks,default=true,group=punctuation"`

	// Whitespace type toggles for which whitespace characters to remove.
	IncludeVerticalWS   bool `json:"includeVerticalWS,omitempty"   schema:"title=Vertical White Space,description=Include vertical whitespace (line feeds and carriage returns) in corrections,default=true,group=whitespace-types"`
	IncludeHorizontalWS bool `json:"includeHorizontalWS,omitempty" schema:"title=Horizontal Tabs,description=Include horizontal tab characters in corrections,default=true,group=whitespace-types"`
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
	c.CorrectFullStop = true
	c.CorrectComma = true
	c.CorrectExclamation = true
	c.CorrectQuestion = true
	c.IncludeVerticalWS = true
	c.IncludeHorizontalWS = true
}

// Validate checks configuration validity.
func (c *WhitespaceCorrectConfig) Validate() error {
	if c.TargetLocale.IsEmpty() {
		return errors.New("whitespace-correct: TargetLocale is required")
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
		WritesTarget:    true,
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

		// Punctuation-specific whitespace correction: remove trailing whitespace
		// after specific punctuation marks (useful for CJK targets).
		targetText = correctPunctuationWhitespace(targetText, conf)

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

// correctPunctuationWhitespace removes whitespace immediately following
// specific punctuation marks, based on configuration. This is primarily
// useful when translating from space-delimited to non-space-delimited languages.
func correctPunctuationWhitespace(text string, conf *WhitespaceCorrectConfig) string {
	var punctChars []rune
	if conf.CorrectFullStop {
		punctChars = append(punctChars, '.', '\uFF0E') // ASCII + fullwidth
	}
	if conf.CorrectComma {
		punctChars = append(punctChars, ',', '\uFF0C')
	}
	if conf.CorrectExclamation {
		punctChars = append(punctChars, '!', '\uFF01')
	}
	if conf.CorrectQuestion {
		punctChars = append(punctChars, '?', '\uFF1F')
	}
	if len(punctChars) == 0 {
		return text
	}

	punctSet := make(map[rune]bool, len(punctChars))
	for _, r := range punctChars {
		punctSet[r] = true
	}

	runes := []rune(text)
	var b strings.Builder
	b.Grow(len(text))
	for i := 0; i < len(runes); i++ {
		b.WriteRune(runes[i])
		if punctSet[runes[i]] {
			// Skip whitespace after this punctuation.
			for i+1 < len(runes) && isConfiguredWhitespace(runes[i+1], conf) {
				i++
			}
		}
	}
	return b.String()
}

// isConfiguredWhitespace checks if a rune is whitespace that the config
// includes for removal.
func isConfiguredWhitespace(r rune, conf *WhitespaceCorrectConfig) bool {
	switch r {
	case ' ':
		return true
	case '\t':
		return conf.IncludeHorizontalWS
	case '\n', '\r', '\v', '\f':
		return conf.IncludeVerticalWS
	default:
		return false
	}
}

// matchSourceWhitespace copies the leading and trailing whitespace pattern
// from source to target text.
func matchSourceWhitespace(source, target string) string {
	// Extract leading whitespace from source.
	sourceLeading := ""
	var sourceLeadingSb216 strings.Builder
	for _, r := range source {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			sourceLeadingSb216.WriteString(string(r))
		} else {
			break
		}
	}
	sourceLeading += sourceLeadingSb216.String()

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
