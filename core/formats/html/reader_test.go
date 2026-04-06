package html_test

import (
	"bytes"
	"testing"

	htmlfmt "github.com/neokapi/neokapi/core/formats/html"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// okapi: HtmlSnippetsTest#minimalCompleteHtml
func TestReadSimpleHTML(t *testing.T) {
	ctx := t.Context()
	reader := htmlfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(`<html><body><p>Hello world</p></body></html>`, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	require.GreaterOrEqual(t, len(blocks), 1)
	assert.Equal(t, "Hello world", blocks[0].SourceText())
}

// okapi: HtmlSnippetsTest#ITextUnitsInARowWithTwoHeaders
func TestReadMultipleBlocks(t *testing.T) {
	ctx := t.Context()
	reader := htmlfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(`<html><body><h1>Title</h1><p>Paragraph</p></body></html>`, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.GreaterOrEqual(t, len(blocks), 2)
	texts := testutil.BlockTexts(blocks)
	assert.Contains(t, texts, "Title")
	assert.Contains(t, texts, "Paragraph")
}

// okapi: HtmlSnippetsTest#testPWithInlines
func TestReadInlineSpans(t *testing.T) {
	ctx := t.Context()
	reader := htmlfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(`<html><body><p>Click <b>here</b> for info</p></body></html>`, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.GreaterOrEqual(t, len(blocks), 1)

	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	assert.Equal(t, "Click here for info", frag.Text())
	assert.True(t, frag.HasSpans())
	require.Len(t, frag.Spans, 2)
	assert.Equal(t, model.SpanOpening, frag.Spans[0].SpanType)
	assert.Equal(t, "fmt:bold", frag.Spans[0].Type)
	assert.Equal(t, "html:b", frag.Spans[0].SubType)
	assert.Equal(t, "1", frag.Spans[0].ID)
	assert.Equal(t, "<b>", frag.Spans[0].Data)
	assert.Equal(t, "[B]", frag.Spans[0].DisplayText)
	assert.True(t, frag.Spans[0].Deletable)
	assert.Equal(t, model.SpanClosing, frag.Spans[1].SpanType)
	assert.Equal(t, "fmt:bold", frag.Spans[1].Type)
	assert.Equal(t, "1", frag.Spans[1].ID) // same ID as opening
	assert.Equal(t, "</b>", frag.Spans[1].Data)
}

// okapi: HtmlSnippetsTest#testHref
func TestReadLinkSpan(t *testing.T) {
	ctx := t.Context()
	reader := htmlfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(`<html><body><p>Visit <a href="http://example.com">our site</a></p></body></html>`, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.GreaterOrEqual(t, len(blocks), 1)

	frag := blocks[0].FirstFragment()
	assert.Equal(t, "Visit our site", frag.Text())
	assert.True(t, frag.HasSpans())

	// Should have opening and closing link:hyperlink spans
	var openingSpan, closingSpan *model.Span
	for _, s := range frag.Spans {
		if s.SpanType == model.SpanOpening && s.Type == "link:hyperlink" {
			openingSpan = s
		}
		if s.SpanType == model.SpanClosing && s.Type == "link:hyperlink" {
			closingSpan = s
		}
	}
	require.NotNil(t, openingSpan)
	require.NotNil(t, closingSpan)
	assert.Contains(t, openingSpan.Data, "href")
	assert.Equal(t, "html:a", openingSpan.SubType)
	assert.Equal(t, "1", openingSpan.ID)
	assert.Equal(t, openingSpan.ID, closingSpan.ID)
}

// okapi: HtmlSnippetsTest#paraWithBreak
func TestReadPlaceholderSpan(t *testing.T) {
	ctx := t.Context()
	reader := htmlfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(`<html><body><p>Line one<br/>Line two</p></body></html>`, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.GreaterOrEqual(t, len(blocks), 1)

	frag := blocks[0].FirstFragment()
	assert.Equal(t, "Line oneLine two", frag.Text())
	assert.True(t, frag.HasSpans())

	// br should be a placeholder span with semantic type
	found := false
	for _, s := range frag.Spans {
		if s.SpanType == model.SpanPlaceholder && s.Type == "struct:break" {
			found = true
			assert.Equal(t, "html:br", s.SubType)
			assert.Equal(t, "1", s.ID)
		}
	}
	assert.True(t, found, "expected <br/> to be a placeholder span")
}

// okapi: HtmlFullFileTest#testSkippedScriptandStyleElements
func TestReadScriptNonTranslatable(t *testing.T) {
	ctx := t.Context()
	reader := htmlfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(`<html><body><p>Hello</p><script>var x = 1;</script><p>World</p></body></html>`, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	texts := testutil.BlockTexts(blocks)

	assert.Contains(t, texts, "Hello")
	assert.Contains(t, texts, "World")
	// Script content should NOT be in blocks
	for _, text := range texts {
		assert.NotContains(t, text, "var x")
	}
}

func TestReadLayerStartEnd(t *testing.T) {
	ctx := t.Context()
	reader := htmlfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(`<html><body><p>Test</p></body></html>`, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	require.GreaterOrEqual(t, len(parts), 2)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "html", layer.Format)
}

// okapi: HtmlConfigurationTest#defaultConfiguration
func TestReadTitle(t *testing.T) {
	ctx := t.Context()
	reader := htmlfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(`<html><head><title>Page Title</title></head><body><p>Content</p></body></html>`, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	texts := testutil.BlockTexts(blocks)

	assert.Contains(t, texts, "Page Title")
	assert.Contains(t, texts, "Content")
}

func TestReaderSignature(t *testing.T) {
	reader := htmlfmt.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "text/html")
	assert.Contains(t, sig.Extensions, ".html")
	assert.Contains(t, sig.Extensions, ".htm")
}

func TestWriteBlockWithSkeleton(t *testing.T) {
	ctx := t.Context()
	reader := htmlfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(`<html><body><p>Hello world</p></body></html>`, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Set French targets
	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			block.SetTargetText(model.LocaleFrench, "Bonjour le monde")
		}
	}

	// Write
	var buf bytes.Buffer
	writer := htmlfmt.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Bonjour le monde")
	assert.NotContains(t, output, "Hello world")
}

func TestReadNilDocument(t *testing.T) {
	ctx := t.Context()
	reader := htmlfmt.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}

func TestReaderMetadata(t *testing.T) {
	reader := htmlfmt.NewReader()
	assert.Equal(t, "html", reader.Name())
	assert.Equal(t, "HTML", reader.DisplayName())
}
