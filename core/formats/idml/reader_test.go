package idml

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Test IDML creation helpers
// ---------------------------------------------------------------------------

// createIDML creates a minimal valid IDML ZIP in memory with the given story XMLs.
func createIDML(t *testing.T, stories map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// Write mimetype (uncompressed, first entry per IDML spec)
	mimeHeader := &zip.FileHeader{Name: "mimetype", Method: zip.Store}
	mf, err := zw.CreateHeader(mimeHeader)
	require.NoError(t, err)
	_, err = mf.Write([]byte("application/vnd.adobe.indesign-idml-package"))
	require.NoError(t, err)

	// Write designmap.xml
	dm, err := zw.Create("designmap.xml")
	require.NoError(t, err)
	_, err = dm.Write([]byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Document DOMVersion="16.0">
</Document>`))
	require.NoError(t, err)

	// Write story files
	for name, content := range stories {
		sf, err := zw.Create("Stories/" + name)
		require.NoError(t, err)
		_, err = sf.Write([]byte(content))
		require.NoError(t, err)
	}

	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// readIDMLBytes reads an IDML from raw bytes and returns parts.
func readIDMLBytes(t *testing.T, data []byte) []*model.Part {
	t.Helper()
	ctx := t.Context()
	reader := NewReader()
	doc := &model.RawDocument{
		URI:          "test.idml",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		MimeType:     "application/vnd.adobe.indesign-idml-package",
		Reader:       io.NopCloser(bytes.NewReader(data)),
	}
	err := reader.Open(ctx, doc)
	require.NoError(t, err)
	defer reader.Close()

	return testutil.CollectParts(t, reader.Read(ctx))
}

// readIDMLBytesWithConfig reads an IDML from raw bytes with a custom config.
func readIDMLBytesWithConfig(t *testing.T, data []byte, cfg *Config) []*model.Part {
	t.Helper()
	ctx := t.Context()
	reader := NewReader()
	reader.cfg = cfg
	reader.Cfg = cfg
	doc := &model.RawDocument{
		URI:          "test.idml",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		MimeType:     "application/vnd.adobe.indesign-idml-package",
		Reader:       io.NopCloser(bytes.NewReader(data)),
	}
	err := reader.Open(ctx, doc)
	require.NoError(t, err)
	defer reader.Close()

	return testutil.CollectParts(t, reader.Read(ctx))
}

// ---------------------------------------------------------------------------
// ExtractionTest equivalents
// ---------------------------------------------------------------------------

// okapi: ExtractionTest#testSimpleEntry
func TestSimple(t *testing.T) {
	data := createIDML(t, map[string]string{
		"Story_u1.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<idPkg:Story xmlns:idPkg="http://ns.adobe.com/AdobeInDesign/idml/1.0/packaging" DOMVersion="16.0">
  <Story Self="u1">
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/$ID/NormalParagraphStyle">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>Hello World!</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
  </Story>
</idPkg:Story>`,
	})

	parts := readIDMLBytes(t, data)

	// Verify structure: LayerStart, [LayerStart, Block..., LayerEnd], LayerEnd
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello World!", blocks[0].SourceText())
}

