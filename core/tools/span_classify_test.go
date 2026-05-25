package tools

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runSpanClassify drives a single Part through the tool's Process (span-classify
// is a Transform handler now, so it can't be invoked via HandleBlockFn).
func runSpanClassify(t *testing.T, tl interface {
	Process(context.Context, <-chan *model.Part, chan<- *model.Part) error
}, part *model.Part) *model.Part {
	t.Helper()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- part
	close(in)
	require.NoError(t, tl.Process(t.Context(), in, out))
	close(out)
	return <-out
}

// pairedRuns builds a `<open>middle</close>` Run sequence with the given
// type and tag data, used by the span-classify tests.
func pairedRuns(typ, openData, middle, closeData string) []model.Run {
	return []model.Run{
		{PcOpen: &model.PcOpenRun{ID: "1", Type: typ, Data: openData}},
		{Text: &model.TextRun{Text: middle}},
		{PcClose: &model.PcCloseRun{ID: "1", Type: typ, Data: closeData}},
	}
}

// pairedRunsSubType is pairedRuns with a non-empty SubType on both halves.
func pairedRunsSubType(typ, subType, openData, middle, closeData string) []model.Run {
	return []model.Run{
		{PcOpen: &model.PcOpenRun{ID: "1", Type: typ, SubType: subType, Data: openData}},
		{Text: &model.TextRun{Text: middle}},
		{PcClose: &model.PcCloseRun{ID: "1", Type: typ, SubType: subType, Data: closeData}},
	}
}

// firstPcOpen returns the first PcOpenRun in a Run sequence, or nil.
func firstPcOpen(runs []model.Run) *model.PcOpenRun {
	for _, r := range runs {
		if r.PcOpen != nil {
			return r.PcOpen
		}
	}
	return nil
}

// firstPcClose returns the first PcCloseRun in a Run sequence, or nil.
func firstPcClose(runs []model.Run) *model.PcCloseRun {
	for _, r := range runs {
		if r.PcClose != nil {
			return r.PcClose
		}
	}
	return nil
}

// firstPh returns the first PlaceholderRun in a Run sequence, or nil.
func firstPh(runs []model.Run) *model.PlaceholderRun {
	for _, r := range runs {
		if r.Ph != nil {
			return r.Ph
		}
	}
	return nil
}

func TestSpanClassifyFromData(t *testing.T) {
	t.Parallel()
	block := model.NewBlock("1", "")
	block.Source = append(pairedRuns("code:markup", "<b>", "Hello", "</b>"), model.Run{Text: &model.TextRun{Text: " world"}})
	block.Translatable = true

	part := &model.Part{Type: model.PartBlock, Resource: block}

	tool := NewSpanClassifyTool(&SpanClassifyConfig{})
	result := runSpanClassify(t, tool, part)

	runs := result.Resource.(*model.Block).Source
	open := firstPcOpen(runs)
	cls := firstPcClose(runs)
	require.NotNil(t, open)
	require.NotNil(t, cls)

	assert.Equal(t, "fmt:bold", open.Type)
	assert.Equal(t, "fmt:bold", cls.Type)
	require.NotNil(t, open.Constraints)
	assert.True(t, open.Constraints.Deletable)
	assert.Equal(t, "[B]", open.Disp)
	// PcClose runs don't carry Disp per RFC 0001 §Block model; the
	// closing tag's display string is derived from the vocabulary at
	// render time (see klf.RenderBlockHTML).
}

func TestSpanClassifyFromSubType(t *testing.T) {
	t.Parallel()
	block := model.NewBlock("1", "")
	block.Source = pairedRunsSubType("code:markup", "okapi:italic", "<em>", "text", "</em>")
	block.Translatable = true

	part := &model.Part{Type: model.PartBlock, Resource: block}

	tool := NewSpanClassifyTool(&SpanClassifyConfig{})
	result := runSpanClassify(t, tool, part)

	runs := result.Resource.(*model.Block).Source
	open := firstPcOpen(runs)
	cls := firstPcClose(runs)
	require.NotNil(t, open)
	require.NotNil(t, cls)
	assert.Equal(t, "fmt:italic", open.Type)
	assert.Equal(t, "fmt:italic", cls.Type)
}

