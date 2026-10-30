package change

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/model"
)

// Text split across adjacent runs says what the same text in one run says. A
// file records no split, so a change that only splits or joins text is the
// edition as it was. unredact puts each withheld value back as a run of its
// own, and the reader of the file returns one.
func TestSameContent_JoinsAdjacentTextRuns(t *testing.T) {
	split := []model.Run{model.TextR("Hello "), model.TextR("Mr Bean"), model.TextR("!")}
	whole := []model.Run{model.TextR("Hello Mr Bean!")}
	assert.True(t, sameContent(split, whole))
	assert.True(t, sameContent(whole, split))

	// A code between two texts keeps them apart.
	coded := []model.Run{model.TextR("Hello "), {Ph: &model.PlaceholderRun{ID: "1"}}, model.TextR("Mr Bean!")}
	assert.False(t, sameContent(coded, whole))
	assert.False(t, sameContent([]model.Run{model.TextR("Hello Mr Bean")}, whole))

	// The runs given are left as they were.
	assert.Equal(t, "Hello ", split[0].Text.Text)

	// Runs that differ in a field besides their text stay apart: a split
	// between translatable text and a do-not-translate span is a change.
	marked := []model.Run{model.TextR("run "), {Text: &model.TextRun{Text: "kapi check", NoTranslate: true}}}
	assert.False(t, sameContent(marked, []model.Run{model.TextR("run kapi check")}))
	assert.True(t, sameContent(marked, marked))
}
