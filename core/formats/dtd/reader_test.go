package dtd_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/formats/dtd"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// blockByName finds a block whose Name matches the given name.
func blockByName(blocks []*model.Block, name string) *model.Block {
	for _, b := range blocks {
		if b.Name == name {
			return b
		}
	}
	return nil
}

// okapi: DTDFilterTest#testDefaultInfo
func TestDefaultInfo(t *testing.T) {
	ctx := t.Context()
	reader := dtd.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(`<!ENTITY greeting "Hello world">`, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

// okapi: DTDFilterTest#testStartDocument
func TestStartDocument(t *testing.T) {
	ctx := t.Context()
	reader := dtd.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(`<!ENTITY greeting "Hello world">`, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)

	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok)
	assert.NotEmpty(t, layer.ID)
}

// okapi: DTDFilterTest#testSimpleEntry
func TestSimpleEntry(t *testing.T) {
	ctx := t.Context()
	reader := dtd.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("<!--Comment-->\n<!ENTITY entry1 \"Text1\"><!ENTITY test2 \"text2\">", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
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

// okapi: DTDFilterTest#testLineBreaks
func TestLineBreaks(t *testing.T) {
	ctx := t.Context()
	reader := dtd.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("<!--Comment-->\r<!ENTITY entry1 \"Text1\">\r", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Text1", blocks[0].SourceText())
}

// okapi: DTDFilterTest#testEntryWithEnitties
func TestEntryWithEntities(t *testing.T) {
	ctx := t.Context()
	reader := dtd.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(`<!ENTITY entry1 "&ent1;=ent1, %pent1;=pent1">`, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.NotEmpty(t, blocks)

	// Unknown entity references become inline placeholder runs (so they
	// round-trip as opaque codes); the surrounding plain text stays in
	// TextRuns. Reassemble the original surface form by walking SourceRuns.
	var surface strings.Builder
	for _, r := range blocks[0].SourceRuns() {
		switch {
		case r.Text != nil:
			surface.WriteString(r.Text.Text)
		case r.Ph != nil:
			surface.WriteString(r.Ph.Data)
		}
	}
	got := surface.String()
	assert.Contains(t, got, "&ent1;")
	assert.Contains(t, got, "=ent1, ")
	assert.Contains(t, got, "%pent1;")
	assert.Contains(t, got, "=pent1")
}

// okapi: DTDFilterTest#testEntryWithNCRs
func TestEntryWithNCRs(t *testing.T) {
	ctx := t.Context()
	reader := dtd.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(`<!ENTITY entry1 "&#xe3;, &#xE3;, &#227;">`, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.NotEmpty(t, blocks)

	// All three numeric character references resolve to U+00E3 (a with tilde).
	assert.Equal(t, "\u00e3, \u00e3, \u00e3", blocks[0].SourceText())
}

func TestMultipleEntities(t *testing.T) {
	ctx := t.Context()
	reader := dtd.NewReader()
	snippet := "<!ENTITY greeting \"Hello\">\n<!ENTITY farewell \"Goodbye\">\n<!ENTITY thanks \"Thank you\">"
	err := reader.Open(ctx, testutil.RawDocFromString(snippet, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 3)

	texts := testutil.BlockTexts(blocks)
	assert.Contains(t, texts, "Hello")
	assert.Contains(t, texts, "Goodbye")
	assert.Contains(t, texts, "Thank you")
}

func TestEntityWithAmpEscape(t *testing.T) {
	ctx := t.Context()
	reader := dtd.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(`<!ENTITY test1 "Text of &amp;test1;">`, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.NotEmpty(t, blocks)

	// &amp; should resolve to & in the text.
	text := blocks[0].SourceText()
	assert.Contains(t, text, "Text of &test1;")
}

func TestEmptyEntity(t *testing.T) {
	ctx := t.Context()
	reader := dtd.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(`<!ENTITY empty "">`, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	if len(blocks) > 0 {
		assert.Empty(t, blocks[0].SourceText())
	}
}

func TestBlockIDs(t *testing.T) {
	ctx := t.Context()
	reader := dtd.NewReader()
	snippet := "<!ENTITY first \"First\">\n<!ENTITY second \"Second\">\n<!ENTITY third \"Third\">"
	err := reader.Open(ctx, testutil.RawDocFromString(snippet, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.GreaterOrEqual(t, len(blocks), 3)

	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID, "block should have an ID")
		assert.False(t, ids[b.ID], "block IDs should be unique, got duplicate: %s", b.ID)
		ids[b.ID] = true
	}
}

func TestLayerStructure(t *testing.T) {
	ctx := t.Context()
	reader := dtd.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(`<!ENTITY greeting "Hello">`, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type, "last part should be LayerEnd")
}

func TestEntityNames(t *testing.T) {
	ctx := t.Context()
	reader := dtd.NewReader()
	snippet := "<!ENTITY fileMenu.label \"File\">\n<!ENTITY editMenu.label \"Edit\">"
	err := reader.Open(ctx, testutil.RawDocFromString(snippet, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.GreaterOrEqual(t, len(blocks), 2)

	b1 := blockByName(blocks, "fileMenu.label")
	require.NotNil(t, b1, "should find block with name fileMenu.label")
	assert.Equal(t, "File", b1.SourceText())

	b2 := blockByName(blocks, "editMenu.label")
	require.NotNil(t, b2, "should find block with name editMenu.label")
	assert.Equal(t, "Edit", b2.SourceText())
}

func TestCommentAttachment(t *testing.T) {
	ctx := t.Context()
	reader := dtd.NewReader()
	snippet := "<!--This is a comment-->\n<!ENTITY greeting \"Hello\">"
	err := reader.Open(ctx, testutil.RawDocFromString(snippet, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
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

func TestUnicodeContent(t *testing.T) {
	ctx := t.Context()
	reader := dtd.NewReader()
	snippet := "<!ENTITY jp \"\xe3\x81\x93\xe3\x82\x93\xe3\x81\xab\xe3\x81\xa1\xe3\x81\xaf\">\n<!ENTITY accented \"H\xc3\xa9llo w\xc3\xb6rld\">"
	err := reader.Open(ctx, testutil.RawDocFromString(snippet, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	texts := testutil.BlockTexts(blocks)

	assert.Contains(t, texts, "\xe3\x81\x93\xe3\x82\x93\xe3\x81\xab\xe3\x81\xa1\xe3\x81\xaf")
	assert.Contains(t, texts, "H\xc3\xa9llo w\xc3\xb6rld")
}

func TestNonEntityDeclarations(t *testing.T) {
	ctx := t.Context()
	reader := dtd.NewReader()
	// Test02.dtd-like content: only ELEMENT/ATTLIST, no ENTITY values.
	snippet := `<!ELEMENT TS (defaultcodec?,context*)>
<!ATTLIST TS version CDATA #IMPLIED>
<!ELEMENT context (name,comment?,message*)>`
	err := reader.Open(ctx, testutil.RawDocFromString(snippet, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	require.NotEmpty(t, parts, "should produce at least layer start/end")
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	blocks := testutil.FilterBlocks(parts)
	assert.Empty(t, blocks, "non-entity declarations should not produce blocks")
}

func TestReaderSignature(t *testing.T) {
	reader := dtd.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "application/xml-dtd")
	assert.Contains(t, sig.Extensions, ".dtd")
}

func TestReaderMetadata(t *testing.T) {
	reader := dtd.NewReader()
	assert.Equal(t, "dtd", reader.Name())
	assert.Equal(t, "DTD", reader.DisplayName())
}

func TestReadNilDocument(t *testing.T) {
	ctx := t.Context()
	reader := dtd.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}

func TestReadEmpty(t *testing.T) {
	ctx := t.Context()
	reader := dtd.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)
	assert.Empty(t, blocks)
}

func TestSingleQuotedEntity(t *testing.T) {
	ctx := t.Context()
	reader := dtd.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("<!ENTITY greeting 'Hello world'>", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello world", blocks[0].SourceText())
	assert.Equal(t, "greeting", blocks[0].Name)
}

// --- Full-file extraction tests ---

func TestFullFile_Simple(t *testing.T) {
	ctx := t.Context()

	f, err := os.Open("testdata/simple.dtd")
	require.NoError(t, err)
	reader := dtd.NewReader()
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/simple.dtd", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 3)

	texts := testutil.BlockTexts(blocks)
	assert.Contains(t, texts, "Hello world")
	assert.Contains(t, texts, "Goodbye")
	assert.Contains(t, texts, "Thank you")
}

func TestFullFile_Complex(t *testing.T) {
	ctx := t.Context()

	f, err := os.Open("testdata/complex.dtd")
	require.NoError(t, err)
	reader := dtd.NewReader()
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/complex.dtd", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.NotEmpty(t, blocks)

	texts := testutil.BlockTexts(blocks)
	assert.Contains(t, texts, "Find Files")
	assert.Contains(t, texts, "File")
	assert.Contains(t, texts, "Edit")

	// Check that comment is attached as note
	b := blockByName(blocks, "findWindow.title")
	require.NotNil(t, b)
	assert.Equal(t, "Find Files", b.SourceText())
	noteAnn, ok := b.Annotations["note"]
	require.True(t, ok)
	note, ok := noteAnn.(*model.NoteAnnotation)
	require.True(t, ok)
	assert.Equal(t, "Window title", note.Text)

	// Check escaped entities resolved
	bEscaped := blockByName(blocks, "escaped")
	require.NotNil(t, bEscaped)
	assert.Equal(t, "Text with & ampersand and <angle brackets>", bEscaped.SourceText())

	// Check NCR resolved
	bNCR := blockByName(blocks, "ncr")
	require.NotNil(t, bNCR)
	assert.Equal(t, "Char A and hex B", bNCR.SourceText())

	// Check unicode content
	bUnicode := blockByName(blocks, "unicode")
	require.NotNil(t, bUnicode)
	assert.Equal(t, "こんにちは世界", bUnicode.SourceText())
}

// --- Roundtrip tests ---

// okapi: DTDFilterTest (roundtrip - simple entity)
func TestSnippetRoundtrip_SimpleEntity(t *testing.T) {
	ctx := t.Context()

	reader := dtd.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(`<!ENTITY greeting "Hello world">`, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := dtd.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	result := buf.String()
	assert.Contains(t, result, "Hello world")
	assert.Contains(t, result, "greeting")
	assert.Contains(t, result, "<!ENTITY")
}

// okapi: DTDFilterTest (roundtrip - multiple entities)
func TestSnippetRoundtrip_MultipleEntities(t *testing.T) {
	ctx := t.Context()

	reader := dtd.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("<!ENTITY first \"Hello\">\n<!ENTITY second \"World\">\n", model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := dtd.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	result := buf.String()
	assert.Contains(t, result, "Hello")
	assert.Contains(t, result, "World")
}

// okapi: DTDFilterTest (roundtrip - entity with comment)
func TestSnippetRoundtrip_EntityWithComment(t *testing.T) {
	ctx := t.Context()

	reader := dtd.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("<!--A comment-->\n<!ENTITY entry1 \"Text1\">\n", model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := dtd.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	result := buf.String()
	assert.Contains(t, result, "Text1")
	assert.Contains(t, result, "comment", "comment should be preserved in output")
}

// okapi: DTDFilterTest (roundtrip - NCRs)
func TestSnippetRoundtrip_NCRs(t *testing.T) {
	ctx := t.Context()

	reader := dtd.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(`<!ENTITY entry1 "&#xe3;, &#xE3;, &#227;">`, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := dtd.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	result := buf.String()
	assert.NotEmpty(t, result)
	// The resolved character (U+00E3) should be present in the output
	assert.Contains(t, result, "\u00e3")
}

// okapi: DTDFilterTest (roundtrip - entities)
func TestSnippetRoundtrip_Entities(t *testing.T) {
	ctx := t.Context()

	reader := dtd.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(`<!ENTITY entry1 "&ent1;=ent1, %pent1;=pent1">`, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := dtd.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	result := buf.String()
	assert.Contains(t, result, "=ent1")
	assert.Contains(t, result, "=pent1")
}

func TestRoundTripWithTargetLocale(t *testing.T) {
	ctx := t.Context()

	input := "<!ENTITY greeting \"Hello\">\n<!ENTITY farewell \"World\">\n"

	reader := dtd.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
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
	writer := dtd.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Bonjour")
	assert.Contains(t, output, "Monde")
	assert.NotContains(t, output, "\"Hello\"")
	assert.NotContains(t, output, "\"World\"")
}

func TestRoundTripFile_Simple(t *testing.T) {
	ctx := t.Context()

	f, err := os.Open("testdata/simple.dtd")
	require.NoError(t, err)
	reader := dtd.NewReader()
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/simple.dtd", model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := dtd.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Hello world")
	assert.Contains(t, output, "Goodbye")
	assert.Contains(t, output, "Thank you")
	assert.Contains(t, output, "greeting")
	assert.Contains(t, output, "farewell")
	assert.Contains(t, output, "thanks")
}
