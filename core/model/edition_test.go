package model_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

func TestParseEditionKey(t *testing.T) {
	tests := []struct {
		in      string
		want    model.EditionKey
		wantErr string
	}{
		{in: "", want: model.EditionKey{}},
		{in: "fr", want: model.EditionKey{Locale: "fr"}},
		{in: "nb_NO", want: model.EditionKey{Locale: "nb-NO"}},
		{in: "en;channel=short", want: model.EditionKey{Locale: "en", Channel: "short"}},
		{in: "fr;channel=web;tone=formal", want: model.EditionKey{Locale: "fr", Tone: "formal", Channel: "web"}},
		{in: ";tone=formal", wantErr: "names no language"},
		{in: "xx-YY", wantErr: "invalid locale"},
		{in: "fr;tone", wantErr: "not name=value"},
		{in: "fr;tone=", wantErr: "not name=value"},
		{in: "fr;tone=a;tone=b", wantErr: "tone twice"},
		{in: "fr;product=app", wantErr: "unknown dimension"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, err := model.ParseEditionKey(tc.in)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func editionBlock() *model.Block {
	b := model.NewBlock("b1", "Hello")
	b.SourceLocale = "en"
	b.SourceStatus = model.SourceStatusEstablished
	b.SetTarget("fr", &model.Target{Runs: []model.Run{model.TextR("Bonjour")}, Status: model.TargetStatusTranslated, Origin: model.Origin{Kind: model.OriginHuman}, Score: 0.8})
	b.SetTargetVariant(model.EditionKey{Locale: "en", Channel: "short"}, &model.Target{Runs: []model.Run{model.TextR("Hi")}})
	return b
}

func TestBlockEdition_RoutesEveryKeyToOneEdition(t *testing.T) {
	b := editionBlock()
	tests := []struct {
		name string
		key  model.EditionKey
		text string
		ok   bool
	}{
		{"the zero key is the document's own edition", model.EditionKey{}, "Hello", true},
		{"the source language is the edition Source holds", model.EditionKey{Locale: "en"}, "Hello", true},
		{"another spelling of the source language", model.EditionKey{Locale: "EN"}, "Hello", true},
		{"a derived edition", model.EditionKey{Locale: "fr"}, "Bonjour", true},
		{"a same-language channel edition is its own edition", model.EditionKey{Locale: "en", Channel: "short"}, "Hi", true},
		{"an edition the block does not hold", model.EditionKey{Locale: "de"}, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e, ok := b.Edition(tc.key)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.text, model.RunsText(e.Runs))
		})
	}
	src, _ := b.Edition(model.EditionKey{})
	assert.Equal(t, model.Status(model.SourceStatusEstablished), src.Status)
	fr, _ := b.Edition(model.Variant("fr"))
	assert.Equal(t, model.Edition{Runs: b.Targets[model.Variant("fr")].Runs, Status: "translated", Origin: model.Origin{Kind: model.OriginHuman}, Score: 0.8}, fr)
}

func TestBlockSetEdition(t *testing.T) {
	t.Run("the source edition is an edit that keeps the read content", func(t *testing.T) {
		b := editionBlock()
		b.SetEdition(model.EditionKey{Locale: "en"}, model.Edition{Runs: []model.Run{model.TextR("Hi there")}, Status: "written"})
		assert.Equal(t, "Hi there", b.SourceText())
		assert.Equal(t, model.SourceStatusWritten, b.SourceStatus)
		read, edited := b.SourceAsRead()
		assert.True(t, edited)
		assert.Equal(t, "Hello", model.RunsText(read))
	})
	t.Run("an existing derived edition is updated in place", func(t *testing.T) {
		b := editionBlock()
		held := b.Target("fr")
		b.SetEdition(model.Variant("fr"), model.Edition{Runs: []model.Run{model.TextR("Salut")}, Status: "draft"})
		assert.Same(t, held, b.Target("fr"))
		assert.Equal(t, "Salut", model.RunsText(held.Runs))
		assert.Equal(t, model.TargetStatusDraft, held.Status)
	})
	t.Run("a new edition is filed under its canonical key", func(t *testing.T) {
		b := editionBlock()
		b.SetEdition(model.EditionKey{Locale: "nb_NO"}, model.Edition{Runs: []model.Run{model.TextR("Hei")}})
		assert.Equal(t, "Hei", b.TargetText("nb-NO"))
		_, raw := b.Targets[model.EditionKey{Locale: "nb_NO"}]
		assert.False(t, raw, "no target is filed under the spelling a caller used")
	})
	t.Run("the source origin follows the edition", func(t *testing.T) {
		b := editionBlock()
		b.SetEdition(model.EditionKey{}, model.Edition{Runs: b.Source, Origin: model.Origin{Kind: model.OriginOCR}})
		o, ok := b.SourceOrigin()
		require.True(t, ok)
		assert.Equal(t, model.OriginOCR, o.Kind)
		b.SetEdition(model.EditionKey{}, model.Edition{Runs: b.Source})
		_, ok = b.SourceOrigin()
		assert.False(t, ok)
	})
}

// A status stamp moves the edition's status and nothing else: on the edition
// the block was read in it records no source as read and leaves the
// source-origin annotation as it is, whatever that annotation holds.
func TestBlockSetEditionStatus(t *testing.T) {
	t.Run("the edition the block was read in", func(t *testing.T) {
		origin := &model.Origin{Kind: model.OriginHuman, Tool: "editor"}
		raw := &model.RawAnnotation{Kind: model.AnnoSourceOrigin, Body: []byte(`{"kind":`)}
		tests := []struct {
			name string
			anno model.Payload
		}{
			{"origin", origin},
			{"undecoded origin", raw},
			{"no origin", nil},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				b := editionBlock()
				if tc.anno != nil {
					b.SetAnno(model.AnnoSourceOrigin, tc.anno)
				}
				runs := b.Source

				assert.True(t, b.SetEditionStatus(model.EditionKey{}, model.Status(model.SourceStatusWritten)))

				assert.Equal(t, model.SourceStatusWritten, b.SourceStatus)
				assert.Same(t, &runs[0], &b.Source[0], "the runs are the ones the block held")
				got, ok := b.Anno(model.AnnoSourceOrigin)
				if tc.anno == nil {
					assert.False(t, ok)
				} else {
					require.True(t, ok)
					assert.Same(t, tc.anno, got)
				}
				read, edited := b.SourceAsRead()
				assert.False(t, edited)
				assert.Equal(t, "Hello", model.RunsText(read))
				// A reader that replaces the content afterwards finds nothing
				// kept from before it.
				b.SetSourceRuns([]model.Run{model.TextR("Hello again")})
				read, edited = b.SourceAsRead()
				assert.False(t, edited, "a status stamp records no source as read")
				assert.Equal(t, "Hello again", model.RunsText(read))
			})
		}
	})
	t.Run("a derived edition", func(t *testing.T) {
		b := editionBlock()
		held := b.Target("fr")
		runs := held.Runs
		assert.True(t, b.SetEditionStatus(model.EditionKey{Locale: "FR"}, "established"))
		assert.Same(t, held, b.Target("fr"))
		assert.Equal(t, model.TargetStatusEstablished, held.Status)
		assert.Same(t, &runs[0], &held.Runs[0])
		assert.Equal(t, model.Origin{Kind: model.OriginHuman}, held.Origin)
		assert.InDelta(t, 0.8, held.Score, 0)
		assert.Equal(t, model.SourceStatusEstablished, b.SourceStatus, "the source keeps its status")
	})
	t.Run("a same-language target", func(t *testing.T) {
		b := model.NewBlock("b1", "colour source")
		b.SourceLocale = "en-US"
		b.SetTarget("en-US", &model.Target{Runs: []model.Run{model.TextR("colour target")}})
		assert.True(t, b.SetEditionStatus(model.Variant("en-US"), "translated"))
		assert.Equal(t, model.TargetStatusTranslated, b.Target("en-US").Status)
		assert.Empty(t, b.SourceStatus)
	})
	t.Run("an edition the block does not hold", func(t *testing.T) {
		b := editionBlock()
		before := b.Editions()
		assert.False(t, b.SetEditionStatus(model.Variant("de"), "translated"))
		assert.Equal(t, before, b.Editions(), "no edition is created")
	})
}

