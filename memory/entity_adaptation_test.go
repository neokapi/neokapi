package memory_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
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
