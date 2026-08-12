package blockstore_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	bwblockstore "github.com/neokapi/neokapi/bowrain/store/blockstore"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
)

// The corpus every case below searches. Each block is shaped to put one kind of
// pressure on the candidate query, and named for it.
const (
	docsCollection  = "docs"
	notesCollection = "notes"
)

// newSearchStore stands up a real Postgres-backed Bowrain content store, wires
// the blockstore adapter over it, and seeds the corpus.
func newSearchStore(t *testing.T) (blockstore.Store, string) {
	t.Helper()
	ctx := t.Context()

	db := pgtest.NewTestDB(t)
	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)

	projectID := "p-search"
	require.NoError(t, cs.CreateProject(ctx, &platstore.Project{
		ID:                    projectID,
		Name:                  "Search",
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"nb"},
	}))

	seedSearchCorpus(t, cs, projectID)

	bs, err := bwblockstore.New(bwblockstore.Options{
		ContentStore: cs,
		DB:           cs.SQLDB(),
		Dialect:      bwblockstore.PostgresDialect,
		ProjectID:    projectID,
		Stream:       "main",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = bs.Close() })
	return bs, projectID
}

func seedSearchCorpus(t *testing.T, cs *bstore.PostgresStore, projectID string) {
	t.Helper()
	ctx := t.Context()

	for _, name := range []string{docsCollection, notesCollection} {
		require.NoError(t, cs.CreateCollection(ctx, &platstore.Collection{
			ID: name, ProjectID: projectID, Name: name, Kind: "uploaded", Stream: "main",
		}))
		require.NoError(t, cs.StoreItem(ctx, projectID, "main", &platstore.Item{
			Name: name + ".md", CollectionID: name,
		}))
	}

	// A plain sentence with a target. The needle differs in case from both.
	greeting := model.NewRunsBlock("greeting", []model.Run{
		model.TextR("The Content Memory keeps decisions"),
	})
	greeting.Translatable = true
	greeting.SetTargetRuns("nb", []model.Run{model.TextR("Innholdsminnet tar vare på valgene")})

	// The needle lives in no single run: a per-run test cannot see it.
	spanning := model.NewRunsBlock("spanning", []model.Run{
		model.TextR("faithful "),
		model.PcOpenR(model.PcOpenRun{ID: "1", Data: "<b>"}),
		model.TextR("write-back"),
		model.PcCloseR(model.PcCloseRun{ID: "1", Data: "</b>"}),
	})
	spanning.Translatable = true

	// A placeholder contributes {equiv} to the flattened text and nothing to
	// any run's own text.
	placeholder := model.NewRunsBlock("placeholder", []model.Run{
		model.TextR("Hei "),
		model.PhR(model.PlaceholderRun{ID: "1", Equiv: "name"}),
	})
	placeholder.Translatable = true

	// A plural's branches sit below the flattening, so the block is a candidate
	// on structure rather than on text.
	plural := model.NewRunsBlock("plural", []model.Run{
		model.PluralR(model.PluralRun{
			Pivot: "count",
			Forms: map[model.PluralForm][]model.Run{
				model.PluralOne:   {model.TextR("one widget")},
				model.PluralOther: {model.TextR("many widgets")},
			},
		}),
	})
	plural.Translatable = true

	// A toned target is filed under the variant key "nb;tone=formal", while the
	// block reads it back under the bare locale.
	toned := model.NewRunsBlock("toned", []model.Run{model.TextR("Please sign in")})
	toned.Translatable = true
	toned.SetTargetVariant(
		model.VariantKey{Locale: "nb", Tone: "formal"},
		model.NewTarget([]model.Run{model.TextR("Vennligst logg inn")}, model.TargetStatusTranslated),
	)

	require.NoError(t, cs.StoreBlocksForItem(ctx, projectID, "main", docsCollection+".md",
		[]*model.Block{greeting, spanning, placeholder, plural, toned}))

	// The only block outside the docs collection.
	unrelated := model.NewRunsBlock("unrelated", []model.Run{
		model.TextR("Unrelated prose about decisions"),
	})
	unrelated.Translatable = true
	require.NoError(t, cs.StoreBlocksForItem(ctx, projectID, "main", notesCollection+".md",
		[]*model.Block{unrelated}))
}

// hitKey names a hit by the coordinates a caller reads it under.
type hitKey struct {
	Collection string
	Locale     string
	Text       string
}

func keysOf(hits []blockstore.TextHit) []hitKey {
	out := make([]hitKey, 0, len(hits))
	for _, h := range hits {
		out = append(out, hitKey{Collection: h.Collection, Locale: h.Locale, Text: h.Text})
	}
	return out
}

// textKey drops the collection, which is the one field the two search paths are
// allowed to disagree on: a scan reads blocks through the session API, which
// carries no collection, and reports only what the search was filtered to.
type textKey struct {
	Locale string
	Text   string
}

