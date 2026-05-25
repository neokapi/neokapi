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

	assert.Equal(t, len(parts), len(result))
	for i, p := range result {
		assert.Equal(t, parts[i].Type, p.Type)
		assert.Equal(t, parts[i].Resource, p.Resource)
	}
}

func TestBaseToolDispatch(t *testing.T) {
	var handledTypes []model.PartType

	bt := &tool.BaseTool{
		ToolName: "tracker",
		HandleBlockFn: func(part *model.Part) (*model.Part, error) {
			handledTypes = append(handledTypes, part.Type)
			return part, nil
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
		HandleBlockFn: func(part *model.Part) (*model.Part, error) {
			return nil, assert.AnError
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
		ToolName:     "uppercase",
		WritesTarget: true,
		HandleBlockFn: func(part *model.Part) (*model.Part, error) {
			block := part.Resource.(*model.Block)
			if block.Translatable {
				block.SetTargetText(model.LocaleFrench, "TRANSLATED")
			}
			return part, nil
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

	t.Run("source mutation without WritesSource is rejected", func(t *testing.T) {
		bt := &tool.BaseTool{ToolName: "bad-src"}
		bt.HandleBlockFn = func(p *model.Part) (*model.Part, error) {
			p.Resource.(*model.Block).SetSourceText("changed")
			return p, nil
		}
		require.ErrorContains(t, run(bt, mkBlock()), "WritesSource")
	})
	t.Run("source mutation with WritesSource is allowed", func(t *testing.T) {
		bt := &tool.BaseTool{ToolName: "src-xform", WritesSource: true}
		bt.HandleBlockFn = func(p *model.Part) (*model.Part, error) {
			p.Resource.(*model.Block).SetSourceText("changed")
			return p, nil
		}
		require.NoError(t, run(bt, mkBlock()))
	})
	t.Run("target mutation without WritesTarget is rejected", func(t *testing.T) {
		bt := &tool.BaseTool{ToolName: "bad-tgt"}
		bt.HandleBlockFn = func(p *model.Part) (*model.Part, error) {
			p.Resource.(*model.Block).SetTargetText("fr", "Bonjour")
			return p, nil
		}
		require.ErrorContains(t, run(bt, mkBlock()), "WritesTarget")
	})
	t.Run("target mutation with WritesTarget is allowed", func(t *testing.T) {
		bt := &tool.BaseTool{ToolName: "translator", WritesTarget: true}
		bt.HandleBlockFn = func(p *model.Part) (*model.Part, error) {
			p.Resource.(*model.Block).SetTargetText("fr", "Bonjour")
			return p, nil
		}
		require.NoError(t, run(bt, mkBlock()))
	})
	t.Run("analysis tool writing only overlays/properties is allowed", func(t *testing.T) {
		bt := &tool.BaseTool{ToolName: "analyzer"} // declares neither capability
		bt.HandleBlockFn = func(p *model.Part) (*model.Part, error) {
			b := p.Resource.(*model.Block)
			b.Properties["word-count"] = "2"
			b.SetSegmentation(nil, oneSpanOverlay)
			return p, nil
		}
		require.NoError(t, run(bt, mkBlock()))
	})
	t.Run("source transform after an overlay is attached is rejected", func(t *testing.T) {
		bt := &tool.BaseTool{ToolName: "late-xform", WritesSource: true}
		bt.HandleBlockFn = func(p *model.Part) (*model.Part, error) {
			p.Resource.(*model.Block).SetSourceText("changed")
			return p, nil
		}
		b := mkBlock()
		b.SetSegmentation(nil, oneSpanOverlay)
		require.ErrorContains(t, run(bt, b), "overlay")
	})
}
