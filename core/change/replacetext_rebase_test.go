package change_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

func itemsPlural(one, other string) model.Run {
	return model.Run{Plural: &model.PluralRun{Pivot: "n", Forms: map[model.PluralForm][]model.Run{
		model.PluralOne:   {model.TextR(one)},
		model.PluralOther: {model.TextR(other)},
	}}}
}

func pluralPath(form model.PluralForm) model.RunPath {
	return model.RunPath{{Kind: model.StepIndex, Index: 1}, {Kind: model.StepPlural, PluralForm: form}}
}

// One replace_text that edits the top level and a plural's branches keeps the
// spans neither touched: the overlays are rebased by the edits themselves, the
// top-level ones and the structure the branch edits changed, rather than by
// the one region the old and new flattened text differ in, which swallowed
// every word between the first edit and the plural.
func TestReplaceText_KeepsSpansAcrossTopLevelAndBranchEdits(t *testing.T) {
	b := model.NewRunsBlock("b", []model.Run{model.TextR("Hello world "), itemsPlural("one item", "many item"), model.TextR(" and bye")})
	b.AddOverlaySpan(model.OverlayTerm, model.Span{ID: "world", Range: model.SpanAnchor(model.RunPos{Run: 0, Offset: 6}, model.RunPos{Run: 0, Offset: 11})})
	b.AddOverlaySpan(model.OverlayTerm, model.Span{ID: "bye", Range: model.SpanAnchor(model.RunPos{Run: 2, Offset: 5}, model.RunPos{Run: 2, Offset: 8})})

	res := apply(t, b, person, replace("", "*",
		find("Hello", "Hi"),
		change.TextEdit{Path: pluralPath(model.PluralOne), Find: new("item"), Text: "thing"},
		change.TextEdit{Path: pluralPath(model.PluralOther), Find: new("item"), Text: "things"},
	))
	requireApplied(t, res)

	assert.Equal(t, "Hi world many things and bye", model.RunsText(b.SourceRuns()))
	for id, want := range map[string]string{"world": "world", "bye": "bye"} {
		sp := b.OverlaySpan(model.OverlayTerm, id)
		require.NotNil(t, sp, "the %s span", id)
		assert.Equal(t, want, model.RunsText(sp.Range.ExtractRuns(b.SourceRuns())))
	}
}

// An edit right after a plural leaves the plural out of the region it
// rebases: a span ending before the plural keeps its end.
func TestReplaceText_AnEditAfterAPluralLeavesItOutOfTheRebase(t *testing.T) {
	b := model.NewRunsBlock("b", []model.Run{model.TextR("You have "), itemsPlural("one item", "many items"), model.TextR(" left")})
	b.AddOverlaySpan(model.OverlayTerm, model.Span{ID: "count", Range: model.SpanAnchor(model.RunPos{Run: 0, Offset: 4}, model.RunPos{Run: 2})})

	requireApplied(t, apply(t, b, person, replace("", "*", find(" left", " remaining"))))

	sp := b.OverlaySpan(model.OverlayTerm, "count")
	require.NotNil(t, sp)
	assert.Equal(t, "have many items", model.RunsText(sp.Range.ExtractRuns(b.SourceRuns())))
}

// replace_text reads each sequence once, so an operation with an edit per
// word of a long text applies in time linear in the text.
func TestReplaceText_ManyEditsScaleWithTheText(t *testing.T) {
	text := strings.Repeat("grind the coffee ", 12_000) // 204,000 code points
	b := model.NewRunsBlock("b", []model.Run{model.TextR(text)})
	var edits []change.TextEdit
	for at := 0; at < len(text); at += len("grind the coffee ") {
		edits = append(edits, span(at+6, at+9, "THE"))
	}

	start := time.Now()
	requireApplied(t, apply(t, b, person, replace("", "*", edits...)))
	assert.Less(t, time.Since(start), 5*time.Second)
	assert.Equal(t, strings.ReplaceAll(text, " the ", " THE "), model.RunsText(b.SourceRuns()))
}