// okapi: ExtractionTest#testDefaultInfo
func TestDefaultInfo(t *testing.T) {
	data := createIDML(t, map[string]string{
		"Story_u1.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<idPkg:Story xmlns:idPkg="http://ns.adobe.com/AdobeInDesign/idml/1.0/packaging">
  <Story Self="u1">
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/$ID/NormalParagraphStyle">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>Test</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
  </Story>
</idPkg:Story>`,
	})

	reader := NewReader()
	assert.Equal(t, "idml", reader.Name())
	assert.Equal(t, "Adobe InDesign Markup Language", reader.DisplayName())

	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "application/vnd.adobe.indesign-idml-package")
	assert.Contains(t, sig.Extensions, ".idml")

	parts := readIDMLBytes(t, data)
	require.NotEmpty(t, parts)

	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok)
	assert.Equal(t, "application/vnd.adobe.indesign-idml-package", layer.MimeType)
}

// okapi: ExtractionTest#testSimpleEntry2
func TestMultipleContentElements(t *testing.T) {
	data := createIDML(t, map[string]string{
		"Story_u1.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<idPkg:Story xmlns:idPkg="http://ns.adobe.com/AdobeInDesign/idml/1.0/packaging">
  <Story Self="u1">
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/$ID/NormalParagraphStyle">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>First paragraph</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/$ID/NormalParagraphStyle">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>Second paragraph</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
  </Story>
</idPkg:Story>`,
	})

	parts := readIDMLBytes(t, data)
	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 2)
	assert.Equal(t, "First paragraph", blocks[0].SourceText())
	assert.Equal(t, "Second paragraph", blocks[1].SourceText())
}

// okapi: ExtractionTest#testWhitespaces
func TestWhitespaces(t *testing.T) {
	data := createIDML(t, map[string]string{
		"Story_u1.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<idPkg:Story xmlns:idPkg="http://ns.adobe.com/AdobeInDesign/idml/1.0/packaging">
  <Story Self="u1">
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/$ID/NormalParagraphStyle">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>Text with	tab</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/$ID/NormalParagraphStyle">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>Text with spaces</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
  </Story>
</idPkg:Story>`,
	})

	parts := readIDMLBytes(t, data)
	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks)

	texts := testutil.BlockTexts(blocks)
	assert.Contains(t, texts, "Text with\ttab")
	assert.Contains(t, texts, "Text with spaces")
}

// okapi: ExtractionTest#testNewline
func TestNewline(t *testing.T) {
	data := createIDML(t, map[string]string{
		"Story_u1.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<idPkg:Story xmlns:idPkg="http://ns.adobe.com/AdobeInDesign/idml/1.0/packaging">
  <Story Self="u1">
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/$ID/NormalParagraphStyle">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>Line one</Content>
      </CharacterStyleRange>
      <Br/>
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>Line two</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
  </Story>
</idPkg:Story>`,
	})

	parts := readIDMLBytes(t, data)
	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks)

	texts := testutil.BlockTexts(blocks)
	assert.Contains(t, texts, "Line one")
	assert.Contains(t, texts, "Line two")
}

// okapi: ExtractionTest#testStartDocument
func TestStartDocument(t *testing.T) {
	data := createIDML(t, map[string]string{
		"Story_u1.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<idPkg:Story xmlns:idPkg="http://ns.adobe.com/AdobeInDesign/idml/1.0/packaging">
  <Story Self="u1">
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/$ID/NormalParagraphStyle">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>Test</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
  </Story>
</idPkg:Story>`,
	})

	parts := readIDMLBytes(t, data)
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)

	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok)
	assert.NotEmpty(t, layer.ID)
	assert.Equal(t, "idml", layer.Format)
}

// okapi: ExtractionTest#testSkipDiscretionaryHyphens
func TestSkipDiscretionaryHyphens(t *testing.T) {
	data := createIDML(t, map[string]string{
		"Story_u1.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<idPkg:Story xmlns:idPkg="http://ns.adobe.com/AdobeInDesign/idml/1.0/packaging">
  <Story Self="u1">
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/$ID/NormalParagraphStyle">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>Binde&#xAD;strich</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
  </Story>
</idPkg:Story>`,
	})

	// Default: skip discretionary hyphens
	parts := readIDMLBytes(t, data)
	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 1)
	assert.Equal(t, "Bindestrich", blocks[0].SourceText())

	// With skipDiscretionaryHyphens=false
	cfg := &Config{}
	cfg.Reset()
	cfg.SkipDiscretionaryHyphens = false
	parts2 := readIDMLBytesWithConfig(t, data, cfg)
	blocks2 := testutil.FilterBlocks(parts2)
	require.Len(t, blocks2, 1)
	assert.Equal(t, "Binde\u00ADstrich", blocks2[0].SourceText())
}

