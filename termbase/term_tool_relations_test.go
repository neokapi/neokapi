package termbase_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/termbase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rebrandTB seeds a termbase with a deprecated concept (c-old), its
// replacement (c-new), and — when withRelation is set — the USE_INSTEAD
// relation between them.
func rebrandTB(t *testing.T, withRelation bool) termbase.TermBase {
	t.Helper()
	tb := termbase.NewInMemoryTermBase()
	require.NoError(t, tb.AddConcept(context.Background(), termbase.Concept{
		ID: "c-old", Domain: "software",
		Terms: []termbase.Term{
			{Text: "Webinar", Locale: model.LocaleEnglish, Status: model.TermDeprecated},
			{Text: "Webinaire", Locale: model.LocaleFrench, Status: model.TermPreferred},
		},
	}))
	require.NoError(t, tb.AddConcept(context.Background(), termbase.Concept{
		ID: "c-new", Domain: "software",
		Terms: []termbase.Term{
			{Text: "Live session", Locale: model.LocaleEnglish, Status: model.TermPreferred},
			{Text: "Session en direct", Locale: model.LocaleFrench, Status: model.TermPreferred},
		},
	}))
	if withRelation {
		require.NoError(t, tb.AddRelation(context.Background(), termbase.ConceptRelation{
			ID: "r-use", SourceID: "c-old", TargetID: "c-new",
			RelationType: graph.LabelUseInstead,
		}))
	}
	return tb
}

// termAnnotations collects the TermAnnotation spans whose ID carries the given
// prefix ("term:" for discovery, "term-violation:" for violations).
func termAnnotations(b *model.Block, prefix string) []*model.TermAnnotation {
	var out []*model.TermAnnotation
	if f := b.OverlayOf(model.OverlayTerm); f != nil {
		for _, span := range f.Spans {
			if !strings.HasPrefix(span.ID, prefix) {
				continue
			}
			if ta, ok := span.Value.(*model.TermAnnotation); ok {
				out = append(out, ta)
			}
		}
	}
	return out
}

// --- Scoped lookup ---

func TestTermLookupTool_MarketScope(t *testing.T) {
	tb := termbase.NewInMemoryTermBase()
	require.NoError(t, tb.AddConcept(context.Background(), termbase.Concept{
		ID: "c-hello", Domain: "software",
		Terms: []termbase.Term{
			{Text: "Hello", Locale: model.LocaleEnglish, Status: model.TermPreferred,
				Validity: &graph.Validity{Tags: map[string]string{"market": "us"}}},
			{Text: "Bonjour", Locale: model.LocaleFrench, Status: model.TermPreferred},
		},
	}))

	tests := []struct {
		name string
		tags map[string]string
		want string // expected term-count property ("" = no annotations)
	}{
		{"matching market", map[string]string{"market": "us"}, "1"},
		{"other market", map[string]string{"market": "dach"}, ""},
		{"no scope", nil, "1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tl := termbase.NewTermLookupTool(tb, termbase.TermLookupConfig{
				SourceLocale: model.LocaleEnglish,
				TargetLocale: model.LocaleFrench,
				Tags:         tt.tags,
			})
			result := processBlock(t, tl, model.NewBlock("b1", "Hello there"))
			assert.Equal(t, tt.want, result.Properties["term-count"])
		})
	}
}

func TestTermLookupTool_AsOfScope(t *testing.T) {
	retired := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tb := termbase.NewInMemoryTermBase()
	require.NoError(t, tb.AddConcept(context.Background(), termbase.Concept{
		ID: "c-howdy", Domain: "software",
		Terms: []termbase.Term{
			{Text: "Howdy", Locale: model.LocaleEnglish, Status: model.TermApproved,
				Validity: &graph.Validity{ValidTo: &retired}},
		},
	}))

	// Before retirement the term matches; after, it does not.
	tl := termbase.NewTermLookupTool(tb, termbase.TermLookupConfig{
		SourceLocale: model.LocaleEnglish,
		AsOf:         "2025-12-01T00:00:00Z",
	})
	result := processBlock(t, tl, model.NewBlock("b1", "Howdy partner"))
	assert.Equal(t, "1", result.Properties["term-count"])

	tl = termbase.NewTermLookupTool(tb, termbase.TermLookupConfig{
		SourceLocale: model.LocaleEnglish,
		AsOf:         "2026-02-01T00:00:00Z",
	})
	result = processBlock(t, tl, model.NewBlock("b1", "Howdy partner"))
	assert.Empty(t, result.Properties["term-count"])
}