// collectEditions gathers what EachEdition yields, in order.
func collectEditions(b *model.Block) ([]model.EditionKey, map[model.EditionKey]string) {
	var keys []model.EditionKey
	text := map[model.EditionKey]string{}
	for k, e := range b.EachEdition {
		keys = append(keys, k)
		text[k] = model.RunsText(e.Runs)
	}
	return keys, text
}

func TestBlockEachEdition(t *testing.T) {
	tests := []struct {
		name  string
		block func() *model.Block
		first model.EditionKey
		want  map[model.EditionKey]string
	}{
		{
			name:  "the edition read in first, then every other one",
			block: editionBlock,
			first: model.EditionKey{Locale: "en"},
			want: map[model.EditionKey]string{
				{Locale: "en"}:                   "Hello",
				{Locale: "fr"}:                   "Bonjour",
				{Locale: "en", Channel: "short"}: "Hi",
			},
		},
		{
			name: "a same-language target keeps its key and the source takes the zero key",
			block: func() *model.Block {
				b := model.NewBlock("b1", "colour source")
				b.SourceLocale = "en-US"
				b.SetTarget("en-US", &model.Target{Runs: []model.Run{model.TextR("colour target")}})
				return b
			},
			first: model.EditionKey{},
			want: map[model.EditionKey]string{
				{}:                "colour source",
				{Locale: "en-US"}: "colour target",
			},
		},
		{
			name: "a target filed under a key that is not canonical",
			block: func() *model.Block {
				b := model.NewBlock("b1", "Hello")
				b.Targets = map[model.VariantKey]*model.Target{
					{Locale: "fr_FR"}: {Runs: []model.Run{model.TextR("Bonjour")}, Status: model.TargetStatusTranslated},
				}
				return b
			},
			first: model.EditionKey{},
			want: map[model.EditionKey]string{
				{}:                "Hello",
				{Locale: "fr-FR"}: "Bonjour",
			},
		},
		{
			name: "a canonical key wins over another spelling of it",
			block: func() *model.Block {
				b := model.NewBlock("b1", "Hello")
				b.Targets = map[model.VariantKey]*model.Target{
					{Locale: "fr_FR"}: {Runs: []model.Run{model.TextR("Salut")}},
					{Locale: "fr-FR"}: {Runs: []model.Run{model.TextR("Bonjour")}},
				}
				return b
			},
			first: model.EditionKey{},
			want: map[model.EditionKey]string{
				{}:                "Hello",
				{Locale: "fr-FR"}: "Bonjour",
			},
		},
		{
			name: "the zero key, the source language and a nil target name no other edition",
			block: func() *model.Block {
				b := model.NewBlock("b1", "Hello")
				b.SourceLocale = "en"
				b.Targets = map[model.VariantKey]*model.Target{
					{}:             {Runs: []model.Run{model.TextR("filed under no language")}},
					{Locale: "EN"}: {Runs: []model.Run{model.TextR("another spelling of the source")}},
					{Locale: "de"}: nil,
					{Locale: "nb"}: {Runs: []model.Run{model.TextR("Hei")}},
				}
				return b
			},
			first: model.EditionKey{Locale: "en"},
			want: map[model.EditionKey]string{
				{Locale: "en"}: "Hello",
				{Locale: "nb"}: "Hei",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := tc.block()
			keys, text := collectEditions(b)
			require.NotEmpty(t, keys)
			assert.Equal(t, tc.first, keys[0])
			assert.Equal(t, b.EditionKeyOf(model.EditionKey{}), keys[0])
			assert.Len(t, keys, len(tc.want), "each edition is yielded once")
			assert.Equal(t, tc.want, text)
			assert.ElementsMatch(t, b.Editions(), keys, "the keys Editions lists")
		})
	}

	t.Run("the edition carries status, origin and score", func(t *testing.T) {
		b := editionBlock()
		for k, e := range b.EachEdition {
			want, ok := b.Edition(k)
			require.True(t, ok)
			assert.Equal(t, want, e)
		}
	})
	t.Run("a loop that stops early stops the walk", func(t *testing.T) {
		n := 0
		for range editionBlock().EachEdition {
			n++
			break
		}
		assert.Equal(t, 1, n)
	})
}