func TestMultipleStories(t *testing.T) {
	data := createIDML(t, map[string]string{
		"Story_u1.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<idPkg:Story xmlns:idPkg="http://ns.adobe.com/AdobeInDesign/idml/1.0/packaging">
  <Story Self="u1">
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/$ID/NormalParagraphStyle">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>Story One</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
  </Story>
</idPkg:Story>`,
		"Story_u2.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<idPkg:Story xmlns:idPkg="http://ns.adobe.com/AdobeInDesign/idml/1.0/packaging">
  <Story Self="u2">
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/$ID/NormalParagraphStyle">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>Story Two</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
  </Story>
</idPkg:Story>`,
	})

	parts := readIDMLBytes(t, data)
	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 2)

	texts := testutil.BlockTexts(blocks)
	assert.Contains(t, texts, "Story One")
	assert.Contains(t, texts, "Story Two")

	// Verify each story gets its own child layer
	layerStarts := 0
	layerEnds := 0
	for _, p := range parts {
		switch p.Type {
		case model.PartLayerStart:
			layerStarts++
		case model.PartLayerEnd:
			layerEnds++
		}
	}
	// 1 root layer + 2 story child layers = 3 starts
	assert.Equal(t, 3, layerStarts)
	assert.Equal(t, 3, layerEnds)
}

func TestEmptyStory(t *testing.T) {
	data := createIDML(t, map[string]string{
		"Story_u1.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<idPkg:Story xmlns:idPkg="http://ns.adobe.com/AdobeInDesign/idml/1.0/packaging">
  <Story Self="u1">
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/$ID/NormalParagraphStyle">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content></Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
  </Story>
</idPkg:Story>`,
	})

	parts := readIDMLBytes(t, data)
	blocks := testutil.FilterBlocks(parts)
	assert.Empty(t, blocks, "empty content should not produce blocks")
}

func TestWhitespaceOnlyContent(t *testing.T) {
	data := createIDML(t, map[string]string{
		"Story_u1.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<idPkg:Story xmlns:idPkg="http://ns.adobe.com/AdobeInDesign/idml/1.0/packaging">
  <Story Self="u1">
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/$ID/NormalParagraphStyle">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>   </Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
  </Story>
</idPkg:Story>`,
	})

	parts := readIDMLBytes(t, data)
	blocks := testutil.FilterBlocks(parts)
	assert.Empty(t, blocks, "whitespace-only content should not produce blocks")
}

// okapi: ExtractionTest#testChangeTracking
func TestChangeTracking(t *testing.T) {
	data := createIDML(t, map[string]string{
		"Story_u1.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<idPkg:Story xmlns:idPkg="http://ns.adobe.com/AdobeInDesign/idml/1.0/packaging">
  <Story Self="u1">
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/$ID/NormalParagraphStyle">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>Original text</Content>
      </CharacterStyleRange>
      <Change ChangeType="InsertedText">
        <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
          <Content>New text.</Content>
        </CharacterStyleRange>
      </Change>
    </ParagraphStyleRange>
  </Story>
</idPkg:Story>`,
	})

	parts := readIDMLBytes(t, data)
	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks)

	texts := testutil.BlockTexts(blocks)
	found := false
	for _, text := range texts {
		if strings.Contains(text, "Original text") || strings.Contains(text, "New text.") {
			found = true
			break
		}
	}
	assert.True(t, found, "should extract text from change tracking")
}

func TestTableContent(t *testing.T) {
	data := createIDML(t, map[string]string{
		"Story_u1.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<idPkg:Story xmlns:idPkg="http://ns.adobe.com/AdobeInDesign/idml/1.0/packaging">
  <Story Self="u1">
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/$ID/NormalParagraphStyle">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Table HeaderRowCount="0" FooterRowCount="0" BodyRowCount="2" ColumnCount="2">
          <Cell Name="0:0" RowSpan="1" ColumnSpan="1">
            <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/$ID/NormalParagraphStyle">
              <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
                <Content>Cell A1</Content>
              </CharacterStyleRange>
            </ParagraphStyleRange>
          </Cell>
          <Cell Name="1:0" RowSpan="1" ColumnSpan="1">
            <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/$ID/NormalParagraphStyle">
              <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
                <Content>Cell B1</Content>
              </CharacterStyleRange>
            </ParagraphStyleRange>
          </Cell>
        </Table>
      </CharacterStyleRange>
    </ParagraphStyleRange>
  </Story>
</idPkg:Story>`,
	})

	parts := readIDMLBytes(t, data)
	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 2)

	texts := testutil.BlockTexts(blocks)
	assert.Contains(t, texts, "Cell A1")
	assert.Contains(t, texts, "Cell B1")
}

