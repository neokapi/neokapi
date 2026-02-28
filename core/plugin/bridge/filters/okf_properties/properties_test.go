//go:build integration

package okf_properties

import (
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.properties.PropertiesFilter"
const mimeType = "text/x-java-properties"

// okapi: PropertiesFilterTest#testEntry
func TestExtract_SimpleProperties(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		"greeting=Hello World\nfarewell=Goodbye",
		"test.properties", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from properties")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Hello World")
	assert.Contains(t, texts, "Goodbye")
}

func TestExtract_BlockIDs(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		"app.name=My App\napp.version=1.0",
		"test.properties", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2)

	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID, "block should have an ID")
		assert.False(t, ids[b.ID], "block IDs should be unique")
		ids[b.ID] = true
	}
}

// okapi: PropertiesFilterTest#testEscapes
func TestExtract_UnicodeEscapes(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		"greeting=\\u3053\\u3093\\u306b\\u3061\\u306f",
		"test.properties", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// Java properties files use \uXXXX unicode escapes.
	text := blocks[0].SourceText()
	assert.Contains(t, text, "こんにちは")
}

// okapi: PropertiesFilterTest#testSpecialChars
func TestExtract_EscapedCharacters(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		"msg=Hello\\=World\\!",
		"test.properties", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// Escaped special characters should be decoded.
	text := blocks[0].SourceText()
	assert.Contains(t, text, "Hello")
}

// okapi: PropertiesFilterTest#testSplicedEntry
func TestExtract_MultiLineValues(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		"msg=Line one \\\n    Line two",
		"test.properties", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	text := blocks[0].SourceText()
	assert.Contains(t, text, "Line one")
	assert.Contains(t, text, "Line two")
}

// okapi: PropertiesFilterTest#testKeySpecial
func TestExtract_ColonSeparator(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	// Properties files also support colon as a key-value separator.
	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		"greeting: Hello World",
		"test.properties", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Hello World")
}

func TestExtract_CommentsNotExtracted(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		"# This is a comment\n! Another comment\nkey=value",
		"test.properties", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "value")
	for _, text := range texts {
		assert.NotContains(t, text, "This is a comment")
		assert.NotContains(t, text, "Another comment")
	}
}

func TestExtract_EmptyValue(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		"empty=\nnotempty=has content",
		"test.properties", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "has content")
}

func TestExtract_LayerStructure(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		"key=value",
		"test.properties", mimeType, nil)

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

func TestExtract_SegmentIDs(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		"key=value",
		"test.properties", mimeType, nil)

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
