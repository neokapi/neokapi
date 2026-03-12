package model_test

import (
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newVocabRegistry(t *testing.T) *model.VocabularyRegistry {
	t.Helper()
	reg := model.NewVocabularyRegistry()
	require.NoError(t, reg.LoadDefaults())
	return reg
}

func TestSemanticHTML(t *testing.T) {
	reg := newVocabRegistry(t)

	frag := model.NewFragment("")
	frag.AppendText("Click ")
	frag.AppendSpan(&model.Span{SpanType: model.SpanOpening, Type: "fmt:bold", ID: "1", Data: `<b class="x">`})
	frag.AppendText("here")
	frag.AppendSpan(&model.Span{SpanType: model.SpanClosing, Type: "fmt:bold", ID: "1", Data: "</b>"})
	frag.AppendText(" for info")

	assert.Equal(t, "Click <b>here</b> for info", frag.SemanticHTML(reg))
}

func TestSemanticHTMLPlaceholder(t *testing.T) {
	reg := newVocabRegistry(t)

	frag := model.NewFragment("")
	frag.AppendText("Line one")
	frag.AppendSpan(&model.Span{SpanType: model.SpanPlaceholder, Type: "struct:break", ID: "1", Data: "<br/>"})
	frag.AppendText("Line two")

	assert.Equal(t, "Line one<br/>Line two", frag.SemanticHTML(reg))
}

func TestSemanticHTMLUnknownType(t *testing.T) {
	reg := newVocabRegistry(t)

	frag := model.NewFragment("")
	frag.AppendText("Hello ")
	frag.AppendSpan(&model.Span{SpanType: model.SpanOpening, Type: "custom:foo", ID: "1", Data: "<x>"})
	frag.AppendText("world")
	frag.AppendSpan(&model.Span{SpanType: model.SpanClosing, Type: "custom:foo", ID: "1", Data: "</x>"})

	assert.Equal(t, `Hello <span data-type="custom:foo">world</span>`,
		frag.SemanticHTML(reg))
}

func TestSemanticHTMLNoSpans(t *testing.T) {
	reg := newVocabRegistry(t)
	frag := model.NewFragment("plain text")
	assert.Equal(t, "plain text", frag.SemanticHTML(reg))
}

func TestPlaceholderText(t *testing.T) {
	frag := model.NewFragment("")
	frag.AppendText("Click ")
	frag.AppendSpan(&model.Span{SpanType: model.SpanOpening, Type: "fmt:bold", ID: "1", Data: "<b>"})
	frag.AppendText("here")
	frag.AppendSpan(&model.Span{SpanType: model.SpanClosing, Type: "fmt:bold", ID: "1", Data: "</b>"})
	frag.AppendText(" for ")
	frag.AppendSpan(&model.Span{SpanType: model.SpanPlaceholder, Type: "media:image", ID: "2", Data: `<img src="x"/>`})

	expected := `Click <x id="1"/>here<x id="/1"/> for <x id="2/"/>`
	assert.Equal(t, expected, frag.PlaceholderText())
}

func TestPlaceholderTextNoSpans(t *testing.T) {
	frag := model.NewFragment("plain text")
	assert.Equal(t, "plain text", frag.PlaceholderText())
}

func TestParsePlaceholderText(t *testing.T) {
	sourceSpans := []*model.Span{
		{SpanType: model.SpanOpening, Type: "fmt:bold", ID: "1", Data: `<b class="x">`, DisplayText: "[B]"},
		{SpanType: model.SpanClosing, Type: "fmt:bold", ID: "1", Data: "</b>", DisplayText: "[/B]"},
		{SpanType: model.SpanPlaceholder, Type: "media:image", ID: "2", Data: `<img src="logo.png"/>`, DisplayText: "[IMG]"},
	}

	// Simulated LLM response with reordered tags.
	response := `Cliquez <x id="1"/>ici<x id="/1"/> pour <x id="2/"/>`

	frag := model.ParsePlaceholderText(response, sourceSpans)
	assert.Equal(t, "Cliquez ici pour ", frag.Text())
	assert.True(t, frag.HasSpans())
	require.Len(t, frag.Spans, 3)

	// Opening span restored from source.
	assert.Equal(t, model.SpanOpening, frag.Spans[0].SpanType)
	assert.Equal(t, "fmt:bold", frag.Spans[0].Type)
	assert.Equal(t, `<b class="x">`, frag.Spans[0].Data)
	assert.Equal(t, "[B]", frag.Spans[0].DisplayText)

	// Closing span.
	assert.Equal(t, model.SpanClosing, frag.Spans[1].SpanType)
	assert.Equal(t, "fmt:bold", frag.Spans[1].Type)
	assert.Equal(t, "</b>", frag.Spans[1].Data)

	// Placeholder span.
	assert.Equal(t, model.SpanPlaceholder, frag.Spans[2].SpanType)
	assert.Equal(t, "media:image", frag.Spans[2].Type)
	assert.Equal(t, `<img src="logo.png"/>`, frag.Spans[2].Data)
}

func TestParseSemanticHTML(t *testing.T) {
	reg := newVocabRegistry(t)

	sourceSpans := []*model.Span{
		{SpanType: model.SpanOpening, Type: "fmt:bold", ID: "1", Data: `<b class="emphasis">`, DisplayText: "[B]"},
		{SpanType: model.SpanClosing, Type: "fmt:bold", ID: "1", Data: "</b>", DisplayText: "[/B]"},
	}

	// MT API response with semantic HTML.
	response := "Cliquez <b>ici</b>"

	frag := model.ParseSemanticHTML(response, sourceSpans, reg)
	assert.Equal(t, "Cliquez ici", frag.Text())
	assert.True(t, frag.HasSpans())
	require.Len(t, frag.Spans, 2)

	// Opening span restored with original Data.
	assert.Equal(t, model.SpanOpening, frag.Spans[0].SpanType)
	assert.Equal(t, "fmt:bold", frag.Spans[0].Type)
	assert.Equal(t, `<b class="emphasis">`, frag.Spans[0].Data)
	assert.Equal(t, "[B]", frag.Spans[0].DisplayText)

	// Closing span.
	assert.Equal(t, model.SpanClosing, frag.Spans[1].SpanType)
	assert.Equal(t, "</b>", frag.Spans[1].Data)
}

func TestParseSemanticHTMLWithPlaceholder(t *testing.T) {
	reg := newVocabRegistry(t)

	sourceSpans := []*model.Span{
		{SpanType: model.SpanPlaceholder, Type: "struct:break", ID: "1", Data: "<br/>"},
	}

	response := "Line one<br/>Line two"

	frag := model.ParseSemanticHTML(response, sourceSpans, reg)
	assert.Equal(t, "Line oneLine two", frag.Text())
	require.Len(t, frag.Spans, 1)
	assert.Equal(t, model.SpanPlaceholder, frag.Spans[0].SpanType)
	assert.Equal(t, "<br/>", frag.Spans[0].Data)
}

func TestFragmentJSONRoundtripWithNewFields(t *testing.T) {
	frag := model.NewFragment("")
	frag.AppendText("Click ")
	frag.AppendSpan(&model.Span{
		SpanType:    model.SpanOpening,
		Type:        "fmt:bold",
		SubType:     "html:b",
		ID:          "1",
		Data:        `<b class="emphasis">`,
		DisplayText: "[B]",
		EquivText:   "",
		Deletable:   true,
		Cloneable:   true,
		CanReorder:  true,
	})
	frag.AppendText("here")
	frag.AppendSpan(&model.Span{
		SpanType:    model.SpanClosing,
		Type:        "fmt:bold",
		SubType:     "html:b",
		ID:          "1",
		Data:        "</b>",
		DisplayText: "[/B]",
		Deletable:   true,
		Cloneable:   true,
		CanReorder:  true,
	})

	data, err := frag.MarshalJSON()
	require.NoError(t, err)

	// Verify JSON contains new fields.
	jsonStr := string(data)
	assert.Contains(t, jsonStr, `"sub_type":"html:b"`)
	assert.Contains(t, jsonStr, `"display_text":"[B]"`)
	assert.Contains(t, jsonStr, `"can_reorder":true`)

	// Roundtrip.
	var decoded model.Fragment
	err = decoded.UnmarshalJSON(data)
	require.NoError(t, err)

	require.Len(t, decoded.Spans, 2)
	assert.Equal(t, "html:b", decoded.Spans[0].SubType)
	assert.Equal(t, "[B]", decoded.Spans[0].DisplayText)
	assert.True(t, decoded.Spans[0].CanReorder)
	assert.True(t, decoded.Spans[0].Deletable)
	assert.True(t, decoded.Spans[0].Cloneable)
}

func TestStructuralTextWithEntityPrefix(t *testing.T) {
	frag := model.NewFragment("")
	frag.AppendSpan(&model.Span{SpanType: model.SpanPlaceholder, Type: "entity:person", ID: "1", Data: "John"})
	frag.AppendText(" works at ")
	frag.AppendSpan(&model.Span{SpanType: model.SpanPlaceholder, Type: "entity:organization", ID: "2", Data: "Acme"})

	// StructuralText uses numbered placeholders.
	assert.Equal(t, "{1/} works at {2/}", frag.StructuralText())

	// GeneralizedText uses typed placeholders for entities.
	assert.Equal(t, "{PERSON} works at {ORGANIZATION}", frag.GeneralizedText())
}