func TestTermEnforceTool_MarketScope(t *testing.T) {
	tb := termbase.NewInMemoryTermBase()
	require.NoError(t, tb.AddConcept(context.Background(), termbase.Concept{
		ID: "c-save", Domain: "software",
		Terms: []termbase.Term{
			{Text: "Save", Locale: model.LocaleEnglish, Status: model.TermPreferred,
				Validity: &graph.Validity{Tags: map[string]string{"market": "dach"}}},
			{Text: "Sauvegarder", Locale: model.LocaleFrench, Status: model.TermPreferred},
		},
	}))

	// In the us market the dach-scoped term is out of scope: nothing enforced.
	te := termbase.NewTermEnforceTool(tb, termbase.TermEnforceConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
		Tags:         map[string]string{"market": "us"},
	})
	block := model.NewBlock("b1", "Click Save")
	block.SetTargetText(model.LocaleFrench, "Cliquez sur Enregistrer")
	result := processBlock(t, te, block)
	assert.Empty(t, result.Properties["term-enforce-passed"])

	// In dach the same translation is a violation.
	te = termbase.NewTermEnforceTool(tb, termbase.TermEnforceConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
		Tags:         map[string]string{"market": "dach"},
	})
	block = model.NewBlock("b1", "Click Save")
	block.SetTargetText(model.LocaleFrench, "Cliquez sur Enregistrer")
	result = processBlock(t, te, block)
	assert.Equal(t, "false", result.Properties["term-enforce-passed"])
	assert.Equal(t, "1", result.Properties["term-enforce-violations"])
}

func TestTermToolConfigs_AsOfValidation(t *testing.T) {
	lookup := &termbase.TermLookupConfig{SourceLocale: model.LocaleEnglish, AsOf: "yesterday"}
	require.Error(t, lookup.Validate())
	lookup.AsOf = "2026-06-01T00:00:00Z"
	require.NoError(t, lookup.Validate())

	enforce := &termbase.TermEnforceConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
		AsOf:         "yesterday",
	}
	require.Error(t, enforce.Validate())
	enforce.AsOf = ""
	enforce.Tags = map[string]string{"market": "dach"}
	require.NoError(t, enforce.Validate())
}

// --- USE_INSTEAD / REPLACED_BY suggestion resolution ---

func TestTermLookupTool_UseInsteadSuggestion(t *testing.T) {
	tb := rebrandTB(t, true)

	tl := termbase.NewTermLookupTool(tb, termbase.TermLookupConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})
	result := processBlock(t, tl, model.NewBlock("b1", "Join our Webinar"))

	annos := termAnnotations(result, "term:")
	require.Len(t, annos, 1)
	assert.Equal(t, "Webinar", annos[0].SourceTerm)
	assert.Equal(t, "c-old", annos[0].ConceptID)
	assert.Equal(t, model.TermDeprecated, annos[0].Status)
	// The suggestion names the replacement concept's preferred term, not the
	// deprecated concept's own translation.
	require.Len(t, annos[0].TargetTerms, 1)
	assert.Equal(t, "Session en direct", annos[0].TargetTerms[0].Text)
	assert.Equal(t, model.TermPreferred, annos[0].TargetTerms[0].Status)
}

func TestTermLookupTool_NoRelationFallsBack(t *testing.T) {
	tb := rebrandTB(t, false)

	tl := termbase.NewTermLookupTool(tb, termbase.TermLookupConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})
	result := processBlock(t, tl, model.NewBlock("b1", "Join our Webinar"))

	annos := termAnnotations(result, "term:")
	require.Len(t, annos, 1)
	// Without a relation, the same concept's own translations are suggested.
	require.Len(t, annos[0].TargetTerms, 1)
	assert.Equal(t, "Webinaire", annos[0].TargetTerms[0].Text)
}

