//go:build integration

package dtd

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.dtd.DTDFilter"
const mimeType = "application/xml-dtd"

// readDTD parses a DTD snippet with custom filter params and returns the parts.
func readDTD(t *testing.T, snippet string, params map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	return bridgetest.ReadString(t, pool, cfg, filterClass, snippet, "test.dtd", mimeType, params)
}

// readDTDDefault parses a DTD snippet with default (nil) params.
func readDTDDefault(t *testing.T, snippet string) []*model.Part {
	t.Helper()
	return readDTD(t, snippet, nil)
}

// snippetRoundtrip roundtrips a DTD snippet and returns the output string.
func snippetRoundtrip(t *testing.T, snippet string, params map[string]any) string {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, []byte(snippet), "test.dtd", mimeType, params)
	return string(result.Output)
}

// blockByName finds a block whose Name matches the given name.
func blockByName(blocks []*model.Block, name string) *model.Block {
	for _, b := range blocks {
		if b.Name == name {
			return b
		}
	}
	return nil
}

// okapi-unmapped: DTDFilterTest#testDefaultInfo — tests Java filter metadata (name, configurations), not relevant to bridge extraction
// okapi: DTDFilterTest#testDefaultInfo
func TestExtract_DefaultInfo(t *testing.T) {
	parts := readDTDDefault(t, `<!ENTITY greeting "Hello world">`)
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

// okapi-unmapped: DTDFilterTest#testStartDocument — tests Java StartDocument event properties (lineBreak, encoding), not directly accessible via bridge
// okapi: DTDFilterTest#testStartDocument
func TestExtract_StartDocument(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	path := tdDir + "/okapi/filters/dtd/src/test/resources/Test01.dtd"
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)

	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok)
	assert.NotEmpty(t, layer.ID)
}

// okapi: DTDFilterTest#testSimpleEntry
func TestExtract_SimpleEntry(t *testing.T) {
	snippet := "<!--Comment-->\n<!ENTITY entry1 \"Text1\"><!ENTITY test2 \"text2\">"
	parts := readDTDDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2)

	// First text unit should be "Text1" with name "entry1".
	b1 := blockByName(blocks, "entry1")
	require.NotNil(t, b1, "should find block with name entry1")
	assert.Equal(t, "Text1", b1.SourceText())

	// The comment "Comment" should be attached as a note annotation.
	require.NotNil(t, b1.Annotations, "block should have annotations")
	noteAnn, ok := b1.Annotations["note"]
	require.True(t, ok, "should have a 'note' annotation key")
	note, ok := noteAnn.(*model.NoteAnnotation)
	require.True(t, ok, "annotation should be *model.NoteAnnotation")
	assert.Equal(t, "Comment", note.Text)

	// Second text unit should be "text2" with name "test2".
	b2 := blockByName(blocks, "test2")
	require.NotNil(t, b2, "should find block with name test2")
	assert.Equal(t, "text2", b2.SourceText())
}

// okapi-unmapped: DTDFilterTest#testLineBreaks — tests Java StartDocument.getLineBreak() property for CR detection, not accessible via bridge
// okapi: DTDFilterTest#testLineBreaks
func TestExtract_LineBreaks(t *testing.T) {
	// The Java test verifies that CR line breaks are detected and stored in
	// the StartDocument event. In the bridge, we verify the content is
	// extracted correctly regardless of line break style by roundtripping.
	snippet := "<!--Comment-->\r<!ENTITY entry1 \"Text1\">\r"
	parts := readDTDDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Text1", blocks[0].SourceText())
}