func textKeysOf(keys []hitKey) []textKey {
	out := make([]textKey, 0, len(keys))
	for _, k := range keys {
		out = append(out, textKey{Locale: k.Locale, Text: k.Text})
	}
	return out
}

// scanHits is the universal fallback, spelled out here so the searcher can be
// held against it. core/blockstore runs exactly this when a store has no index;
// on the same corpus and the same needle the two must find the same texts.
func scanHits(t *testing.T, store blockstore.Store, needle string, opts blockstore.TextSearchOptions) []textKey {
	t.Helper()
	sess, err := store.Begin(t.Context())
	require.NoError(t, err)
	defer sess.Close()

	lower := strings.ToLower(needle)
	var out []textKey
	for b, err := range sess.Blocks(blockstore.BlockFilter{Collection: opts.Collection}) {
		require.NoError(t, err)
		for _, bt := range blockstore.BlockTexts(b) {
			if opts.Locales != nil && !slices.Contains(opts.Locales, bt.Locale) {
				continue
			}
			if !strings.Contains(strings.ToLower(bt.Text), lower) {
				continue
			}
			out = append(out, textKey{Locale: bt.Locale, Text: bt.Text})
		}
	}
	return out
}

func search(t *testing.T, store blockstore.Store, needle string, opts blockstore.TextSearchOptions) []blockstore.TextHit {
	t.Helper()
	hits, err := blockstore.SearchText(t.Context(), store, needle, opts)
	require.NoError(t, err)
	return hits
}

// The Postgres store answers a text search from its own query, so a term
// occurrence lookup does not read the project's every block.
func TestSearchBlockText_ImplementsTextSearcher(t *testing.T) {
	bs, _ := newSearchStore(t)
	_, ok := bs.(blockstore.TextSearcher)
	assert.True(t, ok, "the Postgres store must be found by SearchText's probe")
}

func TestSearchBlockText_Matches(t *testing.T) {
	bs, _ := newSearchStore(t)

	cases := []struct {
		name   string
		needle string
		opts   blockstore.TextSearchOptions
		want   []hitKey
	}{
		{
			name:   "source text matches case-insensitively",
			needle: "content memory",
			want: []hitKey{
				{Collection: docsCollection, Text: "The Content Memory keeps decisions"},
			},
		},
		{
			name:   "a target matches under its own locale",
			needle: "INNHOLDSMINNET",
			want: []hitKey{
				{Collection: docsCollection, Locale: "nb", Text: "Innholdsminnet tar vare på valgene"},
			},
		},
		{
			name:   "a needle straddling an inline code is not missed",
			needle: "faithful write-back",
			want: []hitKey{
				{Collection: docsCollection, Text: "faithful write-back"},
			},
		},
		{
			name:   "a placeholder's equivalent is part of the text",
			needle: "{name}",
			want: []hitKey{
				{Collection: docsCollection, Text: "Hei {name}"},
			},
		},
		{
			name:   "a plural branch is not missed",
			needle: "many widgets",
			want: []hitKey{
				{Collection: docsCollection, Text: "many widgets"},
			},
		},
		{
			name:   "nil locales searches source and targets alike",
			needle: "valgene",
			want: []hitKey{
				{Collection: docsCollection, Locale: "nb", Text: "Innholdsminnet tar vare på valgene"},
			},
		},
		{
			name:   "the source locale is the empty locale key",
			needle: "decisions",
			opts:   blockstore.TextSearchOptions{Locales: []string{blockstore.SourceLocale}},
			want: []hitKey{
				{Collection: docsCollection, Text: "The Content Memory keeps decisions"},
				{Collection: notesCollection, Text: "Unrelated prose about decisions"},
			},
		},
		{
			name:   "a source-only match is never filed under a target locale",
			needle: "decisions",
			opts:   blockstore.TextSearchOptions{Locales: []string{"nb"}},
			want:   nil,
		},
		{
			name:   "one target locale keeps only that locale's text",
			needle: "vare",
			opts:   blockstore.TextSearchOptions{Locales: []string{"nb"}},
			want: []hitKey{
				{Collection: docsCollection, Locale: "nb", Text: "Innholdsminnet tar vare på valgene"},
			},
		},
		{
			name:   "a locale finds its toned variants too",
			needle: "vennligst",
			opts:   blockstore.TextSearchOptions{Locales: []string{"nb"}},
			want: []hitKey{
				{Collection: docsCollection, Locale: "nb", Text: "Vennligst logg inn"},
			},
		},
		{
			name:   "an empty non-nil locale set matches nothing",
			needle: "decisions",
			opts:   blockstore.TextSearchOptions{Locales: []string{}},
			want:   nil,
		},
		{
			name:   "the collection filter narrows to one collection",
			needle: "decisions",
			opts:   blockstore.TextSearchOptions{Collection: notesCollection},
			want: []hitKey{
				{Collection: notesCollection, Text: "Unrelated prose about decisions"},
			},
		},
		{
			name:   "a collection that holds no match yields nothing",
			needle: "Unrelated",
			opts:   blockstore.TextSearchOptions{Collection: docsCollection},
			want:   nil,
		},
		{
			name:   "a needle nobody wrote matches nothing",
			needle: "quicksilver",
			want:   nil,
		},
		{
			name:   "JSON punctuation is not text",
			needle: `{"text"`,
			want:   nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hits := search(t, bs, tc.needle, tc.opts)
			assert.ElementsMatch(t, tc.want, keysOf(hits))
			// Whatever the query answers, the scan must answer the same. It is
			// the definition the searcher is an optimization of.
			assert.ElementsMatch(t, textKeysOf(tc.want), scanHits(t, bs, tc.needle, tc.opts))
		})
	}
}