func TestFootnotes(t *testing.T) {
	data := createIDML(t, map[string]string{
		"Story_u1.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<idPkg:Story xmlns:idPkg="http://ns.adobe.com/AdobeInDesign/idml/1.0/packaging">
  <Story Self="u1">
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/$ID/NormalParagraphStyle">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>Main text</Content>
        <Footnote>
          <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/$ID/NormalParagraphStyle">
            <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
              <Content>Footnote text</Content>
            </CharacterStyleRange>
          </ParagraphStyleRange>
        </Footnote>
      </CharacterStyleRange>
    </ParagraphStyleRange>
  </Story>
</idPkg:Story>`,
	})

	// Default: extract notes
	parts := readIDMLBytes(t, data)
	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 2)

	texts := testutil.BlockTexts(blocks)
	assert.Contains(t, texts, "Main text")
	assert.Contains(t, texts, "Footnote text")

	// With extractNotes=false
	cfg := &Config{}
	cfg.Reset()
	cfg.ExtractNotes = false
	parts2 := readIDMLBytesWithConfig(t, data, cfg)
	blocks2 := testutil.FilterBlocks(parts2)
	require.Len(t, blocks2, 1)
	assert.Equal(t, "Main text", blocks2[0].SourceText())
}

func TestSpecialCharacters(t *testing.T) {
	data := createIDML(t, map[string]string{
		"Story_u1.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<idPkg:Story xmlns:idPkg="http://ns.adobe.com/AdobeInDesign/idml/1.0/packaging">
  <Story Self="u1">
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/$ID/NormalParagraphStyle">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>Text with &amp; special &lt;chars&gt; and "quotes"</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
  </Story>
</idPkg:Story>`,
	})

	parts := readIDMLBytes(t, data)
	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 1)
	assert.Equal(t, `Text with & special <chars> and "quotes"`, blocks[0].SourceText())
}

func TestBlockProperties(t *testing.T) {
	data := createIDML(t, map[string]string{
		"Story_u1.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<idPkg:Story xmlns:idPkg="http://ns.adobe.com/AdobeInDesign/idml/1.0/packaging">
  <Story Self="u1">
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/MyStyle">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/Bold">
        <Content>Styled text</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
  </Story>
</idPkg:Story>`,
	})

	parts := readIDMLBytes(t, data)
	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 1)

	block := blocks[0]
	assert.Equal(t, "Stories/Story_u1.xml", block.Properties["storyPath"])
	assert.Equal(t, "ParagraphStyle/MyStyle", block.Properties["paragraphStyle"])
	assert.Equal(t, "CharacterStyle/Bold", block.Properties["characterStyle"])
}

func TestContextCancellation(t *testing.T) {
	// Build a large document so the reader has plenty of work to do
	stories := make(map[string]string)
	for i := range 20 {
		stories[fmt.Sprintf("Story_u%d.xml", i)] = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<idPkg:Story xmlns:idPkg="http://ns.adobe.com/AdobeInDesign/idml/1.0/packaging">
  <Story Self="u1">
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/$ID/NormalParagraphStyle">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>Should not see all of this</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
  </Story>
</idPkg:Story>`
	}
	data := createIDML(t, stories)

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // Cancel immediately

	reader := NewReader()
	doc := &model.RawDocument{
		URI:          "test.idml",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader(data)),
	}
	err := reader.Open(ctx, doc)
	require.NoError(t, err)
	defer reader.Close()

	ch := reader.Read(ctx)
	var count int
	for range ch {
		count++
	}
	// The channel has a buffer of 64, so some parts may still be emitted
	// before cancellation is noticed. With 20 stories (20 blocks + ~60 layers),
	// a cancelled context should produce fewer than all parts.
	// We expect all 20 stories but context may stop us early.
	// The key test: the reader goroutine exits cleanly without deadlock.
	t.Logf("got %d parts with cancelled context", count)
}

