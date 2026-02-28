//go:build integration

package okf_po

import (
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.po.POFilter"
const mimeType = "application/x-gettext"

func TestExtract_SimplePO(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	po := `msgid "Hello World"
msgstr ""

msgid "Goodbye"
msgstr ""
`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		po, "test.po", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from PO")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Hello World")
	assert.Contains(t, texts, "Goodbye")
}

// okapi: POFilterTest#testOuputSimpleEntry
func TestExtract_WithTarget(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	po := `msgid "Hello"
msgstr "Bonjour"
`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		po, "test.po", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	assert.Equal(t, "Hello", b.SourceText())
	assert.True(t, b.HasTarget("fr"), "should have French target")
	assert.Equal(t, "Bonjour", b.TargetText("fr"))
}

// okapi: POFilterTest#testIDWithContext
func TestExtract_Context(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	po := `msgctxt "menu"
msgid "File"
msgstr ""

msgctxt "dialog"
msgid "File"
msgstr ""
`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		po, "test.po", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2, "should extract both contextual entries")

	// Both entries have the same source text but different contexts.
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "File")
}

// okapi: POFilterTest#testTUCompleteEntry
func TestExtract_Comments(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	po := `# Translator comment
#. Extracted comment
#: src/main.c:42
msgid "Save"
msgstr ""
`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		po, "test.po", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// The PO filter should extract notes from comments.
	b := blocks[0]
	assert.Equal(t, "Save", b.SourceText())
	// Comments may appear as note annotations.
	if b.Annotations != nil {
		if note, ok := b.Annotations["note"]; ok {
			n := note.(*model.NoteAnnotation)
			assert.NotEmpty(t, n.Text)
		}
	}
}

// okapi: POFilterTest#testNoQuoteOnSameLine
func TestExtract_MultilineStrings(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	po := `msgid ""
"This is a "
"multiline string"
msgstr ""
`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		po, "test.po", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "This is a multiline string")
}

// okapi: POFilterTest#testTUPluralEntry_DefaultGroup
// okapi: POFilterTest#testTUPluralEntry_DefaultPlural
func TestExtract_PluralForms(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	po := `msgid "One item"
msgid_plural "%d items"
msgstr[0] ""
msgstr[1] ""
`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		po, "test.po", mimeType, nil)

	// Plural forms should produce at least one translatable block.
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

func TestExtract_UnicodeContent(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	po := `msgid "こんにちは"
msgstr ""

msgid "Héllo wörld"
msgstr ""
`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		po, "test.po", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.Contains(t, texts, "こんにちは")
	assert.Contains(t, texts, "Héllo wörld")
}

func TestExtract_BlockIDs(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	po := `msgid "First"
msgstr ""

msgid "Second"
msgstr ""

msgid "Third"
msgstr ""
`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		po, "test.po", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 3)

	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID, "block should have an ID")
		assert.False(t, ids[b.ID], "block IDs should be unique, got duplicate: %s", b.ID)
		ids[b.ID] = true
	}
}

func TestExtract_LayerStructure(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		"msgid \"Hello\"\nmsgstr \"\"\n",
		"test.po", mimeType, nil)

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type, "last part should be LayerEnd")
}

// okapi: POFilterTest#testEscapes
func TestExtract_EscapedCharacters(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	po := `msgid "Line one\nLine two"
msgstr ""
`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		po, "test.po", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// The PO filter should handle escape sequences.
	text := blocks[0].SourceText()
	assert.Contains(t, text, "Line one")
	assert.Contains(t, text, "Line two")
}