func TestBlockRemoveEditionAndEditions(t *testing.T) {
	b := editionBlock()
	assert.Equal(t, []model.EditionKey{{Locale: "en"}, {Locale: "en", Channel: "short"}, {Locale: "fr"}}, b.Editions())
	assert.False(t, b.RemoveEdition(model.EditionKey{Locale: "en"}), "the edition the block was read in stays")
	assert.False(t, b.RemoveEdition(model.Variant("de")))
	assert.True(t, b.RemoveEdition(model.EditionKey{Locale: "FR"}))
	assert.Equal(t, []model.EditionKey{{Locale: "en"}, {Locale: "en", Channel: "short"}}, b.Editions())
}

// A bilingual file whose two languages are one (an XLIFF file from en-US to
// en-US, a PO catalogue in its source language) holds a target under the key
// of the source language. That key then reaches the target, and the zero key
// alone reaches the edition the block was read in, so neither hides the
// other and a write to one never lands in the other.
func TestBlockEdition_ASameLanguageTargetKeepsItsKey(t *testing.T) {
	b := model.NewBlock("b1", "colour source")
	b.SourceLocale = "en-US"
	b.SourceStatus = model.SourceStatusWritten
	b.SetTarget("en-US", &model.Target{Runs: []model.Run{model.TextR("colour target")}, Status: model.TargetStatusEstablished})

	assert.Equal(t, []model.EditionKey{{}, {Locale: "en-US"}}, b.Editions())
	tgt, ok := b.Edition(model.Variant("en-US"))
	require.True(t, ok)
	assert.Equal(t, "colour target", model.RunsText(tgt.Runs))
	src, ok := b.Edition(model.EditionKey{})
	require.True(t, ok)
	assert.Equal(t, "colour source", model.RunsText(src.Runs))
	assert.False(t, b.IsSourceEdition(model.Variant("en-US")))
	assert.Equal(t, model.EditionKey{}, b.Authoritative(model.AuthorityPolicy{}))
	assert.Equal(t, model.EditionKey{}, b.Authoritative(model.AuthorityPolicy{Locale: "en-US"}), "the source language names the edition the block was read in")
	assert.NotEqual(t, model.EditionRevision(b, model.EditionKey{}), model.EditionRevision(b, model.Variant("en-US")))

	b.SetEdition(model.Variant("en-US"), model.Edition{Runs: []model.Run{model.TextR("color target")}, Status: "draft"})
	assert.Equal(t, "color target", b.TargetText("en-US"))
	assert.Equal(t, "colour source", b.SourceText())
	assert.Equal(t, model.SourceStatusWritten, b.SourceStatus)

	assert.True(t, b.RemoveEdition(model.Variant("en-US")))
	assert.Equal(t, "colour source", b.SourceText())
	assert.Equal(t, []model.EditionKey{{Locale: "en-US"}}, b.Editions(), "with no such target the source language reaches the source again")
}

