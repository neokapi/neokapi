package tools_test

import (
	"context"
	"testing"

	"github.com/gokapi/gokapi/model"
	"github.com/gokapi/gokapi/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// processPart is a helper that sends a single Part through a tool and returns the result.
func processPart(t *testing.T, tl interface {
	Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error
}, part *model.Part) *model.Part {
	t.Helper()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- part
	close(in)

	err := tl.Process(context.Background(), in, out)
	close(out)
	require.NoError(t, err)

	result := <-out
	require.NotNil(t, result)
	return result
}

func TestPseudoTranslateTool(t *testing.T) {
	cfg := &tools.PseudoConfig{
		ExpansionPercent: 0,
		Prefix:           "[",
		Suffix:           "]",
		TargetLocale:     "qps",
	}
	tl := tools.NewPseudoTranslateTool(cfg)

	assert.Equal(t, "pseudo-translate", tl.Name())
	assert.Contains(t, tl.Description(), "pseudo")

	block := model.NewBlock("tu1", "Hello")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	targetText := resultBlock.TargetText("qps")

	// Should be wrapped in brackets.
	assert.True(t, len(targetText) > 0)
	assert.Equal(t, '[', rune(targetText[0]))
	assert.Equal(t, ']', rune(targetText[len(targetText)-1]))

	// Should contain accented characters, not the original ASCII.
	assert.NotContains(t, targetText, "Hello")
	// The 'e' in "Hello" should have been replaced with 'é'.
	assert.Contains(t, targetText, "\u00e9")
	// The 'o' in "Hello" should have been replaced with 'ö'.
	assert.Contains(t, targetText, "\u00f6")
}

func TestPseudoTranslateToolWithExpansion(t *testing.T) {
	cfg := &tools.PseudoConfig{
		ExpansionPercent: 50,
		Prefix:           "[",
		Suffix:           "]",
		TargetLocale:     "qps",
	}
	tl := tools.NewPseudoTranslateTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	targetText := resultBlock.TargetText("qps")

	// With 50% expansion on 5 chars, should add padding of 2 tildes + space.
	// Total should be longer than just accented + brackets.
	assert.Contains(t, targetText, "~~")
	assert.True(t, len([]rune(targetText)) > len([]rune("[Ĥéļļö]")))
}

func TestPseudoTranslateToolSkipsNonTranslatable(t *testing.T) {
	cfg := &tools.PseudoConfig{
		TargetLocale: "qps",
		Prefix:       "[",
		Suffix:       "]",
	}
	tl := tools.NewPseudoTranslateTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.False(t, resultBlock.HasTarget("qps"))
}

func TestPseudoTranslateToolCustomPrefixSuffix(t *testing.T) {
	cfg := &tools.PseudoConfig{
		Prefix:       "<<",
		Suffix:       ">>",
		TargetLocale: "qps",
	}
	tl := tools.NewPseudoTranslateTool(cfg)

	block := model.NewBlock("tu1", "Test")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	targetText := resultBlock.TargetText("qps")

	assert.True(t, len(targetText) >= 4)
	assert.Equal(t, "<<", targetText[:2])
	assert.Equal(t, ">>", targetText[len(targetText)-2:])
}

func TestPseudoTranslateToolPreservesSpans(t *testing.T) {
	cfg := &tools.PseudoConfig{
		Prefix:       "[",
		Suffix:       "]",
		TargetLocale: "qps",
	}
	tl := tools.NewPseudoTranslateTool(cfg)

	// Build a block with coded text: "Click <a>here</a>"
	// represented as "Click \uE001here\uE002"
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
	}
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)

	// Should have a target with spans preserved
	require.True(t, resultBlock.HasTarget("qps"))
	targetSegs := resultBlock.Targets["qps"]
	require.Len(t, targetSegs, 1)

	targetFrag := targetSegs[0].Content
	require.NotNil(t, targetFrag)

	// Coded text should contain the markers
	assert.Contains(t, targetFrag.CodedText, string(model.MarkerOpening))
	assert.Contains(t, targetFrag.CodedText, string(model.MarkerClosing))

	// Should have the same number of spans
	assert.Len(t, targetFrag.Spans, 2)
	assert.Equal(t, model.SpanOpening, targetFrag.Spans[0].SpanType)
	assert.Equal(t, model.SpanClosing, targetFrag.Spans[1].SpanType)

	// Plain text should be wrapped in brackets and accented
	plainText := targetFrag.Text()
	assert.Equal(t, '[', rune(plainText[0]))
	assert.Equal(t, ']', rune(plainText[len(plainText)-1]))
	assert.NotContains(t, plainText, "Click")
	assert.NotContains(t, plainText, "here")
}

func TestPseudoTranslateToolSpansWithExpansion(t *testing.T) {
	cfg := &tools.PseudoConfig{
		ExpansionPercent: 50,
		Prefix:           "[",
		Suffix:           "]",
		TargetLocale:     "qps",
	}
	tl := tools.NewPseudoTranslateTool(cfg)

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
	}
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	targetSegs := resultBlock.Targets["qps"]
	targetFrag := targetSegs[0].Content

	// Expansion padding should be based on text length, not marker count
	plainText := targetFrag.Text()
	assert.Contains(t, plainText, "~~")

	// Markers should still be present
	assert.Contains(t, targetFrag.CodedText, string(model.MarkerOpening))
	assert.Contains(t, targetFrag.CodedText, string(model.MarkerClosing))
}

func TestPseudoConfigValidation(t *testing.T) {
	cfg := &tools.PseudoConfig{ExpansionPercent: -1, TargetLocale: "qps"}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ExpansionPercent")

	cfg.ExpansionPercent = 0
	cfg.TargetLocale = ""
	err = cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TargetLocale")

	cfg.TargetLocale = "qps"
	err = cfg.Validate()
	assert.NoError(t, err)
}
