package memory_test

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corememory "github.com/neokapi/neokapi/core/memory"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/memory/leverage"
)

// entityRuns is "<before><entity><after>", the entity a placeholder run of
// its type carrying its text.
func entityRuns(before, entity, after string) []model.Run {
	return []model.Run{
		{Text: &model.TextRun{Text: before}},
		{Ph: &model.PlaceholderRun{ID: "e1", Type: string(model.EntityOrganization), Data: entity}},
		{Text: &model.TextRun{Text: after}},
	}
}

// locateEntity records the entity at run i of the block's source the way every
// producer does: as a span of the entity overlay over that run.
func locateEntity(b *model.Block, id string, run int, text string) {
	b.AddOverlaySpan(model.OverlayEntity, model.Span{
		ID:    id,
		Range: model.SpanAnchor(model.RunPos{Run: run}, model.RunPos{Run: run + 1}),
		Value: &model.EntityAnnotation{Text: text, Type: model.EntityOrganization},
	})
}

// The entities a lookup pairs with a stored entry's are the ones the block's
// entity overlay locates, in the order they sit in the text. Reading the
// block's annotations map found none, since every producer records an entity
// as an overlay span.
func TestExtractEntityAnnotationsReadsTheEntityOverlay(t *testing.T) {
	t.Parallel()
	b := &model.Block{ID: "b"}
	b.SetSourceRuns([]model.Run{
		{Ph: &model.PlaceholderRun{ID: "e1", Type: string(model.EntityOrganization), Data: "Acme"}},
		{Text: &model.TextRun{Text: " and "}},
		{Ph: &model.PlaceholderRun{ID: "e2", Type: string(model.EntityOrganization), Data: "Globex"}},
	})
	locateEntity(b, "entity:1", 2, "Globex")
	locateEntity(b, "entity:0", 0, "Acme")

	got := memory.ExtractEntityAnnotations(b)
	require.Len(t, got, 2)
	assert.Equal(t, "Acme", got[0].Text, "in text order, whatever order the spans were added in")
	assert.Equal(t, "Globex", got[1].Text)
	assert.Nil(t, memory.ExtractEntityAnnotations(&model.Block{ID: "none"}))
}

// A generalized match adapts the stored entry's entity to the one the lookup
// block carries, on every framework store.
func TestLookupAdaptsTheEntityTheBlockLocates(t *testing.T) {
	t.Parallel()
	stores := map[string]func(t *testing.T) memory.ContentMemory{
		"in memory": func(*testing.T) memory.ContentMemory { return memory.NewInMemoryStore() },
		"sqlite": func(t *testing.T) memory.ContentMemory {
			s, err := memory.NewSQLiteStore(filepath.Join(t.TempDir(), "memory.db"))
			require.NoError(t, err)
			t.Cleanup(func() { _ = s.Close() })
			return s
		},
	}
	for name, open := range stores {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tm := open(t)
			require.NoError(t, tm.Add(t.Context(), memory.Entry{
				ID: "acme",
				Variants: map[model.LocaleID][]model.Run{
					model.LocaleEnglish: entityRuns("Contact ", "Acme", " for support"),
					model.LocaleFrench:  entityRuns("Contactez ", "Acme", " pour le support"),
				},
				HintSrcLang: model.LocaleEnglish,
				Entities: []memory.EntityMapping{{
					PlaceholderID: "e1",
					Type:          model.EntityOrganization,
					Values: map[model.LocaleID]memory.EntityValue{
						model.LocaleEnglish: {Text: "Acme"},
						model.LocaleFrench:  {Text: "Acme"},
					},
				}},
			}))

			b := &model.Block{ID: "lookup", Translatable: true}
			b.SetSourceRuns(entityRuns("Contact ", "Globex", " for support"))
			locateEntity(b, "entity:0", 1, "Globex")

			matches, err := tm.Lookup(t.Context(), b, model.LocaleEnglish, model.LocaleFrench, memory.LookupOptions{MinScore: 0.5})
			require.NoError(t, err)
			require.NotEmpty(t, matches)
			m := matches[0]
			assert.Equal(t, memory.MatchGeneralizedExact, m.MatchType)
			require.Len(t, m.EntityAdaptations, 1, "the stored entity is adapted to the one the block carries")
			a := m.EntityAdaptations[0]
			assert.Equal(t, "e1", a.PlaceholderID)
			assert.Equal(t, "Acme", a.StoredValue)
			assert.Equal(t, "Globex", a.CurrentValue)
		})
	}
}

