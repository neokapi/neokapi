package memory_test

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #1839. One source text is taught one target locale at a time — materialize
// runs per locale, and every locale keys the same entry off the source content
// hash — so a write that replaced the whole variant set left the entry holding
// only the locale written last. Nothing said so: the file for the first locale
// was on disk and the run reported success, while the memory that was supposed
// to recycle it had lost the pair.

// bilingual builds the entry one materialize pass teaches: the source and the
// one target locale it wrote.
func bilingual(id, source, target string, targetLocale model.LocaleID) memory.Entry {
	return memory.Entry{
		ID: id,
		Variants: map[model.LocaleID][]model.Run{
			"en":         {{Text: &model.TextRun{Text: source}}},
			targetLocale: {{Text: &model.TextRun{Text: target}}},
		},
		HintSrcLang: "en",
	}
}

// memoryStores are the write paths an entry can reach the content memory
// through. All three carry the same per-locale rule, so a project keeps its
// variants whether it converges locally, imports in bulk, or runs seeded.
func memoryStores(t *testing.T) map[string]func(context.Context, memory.Entry) memory.Store {
	t.Helper()
	newSQLite := func() *memory.SQLiteStore {
		tm, err := memory.NewSQLiteStore(":memory:")
		require.NoError(t, err)
		t.Cleanup(func() { _ = tm.Close() })
		return tm
	}
	return map[string]func(context.Context, memory.Entry) memory.Store{
		"sqlite Add": func(ctx context.Context, first memory.Entry) memory.Store {
			tm := newSQLite()
			require.NoError(t, tm.Add(ctx, first))
			return tm
		},
		"sqlite BulkAdd": func(ctx context.Context, first memory.Entry) memory.Store {
			tm := newSQLite()
			require.NoError(t, tm.BulkAddWithStream(ctx, []memory.Entry{first}, ""))
			return tm
		},
		"in-memory": func(ctx context.Context, first memory.Entry) memory.Store {
			tm := memory.NewInMemoryStore()
			require.NoError(t, tm.Add(ctx, first))
			return tm
		},
	}
}

// TestMemory_SecondLocaleKeepsTheFirstLocalesVariant is the core regression:
// teaching one entry a second target locale must leave the first one standing,
// in either order.
func TestMemory_SecondLocaleKeepsTheFirstLocalesVariant(t *testing.T) {
	ctx := context.Background()

	orders := []struct {
		name          string
		first, second model.LocaleID
		firstText     string
		secondText    string
	}{
		{name: "nb then de", first: "nb", second: "de", firstText: "Hei verden", secondText: "Hallo Welt"},
		{name: "de then nb", first: "de", second: "nb", firstText: "Hallo Welt", secondText: "Hei verden"},
	}

	for path, open := range memoryStores(t) {
		for _, tc := range orders {
			t.Run(path+"/"+tc.name, func(t *testing.T) {
				tm := open(ctx, bilingual("merge:store:h1", "Hello world", tc.firstText, tc.first))
				require.NoError(t, tm.Add(ctx, bilingual("merge:store:h1", "Hello world", tc.secondText, tc.second)))

				got, ok, err := tm.GetEntry(ctx, "merge:store:h1")
				require.NoError(t, err)
				require.True(t, ok)
				assert.Equal(t, "Hello world", got.VariantText("en"))
				assert.Equal(t, tc.firstText, got.VariantText(tc.first),
					"materializing %s deleted the %s variant of the same entry", tc.second, tc.first)
				assert.Equal(t, tc.secondText, got.VariantText(tc.second))
			})
		}
	}
}

// TestMemory_WriteReplacesTheLocalesItCarries keeps the rule from becoming
// "never overwrite": a corrected target for a locale the write does carry must
// land, or a re-materialized fix would never reach the memory.
func TestMemory_WriteReplacesTheLocalesItCarries(t *testing.T) {
	ctx := context.Background()

	for path, open := range memoryStores(t) {
		t.Run(path, func(t *testing.T) {
			tm := open(ctx, bilingual("merge:store:h1", "Hello world", "Hei verda", "nb"))
			require.NoError(t, tm.Add(ctx, bilingual("merge:store:h1", "Hello world", "Hei verden", "nb")))

			got, ok, err := tm.GetEntry(ctx, "merge:store:h1")
			require.NoError(t, err)
			require.True(t, ok)
			assert.Equal(t, "Hei verden", got.VariantText("nb"))
			assert.Equal(t, []model.LocaleID{"en", "nb"}, got.Locales(),
				"replacing a locale must not leave its old row behind")
		})
	}
}

// TestMemory_EmptyVariantRetiresThatLocale covers how a locale is dropped now
// that omitting it means "I have nothing to say about it": the write names the
// locale with no runs.
func TestMemory_EmptyVariantRetiresThatLocale(t *testing.T) {
	ctx := context.Background()

	for path, open := range memoryStores(t) {
		t.Run(path, func(t *testing.T) {
			tm := open(ctx, memory.Entry{
				ID: "merge:store:h1",
				Variants: map[model.LocaleID][]model.Run{
					"en": {{Text: &model.TextRun{Text: "Hello world"}}},
					"nb": {{Text: &model.TextRun{Text: "Hei verden"}}},
					"de": {{Text: &model.TextRun{Text: "Hallo Welt"}}},
				},
				HintSrcLang: "en",
			})
			require.NoError(t, tm.Add(ctx, memory.Entry{
				ID: "merge:store:h1",
				Variants: map[model.LocaleID][]model.Run{
					"en": {{Text: &model.TextRun{Text: "Hello world"}}},
					"de": {},
				},
				HintSrcLang: "en",
			}))

			got, ok, err := tm.GetEntry(ctx, "merge:store:h1")
			require.NoError(t, err)
			require.True(t, ok)
			assert.Equal(t, []model.LocaleID{"en", "nb"}, got.Locales(),
				"a locale named with no runs is retired; one left unmentioned stands")
		})
	}
}
