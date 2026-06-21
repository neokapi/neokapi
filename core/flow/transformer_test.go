package flow_test

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// srcUpperTool is a transformer producing an opaque uppercase rewrite.
func srcUpperTool() *tool.BaseTool {
	return &tool.BaseTool{
		ToolName: "src-upper",
		Transform: func(v tool.BlockView) (tool.EditPlan, error) {
			upper := strings.ToUpper(v.SourceText())
			return tool.EditPlan{ReplaceAll: &upper}, nil
		},
	}
}

// recordSourceTool is an annotator that records the source text it observes,
// so a test can assert what a downstream step saw.
func recordSourceTool() *tool.BaseTool {
	return &tool.BaseTool{
		ToolName: "record-src",
		Annotate: func(v tool.BlockView) error {
			v.SetProperty("seen-source", v.SourceText())
			return nil
		},
	}
}

// termTagTool is an annotator that tags a term span over [from,to) of the
// source's flattened text — a stand-in for entity-extract / term lookup.
func termTagTool(id string, from, to int) *tool.BaseTool {
	return &tool.BaseTool{
		ToolName: "tag",
		Annotate: func(v tool.BlockView) error {
			v.AddOverlaySpan(model.OverlayTerm, model.Span{ID: id, Range: model.RunRangeFor(v.SourceRuns(), from, to)})
			return nil
		},
	}
}

// TestTransformer_AnnotatorFeedsTransform_EndToEnd drives an ordered chain
// [annotator, structured transformer, annotator] through the executor and
// asserts the first annotator's overlay survives the transformer's source
// rewrite — the applier rebases it — into the downstream step.
func TestTransformer_AnnotatorFeedsTransform_EndToEnd(t *testing.T) {
	// trim deletes the leading "hello " (runes 0..6); the applier rebases.
	trim := &tool.BaseTool{ToolName: "trim", Transform: func(v tool.BlockView) (tool.EditPlan, error) {
		return tool.EditPlan{
			NewRuns: []model.Run{{Text: &model.TextRun{Text: "world"}}},
			Edits:   []model.RunEdit{{Start: 0, End: 6, NewLen: 0}},
		}, nil
	}}
	var seenSpans int
	var seenText string
	rec := &tool.BaseTool{ToolName: "rec", Annotate: func(v tool.BlockView) error {
		seenSpans = len(v.OverlaySpans(model.OverlayTerm))
		seenText = v.SourceText()
		return nil
	}}
	f, err := flow.NewFlow("rebase-e2e").
		AddTool(termTagTool("t1", 6, 11)). // term over "world"
		AddTool(trim).
		AddTool(rec).
		Build()
	require.NoError(t, err)

	ex := flow.NewExecutor()
	in, out, wait := ex.ExecuteWithChannels(t.Context(), f)
	block := model.NewBlock("b1", "hello world")
	block.Translatable = true
	go func() {
		in <- &model.Part{Type: model.PartBlock, Resource: block}
		close(in)
	}()
	for range out { //nolint:revive // drain
	}
	require.NoError(t, wait())

	assert.Equal(t, "world", seenText)
	assert.Equal(t, 1, seenSpans, "the term overlay survived the rewrite, rebased onto the new runs")
}

// TestTransformer_SettlesSourceForLaterSteps asserts in-order application: a
// transformer ahead of an annotator settles the source before the annotator
// observes it — no structural stage needed (AD-006).
func TestTransformer_SettlesSourceForLaterSteps(t *testing.T) {
	f, err := flow.NewFlow("ordered").
		AddTool(srcUpperTool()).
		AddTool(recordSourceTool()).
		Build()
	require.NoError(t, err)

	executor := flow.NewExecutor()
	in, out, wait := executor.ExecuteWithChannels(t.Context(), f)

	block := model.NewBlock("b1", "hello")
	block.Translatable = true
	go func() {
		in <- &model.Part{Type: model.PartBlock, Resource: block}
		close(in)
	}()

	var got *model.Part
	for p := range out {
		got = p
	}
	require.NoError(t, wait())

	rb := got.Resource.(*model.Block)
	// The annotator observed the settled (uppercased) source — the transformer
	// applied inline, in order, before the next step saw the block.
	assert.Equal(t, "HELLO", rb.Properties["seen-source"])
	assert.Equal(t, "HELLO", rb.SourceText())
}
