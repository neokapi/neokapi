package tools_test

import (
	"context"
	"strings"
	"testing"

	"github.com/gokapi/gokapi/core/flow"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/lib/tools"
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

// --- PseudoTranslateTool Tests ---

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

// --- WordCountTool Tests ---

func TestWordCountTool(t *testing.T) {
	cfg := &tools.WordCountConfig{
		Locale: model.LocaleFrench,
	}
	tl := tools.NewWordCountTool(cfg)

	assert.Equal(t, "word-count", tl.Name())

	block := model.NewBlock("tu1", "Hello beautiful world")
	block.SetTargetText(model.LocaleFrench, "Bonjour le beau monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "3", resultBlock.Properties[tools.PropWordCountSource])
	assert.Equal(t, "4", resultBlock.Properties[tools.PropWordCountTarget])
}

func TestWordCountToolSourceOnly(t *testing.T) {
	cfg := &tools.WordCountConfig{}
	tl := tools.NewWordCountTool(cfg)

	block := model.NewBlock("tu1", "One two three four")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "4", resultBlock.Properties[tools.PropWordCountSource])
	// No target set, no locale configured -> no target count.
	_, hasTargetCount := resultBlock.Properties[tools.PropWordCountTarget]
	assert.False(t, hasTargetCount)
}

func TestWordCountToolEmptyText(t *testing.T) {
	cfg := &tools.WordCountConfig{}
	tl := tools.NewWordCountTool(cfg)

	block := model.NewBlock("tu1", "")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "0", resultBlock.Properties[tools.PropWordCountSource])
}

func TestWordCountToolSkipsNonTranslatable(t *testing.T) {
	cfg := &tools.WordCountConfig{}
	tl := tools.NewWordCountTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	_, hasSourceCount := resultBlock.Properties[tools.PropWordCountSource]
	assert.False(t, hasSourceCount)
}

// --- SearchReplaceTool Tests ---

func TestSearchReplaceTool(t *testing.T) {
	cfg := &tools.SearchReplaceConfig{
		Pairs: []tools.ReplacePair{
			{Search: "Hello", Replace: "Hi"},
			{Search: "world", Replace: "earth"},
		},
	}
	tl := tools.NewSearchReplaceTool(cfg)

	assert.Equal(t, "search-replace", tl.Name())

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Hi earth", resultBlock.SourceText())
}

func TestSearchReplaceToolRegex(t *testing.T) {
	cfg := &tools.SearchReplaceConfig{
		Pairs: []tools.ReplacePair{
			{Search: `\b\d{3}\b`, Replace: "XXX", IsRegex: true},
		},
	}
	tl := tools.NewSearchReplaceTool(cfg)

	block := model.NewBlock("tu1", "Call 555 now or 123 later")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Call XXX now or XXX later", resultBlock.SourceText())
}

func TestSearchReplaceToolTarget(t *testing.T) {
	cfg := &tools.SearchReplaceConfig{
		Pairs: []tools.ReplacePair{
			{Search: "monde", Replace: "terre"},
		},
		TargetLocale: model.LocaleFrench,
	}
	tl := tools.NewSearchReplaceTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Hello world", resultBlock.SourceText()) // Source unchanged (no match).
	assert.Equal(t, "Bonjour le terre", resultBlock.TargetText(model.LocaleFrench))
}

func TestSearchReplaceToolSkipsNonTranslatable(t *testing.T) {
	cfg := &tools.SearchReplaceConfig{
		Pairs: []tools.ReplacePair{
			{Search: "Hello", Replace: "Hi"},
		},
	}
	tl := tools.NewSearchReplaceTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Hello world", resultBlock.SourceText()) // Unchanged.
}

