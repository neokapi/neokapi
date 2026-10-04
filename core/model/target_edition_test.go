package model_test

import (
	"maps"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// twoStepLocale is a malformed tag x/text reads in two steps, first as
// "aa-u-00-u-00-00" and then as "aa-u-00-u-00", which NormalizeLocale returns.
const twoStepLocale = model.LocaleID("AA-u-00-00-u-00-00")

// targetEditionBlocks are blocks holding a target under each kind of key a
// reader or a tool files one under.
func targetEditionBlocks() map[string]func() *model.Block {
	withTarget := func(src, loc model.LocaleID, text string) func() *model.Block {
		return func() *model.Block {
			b := model.NewBlock("b1", "Hello")
			b.SourceLocale = src
			b.SetEditionStatus(model.EditionKey{}, model.Status(model.SourceStatusEstablished))
			b.SetTargetRuns(loc, []model.Run{model.TextR(text)})
			b.StampTargetProvenance(loc, model.TargetStatusTranslated, model.Origin{Kind: model.OriginAI, ContextFingerprint: "fp-" + text})
			return b
		}
	}
	return map[string]func() *model.Block{
		"no target": func() *model.Block {
			b := model.NewBlock("b1", "Hello")
			b.SourceLocale = "en-US"
			return b
		},
		"fr":                               withTarget("en-US", "fr", "Bonjour"),
		"same language":                    withTarget("en-US", "en-US", "colour"),
		"no language":                      withTarget("en-US", "", "zero"),
		"no language, no source locale":    withTarget("", "", "zero"),
		"two-step locale":                  withTarget("en-US", twoStepLocale, "two"),
		"two-step source, same language":   withTarget(twoStepLocale, twoStepLocale, "same"),
		"two-step source, normalized once": withTarget(model.NormalizeLocale(twoStepLocale), twoStepLocale, "once"),
		"tone only": func() *model.Block {
			b := model.NewBlock("b1", "Hello")
			b.SourceLocale = "en-US"
			b.SetEdition(model.EditionKey{Locale: "fr", Tone: "formal"}, model.Edition{Runs: []model.Run{model.TextR("Bonjour")}})
			return b
		},
		"non-canonical key": func() *model.Block {
			b := model.NewBlock("b1", "Hello")
			b.SourceLocale = "en-US"
			b.Editions[model.EditionKey{Locale: "nb_NO"}] = &model.Edition{Runs: []model.Run{model.TextR("Hei")}}
			return b
		},
	}
}

func targetEditionLocales() []model.LocaleID {
	once := model.NormalizeLocale(twoStepLocale)
	return []model.LocaleID{"", "fr", "en-US", "en_us", "en", "nb_NO", "nb-NO", twoStepLocale, once, model.NormalizeLocale(once)}
}

// TargetEdition reads the target filed under the locale's canonical key, the
// one EachTargetEdition yields under that key, for every locale, on every kind
// of block: a target under the source language, under no language, under a
// malformed locale x/text reads in two steps, under a key that is not
// canonical, and none at all. It never reads the edition the block was read
// in.
func TestBlockTargetEdition_ReadsTheTargetFiledUnderTheLocale(t *testing.T) {
	for name, mk := range targetEditionBlocks() {
		for _, loc := range targetEditionLocales() {
			b := mk()
			want, filed := maps.Collect(b.EachTargetEdition)[model.Variant(loc)]
			got, ok := b.TargetEdition(loc)
			if !filed {
				assert.False(t, ok, "%s: TargetEdition(%q)", name, loc)
				assert.Equal(t, model.Edition{}, got, "%s: TargetEdition(%q)", name, loc)
				assert.Empty(t, b.TargetRuns(loc), "%s: TargetRuns(%q)", name, loc)
				continue
			}
			require.True(t, ok, "%s: TargetEdition(%q)", name, loc)
			assert.Equal(t, want, got, "%s: TargetEdition(%q)", name, loc)
			assert.Equal(t, b.TargetText(loc), model.RunsText(got.Runs), "%s: TargetText(%q)", name, loc)
			if src, _ := b.Edition(model.EditionKey{}); len(src.Runs) > 0 && len(got.Runs) > 0 {
				assert.NotSame(t, &src.Runs[0], &got.Runs[0], "%s: TargetEdition(%q) read the source", name, loc)
			}
		}
	}
}

// The empty locale reads the target filed under no language, and the zero key
// still names the edition the block was read in for Edition, Editions and
// EachEdition.
func TestBlockTargetEdition_NoLanguage(t *testing.T) {
	_, ok := targetEditionBlocks()["no target"]().TargetEdition("")
	assert.False(t, ok, "a block with no target under no language")

	b := targetEditionBlocks()["no language"]()
	b.SetTargetText("fr", "Bonjour")
	got, ok := b.TargetEdition("")
	require.True(t, ok)
	assert.Equal(t, "zero", model.RunsText(got.Runs))
	assert.Equal(t, model.Status(model.TargetStatusTranslated), got.Status)
	assert.Equal(t, "fp-zero", got.Origin.ContextFingerprint)

	src, ok := b.Edition(model.EditionKey{})
	require.True(t, ok)
	assert.Equal(t, "Hello", model.RunsText(src.Runs))
	assert.Equal(t, []model.EditionKey{{Locale: "en-US"}, {Locale: "fr"}}, b.EditionKeys())
	for k := range b.EachEdition {
		assert.False(t, k.IsZero(), "the target under no language is not an edition EachEdition yields")
	}

	_, ok = targetEditionBlocks()["no target"]().TargetEdition("en-US")
	assert.False(t, ok, "the source language names no target while the block holds none")

	t.Run("one with no runs is held", func(t *testing.T) {
		b := model.NewBlock("b1", "Hello")
		b.SetTargetRuns("", nil)
		got, ok := b.TargetEdition("")
		assert.True(t, ok)
		assert.Empty(t, got.Runs)
	})
}

// SetTargetEdition files the edition under the key's canonical form, on every
// kind of block and for every kind of key, the zero key filing a translation
// under no language. It leaves the edition the block was read in, and every
// other target, as they were.
func TestBlockSetTargetEdition_FilesUnderTheCanonicalKey(t *testing.T) {
	e := model.Edition{
		Runs:   []model.Run{model.TextR("written")},
		Status: model.Status(model.TargetStatusEstablished),
		Origin: model.Origin{Kind: model.OriginHuman},
		Score:  0.75,
	}
	keys := []model.EditionKey{
		{}, {Locale: "en-US"}, {Locale: "en_us"}, {Locale: "fr"}, {Locale: "fr", Tone: "formal"},
		{Locale: "en-US", Channel: "short"}, {Tone: "formal"}, {Locale: twoStepLocale},
		{Locale: model.NormalizeLocale(twoStepLocale)}, {Locale: "nb_NO"},
	}
	for name, mk := range targetEditionBlocks() {
		for _, k := range keys {
			got, before := mk(), mk()
			got.SetTargetEdition(k, e)

			wantSrc, _ := before.Edition(model.EditionKey{})
			gotSrc, _ := got.Edition(model.EditionKey{})
			assert.Equal(t, wantSrc, gotSrc, "%s %+v: source", name, k)
			_, edited := got.SourceAsRead()
			assert.False(t, edited, "%s %+v: the source was edited", name, k)

			want := maps.Collect(before.EachTargetEdition)
			want[k.Canonical()] = e
			assert.Equal(t, want, maps.Collect(got.EachTargetEdition), "%s %+v: targets", name, k)
		}
	}
}

// An existing target is updated in place, as SetEdition updates one.
func TestBlockSetTargetEdition_UpdatesInPlace(t *testing.T) {
	b := targetEditionBlocks()["fr"]()
	held := b.Editions[model.Variant("fr")]
	b.SetTargetEdition(model.Variant("fr"), model.Edition{Runs: []model.Run{model.TextR("Salut")}})
	assert.Equal(t, "Salut", model.RunsText(held.Runs))
	assert.Empty(t, held.Status, "the edition written carries no status")
	assert.Empty(t, held.Origin.Kind, "the edition written carries no origin")
}
