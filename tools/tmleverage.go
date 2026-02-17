package tools

import (
	"fmt"
	"strconv"

	"github.com/gokapi/gokapi/model"
	"github.com/gokapi/gokapi/tool"
)

// TM leverage property keys stored on Block.Properties.
const (
	PropTMMatchScore = "tm-match-score"
	PropTMMatchType  = "tm-match-type"
)

// TMProvider is the interface for translation memory lookup.
type TMProvider interface {
	// LookupExact looks up an exact match for the source text.
	// Returns the translation and true if found.
	LookupExact(source string, sourceLocale, targetLocale model.LocaleID) (string, bool)

	// LookupFuzzy looks up a fuzzy match for the source text.
	// Returns the translation, match score (0-100), and true if found above threshold.
	LookupFuzzy(source string, sourceLocale, targetLocale model.LocaleID, threshold int) (string, int, bool)
}

// NullTMProvider is a TMProvider that returns no matches.
// Useful for testing and as a default when no TM is available.
type NullTMProvider struct{}

// LookupExact always returns no match.
func (NullTMProvider) LookupExact(string, model.LocaleID, model.LocaleID) (string, bool) {
	return "", false
}

// LookupFuzzy always returns no match.
func (NullTMProvider) LookupFuzzy(string, model.LocaleID, model.LocaleID, int) (string, int, bool) {
	return "", 0, false
}

// TMLeverageConfig holds configuration for the TM leverage tool.
type TMLeverageConfig struct {
	TargetLocale   model.LocaleID
	SourceLocale   model.LocaleID
	FuzzyThreshold int        // 0-100; minimum score for fuzzy matches (default: 70)
	Provider       TMProvider // TM provider for lookups
}

// ToolName returns the tool name this config applies to.
func (c *TMLeverageConfig) ToolName() string { return "tm-leverage" }

// Reset restores default values.
func (c *TMLeverageConfig) Reset() {
	c.TargetLocale = ""
	c.SourceLocale = ""
	c.FuzzyThreshold = 70
	c.Provider = nil
}

// Validate checks configuration validity.
func (c *TMLeverageConfig) Validate() error {
	if c.TargetLocale.IsEmpty() {
		return fmt.Errorf("tm-leverage: TargetLocale is required")
	}
	if c.FuzzyThreshold < 0 || c.FuzzyThreshold > 100 {
		return fmt.Errorf("tm-leverage: FuzzyThreshold must be between 0 and 100")
	}
	if c.Provider == nil {
		return fmt.Errorf("tm-leverage: Provider is required")
	}
	return nil
}

// NewTMLeverageTool creates a TM leveraging tool that pre-fills translations
// from a translation memory. It first attempts exact matches, then falls back
// to fuzzy matching if a threshold is configured.
func NewTMLeverageTool(cfg *TMLeverageConfig) *tool.BaseTool {
	if cfg.FuzzyThreshold == 0 {
		cfg.FuzzyThreshold = 70
	}

	t := &tool.BaseTool{
		ToolName:        "tm-leverage",
		ToolDescription: "Pre-fills translations from translation memory using exact and fuzzy matching",
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

		conf := t.Cfg.(*TMLeverageConfig)
		if conf.Provider == nil {
			return part, nil
		}

		if block.Properties == nil {
			block.Properties = make(map[string]string)
		}

		sourceText := block.SourceText()
		if sourceText == "" {
			return part, nil
		}

		// Try exact match first.
		if translation, found := conf.Provider.LookupExact(sourceText, conf.SourceLocale, conf.TargetLocale); found {
			block.SetTargetText(conf.TargetLocale, translation)
			block.Properties[PropTMMatchScore] = "100"
			block.Properties[PropTMMatchType] = "exact"
			return part, nil
		}

		// Try fuzzy match.
		if translation, score, found := conf.Provider.LookupFuzzy(sourceText, conf.SourceLocale, conf.TargetLocale, conf.FuzzyThreshold); found {
			block.SetTargetText(conf.TargetLocale, translation)
			block.Properties[PropTMMatchScore] = strconv.Itoa(score)
			block.Properties[PropTMMatchType] = "fuzzy"
			return part, nil
		}

		return part, nil
	}
	return t
}
