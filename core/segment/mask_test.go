package segment

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func text(s string) model.Run { return model.Run{Text: &model.TextRun{Text: s}} }
func ph(id string) model.Run {
	return model.Run{Ph: &model.PlaceholderRun{ID: id, Equiv: "{" + id + "}"}}
}

// segText renders the runs covered by a span as plain text, for assertions.
func segText(runs []model.Run, sp model.Span) string {
	return model.RunsText(sp.Range.ExtractRuns(runs))
}

func TestFlattenSpans_PlainText(t *testing.T) {
	runs := []model.Run{text("One. Two. Three.")}
	fl := Flatten(runs, MaskOptions{})
	assert.Equal(t, "One. Two. Three.", fl.Text())

	// Break after "One. " (rune 5) and "Two. " (rune 10).
	spans := fl.Spans([]int{5, 10})
	require.Len(t, spans, 3)
	assert.Equal(t, "One. ", segText(runs, spans[0]))
	assert.Equal(t, "Two. ", segText(runs, spans[1]))
	assert.Equal(t, "Three.", segText(runs, spans[2]))
	assert.Equal(t, "s1", spans[0].ID)
	assert.Equal(t, "s3", spans[2].ID)
}

func TestFlattenSpans_CodesAreZeroWidth(t *testing.T) {
	// Codes contribute nothing to the masked text by default; the run mapping
	// stays correct around them.
	runs := []model.Run{text("Hi. "), ph("br"), text("Bye.")}
	fl := Flatten(runs, MaskOptions{})
	assert.Equal(t, "Hi. Bye.", fl.Text())

	spans := fl.Spans([]int{4}) // break after "Hi. "
	require.Len(t, spans, 2)
	assert.Equal(t, "Hi. ", segText(runs, spans[0]))
	// The placeholder run sits at the boundary; it attaches to the following
	// segment per the run-position rule.
	assert.Equal(t, "Bye.", segText(runs, spans[1]))
}

func TestFlattenSpans_TreatIsolatedCodesAsWhitespace(t *testing.T) {
	runs := []model.Run{text("Hi."), ph("br"), text("Bye.")}
	plain := Flatten(runs, MaskOptions{})
	assert.Equal(t, "Hi.Bye.", plain.Text())

	ws := Flatten(runs, MaskOptions{TreatIsolatedCodesAsWhitespace: true})
	assert.Equal(t, "Hi. Bye.", ws.Text(), "isolated code stands in for a space")
	// The injected space is non-real, so a break at the space still maps
	// cleanly to a real-text boundary.
	spans := ws.Spans([]int{4})
	require.Len(t, spans, 2)
	assert.Equal(t, "Hi.", segText(runs, spans[0]))
	assert.Equal(t, "Bye.", segText(runs, spans[1]))
}

func TestFlattenSpans_Trim(t *testing.T) {
	runs := []model.Run{text("One.  Two.")}
	fl := Flatten(runs, MaskOptions{TrimTrailingWS: true, TrimLeadingWS: true})
	spans := fl.Spans([]int{6}) // break inside the double space
	require.Len(t, spans, 2)
	assert.Equal(t, "One.", segText(runs, spans[0]), "trailing whitespace trimmed")
	assert.Equal(t, "Two.", segText(runs, spans[1]), "leading whitespace trimmed")
}

func TestFlattenSpans_NonASCII(t *testing.T) {
	runs := []model.Run{text("Café. Déjà.")}
	fl := Flatten(runs, MaskOptions{})
	spans := fl.Spans([]int{6}) // after "Café. "
	require.Len(t, spans, 2)
	assert.Equal(t, "Café. ", segText(runs, spans[0]))
	assert.Equal(t, "Déjà.", segText(runs, spans[1]))
}

func TestFlattenSpans_OutOfRangeIgnored(t *testing.T) {
	runs := []model.Run{text("Solo.")}
	fl := Flatten(runs, MaskOptions{})
	spans := fl.Spans([]int{0, 5, 99, -3}) // all invalid as internal breaks
	require.Len(t, spans, 1)
	assert.Equal(t, "Solo.", segText(runs, spans[0]))
}

func TestRegistry(t *testing.T) {
	assert.False(t, HasEngine("nope-engine"))
	_, err := NewEngine("nope-engine", Config{})
	assert.ErrorIs(t, err, ErrEngineUnavailable)
}
