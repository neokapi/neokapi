package tool_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const frFR = model.LocaleID("fr-FR")

// twoSegBlock builds a two-run block split into two source segments on the run
// boundary; concatenating the segment texts reproduces the source.
func twoSegBlock(s1, s2 string) *model.Block {
	b := model.NewRunsBlock("b1", []model.Run{
		{Text: &model.TextRun{Text: s1}},
		{Text: &model.TextRun{Text: s2}},
	})
	b.SetSegmentation(nil, []model.Span{
		{ID: "s1", Range: model.RunRange{StartRun: 0, EndRun: 1}},
		{ID: "s2", Range: model.RunRange{StartRun: 1, EndRun: 2}},
	})
	return b
}

func unitTexts(units []tool.Unit) []string {
	out := make([]string, len(units))
	for i, u := range units {
		out[i] = model.RunsText(u.SourceRuns())
	}
	return out
}

func TestSourceUnitsWholeBlock(t *testing.T) {
	t.Parallel()
	b := model.NewRunsBlock("b1", []model.Run{{Text: &model.TextRun{Text: "Hello world."}}})
	v := tool.NewBlockView(b)

	var units []tool.Unit
	for u := range v.SourceUnits(model.LayerPrimary) {
		units = append(units, u)
	}
	require.Len(t, units, 1)
	assert.Equal(t, 0, units[0].Index())
	assert.Nil(t, units[0].Range(), "whole-block unit has a nil range")
	assert.False(t, units[0].Ignorable())
	assert.Equal(t, "Hello world.", model.RunsText(units[0].SourceRuns()))
}

func TestSourceUnitsEmptySourceYieldsNothing(t *testing.T) {
	t.Parallel()
	b := model.NewRunsBlock("b1", nil)
	v := tool.NewBlockView(b)

	count := 0
	for range v.SourceUnits(model.LayerPrimary) {
		count++
	}
	assert.Equal(t, 0, count, "an empty source yields no units")
}

func TestSourceUnitsSegmented(t *testing.T) {
	t.Parallel()
	b := twoSegBlock("Hello world. ", "Goodbye.")
	v := tool.NewBlockView(b)

	var units []tool.Unit
	for u := range v.SourceUnits(model.LayerPrimary) {
		units = append(units, u)
	}
	require.Len(t, units, 2)
	assert.Equal(t, []string{"Hello world. ", "Goodbye."}, unitTexts(units))
	assert.Equal(t, 0, units[0].Index())
	assert.Equal(t, 1, units[1].Index())
	require.NotNil(t, units[0].Range())
	assert.Equal(t, 0, units[0].Range().StartRun)
	assert.Equal(t, 1, units[1].Range().StartRun)
}

func TestSourceUnitsRangeCopyIsIsolated(t *testing.T) {
	t.Parallel()
	b := twoSegBlock("Hello world. ", "Goodbye.")
	v := tool.NewBlockView(b)

	var first tool.Unit
	for u := range v.SourceUnits(model.LayerPrimary) {
		first = u
		break
	}
	require.NotNil(t, first.Range())
	first.Range().StartRun = 99 // mutate the returned copy

	// The block's overlay span is untouched.
	assert.Equal(t, 0, b.SourceSegmentation().Spans[0].Range.StartRun)
}

func TestSourceUnitsEarlyStop(t *testing.T) {
	t.Parallel()
	b := twoSegBlock("Hello world. ", "Goodbye.")
	v := tool.NewBlockView(b)

	count := 0
	for range v.SourceUnits(model.LayerPrimary) {
		count++
		break
	}
	assert.Equal(t, 1, count, "break stops iteration after one unit")
}

func TestSourceUnitsIgnorable(t *testing.T) {
	t.Parallel()
	b := model.NewRunsBlock("b1", []model.Run{
		{Text: &model.TextRun{Text: "Hello."}},
		{Text: &model.TextRun{Text: " "}},
	})
	b.SetSegmentation(nil, []model.Span{
		{ID: "s1", Range: model.RunRange{StartRun: 0, EndRun: 1}},
		{ID: "ws", Range: model.RunRange{StartRun: 1, EndRun: 2}, Props: map[string]string{model.SpanPropIgnorable: "true"}},
	})
	v := tool.NewBlockView(b)

	var units []tool.Unit
	for u := range v.SourceUnits(model.LayerPrimary) {
		units = append(units, u)
	}
	require.Len(t, units, 2)
	assert.False(t, units[0].Ignorable())
	assert.True(t, units[1].Ignorable(), "the whitespace span is ignorable")
}

