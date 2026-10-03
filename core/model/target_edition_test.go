package model_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// twoStepLocale is a tag NormalizeLocale reads in two steps: once it is
// "aa-u-00-u-00-00", and that reads as "aa-u-00-u-00".
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
			b.Targets[model.VariantKey{Locale: "nb_NO"}] = &model.Target{Runs: []model.Run{model.TextR("Hei")}}
			return b
		},
	}
}

func targetEditionLocales() []model.LocaleID {
	once := model.NormalizeLocale(twoStepLocale)
	return []model.LocaleID{"", "fr", "en-US", "en_us", "en", "nb_NO", "nb-NO", twoStepLocale, once, model.NormalizeLocale(once)}
}

// TargetEdition reads the target Target reads, for every locale, on every kind
// of block: a target under the source language, under no language, under a
// locale that normalizes in two steps, under a key that is not canonical, and
// none at all.
func TestBlockTargetEdition_ReadsTheTargetTargetReads(t *testing.T) {
	for name, mk := range targetEditionBlocks() {
		for _, loc := range targetEditionLocales() {
			b := mk()
			want := b.Target(loc)
			got, ok := b.TargetEdition(loc)
			if want == nil {
				assert.False(t, ok, "%s: TargetEdition(%q)", name, loc)
				assert.Equal(t, model.Edition{}, got, "%s: TargetEdition(%q)", name, loc)
				continue
			}
			require.True(t, ok, "%s: TargetEdition(%q)", name, loc)
			assert.Equal(t, model.Edition{Runs: want.Runs, Status: model.Status(want.Status), Origin: want.Origin, Score: want.Score},
				got, "%s: TargetEdition(%q)", name, loc)
			assert.Equal(t, b.TargetText(loc), model.RunsText(got.Runs), "%s: TargetText(%q)", name, loc)
		}
	}
}

// The empty locale reads the target filed under no language, and the zero key
// still names the edition the block was read in for Edition.
func TestBlockTargetEdition_NoLanguage(t *testing.T) {
	b := targetEditionBlocks()["no language"]()

	got, ok := b.TargetEdition("")
	require.True(t, ok)
	assert.Equal(t, "zero", model.RunsText(got.Runs))
	assert.Equal(t, model.Status(model.TargetStatusTranslated), got.Status)
	assert.Equal(t, "fp-zero", got.Origin.ContextFingerprint)

	src, ok := b.Edition(model.EditionKey{})
	require.True(t, ok)
	assert.Equal(t, "Hello", model.RunsText(src.Runs))

	_, ok = targetEditionBlocks()["no target"]().TargetEdition("en-US")
	assert.False(t, ok, "the source language names no target while the block holds none")
}

// SetTargetEdition writes where SetTargetVariant writes, on every kind of
// block and for every kind of key, and leaves the edition the block was read
// in as it is.
func TestBlockSetTargetEdition_WritesWhereSetTargetVariantWrites(t *testing.T) {
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
			got, want := mk(), mk()
			got.SetTargetEdition(k, e)
			want.SetTargetVariant(k, &model.Target{Runs: e.Runs, Status: model.TargetStatus(e.Status), Origin: e.Origin, Score: e.Score})

			assert.Equal(t, want.Source, got.Source, "%s %+v: source", name, k)
			assert.Equal(t, want.SourceStatus, got.SourceStatus, "%s %+v: source status", name, k)
			_, edited := got.SourceAsRead()
			assert.False(t, edited, "%s %+v: the source was edited", name, k)
			require.Len(t, got.Targets, len(want.Targets), "%s %+v: targets", name, k)
			for key, wt := range want.Targets {
				gt, ok := got.Targets[key]
				require.True(t, ok, "%s %+v: target %+v", name, k, key)
				assert.Equal(t, *wt, *gt, "%s %+v: target %+v", name, k, key)
			}
		}
	}
}

// An existing target is updated in place, as SetEdition updates one.
func TestBlockSetTargetEdition_UpdatesInPlace(t *testing.T) {
	b := targetEditionBlocks()["fr"]()
	held := b.Target("fr")
	b.SetTargetEdition(model.Variant("fr"), model.Edition{Runs: []model.Run{model.TextR("Salut")}})
	assert.Equal(t, "Salut", model.RunsText(held.Runs))
	assert.Empty(t, held.Status, "the edition written carries no status")
	assert.Empty(t, held.Origin.Kind, "the edition written carries no origin")
}
