package tool_test

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/require"
)

// TestValidateHandlers pins the "exactly one capability handler" contract: a
// BaseTool with two typed handlers used to silently drop one by precedence;
// now every dispatch entry point rejects it loudly.
func TestValidateHandlers(t *testing.T) {
	annotate := func(tool.BlockView) error { return nil }
	produce := func(tool.VariantView) error { return nil }
	transform := func(tool.BlockView) (tool.EditPlan, error) { return tool.EditPlan{}, nil }

	blockPart := func() *model.Part {
		return &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Hello")}
	}
	process := func(bt *tool.BaseTool) error {
		in := make(chan *model.Part, 1)
		out := make(chan *model.Part, 1)
		in <- blockPart()
		close(in)
		return bt.Process(context.Background(), in, out)
	}

	t.Run("two handlers are rejected", func(t *testing.T) {
		bt := &tool.BaseTool{ToolName: "double", Annotate: annotate, Transform: transform}
		err := bt.ValidateHandlers()
		require.ErrorContains(t, err, "exactly one")
		require.ErrorContains(t, err, "Annotate, Transform")

		require.ErrorContains(t, process(bt), "exactly one")

		_, err = bt.Apply(blockPart())
		require.ErrorContains(t, err, "exactly one")

		_, err = bt.ApplyContext(context.Background(), blockPart())
		require.ErrorContains(t, err, "exactly one")

		par := tool.NewParallelBlockTool(bt, 4)
		in := make(chan *model.Part, 1)
		out := make(chan *model.Part, 1)
		in <- blockPart()
		close(in)
		require.ErrorContains(t, par.Process(context.Background(), in, out), "exactly one")
	})

	t.Run("three handlers are rejected", func(t *testing.T) {
		bt := &tool.BaseTool{ToolName: "triple", Annotate: annotate, Produce: produce, Transform: transform}
		err := bt.ValidateHandlers()
		require.ErrorContains(t, err, "3 capability handlers")
	})

	t.Run("one handler is fine", func(t *testing.T) {
		for name, bt := range map[string]*tool.BaseTool{
			"annotate":  {ToolName: "a", Annotate: annotate},
			"produce":   {ToolName: "p", Produce: produce},
			"transform": {ToolName: "t", Transform: transform},
		} {
			require.NoError(t, bt.ValidateHandlers(), name)
			require.NoError(t, process(bt), name)
		}
	})

	t.Run("zero handlers with untyped part handlers is fine", func(t *testing.T) {
		bt := &tool.BaseTool{
			ToolName:     "untyped",
			HandleDataFn: func(p *model.Part) (*model.Part, error) { return p, nil },
		}
		require.NoError(t, bt.ValidateHandlers())
		require.NoError(t, process(bt))
	})
}

// TestWithImmutabilityCheck pins the per-dispatch context override: it wins
// over the package-level default (which the tool-package TestMain sets to true
// for the test binary).
func TestWithImmutabilityCheck(t *testing.T) {
	mutator := &tool.BaseTool{ToolName: "sneaky"}
	mutator.Annotate = func(v tool.BlockView) error {
		v.SourceRuns()[0].Text.Text = "changed"
		return nil
	}
	blockPart := func() *model.Part {
		return &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Hello world")}
	}

	t.Run("context override disables the backstop", func(t *testing.T) {
		ctx := tool.WithImmutabilityCheck(context.Background(), false)
		_, err := mutator.ApplyContext(ctx, blockPart())
		require.NoError(t, err)
	})

	t.Run("context override enables the backstop", func(t *testing.T) {
		ctx := tool.WithImmutabilityCheck(context.Background(), true)
		_, err := mutator.ApplyContext(ctx, blockPart())
		require.ErrorContains(t, err, "immutability")
	})

	t.Run("no context value falls back to the package default", func(t *testing.T) {
		// TestMain sets EnforceImmutability = true for this test binary.
		_, err := mutator.ApplyContext(context.Background(), blockPart())
		require.ErrorContains(t, err, "immutability")
	})
}