func TestSearchReplaceConfigValidation(t *testing.T) {
	// Empty search string.
	cfg := &tools.SearchReplaceConfig{
		Pairs: []tools.ReplacePair{
			{Search: "", Replace: "x"},
		},
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty search")

	// Invalid regex.
	cfg = &tools.SearchReplaceConfig{
		Pairs: []tools.ReplacePair{
			{Search: "[invalid", Replace: "x", IsRegex: true},
		},
	}
	err = cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid regex")

	// Valid config.
	cfg = &tools.SearchReplaceConfig{
		Pairs: []tools.ReplacePair{
			{Search: `\d+`, Replace: "NUM", IsRegex: true},
			{Search: "foo", Replace: "bar"},
		},
	}
	err = cfg.Validate()
	assert.NoError(t, err)
}

func TestSearchReplaceToolNoPairs(t *testing.T) {
	cfg := &tools.SearchReplaceConfig{}
	tl := tools.NewSearchReplaceTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Hello world", resultBlock.SourceText()) // Unchanged.
}

// --- CharCountTool Tests ---

func TestCharCountTool(t *testing.T) {
	cfg := &tools.CharCountConfig{
		Locale: model.LocaleFrench,
	}
	tl := tools.NewCharCountTool(cfg)

	assert.Equal(t, "char-count", tl.Name())

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	// "Hello world" = 11 chars, 10 without spaces.
	assert.Equal(t, "11", resultBlock.Properties[tools.PropCharCountSource])
	assert.Equal(t, "10", resultBlock.Properties[tools.PropCharCountSourceNospace])
	// "Bonjour le monde" = 16 chars, 14 without spaces.
	assert.Equal(t, "16", resultBlock.Properties[tools.PropCharCountTarget])
	assert.Equal(t, "14", resultBlock.Properties[tools.PropCharCountTargetNospace])
}

func TestCharCountToolSourceOnly(t *testing.T) {
	cfg := &tools.CharCountConfig{}
	tl := tools.NewCharCountTool(cfg)

	block := model.NewBlock("tu1", "Test text")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	// "Test text" = 9 chars, 8 without spaces.
	assert.Equal(t, "9", resultBlock.Properties[tools.PropCharCountSource])
	assert.Equal(t, "8", resultBlock.Properties[tools.PropCharCountSourceNospace])
	// No target count since no locale and no target.
	_, hasTargetCount := resultBlock.Properties[tools.PropCharCountTarget]
	assert.False(t, hasTargetCount)
}

func TestCharCountToolUnicode(t *testing.T) {
	cfg := &tools.CharCountConfig{}
	tl := tools.NewCharCountTool(cfg)

	// Unicode text: "Bonjour" = 7 chars.
	block := model.NewBlock("tu1", "\u00e9l\u00e8ve") // "eleve" with accents.
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "5", resultBlock.Properties[tools.PropCharCountSource])
	assert.Equal(t, "5", resultBlock.Properties[tools.PropCharCountSourceNospace])
}

func TestCharCountToolSkipsNonTranslatable(t *testing.T) {
	cfg := &tools.CharCountConfig{}
	tl := tools.NewCharCountTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	_, hasSourceCount := resultBlock.Properties[tools.PropCharCountSource]
	assert.False(t, hasSourceCount)
}

func TestCharCountToolEmptyText(t *testing.T) {
	cfg := &tools.CharCountConfig{}
	tl := tools.NewCharCountTool(cfg)

	block := model.NewBlock("tu1", "")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "0", resultBlock.Properties[tools.PropCharCountSource])
	assert.Equal(t, "0", resultBlock.Properties[tools.PropCharCountSourceNospace])
}

// --- XMLValidationTool Tests ---

func TestXMLValidationToolValid(t *testing.T) {
	cfg := &tools.XMLValidationConfig{CheckSource: true, WrapRoot: true}
	tl := tools.NewXMLValidationTool(cfg)

	assert.Equal(t, "xml-validation", tl.Name())

	block := model.NewBlock("tu1", "Hello <b>world</b>")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "true", resultBlock.Properties[tools.PropXMLValid])
}

