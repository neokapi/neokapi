package sievepen

import (
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
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

// TMLeverageTool applies content-aware translation memory matches to
// translatable blocks. When a TM match is found, it is attached as an
// AltTranslation annotation. For exact matches (including generalized-exact),
// entity adaptations are applied and the target is set directly.
type TMLeverageTool struct {
	tool.BaseTool
	tm  TranslationMemory
	cfg TMLeverageConfig
}

// NewTMLeverageTool creates a new content-aware TM leverage tool.
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
	t.ToolDescription = "Content-aware TM leverage with generalized, structural, and plain matching"
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

	// Use the full Block for content-aware matching (entity annotations, spans).
	matches, err := t.tm.Lookup(block, t.cfg.SourceLocale, t.cfg.TargetLocale, LookupOptions{
		MinScore:   t.cfg.MinScore,
		MaxResults: t.cfg.MaxResults,
	})
	if err != nil {
		return part, nil // Continue processing even if TM lookup fails.
	}

	if len(matches) == 0 {
		return part, nil
	}

	best := matches[0]

	sourceVariant := best.Entry.Variant(t.cfg.SourceLocale)
	targetVariant := best.Entry.Variant(t.cfg.TargetLocale)
	if targetVariant == nil {
		return part, nil
	}

	// For exact matches (any tier), apply the target directly.
	if best.MatchType.IsExact() {
		adapted := applyEntityAdaptations(targetVariant, best.EntityAdaptations)
		block.SetTargetFragment(t.cfg.TargetLocale, adapted)
	}

	// Add the best match as an AltTranslation annotation.
	if block.Annotations == nil {
		block.Annotations = make(map[string]model.Annotation)
	}
	block.Annotations["alt-translation"] = &model.AltTranslation{
		Source:    sourceVariant,
		Target:    targetVariant,
		Locale:    t.cfg.TargetLocale,
		Origin:    "tm:sievepen",
		Score:     best.Score,
		MatchType: model.MatchType(best.MatchType),
	}

	return part, nil
}

// applyEntityAdaptations substitutes entity values in a target Fragment
// based on the adaptations computed during matching. Returns a new Fragment
// with adapted values. Preserves inline spans by operating on CodedText
// directly (replacing text segments while keeping marker runes intact).
func applyEntityAdaptations(target *model.Fragment, adaptations []EntityAdaptation) *model.Fragment {
	if target == nil || len(adaptations) == 0 {
		return target
	}

	// Clone to avoid mutating the stored TM entry.
	result := target.Clone()

	// Apply adaptations to entity span Data values (for placeholder entities)
	// and to the plain text segments of CodedText.
	for _, adapt := range adaptations {
		// Update span Data for entity placeholder spans.
		for _, s := range result.Spans {
			if model.IsEntityTypeString(s.Type) && s.Data == adapt.StoredValue {
				s.Data = adapt.CurrentValue
			}
		}

		// Update text segments in CodedText (between markers).
		result.CodedText = strings.Replace(result.CodedText, adapt.StoredValue, adapt.CurrentValue, 1)
	}

	return result
}
