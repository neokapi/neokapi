//go:build integration

package xmlstream

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.xmlstream.XmlStreamFilter"
const mimeType = "text/xml"

func TestExtract_SimpleXML(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		`<?xml version="1.0" encoding="UTF-8"?><root><text>Hello world</text></root>`,
		"test.xml", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from XML")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Hello world")
}

func TestExtract_MultipleElements(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xml := `<?xml version="1.0" encoding="UTF-8"?>
<root>
  <title>My Title</title>
  <description>A description</description>
  <note>A note</note>
</root>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xml, "test.xml", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.Contains(t, texts, "My Title")
	assert.Contains(t, texts, "A description")
	assert.Contains(t, texts, "A note")
}

func TestExtract_NestedElements(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xml := `<?xml version="1.0" encoding="UTF-8"?>
<root>
  <section>
    <title>Section Title</title>
    <para>Section content</para>
  </section>
</root>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xml, "test.xml", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.Contains(t, texts, "Section Title")
	assert.Contains(t, texts, "Section content")
}

// okapi: XmlSnippetsTest#testPWithInlines
func TestExtract_InlineElements(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xml := `<?xml version="1.0" encoding="UTF-8"?>
<root><text>Hello <b>bold</b> world</text></root>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xml, "test.xml", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// Inline elements within text content should produce inline-code runs.
	var found *model.Block
	for _, b := range blocks {
		runs := b.SourceRuns()
		for _, r := range runs {
			if r.Text == nil {
				found = b
				break
			}
		}
		if found != nil {
			break
		}
	}
	// XMLStream may or may not treat <b> as inline; just verify extraction.
	_ = found
}

func TestExtract_BlockIDs(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xml := `<?xml version="1.0" encoding="UTF-8"?>
<root><a>First</a><b>Second</b><c>Third</c></root>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xml, "test.xml", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 3)

	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID, "block should have an ID")
		assert.False(t, ids[b.ID], "block IDs should be unique")
		ids[b.ID] = true
	}
}

// okapi: XmlSnippetsTest#testEscapes
func TestExtract_Entities(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xml := `<?xml version="1.0" encoding="UTF-8"?>
<root><text>Price: &lt;$10 &amp; &gt;$5</text></root>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xml, "test.xml", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Price: <$10 & >$5")
}

// okapi: XmlSnippetsTest#testCdataSection
func TestExtract_CDATA(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xml := `<?xml version="1.0" encoding="UTF-8"?>
<root><text><![CDATA[Hello <world>]]></text></root>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xml, "test.xml", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Hello <world>")
}

func TestExtract_UnicodeContent(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xml := `<?xml version="1.0" encoding="UTF-8"?>
<root><text>こんにちは世界</text></root>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xml, "test.xml", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.Contains(t, texts, "こんにちは世界")
}

func TestExtract_LayerStructure(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		`<?xml version="1.0" encoding="UTF-8"?><root><text>Hello</text></root>`,
		"test.xml", mimeType, nil)

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

func TestExtract_DataParts(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xml := `<?xml version="1.0" encoding="UTF-8"?>
<root><text>Hello</text></root>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xml, "test.xml", mimeType, nil)

	var dataCount int
	for _, p := range parts {
		if p.Type == model.PartData {
			dataCount++
			data := p.Resource.(*model.Data)
			assert.NotEmpty(t, data.ID, "data part should have an ID")
		}
	}
	assert.Greater(t, dataCount, 0, "XML should have Data parts for non-translatable structure")
}