func TestNilDocument(t *testing.T) {
	ctx := t.Context()
	reader := NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil document")
}

func TestInvalidZIPError(t *testing.T) {
	ctx := t.Context()
	reader := NewReader()
	doc := &model.RawDocument{
		URI:          "test.idml",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(strings.NewReader("not a zip file")),
	}
	err := reader.Open(ctx, doc)
	require.NoError(t, err)
	defer reader.Close()

	ch := reader.Read(ctx)
	var gotError bool
	for result := range ch {
		if result.Error != nil {
			gotError = true
			assert.Contains(t, result.Error.Error(), "not a valid ZIP")
		}
	}
	assert.True(t, gotError, "should produce an error for invalid ZIP")
}

// ---------------------------------------------------------------------------
// Roundtrip tests
// ---------------------------------------------------------------------------

// okapi: RoundTripTest#testDoubleExtraction
func TestRoundTrip(t *testing.T) {
	storyXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<idPkg:Story xmlns:idPkg="http://ns.adobe.com/AdobeInDesign/idml/1.0/packaging">
  <Story Self="u1">
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/$ID/NormalParagraphStyle">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>Hello World!</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
  </Story>
</idPkg:Story>`

	data := createIDML(t, map[string]string{
		"Story_u1.xml": storyXML,
	})

	ctx := t.Context()

	// Create skeleton store for roundtrip
	skelStore, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer skelStore.Close()

	// Read
	reader := NewReader()
	reader.SetSkeletonStore(skelStore)
	doc := &model.RawDocument{
		URI:          "test.idml",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		MimeType:     "application/vnd.adobe.indesign-idml-package",
		Reader:       io.NopCloser(bytes.NewReader(data)),
	}
	err = reader.Open(ctx, doc)
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello World!", blocks[0].SourceText())

	// Write
	var buf bytes.Buffer
	writer := NewWriter()
	writer.SetSkeletonStore(skelStore)
	writer.SetOriginalContent(data)
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)

	// Verify output is a valid ZIP
	outData := buf.Bytes()
	require.NotEmpty(t, outData)
	outZR, err := zip.NewReader(bytes.NewReader(outData), int64(len(outData)))
	require.NoError(t, err)

	// Verify story file is present
	var foundStory bool
	for _, f := range outZR.File {
		if f.Name == "Stories/Story_u1.xml" {
			foundStory = true
			content, err := readZipFile(f)
			require.NoError(t, err)
			assert.Contains(t, string(content), "Hello World!")
		}
	}
	assert.True(t, foundStory, "output should contain the story file")
}

func TestRoundTripWithTranslation(t *testing.T) {
	data := createIDML(t, map[string]string{
		"Story_u1.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<idPkg:Story xmlns:idPkg="http://ns.adobe.com/AdobeInDesign/idml/1.0/packaging">
  <Story Self="u1">
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/$ID/NormalParagraphStyle">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>Hello World!</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
  </Story>
</idPkg:Story>`,
	})

	ctx := t.Context()

	// Create skeleton store
	skelStore, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer skelStore.Close()

	// Read
	reader := NewReader()
	reader.SetSkeletonStore(skelStore)
	doc := &model.RawDocument{
		URI:          "test.idml",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader(data)),
	}
	err = reader.Open(ctx, doc)
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Translate blocks
	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			if block.SourceText() == "Hello World!" {
				block.SetTargetText(model.LocaleFrench, "Bonjour le monde !")
			}
		}
	}

	// Write with French locale
	var buf bytes.Buffer
	writer := NewWriter()
	writer.SetSkeletonStore(skelStore)
	writer.SetOriginalContent(data)
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)

	// Verify translation in output
	outData := buf.Bytes()
	outZR, err := zip.NewReader(bytes.NewReader(outData), int64(len(outData)))
	require.NoError(t, err)

	for _, f := range outZR.File {
		if f.Name == "Stories/Story_u1.xml" {
			content, err := readZipFile(f)
			require.NoError(t, err)
			assert.Contains(t, string(content), "Bonjour le monde !")
		}
	}
}