func org(id, text string) model.Run {
	return model.Run{Ph: &model.PlaceholderRun{ID: id, Type: string(model.EntityOrganization), Data: text}}
}

func txt(s string) model.Run { return model.Run{Text: &model.TextRun{Text: s}} }

// frameworkStores opens each framework store.
func frameworkStores(t *testing.T) map[string]memory.ContentMemory {
	s, err := memory.NewSQLiteStore(filepath.Join(t.TempDir(), "memory.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return map[string]memory.ContentMemory{"in memory": memory.NewInMemoryStore(), "sqlite": s}
}

// orgMapping maps a placeholder to one organization in English and French.
func orgMapping(id, en, fr string) memory.EntityMapping {
	return memory.EntityMapping{PlaceholderID: id, Type: model.EntityOrganization,
		Values: map[model.LocaleID]memory.EntityValue{model.LocaleEnglish: {Text: en}, model.LocaleFrench: {Text: fr}}}
}

// lookupOrgs is a lookup block of runs whose organization placeholders the
// entity overlay locates.
func lookupOrgs(runs ...model.Run) *model.Block {
	b := &model.Block{ID: "lookup", Translatable: true}
	b.SetSourceRuns(runs)
	for i, r := range runs {
		if r.Ph != nil {
			locateEntity(b, "entity:"+r.Ph.ID, i, r.Ph.Data)
		}
	}
	return b
}

// Leverage adapts the entity where the target holds it, its placeholder or
// the one whole word that spells it, and never text that only contains the
// stored value: a town named after the company keeps its name.
func TestLeverageAdaptsTheEntityAndNothingElse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		fr     []model.Run
		lookup []model.Run
		want   []model.Run
	}{
		{
			name:   "the placeholder",
			fr:     []model.Run{txt("Contactez "), org("e1", "Acme"), txt(" à Acmeville.")},
			lookup: []model.Run{txt("Contact "), org("e1", "Globex"), txt(" in Acmeville.")},
			want:   []model.Run{txt("Contactez "), org("e1", "Globex"), txt(" à Acmeville.")},
		},
		{
			name:   "the whole word",
			fr:     []model.Run{txt("Contactez Acme à Acmeville.")},
			lookup: []model.Run{txt("Contact "), org("e1", "Globex"), txt(" in Acmeville.")},
			want:   []model.Run{txt("Contactez Globex à Acmeville.")},
		},
		{
			name:   "a value the target spells twice is left as it is",
			fr:     []model.Run{txt("Contactez Acme, Acme répond.")},
			lookup: []model.Run{txt("Contact "), org("e1", "Globex"), txt(" in Acmeville.")},
			want:   []model.Run{txt("Contactez Acme, Acme répond.")},
		},
	}
	for name, tm := range frameworkStores(t) {
		for i, tc := range tests {
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				id := fmt.Sprintf("acme-%d", i)
				require.NoError(t, tm.Add(t.Context(), memory.Entry{
					ID: id,
					Variants: map[model.LocaleID][]model.Run{
						model.LocaleEnglish: {txt("Contact "), org("e1", "Acme"), txt(fmt.Sprintf(" in Acmeville %d.", i))},
						model.LocaleFrench:  tc.fr,
					},
					HintSrcLang: model.LocaleEnglish,
					Entities:    []memory.EntityMapping{orgMapping("e1", "Acme", "Acme")},
				}))
				lookup := slices.Clone(tc.lookup)
				lookup[2] = txt(fmt.Sprintf(" in Acmeville %d.", i))
				p := leverage.NewProvider(tm)
				got, ok := p.Lookup(t.Context(), corememory.Request{Block: lookupOrgs(lookup...), Source: model.LocaleEnglish, Target: model.LocaleFrench, MinScore: 50})
				require.True(t, ok)
				assert.Equal(t, string(model.CanonicalRunsJSON(tc.want)), string(model.CanonicalRunsJSON(got.TargetRuns)))
			})
		}
	}
}

