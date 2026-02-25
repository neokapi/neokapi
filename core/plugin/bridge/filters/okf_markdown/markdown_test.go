//go:build integration

package okf_markdown

import (
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.markdown.MarkdownFilter"
const mimeType = "text/markdown"

func TestExtract_SimpleMarkdown(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	md := "# Hello World\n\nThis is a paragraph.\n"

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		md, "test.md", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from Markdown")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Hello World")
}

func TestExtract_MultipleHeadings(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	md := "# Title\n\n## Subtitle\n\n### Section\n"

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		md, "test.md", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.Contains(t, texts, "Title")
	assert.Contains(t, texts, "Subtitle")
	assert.Contains(t, texts, "Section")
}

func TestExtract_InlineFormatting(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	md := "This has **bold** and *italic* text.\n"

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		md, "test.md", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// Find the block with inline formatting.
	var found *model.Block
	for _, b := range blocks {
		text := b.SourceText()
		if text == "This has bold and italic text." {
			found = b
			break
		}
	}
	require.NotNil(t, found, "should find block with inline formatting")

	// Inline markdown formatting should produce spans.
	frag := found.FirstFragment()
	require.NotNil(t, frag)
	assert.GreaterOrEqual(t, len(frag.Spans), 2,
		"should have spans for bold/italic markdown")
}

func TestExtract_Links(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	md := "Click [here](http://example.com) for more info.\n"

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		md, "test.md", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// Link text should be extracted with inline codes for the URL.
	texts := bridgetest.BlockTexts(blocks)
	found := false
	for _, text := range texts {
		if text == "Click here for more info." {
			found = true
			break
		}
	}
	assert.True(t, found, "should extract link text without raw URL in text")
}

func TestExtract_CodeBlockExtracted(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	md := "# Title\n\n```\nvar x = 1;\n```\n\nSome text.\n"

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		md, "test.md", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.Contains(t, texts, "Title")
	assert.Contains(t, texts, "Some text.")
	// The Okapi Markdown filter extracts code block content as translatable text.
	allText := ""
	for _, text := range texts {
		allText += text + "\n"
	}
	assert.Contains(t, allText, "var x = 1",
		"code block content should be extracted as translatable")
}

func TestExtract_UnorderedList(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	md := "- Item one\n- Item two\n- Item three\n"

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		md, "test.md", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.Contains(t, texts, "Item one")
	assert.Contains(t, texts, "Item two")
	assert.Contains(t, texts, "Item three")
}

func TestExtract_BlockIDs(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	md := "# Title\n\nFirst paragraph.\n\nSecond paragraph.\n"

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		md, "test.md", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2)

	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID)
		assert.False(t, ids[b.ID], "block IDs should be unique")
		ids[b.ID] = true
	}
}

func TestExtract_LayerStructure(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		"# Hello\n", "test.md", mimeType, nil)

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

func TestExtract_UnicodeContent(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	md := "# こんにちは\n\nHello 🌍\n"

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		md, "test.md", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.Contains(t, texts, "こんにちは")
	assert.Contains(t, texts, "Hello 🌍")
}
