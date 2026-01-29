package pensieve

import (
	"github.com/asgeirf/gokapi/core/model"
	"github.com/asgeirf/gokapi/core/tool"
)

// TMLeverageConfig holds configuration for the TM leverage tool.
type TMLeverageConfig struct {
	MinScore     float64
	MaxResults   int
	SourceLocale model.LocaleID
	TargetLocale model.LocaleID
}

// ToolName returns the name of the tool this config applies to.
func (c *TMLeverageConfig) ToolName() string { return "tm-leverage" }

// Reset restores default values.
func (c *TMLeverageConfig) Reset() {
	c.MinScore = 0.7
	c.MaxResults = 5
}

// Validate checks configuration validity.
func (c *TMLeverageConfig) Validate() error { return nil }

// TMLeverageTool applies translation memory matches to translatable blocks.
// When a TM match is found, it is attached as an AltTranslation annotation.
// For exact matches (score = 1.0), the target text is also set directly.
type TMLeverageTool struct {
	tool.BaseTool
	tm  TranslationMemory
	cfg TMLeverageConfig
}

// NewTMLeverageTool creates a new TM leverage tool.
func NewTMLeverageTool(tm TranslationMemory, cfg TMLeverageConfig) *TMLeverageTool {
	if cfg.MinScore <= 0 {
		cfg.MinScore = 0.7
	}
	if cfg.MaxResults <= 0 {
		cfg.MaxResults = 5
	}

	t := &TMLeverageTool{
		tm:  tm,
		cfg: cfg,
	}
	t.ToolName = "tm-leverage"
	t.ToolDescription = "Leverages translation memory to find and apply matches"
	t.HandleBlockFn = t.handleBlock
	return t
}

func (t *TMLeverageTool) handleBlock(part *model.Part) (*model.Part, error) {
	block, ok := part.Resource.(*model.Block)
	if !ok || !block.Translatable {
		return part, nil
	}

	sourceText := block.SourceText()
	if sourceText == "" {
		return part, nil
	}

	matches, err := t.tm.Lookup(sourceText, t.cfg.SourceLocale, t.cfg.TargetLocale, LookupOptions{
		MinScore:   t.cfg.MinScore,
		MaxResults: t.cfg.MaxResults,
	})
	if err != nil {
		return part, nil // Continue processing even if TM lookup fails
	}

	if len(matches) == 0 {
		return part, nil
	}

	best := matches[0]

	// For exact matches, set the target text directly.
	if best.MatchType == MatchExact {
		block.SetTargetText(t.cfg.TargetLocale, best.Entry.Target)
	}

	// Add the best match as an AltTranslation annotation.
	if block.Annotations == nil {
		block.Annotations = make(map[string]model.Annotation)
	}
	block.Annotations["alt-translation"] = &model.AltTranslation{
		Source:    model.NewFragment(best.Entry.Source),
		Target:    model.NewFragment(best.Entry.Target),
		Locale:    t.cfg.TargetLocale,
		Origin:    "tm:pensieve",
		Score:     best.Score,
		MatchType: best.MatchType.String(),
	}

	return part, nil
}
