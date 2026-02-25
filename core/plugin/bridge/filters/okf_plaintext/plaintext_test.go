//go:build integration

package okf_plaintext

import (
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.plaintext.PlainTextFilter"
const mimeType = "text/plain"

func TestExtract_SimplePlainText(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		"Hello world\nThis is a test.",
		"test.txt", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from plain text")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Hello world")
}

func TestExtract_MultipleLines(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		"First line\nSecond line\nThird line",
		"test.txt", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 3)

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "First line")
	assert.Contains(t, texts, "Second line")
	assert.Contains(t, texts, "Third line")
}

func TestExtract_BlockIDs(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		"Line A\nLine B\nLine C",
		"test.txt", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 3)

	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID, "block should have an ID")
		assert.False(t, ids[b.ID], "block IDs should be unique")
		ids[b.ID] = true
	}
}

func TestExtract_UnicodeContent(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		"こんにちは世界\nHello 🌍",
		"test.txt", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.Contains(t, texts, "こんにちは世界")
	assert.Contains(t, texts, "Hello 🌍")
}

func TestExtract_LayerStructure(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		"Hello", "test.txt", mimeType, nil)

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type, "last part should be LayerEnd")
}

func TestExtract_SegmentIDs(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		"Hello world",
		"test.txt", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	for _, b := range blocks {
		require.NotEmpty(t, b.Source, "block should have source segments")
		for _, seg := range b.Source {
			assert.NotEmpty(t, seg.ID, "segment should have an ID")
			assert.NotNil(t, seg.Content, "segment should have content")
		}
	}
}

func TestExtract_NoSpans(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	// Plain text has no inline markup, so blocks should have no spans.
	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		"Simple plain text",
		"test.txt", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	for _, b := range blocks {
		frag := b.FirstFragment()
		if frag != nil {
			assert.Empty(t, frag.Spans, "plain text should have no inline spans")
		}
	}
}