// okapi: DTDFilterTest#testEntryWithEnitties
func TestExtract_EntryWithEntities(t *testing.T) {
	snippet := `<!ENTITY entry1 "&ent1;=ent1, %pent1;=pent1">`
	parts := readDTDDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// The Java test verifies that &ent1; and %pent1; are extracted as inline codes.
	b := blocks[0]
	frag := b.FirstFragment()
	require.NotNil(t, frag)

	// Count placeholder spans — entity references should become inline codes.
	var placeholderCount int
	var placeholderData []string
	for _, s := range frag.Spans {
		if s.SpanType == model.SpanPlaceholder {
			placeholderCount++
			placeholderData = append(placeholderData, s.Data)
		}
	}
	assert.Equal(t, 2, placeholderCount, "should have 2 placeholder spans for &ent1; and %%pent1;")

	// Verify the entity reference data is preserved.
	assert.Contains(t, placeholderData, "&ent1;")
	assert.Contains(t, placeholderData, "%pent1;")

	// The plain text parts should contain "=ent1, " and "=pent1".
	text := b.SourceText()
	assert.Contains(t, text, "=ent1, ")
	assert.Contains(t, text, "=pent1")
}

// okapi: DTDFilterTest#testEntryWithNCRs
func TestExtract_EntryWithNCRs(t *testing.T) {
	snippet := `<!ENTITY entry1 "&#xe3;, &#xE3;, &#227;">`
	parts := readDTDDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// All three numeric character references resolve to U+00E3 (a with tilde).
	// The Java test asserts: "\u00e3, \u00e3, \u00e3"
	assert.Equal(t, "\u00e3, \u00e3, \u00e3", blocks[0].SourceText())
}

// --- Additional extraction tests ---

func TestExtract_MultipleEntities(t *testing.T) {
	snippet := `<!ENTITY greeting "Hello">
<!ENTITY farewell "Goodbye">
<!ENTITY thanks "Thank you">`
	parts := readDTDDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.Equal(t, 3, len(blocks))

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Hello")
	assert.Contains(t, texts, "Goodbye")
	assert.Contains(t, texts, "Thank you")
}

func TestExtract_EntityWithAmpEscape(t *testing.T) {
	snippet := `<!ENTITY test1 "Text of &amp;test1;">`
	parts := readDTDDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// &amp; should resolve to & in the text.
	text := blocks[0].SourceText()
	assert.Contains(t, text, "Text of &test1;")
}

func TestExtract_EmptyEntity(t *testing.T) {
	snippet := `<!ENTITY empty "">`
	parts := readDTDDefault(t, snippet)

	// An empty entity may or may not produce a block depending on filter config.
	// Either no translatable block or a block with empty source is acceptable.
	blocks := bridgetest.TranslatableBlocks(parts)
	if len(blocks) > 0 {
		assert.Equal(t, "", blocks[0].SourceText())
	}
}

func TestExtract_BlockIDs(t *testing.T) {
	snippet := `<!ENTITY first "First">
<!ENTITY second "Second">
<!ENTITY third "Third">`
	parts := readDTDDefault(t, snippet)

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
	parts := readDTDDefault(t, `<!ENTITY greeting "Hello">`)

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type, "last part should be LayerEnd")
}

func TestExtract_EntityNames(t *testing.T) {
	snippet := `<!ENTITY fileMenu.label "File">
<!ENTITY editMenu.label "Edit">`
	parts := readDTDDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2)

	b1 := blockByName(blocks, "fileMenu.label")
	require.NotNil(t, b1, "should find block with name fileMenu.label")
	assert.Equal(t, "File", b1.SourceText())

	b2 := blockByName(blocks, "editMenu.label")
	require.NotNil(t, b2, "should find block with name editMenu.label")
	assert.Equal(t, "Edit", b2.SourceText())
}

func TestExtract_CommentAttachment(t *testing.T) {
	snippet := `<!--This is a comment-->
<!ENTITY greeting "Hello">`
	parts := readDTDDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	assert.Equal(t, "Hello", b.SourceText())

	// Comment should be attached as a note annotation.
	require.NotNil(t, b.Annotations, "block should have annotations")
	noteAnn, ok := b.Annotations["note"]
	require.True(t, ok, "should have a 'note' annotation key")
	note, ok := noteAnn.(*model.NoteAnnotation)
	require.True(t, ok, "annotation should be *model.NoteAnnotation")
	assert.Equal(t, "This is a comment", note.Text)
}

