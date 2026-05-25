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
	// An annotator may not sit in the source-transform stage.
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
