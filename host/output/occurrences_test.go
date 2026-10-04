package output

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/occurrence"
)

// A use found in a tone edition is listed under the edition's key, in the text
// table and in the JSON a script reads, beside the plain edition of its
// language and the source.
func TestTermsOccurrencesOutputShowsAnEditionKey(t *testing.T) {
	out := NewTermsOccurrencesOutput(&occurrence.Result{
		Subject: "innholdsminne",
		Occurrences: []occurrence.Occurrence{
			{Term: "innholdsminne", Document: "site/index.html", BlockID: "hero", Locale: "", Snippet: "content memory"},
			{Term: "innholdsminne", Document: "site/index.html", BlockID: "hero", Locale: "nb", Snippet: "Innholdsminne i bruk."},
			{Term: "innholdsminne", Document: "site/index.html", BlockID: "hero", Locale: "nb;tone=formal", Snippet: "Deres innholdsminne."},
		},
		Total:  3,
		Blocks: 1,
	})

	var text bytes.Buffer
	require.NoError(t, PrintTo(&text, FormatText, out))
	assert.Contains(t, text.String(), "nb;tone=formal")
	assert.Contains(t, text.String(), "source")

	var js bytes.Buffer
	require.NoError(t, PrintTo(&js, FormatJSON, out))
	var decoded TermsOccurrencesOutput
	require.NoError(t, json.Unmarshal(js.Bytes(), &decoded))
	require.Len(t, decoded.Occurrences, 3)
	assert.Equal(t, "nb;tone=formal", decoded.Occurrences[2].Locale)
}