func TestXMLValidationToolInvalid(t *testing.T) {
	cfg := &tools.XMLValidationConfig{CheckSource: true, WrapRoot: true}
	tl := tools.NewXMLValidationTool(cfg)

	block := model.NewBlock("tu1", "Hello <b>world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "false", resultBlock.Properties[tools.PropXMLValid])
	assert.NotEmpty(t, resultBlock.Properties[tools.PropXMLValidError])
}

func TestXMLValidationToolSkipsNonTranslatable(t *testing.T) {
	cfg := &tools.XMLValidationConfig{CheckSource: true}
	tl := tools.NewXMLValidationTool(cfg)

	block := model.NewBlock("tu1", "<invalid")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	_, hasValid := resultBlock.Properties[tools.PropXMLValid]
	assert.False(t, hasValid)
}

func TestXMLValidationConfigValidation(t *testing.T) {
	cfg := &tools.XMLValidationConfig{CheckTarget: true}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Locale")

	cfg.Locale = model.LocaleFrench
	err = cfg.Validate()
	assert.NoError(t, err)
}

// --- XSLTTransformTool Tests ---

func TestXSLTTransformTool(t *testing.T) {
	cfg := &tools.XSLTTransformConfig{
		Rules: []tools.TransformRule{
			{Pattern: `<b>(.*?)</b>`, Replace: `<strong>$1</strong>`},
		},
	}
	tl := tools.NewXSLTTransformTool(cfg)

	assert.Equal(t, "xslt-transform", tl.Name())

	block := model.NewBlock("tu1", "Hello <b>world</b>")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Hello <strong>world</strong>", resultBlock.SourceText())
}

func TestXSLTTransformToolMultipleRules(t *testing.T) {
	cfg := &tools.XSLTTransformConfig{
		Rules: []tools.TransformRule{
			{Pattern: `<i>`, Replace: `<em>`},
			{Pattern: `</i>`, Replace: `</em>`},
		},
	}
	tl := tools.NewXSLTTransformTool(cfg)

	block := model.NewBlock("tu1", "<i>emphasis</i>")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "<em>emphasis</em>", resultBlock.SourceText())
}

func TestXSLTTransformConfigValidation(t *testing.T) {
	cfg := &tools.XSLTTransformConfig{
		Rules: []tools.TransformRule{
			{Pattern: "", Replace: "x"},
		},
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty pattern")

	cfg = &tools.XSLTTransformConfig{
		Rules: []tools.TransformRule{
			{Pattern: "[invalid", Replace: "x"},
		},
	}
	err = cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid pattern")

	cfg = &tools.XSLTTransformConfig{
		Rules: []tools.TransformRule{
			{Pattern: `\d+`, Replace: "NUM"},
		},
	}
	err = cfg.Validate()
	assert.NoError(t, err)
}

// --- EncodingDetectTool Tests ---

func TestEncodingDetectToolASCII(t *testing.T) {
	cfg := &tools.EncodingDetectConfig{}
	tl := tools.NewEncodingDetectTool(cfg)

	assert.Equal(t, "encoding-detect", tl.Name())

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "ascii", resultBlock.Properties[tools.PropEncodingDetected])
	assert.Equal(t, "true", resultBlock.Properties[tools.PropEncodingIsASCII])
	assert.Equal(t, "true", resultBlock.Properties[tools.PropEncodingIsUTF8])
}

func TestEncodingDetectToolUTF8(t *testing.T) {
	cfg := &tools.EncodingDetectConfig{}
	tl := tools.NewEncodingDetectTool(cfg)

	block := model.NewBlock("tu1", "Héllo wörld")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "utf-8", resultBlock.Properties[tools.PropEncodingDetected])
	assert.Equal(t, "false", resultBlock.Properties[tools.PropEncodingIsASCII])
	assert.Equal(t, "true", resultBlock.Properties[tools.PropEncodingIsUTF8])
}

func TestEncodingDetectToolSkipsNonTranslatable(t *testing.T) {
	cfg := &tools.EncodingDetectConfig{}
	tl := tools.NewEncodingDetectTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	_, hasEncoding := resultBlock.Properties[tools.PropEncodingDetected]
	assert.False(t, hasEncoding)
}

// --- SegCountTool Tests ---

func TestSegCountTool(t *testing.T) {
	cfg := &tools.SegCountConfig{Locale: model.LocaleFrench}
	tl := tools.NewSegCountTool(cfg)

	assert.Equal(t, "segment-count", tl.Name())

	block := model.NewBlock("tu1", "Hello")
	block.SetTargetText(model.LocaleFrench, "Bonjour")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "1", resultBlock.Properties[tools.PropSegCountSource])
	assert.Equal(t, "1", resultBlock.Properties[tools.PropSegCountTarget])
}

