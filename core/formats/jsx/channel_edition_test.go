package jsx_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/kbf"
	"github.com/neokapi/neokapi/core/model"
)

// A bundle keys each edition by its full key, so a same-language channel
// edition and a tone edition each have a key of their own. Writing a block that
// holds them writes the source, each language's own edition and every
// qualified edition as they were, and reading the bundle back finds the block
// as it was written, with no source-language target beside the source.
func TestKBFCarriesChannelAndToneEditions(t *testing.T) {
	blocks := readBlocks(t, emptyLocaleBundle(`{"nb": [{"text": "Logg inn"}]}`, `{}`))
	require.Len(t, blocks, 1)
	b := blocks[0]
	b.SourceLocale = "en" // the bundle's project language
	short := model.EditionKey{Locale: "en", Channel: "short"}
	b.SetEdition(short, model.Edition{
		Runs:    []model.Run{model.TextR("Sign")},
		Derived: &model.Derivation{From: model.Variant("en"), Rev: model.EditionRevision(b, model.Variant("en"))},
	})
	formal := model.EditionKey{Locale: "nb", Tone: "formal"}
	b.SetEdition(formal, model.Edition{Runs: []model.Run{model.TextR("Logg Dem inn")}})

	body := writeBundle(t, b)
	var file kbf.File
	require.NoError(t, json.Unmarshal([]byte(body), &file))
	require.Len(t, file.Documents, 1)
	require.Len(t, file.Documents[0].Blocks, 1)
	out := file.Documents[0].Blocks[0]
	assert.Equal(t, "Sign in", model.RunsText(out.SourceRuns()))
	got := map[string]string{}
	for _, key := range out.TargetKeys() {
		got[key] = model.RunsText(out.Editions[key].Runs)
	}
	assert.Equal(t, map[string]string{
		"en;channel=short": "Sign",
		"nb":               "Logg inn",
		"nb;tone=formal":   "Logg Dem inn",
	}, got, "every edition has a key of its own, and no plain en target appears")
	require.NotNil(t, out.Editions["en;channel=short"].Derived)
	assert.Equal(t, "en", out.Editions["en;channel=short"].Derived.From)

	back := readBlocks(t, body)
	require.Len(t, back, 1)
	back[0].SourceLocale = "en"
	assert.Equal(t, "Sign in", back[0].SourceText())
	assert.Equal(t, "Logg inn", back[0].TargetText("nb"))
	gotShort, held := back[0].Edition(short)
	require.True(t, held, "the channel edition reads back")
	assert.Equal(t, "Sign", model.RunsText(gotShort.Runs))
	require.NotNil(t, gotShort.Derived)
	assert.Equal(t, model.Variant("en"), gotShort.Derived.From)
	gotFormal, held := back[0].Edition(formal)
	require.True(t, held, "the tone edition reads back")
	assert.Equal(t, "Logg Dem inn", model.RunsText(gotFormal.Runs))
	_, sameLanguage := back[0].TargetEdition("en")
	assert.False(t, sameLanguage, "the channel edition leaves no plain target in the source language")
}