// A store reports only the adaptations the target can take: one whose stored
// value the target neither holds as a placeholder nor spells once as a whole
// word is left out of the match.
func TestComputeEntityAdaptationsLeavesOutWhatTheTargetCannotLocate(t *testing.T) {
	t.Parallel()
	entry := memory.Entry{
		Variants: map[model.LocaleID][]model.Run{
			model.LocaleEnglish: {txt("Ask "), org("e1", "Acme"), txt(" or "), org("e2", "Initech"), txt(".")},
			model.LocaleFrench:  {txt("Demandez à Acmeville ou à Initech, Initech.")},
		},
		Entities: []memory.EntityMapping{orgMapping("e1", "Acme", "Acme"), orgMapping("e2", "Initech", "Initech")},
	}
	current := []*model.EntityAnnotation{
		{Text: "Globex", Type: model.EntityOrganization},
		{Text: "Hooli", Type: model.EntityOrganization},
	}
	assert.Empty(t, memory.ComputeEntityAdaptations(entry, model.LocaleEnglish, model.LocaleFrench, current),
		"Acme is part of a longer word, Initech is spelled twice")

	entry.Variants[model.LocaleFrench] = []model.Run{txt("Demandez à "), org("e1", "Acme"), txt(" ou à Initech.")}
	got := memory.ComputeEntityAdaptations(entry, model.LocaleEnglish, model.LocaleFrench, current)
	require.Len(t, got, 2)
	assert.Equal(t, "Globex", got[0].CurrentValue)
	assert.Equal(t, "Hooli", got[1].CurrentValue)
}

// Every adaptation is located in the target as given, so one substitution
// never feeds the next, and a value joined to letters is not a word.
func TestAdaptEntities(t *testing.T) {
	t.Parallel()
	swap := []memory.EntityAdaptation{
		{PlaceholderID: "e1", StoredValue: "Acme", CurrentValue: "Initech"},
		{PlaceholderID: "e2", StoredValue: "Initech", CurrentValue: "Globex"},
	}
	got := memory.AdaptEntities([]model.Run{txt("Acme et Initech.")}, swap)
	assert.Equal(t, "Initech et Globex.", model.FlattenRuns(got))

	joined := []memory.EntityAdaptation{{PlaceholderID: "e1", StoredValue: "東京", CurrentValue: "大阪"}}
	assert.Equal(t, "東京都に行く", model.FlattenRuns(memory.AdaptEntities([]model.Run{txt("東京都に行く")}, joined)))
	assert.Equal(t, "大阪 に行く", model.FlattenRuns(memory.AdaptEntities([]model.Run{txt("東京 に行く")}, joined)))

	// A placeholder keeps its id and type and takes the new value, in every
	// branch of a plural, with the input runs unchanged.
	plural := []model.Run{txt("From "), {Plural: &model.PluralRun{Pivot: "n", Forms: map[model.PluralForm][]model.Run{
		model.PluralOne:   {org("e1", "Acme"), txt(" item")},
		model.PluralOther: {org("e1", "Acme"), txt(" items")},
	}}}}
	out := memory.AdaptEntities(plural, []memory.EntityAdaptation{{PlaceholderID: "e1", StoredValue: "Acme", CurrentValue: "Globex"}})
	assert.Equal(t, "Globex", out[1].Plural.Forms[model.PluralOne][0].Ph.Data)
	assert.Equal(t, "Globex", out[1].Plural.Forms[model.PluralOther][0].Ph.Data)
	assert.Equal(t, "Acme", plural[1].Plural.Forms[model.PluralOne][0].Ph.Data, "the input is not mutated")
}

// A segment's lookup pairs the stored entry with the entities of that
// segment, not with the block's first.
func TestLookupSegmentAdaptsTheSegmentsEntity(t *testing.T) {
	t.Parallel()
	for name, tm := range frameworkStores(t) {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, tm.Add(t.Context(), memory.Entry{
				ID: "ask",
				Variants: map[model.LocaleID][]model.Run{
					model.LocaleEnglish: {txt("Ask "), org("e1", "Initech"), txt(" now.")},
					model.LocaleFrench:  {txt("Demandez à "), org("e1", "Initech"), txt(" maintenant.")},
				},
				HintSrcLang: model.LocaleEnglish,
				Entities:    []memory.EntityMapping{orgMapping("e1", "Initech", "Initech")},
			}))
			b := lookupOrgs(txt("Contact "), org("e1", "Acme"), txt(" today. "), txt("Ask "), org("e2", "Globex"), txt(" now."))
			b.SetSegmentation(nil, []model.Span{
				{ID: "s1", Range: model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 3})},
				{ID: "s2", Range: model.SpanAnchor(model.RunPos{Run: 3}, model.RunPos{Run: 6})},
			})
			matches, err := tm.LookupSegment(t.Context(), b, 1, model.LocaleEnglish, model.LocaleFrench, memory.LookupOptions{MinScore: 0.5})
			require.NoError(t, err)
			require.NotEmpty(t, matches)
			require.Len(t, matches[0].EntityAdaptations, 1)
			assert.Equal(t, "Globex", matches[0].EntityAdaptations[0].CurrentValue)
		})
	}
}