func TestSegCountToolSourceOnly(t *testing.T) {
	cfg := &tools.SegCountConfig{}
	tl := tools.NewSegCountTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "1", resultBlock.Properties[tools.PropSegCountSource])
	_, hasTarget := resultBlock.Properties[tools.PropSegCountTarget]
	assert.False(t, hasTarget)
}

// --- CaseTransformTool Tests ---

func TestCaseTransformToolUpper(t *testing.T) {
	cfg := &tools.CaseTransformConfig{
		Mode:        tools.CaseUpper,
		ApplySource: true,
	}
	tl := tools.NewCaseTransformTool(cfg)

	assert.Equal(t, "case-transform", tl.Name())

	block := model.NewBlock("tu1", "Hello World")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "HELLO WORLD", resultBlock.SourceText())
}

func TestCaseTransformToolLower(t *testing.T) {
	cfg := &tools.CaseTransformConfig{
		Mode:        tools.CaseLower,
		ApplySource: true,
	}
	tl := tools.NewCaseTransformTool(cfg)

	block := model.NewBlock("tu1", "Hello World")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "hello world", resultBlock.SourceText())
}

func TestCaseTransformToolTarget(t *testing.T) {
	cfg := &tools.CaseTransformConfig{
		Mode:         tools.CaseUpper,
		ApplySource:  false,
		ApplyTarget:  true,
		TargetLocale: model.LocaleFrench,
	}
	tl := tools.NewCaseTransformTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	block.SetTargetText(model.LocaleFrench, "Bonjour")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Hello", resultBlock.SourceText()) // Unchanged
	assert.Equal(t, "BONJOUR", resultBlock.TargetText(model.LocaleFrench))
}

func TestCaseTransformConfigValidation(t *testing.T) {
	cfg := &tools.CaseTransformConfig{Mode: "invalid"}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid Mode")

	cfg = &tools.CaseTransformConfig{Mode: tools.CaseUpper, ApplyTarget: true}
	err = cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TargetLocale")

	cfg = &tools.CaseTransformConfig{Mode: tools.CaseLower}
	err = cfg.Validate()
	assert.NoError(t, err)
}

// --- TagProtectTool Tests ---

func TestTagProtectTool(t *testing.T) {
	cfg := &tools.TagProtectConfig{}
	tl := tools.NewTagProtectTool(cfg)

	assert.Equal(t, "tag-protect", tl.Name())

	block := model.NewBlock("tu1", "Hello <b>world</b>, value is {count}")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	count := resultBlock.Properties[tools.PropTagProtectCount]
	// Should find HTML tags and curly brace placeholder.
	assert.NotEqual(t, "0", count)

	// Check annotation.
	ann, ok := resultBlock.Annotations["protected-tags"]
	assert.True(t, ok)
	assert.NotNil(t, ann)
}

func TestTagProtectToolCustomPatterns(t *testing.T) {
	cfg := &tools.TagProtectConfig{
		Patterns: []string{`\[\[.*?\]\]`},
	}
	tl := tools.NewTagProtectTool(cfg)

	block := model.NewBlock("tu1", "Hello [[name]], welcome to [[place]]")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "2", resultBlock.Properties[tools.PropTagProtectCount])
}

func TestTagProtectToolNoTags(t *testing.T) {
	cfg := &tools.TagProtectConfig{
		Patterns: []string{`\[\[.*?\]\]`},
	}
	tl := tools.NewTagProtectTool(cfg)

	block := model.NewBlock("tu1", "Just plain text here")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "0", resultBlock.Properties[tools.PropTagProtectCount])
}

func TestTagProtectConfigValidation(t *testing.T) {
	cfg := &tools.TagProtectConfig{Patterns: []string{""}}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty")

	cfg = &tools.TagProtectConfig{Patterns: []string{"[invalid"}}
	err = cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid")

	cfg = &tools.TagProtectConfig{Patterns: []string{`<[^>]+>`}}
	err = cfg.Validate()
	assert.NoError(t, err)
}

