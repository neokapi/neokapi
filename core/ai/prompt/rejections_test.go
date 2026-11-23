package prompt

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A block's rejected drafts reach the prompt as their own section, in the
// words a person reads, and move the context digest.
func TestContextRejections(t *testing.T) {
	c := Context{Rejections: []Rejection{
		{Reasons: []string{"the tone does not fit"}, Note: "Too formal."},
		{Text: "Bonjour madame", Note: "Still too formal."},
	}}
	assert.False(t, c.Empty())
	secs := c.sections()
	require.Len(t, secs, 1)
	assert.Equal(t, "review (2 rejected drafts of this block)", secs[0].Origin)
	assert.Equal(t, "rejected draft: (its wording is no longer held)\nwhy: the tone does not fit\nreviewer's note: Too formal.\n\n"+
		"rejected draft: Bonjour madame\nreviewer's note: Still too formal.", secs[0].Text)

	edited := c
	edited.Rejections = []Rejection{c.Rejections[0], {Text: "Bonjour madame", Note: "Use tu."}}
	assert.NotEqual(t, c.Digest(), edited.Digest(), "a changed note changes what the model is asked")
}

// A batch carries each segment's rejected drafts in its own payload entry,
// with the rule stated once.
func TestBatchCarriesRejectedDrafts(t *testing.T) {
	segs := BatchSegments([]string{"Hello", "Bye"})
	segs[0].RejectedDrafts = []Rejection{{Text: "Bonjour madame", Reasons: []string{"the tone does not fit"}}}
	turns := Translate{SourceLocale: "en", TargetLocale: "fr"}.Batch(segs)
	var all strings.Builder
	for _, tr := range turns {
		for _, s := range tr.Sections {
			all.WriteString(s.Text)
		}
	}
	assert.Contains(t, all.String(), "`rejected_drafts`")
	assert.Contains(t, all.String(), `"rejected": "Bonjour madame"`)
	assert.Equal(t, 1, strings.Count(all.String(), "rejected_drafts`"), "the rule is stated once")
}
