//go:build integration

package okf_yaml

import (
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.yaml.YamlFilter"
const mimeType = "application/x-yaml"

func TestExtract_SimpleYAML(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		"greeting: Hello World\nfarewell: Goodbye\n",
		"test.yaml", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from YAML")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Hello World")
}

func TestExtract_MultipleKeys(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	yaml := "title: My App\ndescription: A great application\nversion: 1.0.0\n"

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		yaml, "test.yaml", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.Contains(t, texts, "My App")
	assert.Contains(t, texts, "A great application")
	assert.Contains(t, texts, "1.0.0")
}

func TestExtract_NestedKeys(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	yaml := "page:\n  title: Welcome\n  body: Hello World\n"

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		yaml, "test.yaml", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.Contains(t, texts, "Welcome")
	assert.Contains(t, texts, "Hello World")
}

func TestExtract_DeepNesting(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	yaml := "level1:\n  level2:\n    level3:\n      deep: Deep value\n"

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		yaml, "test.yaml", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.Contains(t, texts, "Deep value")
}

func TestExtract_GroupStructure(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	yaml := "section:\n  title: Section Title\n  content: Section Content\n"

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		yaml, "test.yaml", mimeType, nil)

	// Nested YAML objects should produce group start/end parts.
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

func TestExtract_BlockIDs(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	yaml := "a: First\nb: Second\nc: Third\n"

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		yaml, "test.yaml", mimeType, nil)

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

	yaml := "greeting: こんにちは世界\nemoji: Hello 🌍\n"

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		yaml, "test.yaml", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.Contains(t, texts, "こんにちは世界")
	assert.Contains(t, texts, "Hello 🌍")
}

func TestExtract_LayerStructure(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		"key: value\n", "test.yaml", mimeType, nil)

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type, "last part should be LayerEnd")
}

func TestExtract_DataParts(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		"key: value\n", "test.yaml", mimeType, nil)

	var dataCount int
	for _, p := range parts {
		if p.Type == model.PartData {
			dataCount++
			data := p.Resource.(*model.Data)
			assert.NotEmpty(t, data.ID, "data part should have an ID")
		}
	}
	assert.Greater(t, dataCount, 0, "YAML should have Data parts")
}

func TestExtract_AllValuesExtracted(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	// The YAML filter treats all scalar values as translatable text,
	// including numbers and booleans (they become string representations).
	yaml := "name: John\nage: 30\nactive: true\n"

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		yaml, "test.yaml", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.Contains(t, texts, "John")
	assert.Contains(t, texts, "30")
	assert.Contains(t, texts, "true")
}