// Stored entities pair with the block's in the order they sit in the stored
// source, whatever order the store keeps them in: SQLite reads them back by
// id, where e10 sorts before e2.
func TestLookupPairsEntitiesInTextOrder(t *testing.T) {
	t.Parallel()
	for name, tm := range frameworkStores(t) {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, tm.Add(t.Context(), memory.Entry{
				ID: "pair",
				Variants: map[model.LocaleID][]model.Run{
					model.LocaleEnglish: {txt("Ask "), org("e2", "Acme"), txt(" or "), org("e10", "Initech"), txt(".")},
					model.LocaleFrench:  {txt("Demandez à "), org("e2", "Acme"), txt(" ou "), org("e10", "Initech"), txt(".")},
				},
				HintSrcLang: model.LocaleEnglish,
				Entities:    []memory.EntityMapping{orgMapping("e10", "Initech", "Initech"), orgMapping("e2", "Acme", "Acme")},
			}))
			b := lookupOrgs(txt("Ask "), org("e1", "Globex"), txt(" or "), org("e2", "Hooli"), txt("."))
			matches, err := tm.Lookup(t.Context(), b, model.LocaleEnglish, model.LocaleFrench, memory.LookupOptions{MinScore: 0.5})
			require.NoError(t, err)
			require.NotEmpty(t, matches)
			got := map[string]string{}
			for _, a := range matches[0].EntityAdaptations {
				got[a.StoredValue] = a.CurrentValue
			}
			assert.Equal(t, map[string]string{"Acme": "Globex", "Initech": "Hooli"}, got)
		})
	}
}

// An entity inside a plural sorts at the run that holds the plural, after an
// entity that comes before it.
func TestExtractEntityAnnotationsInsideAPlural(t *testing.T) {
	t.Parallel()
	b := &model.Block{ID: "p"}
	b.SetSourceRuns([]model.Run{
		txt("From "), org("e1", "Acme"), txt(": "),
		{Plural: &model.PluralRun{Forms: map[model.PluralForm][]model.Run{model.PluralOther: {org("e2", "Initech"), txt(" items")}}}},
	})
	in := model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 1})
	in.Path = model.RunPath{{Kind: model.StepIndex, Index: 3}, {Kind: model.StepPlural, PluralForm: model.PluralOther}}
	b.AddOverlaySpan(model.OverlayEntity, model.Span{ID: "inplural", Range: in,
		Value: &model.EntityAnnotation{Text: "Initech", Type: model.EntityOrganization}})
	locateEntity(b, "top", 1, "Acme")

	var texts []string
	for _, ea := range memory.ExtractEntityAnnotations(b) {
		texts = append(texts, ea.Text)
	}
	assert.Equal(t, []string{"Acme", "Initech"}, texts)
}

// The entities of a segment are the ones that start inside it, a segment
// boundary given at the end of a text run or at the start of the next.
func TestExtractSegmentEntityAnnotations(t *testing.T) {
	t.Parallel()
	b := lookupOrgs(txt("Contact "), org("e1", "Acme"), txt(" today. "), txt("Ask "), org("e2", "Globex"), txt(" now."))
	b.SetSegmentation(nil, []model.Span{
		{ID: "s1", Range: model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 2, Offset: 8})},
		{ID: "s2", Range: model.SpanAnchor(model.RunPos{Run: 3}, model.RunPos{Run: 6})},
	})
	texts := func(eas []*model.EntityAnnotation) []string {
		var out []string
		for _, ea := range eas {
			out = append(out, ea.Text)
		}
		return out
	}
	assert.Equal(t, []string{"Acme"}, texts(memory.ExtractSegmentEntityAnnotations(b, 0)))
	assert.Equal(t, []string{"Globex"}, texts(memory.ExtractSegmentEntityAnnotations(b, 1)))
	assert.Empty(t, memory.ExtractSegmentEntityAnnotations(b, 2))

	whole := lookupOrgs(txt("Ask "), org("e1", "Acme"))
	assert.Equal(t, []string{"Acme"}, texts(memory.ExtractSegmentEntityAnnotations(whole, 0)), "with no segmentation, segment 0 is the block")
	assert.Empty(t, memory.ExtractSegmentEntityAnnotations(whole, 1))
}
