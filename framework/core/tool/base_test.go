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

	ctx := context.Background()
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

	ctx := context.Background()
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

	ctx, cancel := context.WithCancel(context.Background())
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

	ctx := context.Background()
	err := bt.Process(ctx, in, out)
	assert.Error(t, err)
}

func TestBaseToolModifyBlock(t *testing.T) {
	bt := &tool.BaseTool{
		ToolName: "uppercase",
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

	ctx := context.Background()
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
	assert.NoError(t, err)
}