func TestTermLookupTool_ReplacedByForbiddenTerm(t *testing.T) {
	tb := termbase.NewInMemoryTermBase()
	require.NoError(t, tb.AddConcept(context.Background(), termbase.Concept{
		ID: "c-comp", Domain: "brand",
		Terms: []termbase.Term{
			{Text: "QuickBoard", Locale: model.LocaleEnglish, Status: model.TermForbidden, CompetitorTerm: true},
			{Text: "QuickBoard", Locale: model.LocaleFrench, Status: model.TermForbidden},
		},
	}))
	require.NoError(t, tb.AddConcept(context.Background(), termbase.Concept{
		ID: "c-ours", Domain: "brand",
		Terms: []termbase.Term{
			{Text: "FlowBoard", Locale: model.LocaleEnglish, Status: model.TermPreferred},
			{Text: "FlowBoard", Locale: model.LocaleFrench, Status: model.TermPreferred},
		},
	}))
	require.NoError(t, tb.AddRelation(context.Background(), termbase.ConceptRelation{
		ID: "r-rep", SourceID: "c-comp", TargetID: "c-ours",
		RelationType: graph.LabelReplacedBy,
	}))

	tl := termbase.NewTermLookupTool(tb, termbase.TermLookupConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})
	result := processBlock(t, tl, model.NewBlock("b1", "Better than QuickBoard"))

	annos := termAnnotations(result, "term:")
	require.Len(t, annos, 1)
	assert.Equal(t, model.TermForbidden, annos[0].Status)
	require.Len(t, annos[0].TargetTerms, 1)
	assert.Equal(t, "FlowBoard", annos[0].TargetTerms[0].Text)
}

func TestTermLookupTool_UseInsteadWinsOverReplacedBy(t *testing.T) {
	tb := rebrandTB(t, false)
	require.NoError(t, tb.AddConcept(context.Background(), termbase.Concept{
		ID: "c-other", Domain: "software",
		Terms: []termbase.Term{
			{Text: "Atelier", Locale: model.LocaleFrench, Status: model.TermPreferred},
		},
	}))
	// The REPLACED_BY relation sorts first by ID; USE_INSTEAD must still win.
	require.NoError(t, tb.AddRelation(context.Background(), termbase.ConceptRelation{
		ID: "r-a", SourceID: "c-old", TargetID: "c-other",
		RelationType: graph.LabelReplacedBy,
	}))
	require.NoError(t, tb.AddRelation(context.Background(), termbase.ConceptRelation{
		ID: "r-b", SourceID: "c-old", TargetID: "c-new",
		RelationType: graph.LabelUseInstead,
	}))

	tl := termbase.NewTermLookupTool(tb, termbase.TermLookupConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})
	result := processBlock(t, tl, model.NewBlock("b1", "Join our Webinar"))

	annos := termAnnotations(result, "term:")
	require.Len(t, annos, 1)
	require.Len(t, annos[0].TargetTerms, 1)
	assert.Equal(t, "Session en direct", annos[0].TargetTerms[0].Text)
}

func TestTermLookupTool_ReplacementWithoutTargetLocaleFallsBack(t *testing.T) {
	tb := rebrandTB(t, false)
	require.NoError(t, tb.AddConcept(context.Background(), termbase.Concept{
		ID: "c-en-only", Domain: "software",
		Terms: []termbase.Term{
			{Text: "Live session", Locale: model.LocaleEnglish, Status: model.TermPreferred},
		},
	}))
	require.NoError(t, tb.AddRelation(context.Background(), termbase.ConceptRelation{
		ID: "r-use", SourceID: "c-old", TargetID: "c-en-only",
		RelationType: graph.LabelUseInstead,
	}))

	tl := termbase.NewTermLookupTool(tb, termbase.TermLookupConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})
	result := processBlock(t, tl, model.NewBlock("b1", "Join our Webinar"))

	annos := termAnnotations(result, "term:")
	require.Len(t, annos, 1)
	// The replacement has no French term, so the suggestion falls back to the
	// matched concept's own translation.
	require.Len(t, annos[0].TargetTerms, 1)
	assert.Equal(t, "Webinaire", annos[0].TargetTerms[0].Text)
}

func TestTermEnforceTool_UseInsteadViolation(t *testing.T) {
	tb := rebrandTB(t, true)

	te := termbase.NewTermEnforceTool(tb, termbase.TermEnforceConfig{
		SourceLocale:  model.LocaleEnglish,
		TargetLocale:  model.LocaleFrench,
		CheckStatuses: []model.TermStatus{model.TermPreferred, model.TermDeprecated},
	})

	// Translating with the deprecated concept's own term is now a violation:
	// the replacement's preferred term is expected instead.
	block := model.NewBlock("b1", "Join our Webinar")
	block.SetTargetText(model.LocaleFrench, "Rejoignez notre Webinaire")
	result := processBlock(t, te, block)

	assert.Equal(t, "false", result.Properties["term-enforce-passed"])
	assert.Equal(t, "1", result.Properties["term-enforce-violations"])
	errs := result.Properties["term-enforce-errors"]
	assert.Contains(t, errs, "Webinar")
	assert.Contains(t, errs, "deprecated")
	assert.Contains(t, errs, "Session en direct")
	assert.Contains(t, errs, "c-new")

	violations := termAnnotations(result, "term-violation:")
	require.Len(t, violations, 1)
	assert.Equal(t, "c-old", violations[0].ConceptID)
	require.Len(t, violations[0].TargetTerms, 1)
	assert.Equal(t, "Session en direct", violations[0].TargetTerms[0].Text)
}

