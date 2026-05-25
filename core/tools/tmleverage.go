package tools

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
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
	TargetLocale model.LocaleID `json:"targetLocale,omitempty"   schema:"-"`
	SourceLocale model.LocaleID `json:"sourceLocale,omitempty"   schema:"-"`
	Provider     TMProvider     `json:"-"                        schema:"-"`

	// Schema-visible properties matching the bridge schema.
	FuzzyThreshold                int    `json:"fuzzyThreshold,omitempty"   schema:"title=Fuzzy Match Threshold,description=Minimum score for fuzzy matches (0-100),default=70,min=0,max=100"`
	FillTarget                    bool   `json:"fillTarget,omitempty"       schema:"title=Fill Target with Translation,description=Copy the best translation candidate into the target content,default=true"`
	FillTargetThreshold           int    `json:"fillTargetThreshold,omitempty" schema:"title=Fill Target Threshold,description=Minimum match score required to fill the target,default=95,min=0,max=100"`
	FillIfTargetIsEmpty           bool   `json:"fillIfTargetIsEmpty,omitempty" schema:"title=Only If Target Is Empty,description=Fill the target only when it has no existing content"`
	NoQueryThreshold              int    `json:"noQueryThreshold,omitempty" schema:"title=No-Query Threshold,description=Skip TM query if existing candidate scores at or above this value (101 = always query),default=101,min=0,max=101"`
	MakeTMX                       bool   `json:"makeTmx,omitempty"          schema:"title=Generate TMX Document,description=Create a TMX file with all leveraged matches"`
	TMXPath                       string `json:"tmxPath,omitempty"         schema:"title=TMX Output Path,description=File path for the generated TMX document"`
	DowngradeIdenticalBestMatches bool   `json:"downgradeIdenticalBestMatches,omitempty" schema:"title=Downgrade Identical Exact Matches,description=Reduce score by 1%% when multiple identical exact matches are returned"`
}

// ToolName returns the tool name this config applies to.
func (c *TMLeverageConfig) ToolName() string { return "tm-leverage" }

// Reset restores default values.
func (c *TMLeverageConfig) Reset() {
	c.TargetLocale = ""
	c.SourceLocale = ""
	c.Provider = nil
	c.FuzzyThreshold = 70
	c.FillTarget = true
	c.FillTargetThreshold = 95
	c.FillIfTargetIsEmpty = false
	c.NoQueryThreshold = 101
	c.MakeTMX = false
	c.TMXPath = ""
	c.DowngradeIdenticalBestMatches = false
}

// Validate checks configuration validity.
func (c *TMLeverageConfig) Validate() error {
	if c.TargetLocale.IsEmpty() {
		return errors.New("tm-leverage: TargetLocale is required")
	}
	if c.FuzzyThreshold < 0 || c.FuzzyThreshold > 100 {
		return errors.New("tm-leverage: FuzzyThreshold must be between 0 and 100")
	}
	if c.Provider == nil {
		return errors.New("tm-leverage: Provider is required")
	}
	return nil
}

// TMLeverageSchema returns the auto-generated schema for the TM leverage tool.
func TMLeverageSchema() *schema.ComponentSchema {
	cfg := &TMLeverageConfig{}
	cfg.Reset()
	return schema.FromStruct(cfg, schema.ToolMeta{
		ID:          "tm-leverage",
		Category:    schema.CategoryTranslation,
		DisplayName: "TM Leverage",
		Description: "Pre-fill translations from translation memory",
		Inputs:      []string{schema.PartTypeBlock},
		Requires:    []string{schema.RequiresTargetLanguage, schema.RequiresSourceLanguage, schema.RequiresTM},
	})
}

// NewTMLeverageFromConfig creates a TM leverage tool from a config map.
func NewTMLeverageFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	cfg := &TMLeverageConfig{}
	cfg.Reset()
	if err := schema.ApplyConfig(config, cfg); err != nil {
		return nil, fmt.Errorf("tm-leverage config: %w", err)
	}
	if targetLang != "" {
		cfg.TargetLocale = model.LocaleID(targetLang)
	}
	if cfg.FuzzyThreshold == 0 {
		cfg.FuzzyThreshold = 70
	}
	cfg.Provider = NullTMProvider{}
	return NewTMLeverageTool(cfg), nil
}

// NewTMLeverageTool creates a TM leveraging tool that pre-fills translations
// from a translation memory. It first attempts exact matches, then falls back
// to fuzzy matching if a threshold is configured.
func NewTMLeverageTool(cfg *TMLeverageConfig) *tool.BaseTool {
	if cfg.FuzzyThreshold == 0 {
		cfg.FuzzyThreshold = 70
	}
	// Default FillTarget to true if not explicitly configured (backward compat).
	if !cfg.FillTarget && cfg.FillTargetThreshold == 0 {
		cfg.FillTarget = true
		cfg.FillTargetThreshold = 0 // 0 means accept any score
	}

	t := &tool.BaseTool{
		ToolName:        "tm-leverage",
		ToolDescription: "Pre-fills translations from translation memory using exact and fuzzy matching",
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

		// Check no-query threshold: skip TM query if an existing match scores at/above.
		if existingScore, ok := block.Properties[PropTMMatchScore]; ok && conf.NoQueryThreshold <= 101 {
			if score, err := strconv.Atoi(existingScore); err == nil && score >= conf.NoQueryThreshold {
				return part, nil
			}
		}

		// Try exact match first.
		if translation, found := conf.Provider.LookupExact(sourceText, conf.SourceLocale, conf.TargetLocale); found {
			score := 100
			if conf.DowngradeIdenticalBestMatches {
				score = 99
			}
			if shouldFillTarget(conf, block, score) {
				block.SetTargetText(conf.TargetLocale, translation)
			}
			block.Properties[PropTMMatchScore] = strconv.Itoa(score)
			block.Properties[PropTMMatchType] = "exact"
			return part, nil
		}

		// Try fuzzy match.
		if translation, score, found := conf.Provider.LookupFuzzy(sourceText, conf.SourceLocale, conf.TargetLocale, conf.FuzzyThreshold); found {
			if shouldFillTarget(conf, block, score) {
				block.SetTargetText(conf.TargetLocale, translation)
			}
			block.Properties[PropTMMatchScore] = strconv.Itoa(score)
			block.Properties[PropTMMatchType] = "fuzzy"
			return part, nil
		}

		return part, nil
	}
	return t
}

// shouldFillTarget decides whether to copy the translation into the target based on config.
func shouldFillTarget(conf *TMLeverageConfig, block *model.Block, score int) bool {
	if !conf.FillTarget {
		return false
	}
	if score < conf.FillTargetThreshold {
		return false
	}
	if conf.FillIfTargetIsEmpty {
		// Only fill if target is empty.
		if block.HasTarget(conf.TargetLocale) && block.TargetText(conf.TargetLocale) != "" {
			return false
		}
	}
	return true
}