func TestWriterRequiresOriginalContent(t *testing.T) {
	ctx := t.Context()
	writer := NewWriter()
	var buf bytes.Buffer
	err := writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := make(chan *model.Part)
	close(ch)

	err = writer.Write(ctx, ch)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires original content")
}

func TestConfig(t *testing.T) {
	cfg := &Config{}
	cfg.Reset()

	assert.Equal(t, "idml", cfg.FormatName())
	assert.True(t, cfg.ExtractNotes)
	assert.True(t, cfg.SkipDiscretionaryHyphens)
	assert.False(t, cfg.ExtractMasterSpreads)
	require.NoError(t, cfg.Validate())

	err := cfg.ApplyMap(map[string]any{
		"extractNotes":             false,
		"skipDiscretionaryHyphens": false,
		"extractMasterSpreads":     true,
	})
	require.NoError(t, err)
	assert.False(t, cfg.ExtractNotes)
	assert.False(t, cfg.SkipDiscretionaryHyphens)
	assert.True(t, cfg.ExtractMasterSpreads)

	err = cfg.ApplyMap(map[string]any{"unknownKey": true})
	require.Error(t, err)
}

func TestMultipleParagraphsInStory(t *testing.T) {
	data := createIDML(t, map[string]string{
		"Story_u1.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<idPkg:Story xmlns:idPkg="http://ns.adobe.com/AdobeInDesign/idml/1.0/packaging">
  <Story Self="u1">
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/Title">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>Title text</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/Body">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>Body paragraph one.</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/Body">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>Body paragraph two.</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
  </Story>
</idPkg:Story>`,
	})

	parts := readIDMLBytes(t, data)
	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 3)

	texts := testutil.BlockTexts(blocks)
	assert.Equal(t, "Title text", texts[0])
	assert.Equal(t, "Body paragraph one.", texts[1])
	assert.Equal(t, "Body paragraph two.", texts[2])
}

func TestMultipleCharacterStyleRanges(t *testing.T) {
	data := createIDML(t, map[string]string{
		"Story_u1.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<idPkg:Story xmlns:idPkg="http://ns.adobe.com/AdobeInDesign/idml/1.0/packaging">
  <Story Self="u1">
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/$ID/NormalParagraphStyle">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>Normal </Content>
      </CharacterStyleRange>
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/Bold">
        <Content>bold </Content>
      </CharacterStyleRange>
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>text</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
  </Story>
</idPkg:Story>`,
	})

	parts := readIDMLBytes(t, data)
	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks)

	// Multiple CharacterStyleRanges in the same paragraph should produce
	// separate blocks (each Content element in its own CSR is a separate block).
	texts := testutil.BlockTexts(blocks)
	allText := strings.Join(texts, "")
	assert.Contains(t, allText, "Normal")
	assert.Contains(t, allText, "bold")
	assert.Contains(t, allText, "text")
}

func TestWriterName(t *testing.T) {
	writer := NewWriter()
	assert.Equal(t, "idml", writer.Name())
}
