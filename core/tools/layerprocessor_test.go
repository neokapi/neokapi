package tools

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLayerProcessor_PassThrough(t *testing.T) {
	t.Parallel()
	// Without any pipelines, all parts pass through unchanged
	lp := NewLayerProcessorTool(&LayerProcessorConfig{})

	rootLayer := &model.Layer{ID: "doc1", Format: "json"}
	block := model.NewBlock("tu1", "Hello")

	parts := []*model.Part{
		{Type: model.PartLayerStart, Resource: rootLayer},
		{Type: model.PartBlock, Resource: block},
		{Type: model.PartLayerEnd, Resource: rootLayer},
	}

	result := processAll(t, lp, parts)

	require.Len(t, result, 3)
	assert.Equal(t, model.PartLayerStart, result[0].Type)
	assert.Equal(t, model.PartBlock, result[1].Type)
	assert.Equal(t, model.PartLayerEnd, result[2].Type)
}

func TestLayerProcessor_ChildLayerNoMatch(t *testing.T) {
	t.Parallel()
	// Child layer with format that has no pipeline — passes through unchanged
	lp := NewLayerProcessorTool(&LayerProcessorConfig{
		Pipelines: map[string][]tool.Tool{
			"markdown": {NewCaseTransformTool(&CaseTransformConfig{Mode: CaseUpper, ApplySource: true})},
		},
	})

	rootLayer := &model.Layer{ID: "doc1", Format: "json"}
	childLayer := &model.Layer{ID: "sf1", Format: "html", ParentID: "doc1"}
	block := model.NewBlock("tu1", "Hello World")

	parts := []*model.Part{
		{Type: model.PartLayerStart, Resource: rootLayer},
		{Type: model.PartLayerStart, Resource: childLayer},
		{Type: model.PartBlock, Resource: block},
		{Type: model.PartLayerEnd, Resource: childLayer},
		{Type: model.PartLayerEnd, Resource: rootLayer},
	}

	result := processAll(t, lp, parts)

	// All parts pass through — child layer parts are unchanged
	require.Len(t, result, 5)
	// Block text is unchanged (no pipeline for "html")
	resultBlock := result[2].Resource.(*model.Block)
	assert.Equal(t, "Hello World", resultBlock.SourceText())
}

func TestLayerProcessor_ChildLayerWithPipeline(t *testing.T) {
	t.Parallel()
	// Child layer with format that has a pipeline — parts are processed
	upperTool := NewCaseTransformTool(&CaseTransformConfig{Mode: CaseUpper, ApplySource: true})

	lp := NewLayerProcessorTool(&LayerProcessorConfig{
		Pipelines: map[string][]tool.Tool{
			"html": {upperTool},
		},
	})

	rootLayer := &model.Layer{ID: "doc1", Format: "json"}
	childLayer := &model.Layer{ID: "sf1", Format: "html", ParentID: "doc1"}
	rootBlock := model.NewBlock("tu1", "root text")
	childBlock := model.NewBlock("tu2", "child text")

	parts := []*model.Part{
		{Type: model.PartLayerStart, Resource: rootLayer},
		{Type: model.PartBlock, Resource: rootBlock},
		{Type: model.PartLayerStart, Resource: childLayer},
		{Type: model.PartBlock, Resource: childBlock},
		{Type: model.PartLayerEnd, Resource: childLayer},
		{Type: model.PartLayerEnd, Resource: rootLayer},
	}

	result := processAll(t, lp, parts)

	require.Len(t, result, 6)

	// Root block passes through unchanged
	rb := result[1].Resource.(*model.Block)
	assert.Equal(t, "root text", rb.SourceText())

	// Child block was processed by the uppercase tool
	cb := result[3].Resource.(*model.Block)
	assert.Equal(t, "CHILD TEXT", cb.SourceText())
}

