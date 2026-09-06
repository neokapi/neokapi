package memory

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The store normalizes the locale beside the text: an entry written under a
// POSIX or odd-cased spelling is the entry a canonical lookup finds, and the
// other way round, on every backend.

func localeStores(t *testing.T) map[string]Store {
	t.Helper()
	sq, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = sq.Close() })
	return map[string]Store{
		"inmemory": NewInMemoryStore(),
		"sqlite":   sq,
	}
}

func textRuns(s string) []model.Run { return []model.Run{{Text: &model.TextRun{Text: s}}} }

func TestStore_NormalizesLocaleOnWriteAndLookup(t *testing.T) {
	ctx := context.Background()
	for name, tm := range localeStores(t) {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, tm.Add(ctx, Entry{
				ID:          "e1",
				HintSrcLang: "en_US",
				Variants: map[model.LocaleID][]model.Run{
					"en_US": textRuns("Berth"),
					"nb_NO": textRuns("Kai"),
				},
				Entities: []EntityMapping{{
					PlaceholderID: "e1",
					Type:          model.EntityType("product"),
					Values: map[model.LocaleID]EntityValue{
						"en_US": {Text: "Berth"},
						"nb_NO": {Text: "Kai"},
					},
				}},
			}))

			// Stored canonical.
			got, ok, err := tm.GetEntry(ctx, "e1")
			require.NoError(t, err)
			require.True(t, ok)
			assert.Equal(t, model.LocaleID("en-US"), got.HintSrcLang)
			assert.Equal(t, []model.LocaleID{"en-US", "nb-NO"}, got.Locales())
			require.Len(t, got.Entities, 1)
			v, ok := got.Entities[0].Value("en_US")
			require.True(t, ok, "entity values are keyed canonically and looked up leniently")
			assert.Equal(t, "Berth", v.Text)

			// Looked up in canonical form, and in every other spelling.
			for _, pair := range [][2]model.LocaleID{
				{"en-US", "nb-NO"},
				{"en_US", "nb_NO"},
				{"EN-us", "NB_no"},
			} {
				matches, err := tm.LookupText(ctx, "Berth", pair[0], pair[1], LookupOptions{})
				require.NoError(t, err)
				require.Len(t, matches, 1, "lookup %s -> %s", pair[0], pair[1])
				assert.Equal(t, "Kai", matches[0].Entry.VariantText(pair[1]))
				assert.Equal(t, "Kai", matches[0].Entry.VariantText("nb-NO"))
			}

			// A write in the canonical spelling replaces the variant rather than
			// adding a second row for the same locale.
			require.NoError(t, tm.Add(ctx, Entry{
				ID:       "e1",
				Variants: map[model.LocaleID][]model.Run{"nb-NO": textRuns("Kaiplass")},
			}))
			got, _, err = tm.GetEntry(ctx, "e1")
			require.NoError(t, err)
			assert.Equal(t, []model.LocaleID{"en-US", "nb-NO"}, got.Locales())
			assert.Equal(t, "Kaiplass", got.VariantText("nb_NO"))

			// Search filters and facets ask in canonical form too.
			entries, total, err := tm.SearchEntries(ctx, SearchParams{AnyLocale: "nb_NO", Limit: 10})
			require.NoError(t, err)
			assert.Equal(t, 1, total)
			assert.Len(t, entries, 1)
			entries, total, err = tm.SearchEntries(ctx, SearchParams{RequireLocale: "EN-us", Limit: 10})
			require.NoError(t, err)
			assert.Equal(t, 1, total)
			assert.Len(t, entries, 1)

			stats, err := tm.LocaleStats(ctx)
			require.NoError(t, err)
			var locales []string
			for _, s := range stats {
				locales = append(locales, s.Locale)
			}
			assert.ElementsMatch(t, []string{"en-US", "nb-NO"}, locales)
		})
	}
}

func TestBulkAdd_NormalizesLocale(t *testing.T) {
	ctx := context.Background()
	sq, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer sq.Close()

	require.NoError(t, sq.BulkAddWithStream(ctx, []Entry{{
		ID: "b1",
		Variants: map[model.LocaleID][]model.Run{
			"en_US": textRuns("Departing"),
			"nb_NO": textRuns("Avgang"),
		},
	}}, ""))
	require.NoError(t, sq.RebuildSearchIndex(ctx))
	require.NoError(t, sq.RebuildFuzzyIndex(ctx))

	matches, err := sq.LookupText(ctx, "Departing", "en-US", "nb-NO", LookupOptions{})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Avgang", matches[0].Entry.VariantText("nb-NO"))

	full, err := sq.FullScoreEntries(ctx, textRuns("Departing"), "en_US")
	require.NoError(t, err)
	assert.Len(t, full, 1, "the ambiguity set is keyed canonically too")
}

func TestNormalizeEntryLocales(t *testing.T) {
	e := Entry{
		HintSrcLang: "EN",
		Variants: map[model.LocaleID][]model.Run{
			"en-US": textRuns("a"),
			"nb_NO": textRuns("b"),
		},
	}
	NormalizeEntryLocales(&e)
	assert.Equal(t, model.LocaleID("en"), e.HintSrcLang)
	assert.Equal(t, []model.LocaleID{"en-US", "nb-NO"}, e.Locales())

	// Already-canonical keys are returned as they are: no copy.
	clean := map[model.LocaleID][]model.Run{"fr": textRuns("c")}
	e2 := Entry{Variants: clean}
	NormalizeEntryLocales(&e2)
	clean["de"] = textRuns("d")
	assert.Len(t, e2.Variants, 2, "the canonical map is shared, not copied")

	NormalizeEntryLocales(nil)
}
