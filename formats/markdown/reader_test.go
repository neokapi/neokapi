package markdown_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/gokapi/gokapi/model"
	"github.com/gokapi/gokapi/formats/markdown"
	"github.com/gokapi/gokapi/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadSimpleParagraphs(t *testing.T) {
	ctx := context.Background()
	reader := markdown.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("Hello world\n\nSecond paragraph", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	assert.Len(t, blocks, 2)
	assert.Equal(t, "Hello world", blocks[0].SourceText())
	assert.Equal(t, "Second paragraph", blocks[1].SourceText())
}

func TestReadHeadings(t *testing.T) {
	ctx := context.Background()
	reader := markdown.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("# Title\n\n## Subtitle\n\nText", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 3)
	assert.Equal(t, "Title", blocks[0].SourceText())
	assert.Equal(t, "heading", blocks[0].Type)
	assert.Equal(t, "1", blocks[0].Properties["level"])
	assert.Equal(t, "Subtitle", blocks[1].SourceText())
	assert.Equal(t, "heading", blocks[1].Type)
	assert.Equal(t, "2", blocks[1].Properties["level"])
	assert.Equal(t, "Text", blocks[2].SourceText())
}

func TestReadBoldItalicInline(t *testing.T) {
	ctx := context.Background()
	reader := markdown.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("This has **bold** and *italic* text", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "This has bold and italic text", blocks[0].SourceText())

	// Check that spans were detected
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	assert.True(t, frag.HasSpans())
}

func TestReadCodeBlockAsData(t *testing.T) {
	ctx := context.Background()
	reader := markdown.NewReader()
	input := "# Title\n\n```go\nfmt.Println(\"hello\")\n```\n\nText after code"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	// Should have Title and "Text after code" as blocks
	assert.Len(t, blocks, 2)
	assert.Equal(t, "Title", blocks[0].SourceText())
	assert.Equal(t, "Text after code", blocks[1].SourceText())

	// Verify there is a Data part for the code block
	hasCodeData := false
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if data.Name == "code-block" {
				hasCodeData = true
				assert.Equal(t, "go", data.Properties["language"])
			}
		}
	}
	assert.True(t, hasCodeData, "expected code block as Data")
}

func TestReadLists(t *testing.T) {
	ctx := context.Background()
	reader := markdown.NewReader()
	input := "- First item\n- Second item\n- Third item"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 3)
	assert.Equal(t, "First item", blocks[0].SourceText())
	assert.Equal(t, "Second item", blocks[1].SourceText())
	assert.Equal(t, "Third item", blocks[2].SourceText())
	assert.Equal(t, "list-item", blocks[0].Type)
}

func TestReadLayerStartEnd(t *testing.T) {
	ctx := context.Background()
	reader := markdown.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("Hello", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	require.GreaterOrEqual(t, len(parts), 3)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "markdown", layer.Format)
}

func TestReaderSignature(t *testing.T) {
	reader := markdown.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "text/markdown")
	assert.Contains(t, sig.Extensions, ".md")
}

func TestReaderMetadata(t *testing.T) {
	reader := markdown.NewReader()
	assert.Equal(t, "markdown", reader.Name())
	assert.Equal(t, "Markdown", reader.DisplayName())
}

func TestReadNilDocument(t *testing.T) {
	ctx := context.Background()
	reader := markdown.NewReader()
	err := reader.Open(ctx, nil)
	assert.Error(t, err)
}

func TestReadEmpty(t *testing.T) {
	ctx := context.Background()
	reader := markdown.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	assert.Empty(t, blocks)
}

func TestRoundTrip(t *testing.T) {
	ctx := context.Background()

	input := "# Hello\n\nThis is text\n\n- Item one\n- Item two"

	reader := markdown.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := markdown.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "# Hello")
	assert.Contains(t, output, "This is text")
	assert.Contains(t, output, "- Item one")
	assert.Contains(t, output, "- Item two")
}

func TestRoundTripWithTargetLocale(t *testing.T) {
	ctx := context.Background()

	reader := markdown.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("# Hello\n\nWorld", model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			if block.SourceText() == "Hello" {
				block.SetTargetText(model.LocaleFrench, "Bonjour")
			} else if block.SourceText() == "World" {
				block.SetTargetText(model.LocaleFrench, "Monde")
			}
		}
	}

	var buf bytes.Buffer
	writer := markdown.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "# Bonjour")
	assert.Contains(t, output, "Monde")
	assert.NotContains(t, output, "Hello")
	assert.NotContains(t, output, "World")
}

func TestReadHTMLBlockAsData(t *testing.T) {
	ctx := context.Background()
	reader := markdown.NewReader()
	input := "Text before\n\n<div>HTML content</div>\n\nText after"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	hasHTMLData := false
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if data.Name == "html-block" {
				hasHTMLData = true
			}
		}
	}
	assert.True(t, hasHTMLData, "expected HTML block as Data")
}