func TestTermEnforceTool_ReplacementUsedPasses(t *testing.T) {
	tb := rebrandTB(t, true)

	te := termbase.NewTermEnforceTool(tb, termbase.TermEnforceConfig{
		SourceLocale:  model.LocaleEnglish,
		TargetLocale:  model.LocaleFrench,
		CheckStatuses: []model.TermStatus{model.TermPreferred, model.TermDeprecated},
	})

	block := model.NewBlock("b1", "Join our Webinar")
	block.SetTargetText(model.LocaleFrench, "Rejoignez notre Session en direct")
	result := processBlock(t, te, block)

	assert.Equal(t, "true", result.Properties["term-enforce-passed"])
	assert.Equal(t, "0", result.Properties["term-enforce-violations"])
}

func TestTermEnforceTool_NoRelationFallsBack(t *testing.T) {
	tb := rebrandTB(t, false)

	te := termbase.NewTermEnforceTool(tb, termbase.TermEnforceConfig{
		SourceLocale:  model.LocaleEnglish,
		TargetLocale:  model.LocaleFrench,
		CheckStatuses: []model.TermStatus{model.TermPreferred, model.TermDeprecated},
	})

	// Without a relation, the existing behavior holds: the concept's own
	// preferred translation satisfies enforcement…
	block := model.NewBlock("b1", "Join our Webinar")
	block.SetTargetText(model.LocaleFrench, "Rejoignez notre Webinaire")
	result := processBlock(t, te, block)
	assert.Equal(t, "true", result.Properties["term-enforce-passed"])

	// …and its absence is reported against the same concept.
	block = model.NewBlock("b1", "Join our Webinar")
	block.SetTargetText(model.LocaleFrench, "Rejoignez notre atelier")
	result = processBlock(t, te, block)
	assert.Equal(t, "false", result.Properties["term-enforce-passed"])
	assert.Contains(t, result.Properties["term-enforce-errors"], "Webinaire")
	assert.NotContains(t, result.Properties["term-enforce-errors"], "c-new")
}

func TestTermEnforceTool_ScopedRelation(t *testing.T) {
	tb := rebrandTB(t, false)
	// The rebrand only holds in the dach market.
	require.NoError(t, tb.AddRelation(context.Background(), termbase.ConceptRelation{
		ID: "r-dach", SourceID: "c-old", TargetID: "c-new",
		RelationType: graph.LabelUseInstead,
		Validity:     &graph.Validity{Tags: map[string]string{"market": "dach"}},
	}))

	newEnforce := func(tags map[string]string) *termbase.TermEnforceTool {
		return termbase.NewTermEnforceTool(tb, termbase.TermEnforceConfig{
			SourceLocale:  model.LocaleEnglish,
			TargetLocale:  model.LocaleFrench,
			CheckStatuses: []model.TermStatus{model.TermPreferred, model.TermDeprecated},
			Tags:          tags,
		})
	}

	// Outside dach the relation is out of scope: the old translation passes.
	block := model.NewBlock("b1", "Join our Webinar")
	block.SetTargetText(model.LocaleFrench, "Rejoignez notre Webinaire")
	result := processBlock(t, newEnforce(map[string]string{"market": "us"}), block)
	assert.Equal(t, "true", result.Properties["term-enforce-passed"])

	// In dach the relation applies: the replacement is required.
	block = model.NewBlock("b1", "Join our Webinar")
	block.SetTargetText(model.LocaleFrench, "Rejoignez notre Webinaire")
	result = processBlock(t, newEnforce(map[string]string{"market": "dach"}), block)
	assert.Equal(t, "false", result.Properties["term-enforce-passed"])
	assert.Contains(t, result.Properties["term-enforce-errors"], "Session en direct")
}
