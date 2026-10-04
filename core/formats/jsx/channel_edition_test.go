package jsx_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/kbf"
	"github.com/neokapi/neokapi/core/model"
)

// A bundle keys a target by its language alone, so a same-language channel
// edition has no slot in it. Writing a block that holds one writes the source
// and each language's own edition as they were, and no target in the source
// language, so reading the bundle back finds the block as the bundle held it.
// Carrying the channel edition itself is the work of the bundle's v2 schema.
func TestKBFWritesNoSourceLanguageTargetForAChannelEdition(t *testing.T) {
	blocks := readBlocks(t, emptyLocaleBundle(`{"nb": [{"text": "Logg inn"}]}`, `{}`))
	require.Len(t, blocks, 1)
	b := blocks[0]
	b.SourceLocale = "en" // the bundle's project language
	short := model.EditionKey{Locale: "en", Channel: "short"}
	b.SetEdition(short, model.Edition{Runs: []model.Run{model.TextR("Sign")}})

	body := writeBundle(t, b)
	var file kbf.File
	require.NoError(t, json.Unmarshal([]byte(body), &file))
	require.Len(t, file.Documents, 1)
	require.Len(t, file.Documents[0].Blocks, 1)
	out := file.Documents[0].Blocks[0]
	assert.Equal(t, "Sign in", model.RunsText(out.Source))
	got := map[kbf.LocaleID]string{}
	for loc, runs := range out.Targets {
		got[loc] = model.RunsText(runs)
	}
	assert.Equal(t, map[kbf.LocaleID]string{"nb": "Logg inn"}, got, "the channel edition takes no slot")

	back := readBlocks(t, body)
	require.Len(t, back, 1)
	assert.Equal(t, "Sign in", back[0].SourceText())
	assert.Equal(t, "Logg inn", back[0].TargetText("nb"))
	_, held := back[0].Edition(short)
	assert.False(t, held, "a bundle in the v1 schema carries no channel edition")
}
