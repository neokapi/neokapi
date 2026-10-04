package tools_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
)

// A pass-through script hands back a block whose only edition in a language
// is one of a tone or a channel as it was: the script never sees that edition
// under its language, so it neither refuses the block nor writes an empty
// edition of the language beside it.
func TestScriptPassThroughKeepsAToneOrChannelEdition(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		key  model.EditionKey
	}{
		{"a same-language channel edition", model.EditionKey{Locale: "en-US", Channel: "short"}},
		{"a tone edition with no plain edition of its language", model.EditionKey{Locale: "fr", Tone: "formal"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tl := tools.NewScriptTool(&tools.ScriptConfig{Code: "emit(part)"})
			block := model.NewBlock("tu1", "Hello")
			block.SourceLocale = "en-US"
			block.SetEdition(tc.key, model.Edition{Runs: []model.Run{model.TextR("Hi")}})
			want := block.EditionKeys()

			out := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: block}).Resource.(*model.Block)
			assert.Equal(t, want, out.EditionKeys(), "no edition is added or removed")
			e, ok := out.Edition(tc.key)
			require.True(t, ok)
			assert.Equal(t, "Hi", model.RunsText(e.Runs))
			assert.Equal(t, "Hello", out.SourceText())
			_, plain := out.TargetEdition(tc.key.Locale)
			assert.False(t, plain, "no edition of the language without the tone or channel is written")
		})
	}
}

// A script that rewrites the targets it sees rewrites the edition of each
// language and leaves an edition of a tone or a channel in that language as it
// was.
func TestScriptTargetsHoldEachLanguagesOwnEdition(t *testing.T) {
	t.Parallel()
	tl := tools.NewScriptTool(&tools.ScriptConfig{Code: `
		if (Object.keys(part.block.targets).join() !== "fr") {
			throw new Error("targets: " + Object.keys(part.block.targets).join());
		}
		part.block.targets["fr"][0].content.text = "Salut";
		emit(part);`})
	formal := model.EditionKey{Locale: "fr", Tone: "formal"}
	block := model.NewBlock("tu1", "Hello")
	block.SourceLocale = "en-US"
	block.SetTargetText("fr", "Bonjour")
	block.SetEdition(formal, model.Edition{Runs: []model.Run{model.TextR("Bonjour, madame")}})

	out := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: block}).Resource.(*model.Block)
	assert.Equal(t, "Salut", out.TargetText("fr"))
	e, ok := out.Edition(formal)
	require.True(t, ok)
	assert.Equal(t, "Bonjour, madame", model.RunsText(e.Runs))
}
