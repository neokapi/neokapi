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
	require.NotNil(t, b1.AnnoMap(), "block should have annotations")
	notes := b1.Notes()
	require.Len(t, notes, 1, "should have a 'note' annotation")
	note := notes[0]
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
	require.NotNil(t, b.AnnoMap(), "block should have annotations")
	notes := b.Notes()
	require.Len(t, notes, 1, "should have a 'note' annotation")
	note := notes[0]
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
	notes := b.Notes()
	require.Len(t, notes, 1)
	note := notes[0]
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

// --- Comment-context surfacing (#928 treatment B) ---

// dataParts returns every model.Data resource emitted in the part stream.
func dataParts(parts []*model.Part) []*model.Data {
	var out []*model.Data
	for _, p := range parts {
		if p.Type == model.PartData {
			if d, ok := p.Resource.(*model.Data); ok {
				out = append(out, d)
			}
		}
	}
	return out
}

// dataByName returns the first Data part with the given Name.
func dataByName(datas []*model.Data, name string) *model.Data {
	for _, d := range datas {
		if d.Name == name {
			return d
		}
	}
	return nil
}

// An unclosed `<!--` (no `-->`) is consumed as a single Data part. Previously
// its prose was dropped; it must now ride on the Data part's Properties so
// ingestion can surface the comment as context. The part stream is unchanged
// (still one comment Data), so this is parity-safe.
func TestUnclosedCommentCarriesProse(t *testing.T) {
	ctx := t.Context()
	reader := dtd.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("<!-- a comment that never closes", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	datas := dataParts(parts)
	d := dataByName(datas, "comment")
	require.NotNil(t, d, "unclosed comment should produce a comment Data part")
	assert.Equal(t, "a comment that never closes", d.Properties["text"])

	// No translatable block is introduced.
	assert.Empty(t, testutil.FilterBlocks(parts), "comment must not become a translatable block")
}

// A comment that is NOT immediately followed by a parseable <!ENTITY> — here a
// structural <!ELEMENT> declaration — must not lose its prose. It rides on the
// declaration's Data part Properties. Round-trip stays byte-exact (bytes live
// in the skeleton).
func TestCommentBeforeElementDeclarationCarriesProse(t *testing.T) {
	ctx := t.Context()
	input := "<!--describe the element-->\n<!ELEMENT note (to,from)>\n"
	reader := dtd.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	d := dataByName(dataParts(parts), "declaration")
	require.NotNil(t, d, "ELEMENT declaration should produce a declaration Data part")
	assert.Equal(t, "describe the element", d.Properties["text"])
	assert.Empty(t, testutil.FilterBlocks(parts), "no translatable block for an ELEMENT declaration")

	// Round-trip is byte-exact — the comment + declaration stay in skeleton.
	assert.Equal(t, input, snippetRoundtripWithSkeleton(t, input))
}

// A comment preceding a parameter entity (`<!ENTITY % ... >`, which parseEntityDecl
// rejects and routes through the non-entity declaration branch) carries its
// prose on that declaration's Data part rather than being dropped.
func TestCommentBeforeParameterEntityCarriesProse(t *testing.T) {
	ctx := t.Context()
	input := "<!--macro doc-->\n<!ENTITY % evilstring '(#PCDATA | byte)*' >\n"
	reader := dtd.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	d := dataByName(dataParts(parts), "declaration")
	require.NotNil(t, d, "parameter entity should produce a declaration Data part")
	assert.Equal(t, "macro doc", d.Properties["text"])
	assert.Empty(t, testutil.FilterBlocks(parts), "parameter entity must not be translatable")

	assert.Equal(t, input, snippetRoundtripWithSkeleton(t, input))
}

// Consecutive comments before one entity accumulate instead of the later
// comment overwriting (and dropping) the earlier one. The joined prose lands
// on the entity's note annotation (parity-safe; annotations are not in the
// canonical stream).
func TestConsecutiveCommentsAccumulateAsNote(t *testing.T) {
	ctx := t.Context()
	input := "<!--first comment-->\n<!--second comment-->\n<!ENTITY greeting \"Hello\">"
	reader := dtd.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	b := blockByName(blocks, "greeting")
	require.NotNil(t, b)
	notes := b.Notes()
	require.Len(t, notes, 1)
	assert.Equal(t, "first comment\nsecond comment", notes[0].Text)
}

// When the only difference is comment-context surfacing, the emitted Data ID
// sequence is unchanged from the pre-change behavior — proving the part stream
// (what parity compares) is byte-identical regardless of the Properties text.
func TestCommentBeforeDeclarationDataIDStable(t *testing.T) {
	ctx := t.Context()
	// Two declarations preceded by comments + one without; IDs must be d1, d2, d3.
	input := "<!--c1-->\n<!ELEMENT a EMPTY>\n<!ELEMENT b EMPTY>\n<!--c3-->\n<!ATTLIST b x CDATA #IMPLIED>\n"
	reader := dtd.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	var decls []*model.Data
	for _, d := range dataParts(parts) {
		if d.Name == "declaration" {
			decls = append(decls, d)
		}
	}
	require.Len(t, decls, 3)
	assert.Equal(t, "d1", decls[0].ID)
	assert.Equal(t, "d2", decls[1].ID)
	assert.Equal(t, "d3", decls[2].ID)
	// Comment prose carried only where a comment preceded the declaration.
	assert.Equal(t, "c1", decls[0].Properties["text"])
	assert.Empty(t, decls[1].Properties["text"])
	assert.Equal(t, "c3", decls[2].Properties["text"])
}