// A hit carries the block itself, so a caller can name and place the occurrence
// without a second round trip.
func TestSearchBlockText_HitCarriesTheBlock(t *testing.T) {
	bs, _ := newSearchStore(t)

	hits := search(t, bs, "content memory", blockstore.TextSearchOptions{})
	require.Len(t, hits, 1)
	h := hits[0]
	require.NotNil(t, h.Block)
	assert.NotEmpty(t, h.Hash, "the hit names the block by its content key")
	assert.Equal(t, h.Hash, h.Block.Hash)
	assert.Equal(t, "The Content Memory keeps decisions", model.FlattenRuns(h.Block.Source))
	assert.Equal(t, "Innholdsminnet tar vare på valgene", model.FlattenRuns(h.Block.Targets["nb"]))
}

func TestSearchBlockText_Limit(t *testing.T) {
	bs, _ := newSearchStore(t)

	uncapped := search(t, bs, "decisions", blockstore.TextSearchOptions{})
	require.Len(t, uncapped, 2)

	capped := search(t, bs, "decisions", blockstore.TextSearchOptions{Limit: 1})
	assert.Len(t, capped, 1)
	assert.Equal(t, uncapped[0].Hash, capped[0].Hash, "the cap keeps the first hits, in order")
}

// Every text in the corpus is found by a search for itself. This is the "never
// miss" half of the contract stated as a property: the searcher may over-return
// as much as it likes, but the block whose text holds the needle must come back.
func TestSearchBlockText_MissesNothing(t *testing.T) {
	bs, _ := newSearchStore(t)
	ctx := t.Context()

	sess, err := bs.Begin(ctx)
	require.NoError(t, err)
	defer sess.Close()

	var corpus []blockstore.BlockText
	for b, err := range sess.Blocks(blockstore.BlockFilter{}) {
		require.NoError(t, err)
		corpus = append(corpus, blockstore.BlockTexts(b)...)
	}
	require.NotEmpty(t, corpus)

	for _, bt := range corpus {
		for _, needle := range needlesOf(bt.Text) {
			hits := search(t, bs, needle, blockstore.TextSearchOptions{})
			found := slices.ContainsFunc(hits, func(h blockstore.TextHit) bool {
				return h.Locale == bt.Locale && h.Text == bt.Text
			})
			assert.True(t, found, "needle %q missed the text %q under locale %q", needle, bt.Text, bt.Locale)
		}
	}
}

// needlesOf returns substrings of a text that a search must find in it: the
// whole text, its halves, and a run of characters from the middle — the shapes
// that straddle run boundaries when the text was assembled from several runs.
func needlesOf(text string) []string {
	r := []rune(text)
	if len(r) < 4 {
		return []string{text}
	}
	mid := len(r) / 2
	return []string{
		text,
		string(r[:mid]),
		string(r[mid:]),
		string(r[mid-2 : min(mid+3, len(r))]),
	}
}

// The SQLite-backed adapter has no candidate query and is not meant to: it
// declines the capability so core/blockstore scans instead, and the answers
// stay the same.
func TestSearchBlockText_SQLiteTakesTheScan(t *testing.T) {
	ctx := context.Background()
	bs, cs, projectID := newTestStore(t)

	_, ok := bs.(blockstore.TextSearcher)
	assert.False(t, ok, "the SQLite store must fall through to the scan")

	b := model.NewRunsBlock("blk", []model.Run{
		model.TextR("faithful "),
		model.PcOpenR(model.PcOpenRun{ID: "1", Data: "<b>"}),
		model.TextR("write-back"),
		model.PcCloseR(model.PcCloseRun{ID: "1", Data: "</b>"}),
	})
	b.Translatable = true
	require.NoError(t, cs.StoreBlocks(ctx, projectID, "main", []*model.Block{b}))

	hits := search(t, bs, "faithful write-back", blockstore.TextSearchOptions{})
	require.Len(t, hits, 1)
	assert.Equal(t, "faithful write-back", hits[0].Text)
	assert.Equal(t, blockstore.SourceLocale, hits[0].Locale)
}