func TestTargetUnitsWholeBlockCommit(t *testing.T) {
	t.Parallel()
	b := model.NewRunsBlock("b1", []model.Run{{Text: &model.TextRun{Text: "Hello."}}})
	v := tool.NewVariantView(b)

	for u := range v.TargetUnits(frFR, model.LayerPrimary) {
		u.SetTargetRuns(frFR, []model.Run{{Text: &model.TextRun{Text: "Bonjour."}}})
	}
	assert.Equal(t, "Bonjour.", b.TargetText(frFR))
}

func TestTargetUnitsWholeBlockNoWriteNoCommit(t *testing.T) {
	t.Parallel()
	b := model.NewRunsBlock("b1", []model.Run{{Text: &model.TextRun{Text: "Hello."}}})
	v := tool.NewVariantView(b)

	for range v.TargetUnits(frFR, model.LayerPrimary) {
		// deliberately write nothing
	}
	assert.False(t, b.HasTarget(frFR), "an untouched whole-block unit commits nothing")
}

func TestTargetUnitsSegmentedAssembly(t *testing.T) {
	t.Parallel()
	b := twoSegBlock("Hello world. ", "Goodbye.")
	v := tool.NewVariantView(b)

	want := map[int]string{0: "Bonjour le monde. ", 1: "Au revoir."}
	for u := range v.TargetUnits(frFR, model.LayerPrimary) {
		u.SetTargetRuns(frFR, []model.Run{{Text: &model.TextRun{Text: want[u.Index()]}}})
	}
	assert.Equal(t, "Bonjour le monde. Au revoir.", b.TargetText(frFR),
		"per-segment writes assemble into the block target in span order")
}

func TestTargetUnitsPartialWriteCommitsNothing(t *testing.T) {
	t.Parallel()
	b := twoSegBlock("Hello world. ", "Goodbye.")
	v := tool.NewVariantView(b)

	for u := range v.TargetUnits(frFR, model.LayerPrimary) {
		if u.Index() == 0 {
			u.SetTargetRuns(frFR, []model.Run{{Text: &model.TextRun{Text: "Bonjour le monde. "}}})
		}
		// segment 1 left unwritten
	}
	assert.False(t, b.HasTarget(frFR),
		"a non-ignorable segment left unwritten means all-or-nothing: no commit")
}

func TestTargetUnitsIgnorablePreservedVerbatim(t *testing.T) {
	t.Parallel()
	b := model.NewRunsBlock("b1", []model.Run{
		{Text: &model.TextRun{Text: "Hello."}},
		{Text: &model.TextRun{Text: " "}},
	})
	b.SetSegmentation(nil, []model.Span{
		{ID: "s1", Range: model.RunRange{StartRun: 0, EndRun: 1}},
		{ID: "ws", Range: model.RunRange{StartRun: 1, EndRun: 2}, Props: map[string]string{model.SpanPropIgnorable: "true"}},
	})
	v := tool.NewVariantView(b)

	for u := range v.TargetUnits(frFR, model.LayerPrimary) {
		if !u.Ignorable() {
			u.SetTargetRuns(frFR, []model.Run{{Text: &model.TextRun{Text: "Bonjour."}}})
		}
	}
	assert.Equal(t, "Bonjour. ", b.TargetText(frFR),
		"ignorable span is preserved verbatim from source; writing the rest still commits")
}

func TestTargetUnitsEarlyStopCommitsNothing(t *testing.T) {
	t.Parallel()
	b := twoSegBlock("Hello world. ", "Goodbye.")
	v := tool.NewVariantView(b)

	for u := range v.TargetUnits(frFR, model.LayerPrimary) {
		u.SetTargetRuns(frFR, []model.Run{{Text: &model.TextRun{Text: "x"}}})
		break // stop early
	}
	assert.False(t, b.HasTarget(frFR), "stopping early commits nothing")
}

func TestTargetUnitsReadsTargetSegment(t *testing.T) {
	t.Parallel()
	b := twoSegBlock("Hello world. ", "Goodbye.")
	b.SetTargetRuns(frFR, []model.Run{
		{Text: &model.TextRun{Text: "Bonjour le monde. "}},
		{Text: &model.TextRun{Text: "Au revoir."}},
	})
	// Mirror the source segmentation onto the target side.
	key := model.Variant(frFR)
	b.SetSegmentation(&key, []model.Span{
		{ID: "s1", Range: model.RunRange{StartRun: 0, EndRun: 1}},
		{ID: "s2", Range: model.RunRange{StartRun: 1, EndRun: 2}},
	})
	v := tool.NewBlockView(b)

	got := map[int]string{}
	for u := range v.SourceUnits(model.LayerPrimary) {
		got[u.Index()] = model.RunsText(u.TargetRuns(frFR))
	}
	assert.Equal(t, "Bonjour le monde. ", got[0])
	assert.Equal(t, "Au revoir.", got[1])
}