func TestReplaceAndRestoreProtectedTags(t *testing.T) {
	tags := []tools.ProtectedTag{
		{Text: "<b>", Offset: 6},
		{Text: "</b>", Offset: 14},
	}

	text := "Hello <b>world</b>"
	replaced, mapping := tools.ReplaceProtectedTags(text, tags)
	assert.NotContains(t, replaced, "<b>")
	assert.NotContains(t, replaced, "</b>")
	assert.Contains(t, replaced, "Hello")
	assert.Len(t, mapping, 2)

	restored := tools.RestoreProtectedTags(replaced, mapping)
	assert.Equal(t, text, restored)
}

// --- TermCheckTool Tests ---

func TestTermCheckToolPass(t *testing.T) {
	cfg := &tools.TermCheckConfig{
		Glossary: []tools.GlossaryEntry{
			{Source: "Save", Target: "Sauvegarder"},
		},
		TargetLocale: model.LocaleFrench,
	}
	tl := tools.NewTermCheckTool(cfg)

	assert.Equal(t, "term-check", tl.Name())

	block := model.NewBlock("tu1", "Save the file")
	block.SetTargetText(model.LocaleFrench, "Sauvegarder le fichier")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "true", resultBlock.Properties[tools.PropTermCheckPassed])
}

func TestTermCheckToolFail(t *testing.T) {
	cfg := &tools.TermCheckConfig{
		Glossary: []tools.GlossaryEntry{
			{Source: "Save", Target: "Sauvegarder"},
		},
		TargetLocale: model.LocaleFrench,
	}
	tl := tools.NewTermCheckTool(cfg)

	block := model.NewBlock("tu1", "Save the file")
	block.SetTargetText(model.LocaleFrench, "Enregistrer le fichier")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "false", resultBlock.Properties[tools.PropTermCheckPassed])
	assert.Contains(t, resultBlock.Properties[tools.PropTermCheckErrors], "Sauvegarder")
}

func TestTermCheckToolCaseInsensitive(t *testing.T) {
	cfg := &tools.TermCheckConfig{
		Glossary: []tools.GlossaryEntry{
			{Source: "save", Target: "sauvegarder"},
		},
		TargetLocale:  model.LocaleFrench,
		CaseSensitive: false,
	}
	tl := tools.NewTermCheckTool(cfg)

	block := model.NewBlock("tu1", "SAVE the file")
	block.SetTargetText(model.LocaleFrench, "SAUVEGARDER le fichier")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "true", resultBlock.Properties[tools.PropTermCheckPassed])
}

func TestTermCheckToolNoTarget(t *testing.T) {
	cfg := &tools.TermCheckConfig{
		Glossary: []tools.GlossaryEntry{
			{Source: "Save", Target: "Sauvegarder"},
		},
		TargetLocale: model.LocaleFrench,
	}
	tl := tools.NewTermCheckTool(cfg)

	// No target text set.
	block := model.NewBlock("tu1", "Save the file")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	_, hasPassed := resultBlock.Properties[tools.PropTermCheckPassed]
	assert.False(t, hasPassed) // No target → no check.
}

func TestTermCheckConfigValidation(t *testing.T) {
	cfg := &tools.TermCheckConfig{}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TargetLocale")

	cfg.TargetLocale = model.LocaleFrench
	cfg.Glossary = []tools.GlossaryEntry{{Source: "", Target: "x"}}
	err = cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty source")

	cfg.Glossary = []tools.GlossaryEntry{{Source: "x", Target: ""}}
	err = cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty target")

	cfg.Glossary = []tools.GlossaryEntry{{Source: "Save", Target: "Sauvegarder"}}
	err = cfg.Validate()
	assert.NoError(t, err)
}

// --- WordCountCollector Tests ---

