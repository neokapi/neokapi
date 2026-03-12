package tools

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpanClassifyFromData(t *testing.T) {
	// Create a block with code:markup spans that have HTML in Data.
	block := model.NewBlock("1", "")
	frag := &model.Fragment{
		CodedText: "\uE001Hello\uE002 world",
		Spans: []*model.Span{
			{SpanType: model.SpanOpening, Type: "code:markup", ID: "1", Data: "<b>"},
			{SpanType: model.SpanClosing, Type: "code:markup", ID: "1", Data: "</b>"},
		},
	}
	block.Source = []*model.Segment{{Content: frag}}
	block.Translatable = true

	part := &model.Part{Type: model.PartBlock, Resource: block}

	tool := NewSpanClassifyTool(&SpanClassifyConfig{})
	result, err := tool.HandleBlockFn(part)
	require.NoError(t, err)

	resultBlock := result.Resource.(*model.Block)
	spans := resultBlock.Source[0].Content.Spans

	assert.Equal(t, "fmt:bold", spans[0].Type)
	assert.Equal(t, "fmt:bold", spans[1].Type)
	assert.True(t, spans[0].Deletable)
	assert.Equal(t, "[B]", spans[0].DisplayText)
	assert.Equal(t, "[/B]", spans[1].DisplayText)
}

func TestSpanClassifyFromSubType(t *testing.T) {
	block := model.NewBlock("1", "")
	frag := &model.Fragment{
		CodedText: "\uE001text\uE002",
		Spans: []*model.Span{
			{SpanType: model.SpanOpening, Type: "code:markup", SubType: "okapi:italic", ID: "1", Data: "<em>"},
			{SpanType: model.SpanClosing, Type: "code:markup", SubType: "okapi:italic", ID: "1", Data: "</em>"},
		},
	}
	block.Source = []*model.Segment{{Content: frag}}
	block.Translatable = true

	part := &model.Part{Type: model.PartBlock, Resource: block}

	tool := NewSpanClassifyTool(&SpanClassifyConfig{})
	result, err := tool.HandleBlockFn(part)
	require.NoError(t, err)

	spans := result.Resource.(*model.Block).Source[0].Content.Spans
	assert.Equal(t, "fmt:italic", spans[0].Type)
	assert.Equal(t, "fmt:italic", spans[1].Type)
}

func TestSpanClassifyBreakPlaceholder(t *testing.T) {
	block := model.NewBlock("1", "")
	frag := &model.Fragment{
		CodedText: "line one\uE003line two",
		Spans: []*model.Span{
			{SpanType: model.SpanPlaceholder, Type: "code:markup", ID: "1", Data: "<br/>"},
		},
	}
	block.Source = []*model.Segment{{Content: frag}}
	block.Translatable = true

	part := &model.Part{Type: model.PartBlock, Resource: block}

	tool := NewSpanClassifyTool(&SpanClassifyConfig{})
	result, err := tool.HandleBlockFn(part)
	require.NoError(t, err)

	spans := result.Resource.(*model.Block).Source[0].Content.Spans
	assert.Equal(t, "struct:break", spans[0].Type)
	assert.False(t, spans[0].Deletable)
	assert.Equal(t, "[BR]", spans[0].DisplayText)
}

func TestSpanClassifyUnknownType(t *testing.T) {
	block := model.NewBlock("1", "")
	frag := &model.Fragment{
		CodedText: "\uE001content\uE002",
		Spans: []*model.Span{
			{SpanType: model.SpanOpening, Type: "code:markup", ID: "1", Data: "<custom-tag>"},
			{SpanType: model.SpanClosing, Type: "code:markup", ID: "1", Data: "</custom-tag>"},
		},
	}
	block.Source = []*model.Segment{{Content: frag}}
	block.Translatable = true

	part := &model.Part{Type: model.PartBlock, Resource: block}

	tool := NewSpanClassifyTool(&SpanClassifyConfig{})
	result, err := tool.HandleBlockFn(part)
	require.NoError(t, err)

	// Unknown tags stay as code:markup.
	spans := result.Resource.(*model.Block).Source[0].Content.Spans
	assert.Equal(t, "code:markup", spans[0].Type)
	assert.Equal(t, "code:markup", spans[1].Type)
}

func TestSpanClassifySkipsNonMarkup(t *testing.T) {
	block := model.NewBlock("1", "")
	frag := &model.Fragment{
		CodedText: "\uE001Hello\uE002",
		Spans: []*model.Span{
			{SpanType: model.SpanOpening, Type: "fmt:bold", ID: "1", Data: "<b>"},
			{SpanType: model.SpanClosing, Type: "fmt:bold", ID: "1", Data: "</b>"},
		},
	}
	block.Source = []*model.Segment{{Content: frag}}
	block.Translatable = true

	part := &model.Part{Type: model.PartBlock, Resource: block}

	tool := NewSpanClassifyTool(&SpanClassifyConfig{})
	result, err := tool.HandleBlockFn(part)
	require.NoError(t, err)

	// Already classified spans are not modified.
	spans := result.Resource.(*model.Block).Source[0].Content.Spans
	assert.Equal(t, "fmt:bold", spans[0].Type)
	assert.Equal(t, "fmt:bold", spans[1].Type)
}

func TestSpanClassifyTargetFragments(t *testing.T) {
	block := model.NewBlock("1", "")
	block.Source = []*model.Segment{{Content: model.NewFragment("Hello")}}
	block.Targets = map[model.LocaleID][]*model.Segment{
		"fr": {{Content: &model.Fragment{
			CodedText: "\uE001Bonjour\uE002",
			Spans: []*model.Span{
				{SpanType: model.SpanOpening, Type: "code:markup", ID: "1", Data: "<i>"},
				{SpanType: model.SpanClosing, Type: "code:markup", ID: "1", Data: "</i>"},
			},
		}}},
	}
	block.Translatable = true

	part := &model.Part{Type: model.PartBlock, Resource: block}

	tool := NewSpanClassifyTool(&SpanClassifyConfig{})
	result, err := tool.HandleBlockFn(part)
	require.NoError(t, err)

	spans := result.Resource.(*model.Block).Targets["fr"][0].Content.Spans
	assert.Equal(t, "fmt:italic", spans[0].Type)
	assert.Equal(t, "fmt:italic", spans[1].Type)
}

func TestExtractTagName(t *testing.T) {
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
