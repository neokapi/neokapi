package tool_test

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBaseToolPassThrough(t *testing.T) {
	bt := &tool.BaseTool{
		ToolName:        "pass-through",
		ToolDescription: "passes all parts through unchanged",
	}

	assert.Equal(t, "pass-through", bt.Name())
	assert.Equal(t, "passes all parts through unchanged", bt.Description())

	parts := []*model.Part{
		{Type: model.PartLayerStart, Resource: &model.Layer{ID: "doc1"}},
		{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Hello")},
		{Type: model.PartData, Resource: &model.Data{ID: "d1"}},
		{Type: model.PartBlock, Resource: model.NewBlock("tu2", "World")},
		{Type: model.PartLayerEnd, Resource: &model.Layer{ID: "doc1"}},
	}

	in := make(chan *model.Part, len(parts))
	out := make(chan *model.Part, len(parts))
	for _, p := range parts {
		in <- p
	}
	close(in)

	ctx := t.Context()
	err := bt.Process(ctx, in, out)
	close(out)
	require.NoError(t, err)

	var result []*model.Part
	for p := range out {
		result = append(result, p)
	}

	require.Len(t, result, len(parts))
	for i, p := range result {
		assert.Equal(t, parts[i].Type, p.Type)
		assert.Equal(t, parts[i].Resource, p.Resource)
	}
}

func TestBaseToolDispatch(t *testing.T) {
	var handledTypes []model.PartType

	bt := &tool.BaseTool{
		ToolName: "tracker",
		Annotate: func(tool.BlockView) error {
			handledTypes = append(handledTypes, model.PartBlock)
			return nil
		},
		HandleDataFn: func(part *model.Part) (*model.Part, error) {
			handledTypes = append(handledTypes, part.Type)
			return part, nil
		},
		HandleMediaFn: func(part *model.Part) (*model.Part, error) {
			handledTypes = append(handledTypes, part.Type)
			return part, nil
		},
		HandleLayerStartFn: func(part *model.Part) (*model.Part, error) {
			handledTypes = append(handledTypes, part.Type)
			return part, nil
		},
		HandleLayerEndFn: func(part *model.Part) (*model.Part, error) {
			handledTypes = append(handledTypes, part.Type)
			return part, nil
		},
	}

	parts := []*model.Part{
		{Type: model.PartLayerStart, Resource: &model.Layer{ID: "doc1"}},
		{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Hello")},
		{Type: model.PartData, Resource: &model.Data{ID: "d1"}},
		{Type: model.PartMedia, Resource: &model.Media{ID: "m1"}},
		{Type: model.PartBlock, Resource: model.NewBlock("tu2", "World")},
		{Type: model.PartLayerEnd, Resource: &model.Layer{ID: "doc1"}},
	}

	in := make(chan *model.Part, len(parts))
	out := make(chan *model.Part, len(parts))
	for _, p := range parts {
		in <- p
	}
	close(in)

	ctx := t.Context()
	err := bt.Process(ctx, in, out)
	close(out)
	require.NoError(t, err)

	expected := []model.PartType{
		model.PartLayerStart,
		model.PartBlock,
		model.PartData,
		model.PartMedia,
		model.PartBlock,
		model.PartLayerEnd,
	}
	assert.Equal(t, expected, handledTypes)
}

func TestBaseToolContextCancellation(t *testing.T) {
	bt := &tool.BaseTool{ToolName: "cancellable"}

	ctx, cancel := context.WithCancel(t.Context())
	in := make(chan *model.Part) // unbuffered, will block
	out := make(chan *model.Part, 10)

	done := make(chan error, 1)
	go func() {
		done <- bt.Process(ctx, in, out)
	}()

	cancel()
	err := <-done
	assert.ErrorIs(t, err, context.Canceled)
}

func TestBaseToolErrorPropagation(t *testing.T) {
	bt := &tool.BaseTool{
		ToolName: "error-tool",
		Annotate: func(tool.BlockView) error {
			return assert.AnError
		},
	}

	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Hello")}
	close(in)

	ctx := t.Context()
	err := bt.Process(ctx, in, out)
	require.Error(t, err)
}

func TestBaseToolModifyBlock(t *testing.T) {
	bt := &tool.BaseTool{
		ToolName: "uppercase",
		Translate: func(v tool.TargetView) error {
			if v.Translatable() {
				v.SetTargetText(model.LocaleFrench, "TRANSLATED")
			}
			return nil
		},
	}

	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	block := model.NewBlock("tu1", "Hello")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	ctx := t.Context()
	err := bt.Process(ctx, in, out)
	close(out)
	require.NoError(t, err)

	result := <-out
	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "TRANSLATED", resultBlock.TargetText(model.LocaleFrench))
}

func TestBaseToolConfig(t *testing.T) {
	bt := &tool.BaseTool{ToolName: "test"}
	assert.Nil(t, bt.Config())

	// nil config is ok
	err := bt.SetConfig(nil)
	require.NoError(t, err)
}

// TestImmutabilityGuard exercises the dev/test backstop in the typed-handler
// dispatch. Capability is enforced primarily by the parameter type — an
// Annotate handler simply has no source/target setters — so the backstop's
// remaining job is to catch in-place edits through the live run slices the
// view hands back. These cases mutate via those aliased slices to trip it.
func TestImmutabilityGuard(t *testing.T) {
	mkBlock := func() *model.Block {
		b := model.NewBlock("b1", "Hello world")
		b.SourceLocale = "en"
		return b
	}
	run := func(bt *tool.BaseTool, b *model.Block) error {
		in := make(chan *model.Part, 1)
		out := make(chan *model.Part, 1)
		in <- &model.Part{Type: model.PartBlock, Resource: b}
		close(in)
		return bt.Process(context.Background(), in, out)
	}
	oneSpanOverlay := []model.Span{{ID: "s1", Range: model.RunRange{StartRun: 0, EndRun: 1}}}

	t.Run("annotate handler mutating source via aliased slice is rejected", func(t *testing.T) {
		bt := &tool.BaseTool{ToolName: "bad-src"}
		bt.Annotate = func(v tool.BlockView) error {
			v.SourceRuns()[0].Text.Text = "changed" // in-place edit through the live slice
			return nil
		}
		require.ErrorContains(t, run(bt, mkBlock()), "changed source")
	})
	t.Run("transform handler may rewrite source", func(t *testing.T) {
		bt := &tool.BaseTool{ToolName: "src-xform"}
		bt.Transform = func(v tool.SourceView) error {
			v.SetSourceText("changed")
			return nil
		}
		require.NoError(t, run(bt, mkBlock()))
	})
	t.Run("annotate handler mutating target via aliased slice is rejected", func(t *testing.T) {
		bt := &tool.BaseTool{ToolName: "bad-tgt"}
		bt.Annotate = func(v tool.BlockView) error {
			v.TargetRuns("fr")[0].Text.Text = "changed" // in-place edit through the live slice
			return nil
		}
		b := mkBlock()
		b.SetTargetText("fr", "Bonjour")
		require.ErrorContains(t, run(bt, b), "changed target")
	})
	t.Run("translate handler may write target", func(t *testing.T) {
		bt := &tool.BaseTool{ToolName: "translator"}
		bt.Translate = func(v tool.TargetView) error {
			v.SetTargetText("fr", "Bonjour")
			return nil
		}
		require.NoError(t, run(bt, mkBlock()))
	})
	t.Run("annotate handler writing only overlays/properties is allowed", func(t *testing.T) {
		bt := &tool.BaseTool{ToolName: "analyzer"}
		bt.Annotate = func(v tool.BlockView) error {
			v.SetProperty("word-count", "2")
			v.SetSegmentation(nil, oneSpanOverlay)
			return nil
		}
		require.NoError(t, run(bt, mkBlock()))
	})
	t.Run("source transform leaving an overlay out of bounds is rejected", func(t *testing.T) {
		bt := &tool.BaseTool{ToolName: "late-xform"}
		bt.Transform = func(v tool.SourceView) error {
			v.SetSourceText("hi") // far shorter than the term span below
			return nil
		}
		b := mkBlock()
		// A term span covering "Hello world" (11 runes). Shrinking the source to
		// "hi" without dropping or rebasing the overlay leaves it dangling, which
		// the backstop must reject.
		b.AddOverlaySpan(model.OverlayTerm, model.Span{ID: "t1", Range: model.RunRange{EndRun: 0, EndOffset: 11}})
		require.ErrorContains(t, run(bt, b), "out of bounds")
	})
	t.Run("source transform rebasing its overlays is allowed", func(t *testing.T) {
		// The transform deletes the leading "Hello " (runes 0..6) and rebases the
		// term overlay over "world", so the surviving span still anchors in-bounds.
		bt := &tool.BaseTool{ToolName: "rebase-xform"}
		bt.Transform = func(v tool.SourceView) error {
			old := v.SourceRuns()
			v.SetSourceText("world")
			v.RemapSourceOverlays(old, []model.RunEdit{{Start: 0, End: 6, NewLen: 0}})
			return nil
		}
		b := mkBlock()
		b.AddOverlaySpan(model.OverlayTerm, model.Span{ID: "t1", Range: model.RunRangeFor(b.Source, 6, 11)})
		require.NoError(t, run(bt, b))
		// The span was rebased onto the new runs and now covers "world" (0..5).
		sp := b.OverlaySpan(model.OverlayTerm, "t1")
		require.NotNil(t, sp)
		s, e := sp.Range.TextSpan(b.Source)
		assert.Equal(t, 0, s)
		assert.Equal(t, 5, e)
	})
}
