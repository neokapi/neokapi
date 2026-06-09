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

// srcUpperTool is a source-transform that uppercases the source text.
func srcUpperTool() *tool.BaseTool {
	return &tool.BaseTool{
		ToolName: "src-upper",
		Transform: func(v tool.SourceView) error {
			v.SetSourceText(strings.ToUpper(v.SourceText()))
			return nil
		},
	}
}

// recordSourceTool is an annotator that records the source text it observes,
// so a test can assert what the downstream stage saw.
func recordSourceTool() *tool.BaseTool {
	return &tool.BaseTool{
		ToolName: "record-src",
		Annotate: func(v tool.BlockView) error {
			v.SetProperty("seen-source", v.SourceText())
			return nil
		},
	}
}

func TestSourceTransformStage_Validation(t *testing.T) {
	// An annotator-only settle stage settles nothing → rejected.
	_, err := flow.NewFlow("bad").AddSourceTransform(recordSourceTool()).Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "source-transform")

	// A Transform-capable tool is accepted.
	_, err = flow.NewFlow("ok").
		AddSourceTransform(srcUpperTool()).
		AddTool(recordSourceTool()).
		Build()
	require.NoError(t, err)
}

// termTagTool is an annotator that tags a term span over [from,to) of the
// source's flattened text — a stand-in for ai-entity-extract / term lookup.
func termTagTool(id string, from, to int) *tool.BaseTool {
	return &tool.BaseTool{
		ToolName: "tag",
		Annotate: func(v tool.BlockView) error {
			v.AddOverlaySpan(model.OverlayTerm, model.Span{ID: id, Range: model.RunRangeFor(v.SourceRuns(), from, to)})
			return nil
		},
	}
}

func TestSettleStage_AnnotatorMayPrecedeTransform(t *testing.T) {
	// An annotator may sit ahead of the settling transform so its overlay can
	// drive the transform (e.g. entity-extract → redact).
	f, err := flow.NewFlow("settle").
		AddSourceTransform(termTagTool("t1", 0, 5)). // annotator
		AddSourceTransform(srcUpperTool()).          // settling transform
		AddTool(recordSourceTool()).
		Build()
	require.NoError(t, err)
	pipe := f.Pipeline()
	require.Len(t, pipe, 3)
	assert.Equal(t, "tag", pipe[0].Name())
	assert.Equal(t, "src-upper", pipe[1].Name())
}

func TestSettleStage_RejectsTranslate(t *testing.T) {
	tr := &tool.BaseTool{ToolName: "translator", Translate: func(tool.TargetView) error { return nil }}
	_, err := flow.NewFlow("bad-translate").
		AddSourceTransform(tr).
		AddSourceTransform(srcUpperTool()).
		Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "annotator or a source-transform")
}

// TestSettleStage_AnnotatorFeedsTransform_EndToEnd drives a settle stage of
// [annotator, structured transform] through the executor and asserts the
// annotator's overlay survives the transform's source rewrite (rebased) into the
// main stage.
func TestSettleStage_AnnotatorFeedsTransform_EndToEnd(t *testing.T) {
	// trim deletes the leading "hello " (runes 0..6) and rebases overlays.
	trim := &tool.BaseTool{ToolName: "trim", Transform: func(v tool.SourceView) error {
		old := v.SourceRuns()
		v.SetSourceText("world")
		v.RemapSourceOverlays(old, []model.RunEdit{{Start: 0, End: 6, NewLen: 0}})
		return nil
	}}
	var seenSpans int
	var seenText string
	rec := &tool.BaseTool{ToolName: "rec", Annotate: func(v tool.BlockView) error {
		seenSpans = len(v.OverlaySpans(model.OverlayTerm))
		seenText = v.SourceText()
		return nil
	}}
	f, err := flow.NewFlow("settle-e2e").
		AddSourceTransform(termTagTool("t1", 6, 11)). // term over "world"
		AddSourceTransform(trim).
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

func TestSourceTransformStage_RunsBeforeMainTools(t *testing.T) {
	f, err := flow.NewFlow("pre").
		AddSourceTransform(srcUpperTool()).
		AddTool(recordSourceTool()).
		Build()
	require.NoError(t, err)

	// Pipeline puts the source-transform stage first.
	pipe := f.Pipeline()
	require.Len(t, pipe, 2)
	assert.Equal(t, "src-upper", pipe[0].Name())
	assert.Equal(t, "record-src", pipe[1].Name())

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
	// The annotator observed the settled (uppercased) source — proof the
	// source-transform stage ran to completion first.
	assert.Equal(t, "HELLO", rb.Properties["seen-source"])
	assert.Equal(t, "HELLO", rb.SourceText())
}