func TestWordCountCollector(t *testing.T) {
	wc := tools.NewWordCountCollector()

	item := &flow.FlowItem{
		Input:        &model.RawDocument{URI: "doc1.html"},
		TargetLocale: model.LocaleFrench,
	}

	block1 := model.NewBlock("tu1", "Hello beautiful world")
	block1.Properties[tools.PropWordCountSource] = "3"
	block1.Properties[tools.PropWordCountTarget] = "4"

	block2 := model.NewBlock("tu2", "Goodbye")
	block2.Properties[tools.PropWordCountSource] = "1"

	parts := []*model.Part{
		{Type: model.PartLayerStart, Resource: &model.Layer{ID: "doc1"}},
		{Type: model.PartBlock, Resource: block1},
		{Type: model.PartBlock, Resource: block2},
		{Type: model.PartLayerEnd, Resource: &model.Layer{ID: "doc1"}},
	}

	err := wc.Collect(context.Background(), item, parts)
	require.NoError(t, err)

	result, err := wc.Result()
	require.NoError(t, err)
	assert.Equal(t, "word-count", result.Name)

	summary := result.Data.(*tools.WordCountSummary)
	assert.Equal(t, 4, summary.TotalSourceWords)
	assert.Equal(t, 4, summary.TotalTargetWords[model.LocaleFrench])
	assert.Equal(t, 1, summary.DocumentCount)

	doc := summary.Documents["doc1.html"]
	assert.Equal(t, 4, doc.SourceWords)
	assert.Equal(t, 4, doc.TargetWords[model.LocaleFrench])
	assert.Equal(t, 2, doc.BlockCount)
}

func TestWordCountCollectorMultipleDocuments(t *testing.T) {
	wc := tools.NewWordCountCollector()

	for _, uri := range []string{"a.html", "b.html", "c.html"} {
		item := &flow.FlowItem{
			Input:        &model.RawDocument{URI: uri},
			TargetLocale: model.LocaleFrench,
		}
		block := model.NewBlock("tu1", "text")
		block.Properties[tools.PropWordCountSource] = "2"
		block.Properties[tools.PropWordCountTarget] = "3"

		parts := []*model.Part{
			{Type: model.PartBlock, Resource: block},
		}
		err := wc.Collect(context.Background(), item, parts)
		require.NoError(t, err)
	}

	result, err := wc.Result()
	require.NoError(t, err)

	summary := result.Data.(*tools.WordCountSummary)
	assert.Equal(t, 6, summary.TotalSourceWords)
	assert.Equal(t, 9, summary.TotalTargetWords[model.LocaleFrench])
	assert.Equal(t, 3, summary.DocumentCount)
	assert.Len(t, summary.Documents, 3)
}

func TestWordCountCollectorSkipsNonBlocks(t *testing.T) {
	wc := tools.NewWordCountCollector()

	item := &flow.FlowItem{
		Input: &model.RawDocument{URI: "doc.html"},
	}

	parts := []*model.Part{
		{Type: model.PartLayerStart, Resource: &model.Layer{ID: "doc1"}},
		{Type: model.PartData, Resource: &model.Data{ID: "d1"}},
		{Type: model.PartLayerEnd, Resource: &model.Layer{ID: "doc1"}},
	}

	err := wc.Collect(context.Background(), item, parts)
	require.NoError(t, err)

	result, err := wc.Result()
	require.NoError(t, err)

	summary := result.Data.(*tools.WordCountSummary)
	assert.Equal(t, 0, summary.TotalSourceWords)
	assert.Empty(t, summary.TotalTargetWords)
	assert.Equal(t, 0, summary.Documents["doc.html"].BlockCount)
}

func TestWordCountCollectorSkipsNonTranslatable(t *testing.T) {
	wc := tools.NewWordCountCollector()

	item := &flow.FlowItem{
		Input: &model.RawDocument{URI: "doc.html"},
	}

	block := model.NewBlock("tu1", "Hello world")
	block.Translatable = false
	block.Properties[tools.PropWordCountSource] = "2"

	parts := []*model.Part{
		{Type: model.PartBlock, Resource: block},
	}

	err := wc.Collect(context.Background(), item, parts)
	require.NoError(t, err)

	result, err := wc.Result()
	require.NoError(t, err)

	summary := result.Data.(*tools.WordCountSummary)
	assert.Equal(t, 0, summary.TotalSourceWords)
}

// --- Multilingual WordCount Tests ---