func TestSpanClassifyBreakPlaceholder(t *testing.T) {
	t.Parallel()
	block := model.NewBlock("1", "")
	block.Source = []model.Run{
		{Text: &model.TextRun{Text: "line one"}},
		{Ph: &model.PlaceholderRun{ID: "1", Type: "code:markup", Data: "<br/>"}},
		{Text: &model.TextRun{Text: "line two"}},
	}
	block.Translatable = true

	part := &model.Part{Type: model.PartBlock, Resource: block}

	tool := NewSpanClassifyTool(&SpanClassifyConfig{})
	result := runSpanClassify(t, tool, part)

	ph := firstPh(result.Resource.(*model.Block).Source)
	require.NotNil(t, ph)
	assert.Equal(t, "struct:break", ph.Type)
	require.NotNil(t, ph.Constraints)
	assert.False(t, ph.Constraints.Deletable)
	assert.Equal(t, "[BR]", ph.Disp)
}

func TestSpanClassifyUnknownType(t *testing.T) {
	t.Parallel()
	block := model.NewBlock("1", "")
	block.Source = pairedRuns("code:markup", "<custom-tag>", "content", "</custom-tag>")
	block.Translatable = true

	part := &model.Part{Type: model.PartBlock, Resource: block}

	tool := NewSpanClassifyTool(&SpanClassifyConfig{})
	result := runSpanClassify(t, tool, part)

	runs := result.Resource.(*model.Block).Source
	open := firstPcOpen(runs)
	cls := firstPcClose(runs)
	require.NotNil(t, open)
	require.NotNil(t, cls)
	// Unknown tags stay as code:markup.
	assert.Equal(t, "code:markup", open.Type)
	assert.Equal(t, "code:markup", cls.Type)
}

func TestSpanClassifySkipsNonMarkup(t *testing.T) {
	t.Parallel()
	block := model.NewBlock("1", "")
	block.Source = pairedRuns("fmt:bold", "<b>", "Hello", "</b>")
	block.Translatable = true

	part := &model.Part{Type: model.PartBlock, Resource: block}

	tool := NewSpanClassifyTool(&SpanClassifyConfig{})
	result := runSpanClassify(t, tool, part)

	runs := result.Resource.(*model.Block).Source
	open := firstPcOpen(runs)
	cls := firstPcClose(runs)
	require.NotNil(t, open)
	require.NotNil(t, cls)
	// Already classified spans are not modified.
	assert.Equal(t, "fmt:bold", open.Type)
	assert.Equal(t, "fmt:bold", cls.Type)
}

func TestSpanClassifyTargetFragments(t *testing.T) {
	t.Parallel()
	block := model.NewBlock("1", "")
	block.Source = []model.Run{{Text: &model.TextRun{Text: "Hello"}}}
	block.SetTargetRuns("fr", pairedRuns("code:markup", "<i>", "Bonjour", "</i>"))
	block.Translatable = true

	part := &model.Part{Type: model.PartBlock, Resource: block}

	tool := NewSpanClassifyTool(&SpanClassifyConfig{})
	result := runSpanClassify(t, tool, part)

	runs := result.Resource.(*model.Block).TargetRuns("fr")
	open := firstPcOpen(runs)
	cls := firstPcClose(runs)
	require.NotNil(t, open)
	require.NotNil(t, cls)
	assert.Equal(t, "fmt:italic", open.Type)
	assert.Equal(t, "fmt:italic", cls.Type)
}

func TestExtractTagName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		data string
		want string
	}{
		{"<b>", "b"},
		{"</b>", "b"},
		{"<br/>", "br"},
		{`<a href="url">`, "a"},
		{"<img/>", "img"},
		{"plain text", ""},
		{"", ""},
		{"<Custom-Tag>", "Custom"},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, extractTagName(tc.data), "data=%q", tc.data)
	}
}
