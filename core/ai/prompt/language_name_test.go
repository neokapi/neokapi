package prompt

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// A bare BCP-47 tag is an identifier, and a model has to resolve it before it
// can act on it. Asked to translate "Order Confirmed!" from "en" to "nb",
// gemma4:e2b returned "Commande confirmée !" on every one of eight runs at the
// temperature kapi uses for local models: French, confidently, for a Norwegian
// target. Told "Norwegian Bokmål (nb)" it returned "Ordre bekreftet!".
func TestLanguageNameNamesTheLanguage(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Norwegian Bokmål (nb)", LanguageName("nb"))
	assert.Equal(t, "French (fr)", LanguageName("fr"))
	assert.Equal(t, "Traditional Chinese (zh-Hant)", LanguageName("zh-Hant"))
}

// The tag stays beside the name because a name does not always travel alone:
// Norwegian has two written standards, and Traditional is not Simplified.
func TestLanguageNameKeepsTheTag(t *testing.T) {
	t.Parallel()

	for _, id := range []model.LocaleID{"nb", "nn", "zh-Hans", "zh-Hant", "pt-BR"} {
		assert.Contains(t, LanguageName(id), "("+string(id)+")",
			"the tag disambiguates what the name alone does not")
	}
}

// qps is our pseudo locale and CLDR has no name for it. "qps (qps)" tells a
// reader nothing twice.
func TestLanguageNameDoesNotDoubleAnUnnamedTag(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "qps", LanguageName("qps"))
	assert.Equal(t, "not-a-tag", LanguageName("not-a-tag"))
}

// The single and batch paths must agree: a run translates in batches and a
// reviewer retranslates one block, and the two must not describe the target
// differently.
func TestBothTranslatePathsNameTheLanguage(t *testing.T) {
	t.Parallel()

	tr := Translate{SourceLocale: "en", TargetLocale: "nb"}

	single := tr.Single("Order Confirmed!", false)
	require.NotEmpty(t, single)
	assert.Contains(t, single[0].Text, "Norwegian Bokmål (nb)")
	assert.NotContains(t, single[0].Text, "to nb,", "the bare tag is not the instruction")

	batch := tr.Batch([]BatchSegment{{ID: "a", Text: "Order Confirmed!"}})
	require.NotEmpty(t, batch)
	assert.Contains(t, batch[0].Text, "Norwegian Bokmål (nb)")
}

// The judge is told what language it is judging, for the same reason: scoring a
// French string as a fine Norwegian translation is the failure this prevents.
func TestReviewPromptNamesTheLanguage(t *testing.T) {
	t.Parallel()

	turns := Review{SourceLocale: "en", TargetLocale: "nb"}.Turns("Order Confirmed!", "Commande confirmée !")
	var joined strings.Builder
	for _, turn := range turns {
		joined.WriteString(turn.Text)
	}
	assert.Contains(t, joined.String(), "Norwegian Bokmål (nb)")
}