func TestWordCountToolAllLocales(t *testing.T) {
	// Empty locale → count all target locales.
	cfg := &tools.WordCountConfig{}
	tl := tools.NewWordCountTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde")
	block.SetTargetText(model.LocaleGerman, "Hallo Welt")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	// Source always counted.
	assert.Equal(t, "2", resultBlock.Properties[tools.PropWordCountSource])
	// Legacy single key should NOT be set.
	_, hasLegacy := resultBlock.Properties[tools.PropWordCountTarget]
	assert.False(t, hasLegacy)
	// Per-locale keys should be set.
	assert.Equal(t, "3", resultBlock.Properties[tools.PropWordCountTargetPrefix+"fr"])
	assert.Equal(t, "2", resultBlock.Properties[tools.PropWordCountTargetPrefix+"de"])
}

func TestWordCountToolSingleLocaleBackwardCompat(t *testing.T) {
	// With locale set → legacy behavior.
	cfg := &tools.WordCountConfig{Locale: model.LocaleFrench}
	tl := tools.NewWordCountTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde")
	block.SetTargetText(model.LocaleGerman, "Hallo Welt")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "2", resultBlock.Properties[tools.PropWordCountSource])
	// Legacy single-locale key set.
	assert.Equal(t, "3", resultBlock.Properties[tools.PropWordCountTarget])
	// Per-locale keys should NOT be set.
	_, hasPerLocale := resultBlock.Properties[tools.PropWordCountTargetPrefix+"fr"]
	assert.False(t, hasPerLocale)
}

func TestWordCountCollectorPerLocaleProperties(t *testing.T) {
	wc := tools.NewWordCountCollector()

	item := &flow.FlowItem{
		Input: &model.RawDocument{URI: "doc.html"},
	}

	block := model.NewBlock("tu1", "Hello world")
	block.Properties[tools.PropWordCountSource] = "2"
	block.Properties[tools.PropWordCountTargetPrefix+"fr"] = "3"
	block.Properties[tools.PropWordCountTargetPrefix+"de"] = "2"

	parts := []*model.Part{
		{Type: model.PartBlock, Resource: block},
	}

	err := wc.Collect(context.Background(), item, parts)
	require.NoError(t, err)

	result, err := wc.Result()
	require.NoError(t, err)

	summary := result.Data.(*tools.WordCountSummary)
	assert.Equal(t, 2, summary.TotalSourceWords)
	assert.Equal(t, 3, summary.TotalTargetWords[model.LocaleFrench])
	assert.Equal(t, 2, summary.TotalTargetWords[model.LocaleGerman])
	assert.Equal(t, 1, summary.DocumentCount)

	doc := summary.Documents["doc.html"]
	assert.Equal(t, 3, doc.TargetWords[model.LocaleFrench])
	assert.Equal(t, 2, doc.TargetWords[model.LocaleGerman])
}

func TestWordCountSummaryFormatTable(t *testing.T) {
	summary := &tools.WordCountSummary{
		TotalSourceWords: 10,
		TotalTargetWords: map[model.LocaleID]int{
			model.LocaleFrench: 12,
		},
		DocumentCount: 2,
		Documents: map[string]tools.DocumentWordCount{
			"a.html": {
				URI: "a.html", SourceWords: 5, BlockCount: 2,
				TargetWords: map[model.LocaleID]int{model.LocaleFrench: 6},
			},
			"b.html": {
				URI: "b.html", SourceWords: 5, BlockCount: 3,
				TargetWords: map[model.LocaleID]int{model.LocaleFrench: 6},
			},
		},
	}

	var buf strings.Builder
	summary.FormatTable(&buf)
	output := buf.String()

	// Should contain header.
	assert.Contains(t, output, "FILE")
	assert.Contains(t, output, "BLOCKS")
	assert.Contains(t, output, "SOURCE WORDS")
	assert.Contains(t, output, "TARGET (fr)")
	// Should contain document rows.
	assert.Contains(t, output, "a.html")
	assert.Contains(t, output, "b.html")
	// Should contain total row.
	assert.Contains(t, output, "Total (2 files)")
}
