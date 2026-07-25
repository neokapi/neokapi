package memory

import (
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// MemoryLeverageConfig holds configuration for the content-memory leverage tool.
type MemoryLeverageConfig struct {
	MinScore     float64
	MaxResults   int
	SourceLocale model.LocaleID
	TargetLocale model.LocaleID
}

// ToolName returns the name of the tool this config applies to.
func (c *MemoryLeverageConfig) ToolName() string { return "recycle" }

// Reset restores default values.
func (c *MemoryLeverageConfig) Reset() {
	c.MinScore = 0.7
	c.MaxResults = 5
}

// Validate checks configuration validity.
func (c *MemoryLeverageConfig) Validate() error { return nil }

// MemoryLeverageTool applies content-aware content memory matches to
// translatable blocks. When a content-memory match is found, it is attached as an
// AltTranslation annotation. For exact matches (including generalized-exact),
// entity adaptations are applied and the target is set directly.
type MemoryLeverageTool struct {
	tool.BaseTool
	tm  TranslationMemory
	cfg MemoryLeverageConfig
}

// NewMemoryLeverageTool creates a new content-aware content-memory leverage tool.
func NewMemoryLeverageTool(tm TranslationMemory, cfg MemoryLeverageConfig) *MemoryLeverageTool {
	if cfg.MinScore <= 0 {
		cfg.MinScore = 0.7
	}
	if cfg.MaxResults <= 0 {
		cfg.MaxResults = 5
	}

	t := &MemoryLeverageTool{
		tm:  tm,
		cfg: cfg,
	}
	t.ToolName = "recycle"
	t.ToolDescription = "Content-aware content-memory leverage with generalized, structural, and plain matching"
	// Translate: applies content-memory matches as targets (exact tiers) and an
	// alt-translation annotation; source stays read-only.
	t.Produce = t.translate
	return t
}

func (t *MemoryLeverageTool) translate(v tool.VariantView) error {
	if !v.Translatable() || v.SourceText() == "" {
		return nil
	}

	// Content-aware matching needs the source runs and entity annotations.
	// Lookup reads only those (read-only), so pass a snapshot projected from
	// the view rather than the live block — no source/target write escapes.
	snapshot := &model.Block{
		Source:       v.SourceRuns(),
		Translatable: true,
	}
	for k, val := range v.Annotations() {
		snapshot.SetAnno(k, val)
	}
	matches, err := t.tm.Lookup(v.Context(), snapshot, t.cfg.SourceLocale, t.cfg.TargetLocale, LookupOptions{
		MinScore:   t.cfg.MinScore,
		MaxResults: t.cfg.MaxResults,
	})
	if err != nil {
		return nil // Continue processing even if content-memory lookup fails.
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
	v.AddAltTranslation(&model.AltTranslation{
		Source:    sourceVariant,
		Target:    targetVariant,
		Locale:    t.cfg.TargetLocale,
		Origin:    "tm:memory",
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