func TestBlockAuthoritative(t *testing.T) {
	b := editionBlock()
	assert.Equal(t, model.EditionKey{Locale: "en"}, b.Authoritative(model.AuthorityPolicy{}))
	assert.Equal(t, model.EditionKey{Locale: "en"}, b.Authoritative(model.AuthorityPolicy{Locale: "en"}))
	assert.Equal(t, model.EditionKey{Locale: "fr"}, b.Authoritative(model.AuthorityPolicy{Locale: "fr"}), "a recipe can name a held edition")
	assert.Equal(t, model.EditionKey{Locale: "en"}, b.Authoritative(model.AuthorityPolicy{Locale: "de"}), "an edition the block lacks names nothing")
}

// sourceLocalesNormalizedTwice are source locales whose normalization is not
// a fixed point: NormalizeLocale of the normalized form differs from it. The
// first is a repeated -u- singleton x/text collapses one step at a time; the
// second takes the casing fallback, which rewrites a byte that is not UTF-8.
var sourceLocalesNormalizedTwice = []model.LocaleID{"AA-u-00-00-u-00-00", "aa-t-BB-AA-0A-\x800"}

// assertAuthoritativeIsSource checks that the key Authoritative returns with
// no policy reaches the edition the block was read in, through every
// accessor, so a writer reading that edition writes the source.
func assertAuthoritativeIsSource(t *testing.T, b *model.Block) {
	t.Helper()
	k := b.Authoritative(model.AuthorityPolicy{})
	e, ok := b.Edition(k)
	require.True(t, ok, "Edition(%+v)", k)
	require.NotEmpty(t, e.Runs)
	assert.Same(t, &b.SourceRuns()[0], &e.Runs[0], "the edition is the source")
	assert.True(t, b.IsSourceEdition(k))
	assert.Equal(t, k, b.EditionKeyOf(k))
	assert.Equal(t, k, b.EditionKeyOf(model.EditionKey{}))
	assert.Equal(t, k, b.Editions()[0])
	for key, ed := range b.EachEdition {
		assert.Equal(t, k, key, "the edition read in is yielded first")
		assert.Same(t, &b.SourceRuns()[0], &ed.Runs[0])
		break
	}
	assert.False(t, b.RemoveEdition(k), "the edition the block was read in stays")
}

