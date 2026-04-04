// Package tools provides additional localization tools for the neokapi pipeline.
package tools

import (
	"errors"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// accentMap maps ASCII characters to accented equivalents for pseudo-translation.
var accentMap = map[rune]rune{
	'a': '\u00e0', // a -> à
	'b': '\u0183', // b -> ƃ
	'c': '\u00e7', // c -> ç
	'd': '\u0111', // d -> đ
	'e': '\u00e9', // e -> é
	'f': '\u0192', // f -> ƒ
	'g': '\u011d', // g -> ĝ
	'h': '\u0125', // h -> ĥ
	'i': '\u00ee', // i -> î
	'j': '\u0135', // j -> ĵ
	'k': '\u0137', // k -> ķ
	'l': '\u013c', // l -> ļ
	'm': '\u1e3f', // m -> ḿ
	'n': '\u00f1', // n -> ñ
	'o': '\u00f6', // o -> ö
	'p': '\u00fe', // p -> þ
	'q': '\u01eb', // q -> ǫ
	'r': '\u0155', // r -> ŕ
	's': '\u0161', // s -> š
	't': '\u0163', // t -> ţ
	'u': '\u00fc', // u -> ü
	'v': '\u1e7d', // v -> ṽ
	'w': '\u0175', // w -> ŵ
	'x': '\u1e8b', // x -> ẋ
	'y': '\u00fd', // y -> ý
	'z': '\u017e', // z -> ž
	'A': '\u00c0', // A -> À
	'B': '\u0182', // B -> Ƃ
	'C': '\u00c7', // C -> Ç
	'D': '\u0110', // D -> Đ
	'E': '\u00c9', // E -> É
	'F': '\u0191', // F -> Ƒ
	'G': '\u011c', // G -> Ĝ
	'H': '\u0124', // H -> Ĥ
	'I': '\u00ce', // I -> Î
	'J': '\u0134', // J -> Ĵ
	'K': '\u0136', // K -> Ķ
	'L': '\u013b', // L -> Ļ
	'M': '\u1e3e', // M -> Ḿ
	'N': '\u00d1', // N -> Ñ
	'O': '\u00d6', // O -> Ö
	'P': '\u00de', // P -> Þ
	'Q': '\u01ea', // Q -> Ǫ
	'R': '\u0154', // R -> Ŕ
	'S': '\u0160', // S -> Š
	'T': '\u0162', // T -> Ţ
	'U': '\u00dc', // U -> Ü
	'V': '\u1e7c', // V -> Ṽ
	'W': '\u0174', // W -> Ŵ
	'X': '\u1e8a', // X -> Ẋ
	'Y': '\u00dd', // Y -> Ý
	'Z': '\u017d', // Z -> Ž
}

// PseudoConfig holds configuration for the pseudo-translation tool.
type PseudoConfig struct {
	ExpansionPercent int            `json:"expansionPercent,omitempty" schema:"title=Expansion Percent,description=Extra padding percentage added to simulate translation expansion (0 = no padding),default=0,min=0"`
	Prefix           string         `json:"prefix,omitempty"           schema:"title=Prefix,description=Characters prepended before each translated segment,default=["`
	Suffix           string         `json:"suffix,omitempty"           schema:"title=Suffix,description=Characters appended after each translated segment,default=]"`
	TargetLocale     model.LocaleID `json:"targetLocale,omitempty"     schema:"-"`
}

// ToolName returns the tool name this config applies to.
func (c *PseudoConfig) ToolName() string { return "pseudo-translate" }

// Reset restores default values.
func (c *PseudoConfig) Reset() {
	c.ExpansionPercent = 0
	c.Prefix = "["
	c.Suffix = "]"
	c.TargetLocale = ""
}

// Validate checks configuration validity.
func (c *PseudoConfig) Validate() error {
	if c.ExpansionPercent < 0 {
		return errors.New("pseudo: ExpansionPercent must be >= 0")
	}
	if c.TargetLocale.IsEmpty() {
		return errors.New("pseudo: TargetLocale is required")
	}
	return nil
}

// PseudoTranslateSchema returns the auto-generated schema for the pseudo-translate tool.
func PseudoTranslateSchema() *schema.ComponentSchema {
	return schema.FromStruct(&PseudoConfig{}, schema.ToolMeta{
		ID:          "pseudo-translate",
		Category:    schema.CategoryTranslate,
		DisplayName: "Pseudo Translate",
		Description: "Generate pseudo-translations for localization testing",
		Inputs:      []string{schema.PartTypeBlock},
		Requires:    []string{schema.RequiresTargetLanguage},
	})
}

// NewPseudoTranslateFromConfig creates a pseudo-translate tool from a config map.
func NewPseudoTranslateFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	var cfg PseudoConfig
	if err := schema.ApplyConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("pseudo-translate config: %w", err)
	}
	if targetLang != "" {
		cfg.TargetLocale = model.LocaleID(targetLang)
	}
	return NewPseudoTranslateTool(&cfg), nil
}

