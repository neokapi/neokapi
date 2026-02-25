//go:build integration

package okf_xliff

import (
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.xliff.XLIFFFilter"
const mimeType = "application/xliff+xml"

func TestExtract_SimpleXLIFF(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <body>
      <trans-unit id="1">
        <source>Hello world</source>
      </trans-unit>
    </body>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff, "test.xlf", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from XLIFF")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Hello world")
}

func TestExtract_MultipleTransUnits(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <body>
      <trans-unit id="1">
        <source>First</source>
      </trans-unit>
      <trans-unit id="2">
        <source>Second</source>
      </trans-unit>
      <trans-unit id="3">
        <source>Third</source>
      </trans-unit>
    </body>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff, "test.xlf", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.Len(t, blocks, 3)

	texts := bridgetest.BlockTexts(blocks)
	assert.Equal(t, "First", texts[0])
	assert.Equal(t, "Second", texts[1])
	assert.Equal(t, "Third", texts[2])
}

func TestExtract_TransUnitIDs(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <body>
      <trans-unit id="greeting">
        <source>Hello</source>
      </trans-unit>
      <trans-unit id="farewell">
        <source>Goodbye</source>
      </trans-unit>
    </body>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff, "test.xlf", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.Len(t, blocks, 2)

	// Block IDs should match the trans-unit IDs.
	assert.Equal(t, "greeting", blocks[0].ID)
	assert.Equal(t, "farewell", blocks[1].ID)
}

func TestExtract_InlineCodes(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <body>
      <trans-unit id="1">
        <source>Click <g id="1">&lt;b&gt;</g>here<g id="2">&lt;/b&gt;</g> to continue</source>
      </trans-unit>
    </body>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff, "test.xlf", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	// Inline <g> elements should produce spans.
	assert.GreaterOrEqual(t, len(frag.Spans), 2, "should have spans for <g> elements")
}

func TestExtract_WithTarget(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <body>
      <trans-unit id="1">
        <source>Hello</source>
        <target>Bonjour</target>
      </trans-unit>
    </body>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff, "test.xlf", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	assert.Equal(t, "Hello", b.SourceText())
	assert.True(t, b.HasTarget("fr"), "should have French target")
	assert.Equal(t, "Bonjour", b.TargetText("fr"))
}

func TestExtract_TranslateNo(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <body>
      <trans-unit id="1" translate="yes">
        <source>Translate me</source>
      </trans-unit>
      <trans-unit id="2" translate="no">
        <source>Do not translate</source>
      </trans-unit>
    </body>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff, "test.xlf", mimeType, nil)

	allBlocks := bridgetest.FilterBlocks(parts)
	require.GreaterOrEqual(t, len(allBlocks), 2)

	translatableBlocks := bridgetest.TranslatableBlocks(parts)
	require.Len(t, translatableBlocks, 1)
	assert.Equal(t, "Translate me", translatableBlocks[0].SourceText())
}

func TestExtract_GroupStructure(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <body>
      <group id="g1">
        <trans-unit id="1">
          <source>In group</source>
        </trans-unit>
      </group>
    </body>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff, "test.xlf", mimeType, nil)

	var hasGroupStart, hasGroupEnd bool
	for _, p := range parts {
		if p.Type == model.PartGroupStart {
			hasGroupStart = true
			if gs, ok := p.Resource.(*model.GroupStart); ok {
				assert.Equal(t, "g1", gs.ID)
			}
		}
		if p.Type == model.PartGroupEnd {
			hasGroupEnd = true
		}
	}
	assert.True(t, hasGroupStart, "should have GroupStart for <group>")
	assert.True(t, hasGroupEnd, "should have GroupEnd for </group>")
}

func TestExtract_MultipleFiles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xliff := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="file1">
    <body>
      <trans-unit id="1">
        <source>From file 1</source>
      </trans-unit>
    </body>
  </file>
  <file source-language="en" target-language="fr" datatype="plaintext" original="file2">
    <body>
      <trans-unit id="1">
        <source>From file 2</source>
      </trans-unit>
    </body>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff, "test.xlf", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.Len(t, blocks, 2)

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "From file 1")
	assert.Contains(t, texts, "From file 2")
}
