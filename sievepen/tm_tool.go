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
	// Translate: applies TM matches as targets (exact tiers) and an
	// alt-translation annotation; source stays read-only.
	t.Translate = t.translate
	return t
}

func (t *TMLeverageTool) translate(v tool.TargetView) error {
	if !v.Translatable() || v.SourceText() == "" {
		return nil
	}

	// Content-aware matching needs the source runs and entity annotations.
	// Lookup reads only those (read-only), so pass a snapshot projected from
	// the view rather than the live block — no source/target write escapes.
	snapshot := &model.Block{
		Source:       v.SourceRuns(),
		Annotations:  v.Annotations(),
		Translatable: true,
	}
	matches, err := t.tm.Lookup(v.Context(), snapshot, t.cfg.SourceLocale, t.cfg.TargetLocale, LookupOptions{
		MinScore:   t.cfg.MinScore,
		MaxResults: t.cfg.MaxResults,
	})
	if err != nil {
		return nil // Continue processing even if TM lookup fails.
	}

	if len(matches) == 0 {
		return nil
	}

	best := matches[0]

	sourceVariant := best.Entry.Variant(t.cfg.SourceLocale)
	targetVariant := best.Entry.Variant(t.cfg.TargetLocale)
	if len(targetVariant) == 0 {
		return nil
	}

	// For exact matches (any tier), apply the target directly.
	if best.MatchType.IsExact() {
		adapted := applyEntityAdaptations(targetVariant, best.EntityAdaptations)
		v.SetTargetRuns(t.cfg.TargetLocale, adapted)
	}

	// Add the best match as an AltTranslation annotation.
	v.Annotate("alt-translation", &model.AltTranslation{
		Source:    sourceVariant,
		Target:    targetVariant,
		Locale:    t.cfg.TargetLocale,
		Origin:    "tm:sievepen",
		Score:     best.Score,
		MatchType: model.MatchType(best.MatchType),
	})

	return nil
}

// applyEntityAdaptations substitutes entity values in a target Run
// sequence based on the adaptations computed during matching. Returns
// a new Run sequence with adapted values; source runs are not mutated.
// Text is substituted inside TextRun only; Ph/PcOpen/PcClose payloads
// (Data, DisplayText, etc.) are passed through unchanged.
func applyEntityAdaptations(target []model.Run, adaptations []EntityAdaptation) []model.Run {
	if len(target) == 0 || len(adaptations) == 0 {
		return target
	}

	out := make([]model.Run, len(target))
	copy(out, target)
	for _, adapt := range adaptations {
		for i := range out {
			if out[i].Text == nil {
				continue
			}
			if strings.Contains(out[i].Text.Text, adapt.StoredValue) {
				newRun := *out[i].Text
				newRun.Text = strings.Replace(newRun.Text, adapt.StoredValue, adapt.CurrentValue, 1)
				out[i] = model.Run{Text: &newRun}
				break
			}
		}
	}
	return out
}
