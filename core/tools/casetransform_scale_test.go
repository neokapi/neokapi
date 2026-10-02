package tools_test

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
)

// longWordsBlock is a block of n characters of words around a bold span and a
// placeholder, with a term span over every tenth word before them.
func longWordsBlock(n int) *model.Block {
	words := strings.Repeat("grind the coffee beans now ", n/27+1)[:n]
	half := n / 2
	b := &model.Block{ID: "long", Translatable: true}
	b.SetSourceRuns([]model.Run{
		{Text: &model.TextRun{Text: words[:half]}},
		{PcOpen: &model.PcOpenRun{ID: "1", Type: "fmt:bold", Data: "<b>"}},
		{Text: &model.TextRun{Text: "bold"}},
		{PcClose: &model.PcCloseRun{ID: "1", Type: "fmt:bold", Data: "</b>"}},
		{Ph: &model.PlaceholderRun{ID: "2", Type: "x-variable", Data: "{n}"}},
		{Text: &model.TextRun{Text: words[half:]}},
	})
	for at := 0; at+5 < half; at += 270 {
		b.AddOverlaySpan(model.OverlayTerm, model.Span{ID: "t" + strconv.Itoa(at),
			Range: model.SpanAnchor(model.RunPos{Run: 0, Offset: at}, model.RunPos{Run: 0, Offset: at + 5})})
	}
	return b
}

// case-transform makes one edit per run of changed characters, and the edits
// are applied in time linear in the block: upper-casing 200,000 characters of
// words, an edit per word, takes milliseconds. Resolving each edit against a
// fresh read of the whole text took seconds at a quarter of this size.
func TestCaseTransformScalesWithTheBlock(t *testing.T) {
	b := longWordsBlock(200_000)
	terms := len(b.OverlayOf(model.OverlayTerm).Spans)
	want := strings.ToUpper(model.RunsText(b.SourceRuns()))
	tl := tools.NewCaseTransformTool(&tools.CaseTransformConfig{Mode: tools.CaseUpper, ApplySource: true})

	start := time.Now()
	got := runTransform(t, tl, b)
	elapsed := time.Since(start)

	assert.Less(t, elapsed, 5*time.Second, "upper-casing 200,000 characters")
	assert.Equal(t, want, model.RunsText(got.SourceRuns()))
	text := model.RunsEditText(got.SourceRuns())
	assert.Contains(t, text, `<x id="1"/>BOLD<x id="/1"/><x id="2/"/>`)
	require.NotNil(t, got.OverlayOf(model.OverlayTerm))
	assert.Len(t, got.OverlayOf(model.OverlayTerm).Spans, terms, "every term span is kept")
}

func BenchmarkCaseTransformLongBlock(b *testing.B) {
	tl := tools.NewCaseTransformTool(&tools.CaseTransformConfig{Mode: tools.CaseUpper, ApplySource: true})
	for b.Loop() {
		b.StopTimer()
		blk := longWordsBlock(50_000)
		in := make(chan *model.Part, 1)
		out := make(chan *model.Part, 1)
		in <- &model.Part{Type: model.PartBlock, Resource: blk}
		close(in)
		b.StartTimer()
		if err := tl.Process(context.Background(), in, out); err != nil {
			b.Fatal(err)
		}
	}
}
