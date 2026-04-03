package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInlineCodesRemoveToolTarget(t *testing.T) {
	cfg := &tools.InlineCodesRemoveConfig{
		ApplyTarget:  true,
		TargetLocale: model.LocaleFrench,
	}
	tl := tools.NewInlineCodesRemoveTool(cfg)

	assert.Equal(t, "inline-codes-remove", tl.Name())

	// Build a block with spans in target.
	frag := &model.Fragment{
		CodedText: "Cliquez \uE001ici\uE002",
		Spans: []*model.Span{
			{SpanType: model.SpanOpening, Type: "link", ID: "1", Data: "<a>"},
			{SpanType: model.SpanClosing, Type: "link", ID: "1", Data: "</a>"},
		},
	}
	block := &model.Block{
		ID:           "tu1",
		Translatable: true,
		Source:       []*model.Segment{{ID: "s1", Content: model.NewFragment("Click here")}},
		Targets: map[model.LocaleID][]*model.Segment{
			model.LocaleFrench: {{ID: "s1", Content: frag}},
		},
		Properties: make(map[string]string),
	}
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	targetSegs := resultBlock.Targets[model.LocaleFrench]
	require.Len(t, targetSegs, 1)

	targetFrag := targetSegs[0].Content
	assert.Equal(t, "Cliquez ici", targetFrag.CodedText)
	assert.False(t, targetFrag.HasSpans())
}

func TestInlineCodesRemoveToolSource(t *testing.T) {
	cfg := &tools.InlineCodesRemoveConfig{
		ApplySource: true,
		ApplyTarget: false,
	}
	tl := tools.NewInlineCodesRemoveTool(cfg)

	// Build a block with spans in source.
	frag := &model.Fragment{
		CodedText: "Click \uE001here\uE002",
		Spans: []*model.Span{
			{SpanType: model.SpanOpening, Type: "link", ID: "1", Data: "<a>"},
			{SpanType: model.SpanClosing, Type: "link", ID: "1", Data: "</a>"},
		},
	}
	block := &model.Block{
		ID:           "tu1",
		Translatable: true,
		Source:       []*model.Segment{{ID: "s1", Content: frag}},
		Targets:      make(map[model.LocaleID][]*model.Segment),
		Properties:   make(map[string]string),
	}
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	sourceFrag := resultBlock.Source[0].Content
	assert.Equal(t, "Click here", sourceFrag.CodedText)
	assert.False(t, sourceFrag.HasSpans())
}

func TestInlineCodesRemoveToolFragmentWithSpansBecomesPlainText(t *testing.T) {
	cfg := &tools.InlineCodesRemoveConfig{
		ApplySource: true,
		ApplyTarget: false,
	}
	tl := tools.NewInlineCodesRemoveTool(cfg)

	// Build fragment with multiple span types.
	frag := &model.Fragment{CodedText: "", Spans: []*model.Span{}}
	frag.AppendText("Hello ")
	frag.AppendSpan(&model.Span{Type: "b", SpanType: model.SpanOpening})
	frag.AppendText("world")
	frag.AppendSpan(&model.Span{Type: "b", SpanType: model.SpanClosing})
	frag.AppendText(" and ")
	frag.AppendSpan(&model.Span{Type: "img", SpanType: model.SpanPlaceholder})

	require.True(t, frag.HasSpans())
	require.Len(t, frag.Spans, 3)

	block := &model.Block{
		ID:           "tu1",
		Translatable: true,
		Source:       []*model.Segment{{ID: "s1", Content: frag}},
		Targets:      make(map[model.LocaleID][]*model.Segment),
		Properties:   make(map[string]string),
	}
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	sourceFrag := resultBlock.Source[0].Content
	assert.Equal(t, "Hello world and ", sourceFrag.CodedText)
	assert.False(t, sourceFrag.HasSpans())
	assert.Equal(t, sourceFrag.Text(), sourceFrag.CodedText)
}

func TestInlineCodesRemoveToolSkipsNonTranslatable(t *testing.T) {
	cfg := &tools.InlineCodesRemoveConfig{
		ApplySource: true,
		ApplyTarget: false,
	}
	tl := tools.NewInlineCodesRemoveTool(cfg)

	frag := &model.Fragment{
		CodedText: "Click \uE001here\uE002",
		Spans: []*model.Span{
			{SpanType: model.SpanOpening, Type: "link", ID: "1"},
			{SpanType: model.SpanClosing, Type: "link", ID: "1"},
		},
	}
	block := &model.Block{
		ID:           "tu1",
		Translatable: false,
		Source:       []*model.Segment{{ID: "s1", Content: frag}},
		Targets:      make(map[model.LocaleID][]*model.Segment),
		Properties:   make(map[string]string),
	}
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	// Spans should still be present since block is non-translatable.
	assert.True(t, resultBlock.Source[0].Content.HasSpans())
	assert.Len(t, resultBlock.Source[0].Content.Spans, 2)
}

func TestInlineCodesRemoveConfigValidation(t *testing.T) {
	cfg := &tools.InlineCodesRemoveConfig{
		ApplyTarget:  true,
		TargetLocale: "",
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TargetLocale")

	cfg.ApplyTarget = false
	cfg.ApplySource = false
	err = cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ApplySource")

	cfg.ApplySource = true
	err = cfg.Validate()
	assert.NoError(t, err)
}