func TestBlockEdition_ASourceLocaleNormalizedTwiceStillReachesTheSource(t *testing.T) {
	for _, loc := range sourceLocalesNormalizedTwice {
		t.Run(string(loc), func(t *testing.T) {
			once := model.NormalizeLocale(loc)
			require.NotEqual(t, once, model.NormalizeLocale(once), "the locale this test covers normalizes in two steps")

			b := model.NewBlock("b1", "Hello world")
			b.SourceLocale = loc
			assertAuthoritativeIsSource(t, b)
			assert.Equal(t, model.EditionKey{Locale: once}, b.Authoritative(model.AuthorityPolicy{}))

			require.True(t, b.SetEditionStatus(b.Authoritative(model.AuthorityPolicy{}), model.Status(model.SourceStatusEstablished)))
			assert.Equal(t, model.SourceStatusEstablished, b.SourceStatus)
			assert.Empty(t, b.Targets, "a status stamp on the source creates no target")

			b.SetTargetText(loc, "Hello target")
			assertAuthoritativeIsSource(t, b)
			assert.Equal(t, model.EditionKey{}, b.Authoritative(model.AuthorityPolicy{}), "a same-language target takes the source language's key")
		})
	}
}

// FuzzBlockAuthoritativeEditionIsTheSource holds every writer's read of the
// source, Edition(Authoritative(AuthorityPolicy{})), to the block's Source
// for any source locale, with and without a same-language target.
func FuzzBlockAuthoritativeEditionIsTheSource(f *testing.F) {
	for _, loc := range sourceLocalesNormalizedTwice {
		f.Add(string(loc), false)
		f.Add(string(loc), true)
	}
	for _, loc := range []string{"", "en", "en-US", "nb_NO", "EN_us.UTF-8", "qps-Ploc", "xx-YY", "not a locale"} {
		f.Add(loc, false)
		f.Add(loc, true)
	}
	f.Fuzz(func(t *testing.T, loc string, sameLanguageTarget bool) {
		b := model.NewBlock("b1", "Hello world")
		b.SourceLocale = model.LocaleID(loc)
		if sameLanguageTarget && loc != "" {
			b.SetTargetText(model.LocaleID(loc), "Hello target")
		}
		assertAuthoritativeIsSource(t, b)
	})
}
