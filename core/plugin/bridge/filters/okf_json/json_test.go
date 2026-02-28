//go:build integration

package okf_json

import (
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.json.JSONFilter"
const mimeType = "application/json"

// okapi: JSONFilterTest#testValue
func TestExtract_SimpleJSON(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		`{"greeting": "Hello World", "count": 42}`,
		"test.json", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from JSON")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Hello World")
}

// okapi: JSONFilterTest#testObject
func TestExtract_NestedObjects(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	json := `{
		"page": {
			"title": "Welcome",
			"body": "Hello World"
		}
	}`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		json, "test.json", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.Contains(t, texts, "Welcome")
	assert.Contains(t, texts, "Hello World")
}

func TestExtract_ArrayOfObjects(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	// Okapi's JSON filter extracts string values from key-value pairs within objects.
	// Array items that are objects with string values are extracted.
	json := `{
		"items": [
			{"name": "First"},
			{"name": "Second"},
			{"name": "Third"}
		]
	}`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		json, "test.json", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.Contains(t, texts, "First")
	assert.Contains(t, texts, "Second")
	assert.Contains(t, texts, "Third")
}

func TestExtract_BlockMetadata(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		`{"key": "value"}`,
		"test.json", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	assert.NotEmpty(t, b.ID, "block should have an ID")
	assert.True(t, b.Translatable, "block should be translatable")
	require.NotEmpty(t, b.Source, "block should have source segments")
	assert.NotEmpty(t, b.Source[0].ID, "segment should have an ID")
}

func TestExtract_LayerStructure(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		`{"key": "value"}`,
		"test.json", mimeType, nil)

	var hasLayerStart, hasLayerEnd, hasBlock bool
	for _, p := range parts {
		switch p.Type {
		case model.PartLayerStart:
			hasLayerStart = true
		case model.PartLayerEnd:
			hasLayerEnd = true
		case model.PartBlock:
			hasBlock = true
		}
	}
	assert.True(t, hasLayerStart, "should have LayerStart")
	assert.True(t, hasLayerEnd, "should have LayerEnd")
	assert.True(t, hasBlock, "should have Block")
}

// okapi: JSONFilterTest#testAllWithKeyNoException
func TestExtract_WithFilterParams(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	json := `{"key": "value", "num": 42, "flag": true}`

	// extractAllPairs=true should extract all string values.
	params := map[string]any{
		"extractAllPairs": true,
	}

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		json, "test.json", mimeType, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "value")
}

func TestExtract_GroupStructure(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	json := `{
		"section": {
			"title": "Section Title",
			"content": "Section Content"
		}
	}`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		json, "test.json", mimeType, nil)

	// Check for group start/end parts (JSON objects create groups).
	var hasGroupStart, hasGroupEnd bool
	for _, p := range parts {
		if p.Type == model.PartGroupStart {
			hasGroupStart = true
		}
		if p.Type == model.PartGroupEnd {
			hasGroupEnd = true
		}
	}
	assert.True(t, hasGroupStart, "nested objects should produce GroupStart")
	assert.True(t, hasGroupEnd, "nested objects should produce GroupEnd")
}

func TestExtract_UniqueBlockIDs(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	json := `{"a": "First", "b": "Second", "c": "Third"}`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		json, "test.json", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 3)

	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID)
		assert.False(t, ids[b.ID], "duplicate block ID: %s", b.ID)
		ids[b.ID] = true
	}
}

// okapi: JSONFilterTest#testDecimalNumber
func TestExtract_NumbersNotExtracted(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	json := `{"name": "John", "age": 30, "active": true, "score": 9.5}`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		json, "test.json", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	// By default, only string values are extracted.
	assert.Contains(t, texts, "John")
	for _, text := range texts {
		assert.NotEqual(t, "30", text, "numbers should not be extracted")
		assert.NotEqual(t, "true", text, "booleans should not be extracted")
		assert.NotEqual(t, "9.5", text, "floats should not be extracted")
	}
}

func TestExtract_DeepNesting(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	json := `{
		"level1": {
			"level2": {
				"level3": {
					"deep": "Deep value"
				}
			}
		}
	}`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		json, "test.json", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Deep value")
}

// okapi: JSONFilterTest#testEmptyValue
func TestExtract_EmptyStrings(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	json := `{"empty": "", "notempty": "has content"}`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		json, "test.json", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "has content")
}

// okapi: JSONFilterTest#testEscapes
func TestExtract_SpecialCharacters(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	json := `{"msg": "Hello \"world\" & <friends>"}`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		json, "test.json", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Hello \"world\" & <friends>")
}

func TestExtract_UnicodeContent(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	json := `{"greeting": "こんにちは世界", "emoji": "Hello 🌍"}`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		json, "test.json", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "こんにちは世界")
	assert.Contains(t, texts, "Hello 🌍")
}

func TestExtract_DataParts(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	json := `{"key": "value"}`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		json, "test.json", mimeType, nil)

	var dataCount int
	for _, p := range parts {
		if p.Type == model.PartData {
			dataCount++
			data := p.Resource.(*model.Data)
			assert.NotEmpty(t, data.ID, "data part should have an ID")
		}
	}
	assert.Greater(t, dataCount, 0, "JSON should have Data parts for structure")
}

func TestExtract_DataSkeleton(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	json := `{"a": "First", "b": "Second"}`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		json, "test.json", mimeType, nil)

	var dataWithSkeleton int
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if data.Skeleton != nil && len(data.Skeleton.Parts) > 0 {
				dataWithSkeleton++
			}
		}
	}
	assert.Greater(t, dataWithSkeleton, 0, "some JSON Data parts should have skeleton data")
}

// okapi: JSONFilterTest#testStandaloneDefaultWhichIsNo
func TestExtract_ArrayOfStringsNotExtracted(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	// Okapi's JSON filter does NOT extract bare string array values by default.
	// Only key-value pairs in objects are extracted.
	json := `{
		"colors": ["Red", "Green", "Blue"],
		"label": "Color list"
	}`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		json, "test.json", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	// Only the key-value pair should be extracted.
	assert.Contains(t, texts, "Color list")
	assert.NotContains(t, texts, "Red", "bare array strings should not be extracted by default")
}

func TestExtract_MultipleKeys(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	json := `{
		"title": "My App",
		"description": "A great application",
		"version": "1.0.0"
	}`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		json, "test.json", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.Contains(t, texts, "My App")
	assert.Contains(t, texts, "A great application")
	assert.Contains(t, texts, "1.0.0")
}