// NewPseudoTranslateTool creates a new pseudo-translation tool.
// It replaces ASCII characters with accented equivalents, wraps text
// with brackets, and adds padding for string length testing.
func NewPseudoTranslateTool(cfg *PseudoConfig) *tool.BaseTool {
	if cfg.Prefix == "" && cfg.Suffix == "" {
		cfg.Prefix = "["
		cfg.Suffix = "]"
	}

	t := &tool.BaseTool{
		ToolName:        "pseudo-translate",
		ToolDescription: "Generates pseudo-translations for testing localization readiness",
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

		conf := t.Cfg.(*PseudoConfig)
		frag := block.FirstFragment()
		if frag != nil && frag.HasSpans() {
			// Pseudo-translate coded text, preserving span markers
			pseudoCoded := pseudoTranslateCoded(frag.CodedText, conf)
			targetFrag := frag.Clone()
			targetFrag.CodedText = pseudoCoded
			block.SetTargetFragment(conf.TargetLocale, targetFrag)
		} else {
			sourceText := block.SourceText()
			if sourceText == "" {
				return part, nil
			}
			pseudoText := pseudoTranslate(sourceText, conf)
			block.SetTargetText(conf.TargetLocale, pseudoText)
		}
		return part, nil
	}
	return t
}

// pseudoTranslateCoded applies pseudo-translation to coded text, preserving
// inline span markers (Unicode private use area characters).
func pseudoTranslateCoded(coded string, cfg *PseudoConfig) string {
	// Step 1: Replace ASCII characters with accented equivalents, skip markers.
	var accented strings.Builder
	markerCount := 0
	for _, r := range coded {
		if r >= '\uE001' && r <= '\uE003' {
			accented.WriteRune(r)
			markerCount++
		} else if replacement, ok := accentMap[r]; ok {
			accented.WriteRune(replacement)
		} else {
			accented.WriteRune(r)
		}
	}

	result := accented.String()

	// Step 2: Add expansion padding (based on text length, excluding markers).
	if cfg.ExpansionPercent > 0 {
		textLen := len([]rune(result)) - markerCount
		paddingLen := (textLen * cfg.ExpansionPercent) / 100
		if paddingLen > 0 {
			padding := strings.Repeat("~", paddingLen)
			result = result + " " + padding
		}
	}

	// Step 3: Wrap with prefix/suffix.
	prefix := cfg.Prefix
	suffix := cfg.Suffix
	if prefix == "" {
		prefix = "["
	}
	if suffix == "" {
		suffix = "]"
	}
	result = prefix + result + suffix

	return result
}

// pseudoTranslate applies pseudo-translation transformations to text.
func pseudoTranslate(text string, cfg *PseudoConfig) string {
	// Step 1: Replace ASCII characters with accented equivalents.
	var accented strings.Builder
	for _, r := range text {
		if replacement, ok := accentMap[r]; ok {
			accented.WriteRune(replacement)
		} else {
			accented.WriteRune(r)
		}
	}

	result := accented.String()

	// Step 2: Add expansion padding.
	if cfg.ExpansionPercent > 0 {
		originalLen := len([]rune(result))
		paddingLen := (originalLen * cfg.ExpansionPercent) / 100
		if paddingLen > 0 {
			padding := strings.Repeat("~", paddingLen)
			result = result + " " + padding
		}
	}

	// Step 3: Wrap with prefix/suffix.
	prefix := cfg.Prefix
	suffix := cfg.Suffix
	if prefix == "" {
		prefix = "["
	}
	if suffix == "" {
		suffix = "]"
	}
	result = prefix + result + suffix

	return result
}