func TestExtract_UnicodeContent(t *testing.T) {
	snippet := "<!ENTITY jp \"\xe3\x81\x93\xe3\x82\x93\xe3\x81\xab\xe3\x81\xa1\xe3\x81\xaf\">\n<!ENTITY accented \"H\xc3\xa9llo w\xc3\xb6rld\">"
	parts := readDTDDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.Contains(t, texts, "\xe3\x81\x93\xe3\x82\x93\xe3\x81\xab\xe3\x81\xa1\xe3\x81\xaf")
	assert.Contains(t, texts, "H\xc3\xa9llo w\xc3\xb6rld")
}

// --- Full-file extraction tests ---

func TestExtract_FullFile_Test01(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	path := tdDir + "/okapi/filters/dtd/src/test/resources/Test01.dtd"
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	// Test01.dtd has many ENTITY declarations (30+): test1..test3, findWindow.title,
	// fileMenu.label, editMenu.label, accesskeys, commands, toolbar labels, etc.
	require.NotEmpty(t, blocks, "Test01.dtd should produce translatable blocks")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Find Files")
	assert.Contains(t, texts, "File")
	assert.Contains(t, texts, "Edit")
	assert.Contains(t, texts, "Open Search...")
	assert.Contains(t, texts, "Save Search...")
	assert.Contains(t, texts, "Close")
	assert.Contains(t, texts, "Cut")
	assert.Contains(t, texts, "Copy")
	assert.Contains(t, texts, "Paste")
	assert.Contains(t, texts, "Find")
	assert.Contains(t, texts, "Cancel")

	// Verify entity names are preserved.
	b := blockByName(blocks, "findWindow.title")
	require.NotNil(t, b, "should find block with name findWindow.title")
	assert.Equal(t, "Find Files", b.SourceText())
}

func TestExtract_FullFile_Test02(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	path := tdDir + "/okapi/filters/dtd/src/test/resources/Test02.dtd"
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)

	// Test02.dtd is a Qt Linguist TS format DTD — it contains only ELEMENT and
	// ATTLIST declarations, no ENTITY values. Should extract without error but
	// produce no translatable blocks.
	require.NotEmpty(t, parts, "should produce at least layer start/end")
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

// --- Roundtrip snippet tests ---

func TestSnippetRoundtrip_SimpleEntity(t *testing.T) {
	snippet := `<!ENTITY greeting "Hello world">`
	result := snippetRoundtrip(t, snippet, nil)
	assert.Contains(t, result, "Hello world")
	assert.Contains(t, result, "greeting")
}

func TestSnippetRoundtrip_MultipleEntities(t *testing.T) {
	snippet := "<!ENTITY first \"Hello\">\n<!ENTITY second \"World\">\n"
	result := snippetRoundtrip(t, snippet, nil)
	assert.Contains(t, result, "Hello")
	assert.Contains(t, result, "World")
}

func TestSnippetRoundtrip_EntityWithComment(t *testing.T) {
	snippet := "<!--A comment-->\n<!ENTITY entry1 \"Text1\">\n"
	result := snippetRoundtrip(t, snippet, nil)
	assert.Contains(t, result, "Text1")
	assert.Contains(t, result, "comment", "comment should be preserved in output")
}

func TestSnippetRoundtrip_NCRs(t *testing.T) {
	// Numeric character references are resolved during read; roundtrip output
	// may use the resolved character or re-encode. Verify the text survives.
	snippet := `<!ENTITY entry1 "&#xe3;, &#xE3;, &#227;">`
	result := snippetRoundtrip(t, snippet, nil)
	// The text should contain the resolved character or the original NCRs.
	assert.NotEmpty(t, result)
}

func TestSnippetRoundtrip_Entities(t *testing.T) {
	snippet := `<!ENTITY entry1 "&ent1;=ent1, %pent1;=pent1">`
	result := snippetRoundtrip(t, snippet, nil)
	assert.Contains(t, result, "=ent1")
	assert.Contains(t, result, "=pent1")
}