func TestLayerProcessor_MultiplePipelines(t *testing.T) {
	t.Parallel()
	// Different pipelines for different formats
	upperTool := NewCaseTransformTool(&CaseTransformConfig{Mode: CaseUpper, ApplySource: true})
	pseudoTool := NewPseudoTranslateTool(&PseudoConfig{Prefix: "[", Suffix: "]", TargetLocale: "qps"})

	lp := NewLayerProcessorTool(&LayerProcessorConfig{
		Pipelines: map[string][]tool.Tool{
			"html":     {upperTool},
			"markdown": {pseudoTool},
		},
	})

	rootLayer := &model.Layer{ID: "doc1", Format: "json"}
	htmlLayer := &model.Layer{ID: "sf1", Format: "html", ParentID: "doc1"}
	mdLayer := &model.Layer{ID: "sf2", Format: "markdown", ParentID: "doc1"}

	htmlBlock := model.NewBlock("tu1", "html text")
	mdBlock := model.NewBlock("tu2", "markdown text")

	parts := []*model.Part{
		{Type: model.PartLayerStart, Resource: rootLayer},
		{Type: model.PartLayerStart, Resource: htmlLayer},
		{Type: model.PartBlock, Resource: htmlBlock},
		{Type: model.PartLayerEnd, Resource: htmlLayer},
		{Type: model.PartLayerStart, Resource: mdLayer},
		{Type: model.PartBlock, Resource: mdBlock},
		{Type: model.PartLayerEnd, Resource: mdLayer},
		{Type: model.PartLayerEnd, Resource: rootLayer},
	}

	result := processAll(t, lp, parts)

	// Find the processed blocks
	var blocks []*model.Block
	for _, p := range result {
		if p.Type == model.PartBlock {
			if b, ok := p.Resource.(*model.Block); ok {
				blocks = append(blocks, b)
			}
		}
	}

	require.Len(t, blocks, 2)
	// HTML block was uppercased
	assert.Equal(t, "HTML TEXT", blocks[0].SourceText())
	// Markdown block was pseudo-translated (has target)
	assert.True(t, blocks[1].HasTarget("qps"))
}

func TestLayerProcessor_ToolChain(t *testing.T) {
	t.Parallel()
	// Multiple tools in a single pipeline (uppercase then search-replace)
	upperTool := NewCaseTransformTool(&CaseTransformConfig{Mode: CaseUpper, ApplySource: true})
	searchReplace := NewSearchReplaceTool(&SearchReplaceConfig{
		Pairs: []ReplacePair{{Search: "HELLO", Replace: "HI"}},
	})

	lp := NewLayerProcessorTool(&LayerProcessorConfig{
		Pipelines: map[string][]tool.Tool{
			"html": {upperTool, searchReplace},
		},
	})

	rootLayer := &model.Layer{ID: "doc1", Format: "json"}
	childLayer := &model.Layer{ID: "sf1", Format: "html", ParentID: "doc1"}
	block := model.NewBlock("tu1", "hello world")

	parts := []*model.Part{
		{Type: model.PartLayerStart, Resource: rootLayer},
		{Type: model.PartLayerStart, Resource: childLayer},
		{Type: model.PartBlock, Resource: block},
		{Type: model.PartLayerEnd, Resource: childLayer},
		{Type: model.PartLayerEnd, Resource: rootLayer},
	}

	result := processAll(t, lp, parts)

	// Block went through: uppercase → search-replace
	for _, p := range result {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			assert.Equal(t, "HI WORLD", b.SourceText())
		}
	}
}

// processAll sends parts through a tool and collects the output.
func processAll(t *testing.T, tl tool.Tool, parts []*model.Part) []*model.Part {
	t.Helper()
	ctx := t.Context()

	inCh := make(chan *model.Part, len(parts))
	for _, p := range parts {
		inCh <- p
	}
	close(inCh)

	outCh := make(chan *model.Part, len(parts)+16)
	err := tl.Process(ctx, inCh, outCh)
	require.NoError(t, err)
	close(outCh)

	var result []*model.Part
	for p := range outCh {
		result = append(result, p)
	}
	return result
}
