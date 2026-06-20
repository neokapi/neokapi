package openxml

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readBytes reads a fixture file fully into memory.
func readBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}

// These tests lock in the #928 non-translatable-content surfacing for openxml:
//
//   - image/shape alt text (descr=) and object title (title=) on
//     <wp:docPr>/<pic:cNvPr> surface as Translatable:false RoleCaption blocks
//     when ExtractNonTranslatableContent is on (the default), and pass through
//     verbatim when off;
//   - PowerPoint (<p:text>) and Excel (<comment><text>) comment text surface as
//     Data parts when the flag is on, and are absent when off.
//
// In every case the flag-off part stream — and the untranslated round-trip
// bytes — stay byte-identical to the prior behaviour, so the parity canon (which
// forces the flag off) is unaffected.

// nonTranslatableBlocks returns the Translatable:false Block parts.
func nonTranslatableBlocks(parts []*model.Part) []*model.Block {
	var blocks []*model.Block
	for _, p := range parts {
		if p.Type == model.PartBlock {
			if b, ok := p.Resource.(*model.Block); ok && !b.Translatable {
				blocks = append(blocks, b)
			}
		}
	}
	return blocks
}

// dataPartsOf returns the Data parts.
func dataPartsOf(parts []*model.Part) []*model.Data {
	var data []*model.Data
	for _, p := range parts {
		if p.Type == model.PartData {
			if d, ok := p.Resource.(*model.Data); ok {
				data = append(data, d)
			}
		}
	}
	return data
}

// blockWithElement returns the first block whose Properties["element"] matches.
func blockWithElement(blocks []*model.Block, element string) *model.Block {
	for _, b := range blocks {
		if b.Properties != nil && b.Properties["element"] == element {
			return b
		}
	}
	return nil
}

// roundtripUntranslatedConfig reads input with a skeleton store + custom config,
// writes it straight back (no translations applied), and returns the output ZIP
// bytes. This exercises the skeleton reconstruction path while leaving every
// block at its source value, so the output should be byte-stable.
func roundtripUntranslatedConfig(t *testing.T, input []byte, uri string, configure func(*Config)) []byte {
	t.Helper()

	skelStore, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer skelStore.Close()

	reader := NewReader()
	if configure != nil {
		configure(reader.cfg)
	}
	reader.SetSkeletonStore(skelStore)
	doc := &model.RawDocument{
		URI:          uri,
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       readCloserFromBytes(input),
	}
	require.NoError(t, reader.Open(t.Context(), doc))
	parts := testutil.CollectParts(t, reader.Read(t.Context()))
	reader.Close()

	var buf bytes.Buffer
	writer := NewWriter()
	writer.SetOriginalContent(input)
	writer.SetSkeletonStore(skelStore)
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(t.Context(), testutil.PartsToChannel(parts)))
	writer.Close()

	require.Positive(t, buf.Len(), "output should not be empty")
	return buf.Bytes()
}

// zipPartBytes returns the decompressed bytes of a named ZIP entry.
func zipPartBytes(t *testing.T, archive []byte, name string) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	require.NoError(t, err)
	zf := zipFileByName(zr, name)
	require.NotNil(t, zf, "archive should contain %s", name)
	data, err := readZipFile(zf)
	require.NoError(t, err)
	return data
}

// --- A: WML drawing alt-text (descr/title) ---

// TestWMLAltTextSurfacedWhenOn: with the default flag, a <wp:docPr>/<pic:cNvPr>
// descr= attribute surfaces as a Translatable:false RoleCaption "property"
// block carrying the alt text verbatim, while the graphic name= stays
// translatable.
func TestWMLAltTextSurfacedWhenOn(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "picture.docx"))

	descr := blockWithElement(nonTranslatableBlocks(parts), "drawing-descr")
	require.NotNil(t, descr, "descr= alt text should surface as a non-translatable block")
	assert.False(t, descr.Translatable, "alt text must not be translatable (MT-skipped)")
	assert.Equal(t, model.RoleCaption, descr.SemanticRole(), "alt text should carry the caption role")
	assert.Equal(t, "Okapi2.jpg", descr.SourceText(), "alt text should be carried verbatim")
	assert.Equal(t, "property", descr.Type)

	// The graphic name remains translatable (unchanged behaviour).
	names := blockWithElement(translatableBlocks(parts), "drawing-name")
	require.NotNil(t, names, "drawing name= should still be a translatable block")
}

// TestWMLAltTextHiddenWhenOff: with the flag off, descr=/title= are NOT
// surfaced as blocks — they stay inside the opaque drawing skeleton.
func TestWMLAltTextHiddenWhenOff(t *testing.T) {
	dir := testdataDir(t)
	parts := readFileWithConfig(t, filepath.Join(dir, "picture.docx"), func(c *Config) {
		c.SetExtractNonTranslatableContent(false)
	})

	for _, b := range nonTranslatableBlocks(parts) {
		if b.Properties != nil {
			el := b.Properties["element"]
			assert.NotEqual(t, "drawing-descr", el, "descr must not surface when the flag is off")
			assert.NotEqual(t, "drawing-title", el, "title must not surface when the flag is off")
		}
	}

	// The graphic name remains translatable regardless of the flag.
	names := blockWithElement(translatableBlocks(parts), "drawing-name")
	require.NotNil(t, names, "drawing name= should still be a translatable block when the flag is off")
}

// TestWMLAltTextRoundTripByteStable: an untranslated round-trip with the flag ON
// produces word/document.xml byte-identical to the flag-OFF round-trip — proving
// the alt-text marker mechanism restores the source value exactly and the parity
// canon (flag off) is unaffected.
func TestWMLAltTextRoundTripByteStable(t *testing.T) {
	dir := testdataDir(t)
	original := readBytes(t, filepath.Join(dir, "picture.docx"))

	on := roundtripUntranslatedConfig(t, original, "picture.docx", func(c *Config) {
		c.SetExtractNonTranslatableContent(true)
	})
	off := roundtripUntranslatedConfig(t, original, "picture.docx", func(c *Config) {
		c.SetExtractNonTranslatableContent(false)
	})

	onDoc := zipPartBytes(t, on, "word/document.xml")
	offDoc := zipPartBytes(t, off, "word/document.xml")
	assert.Equal(t, offDoc, onDoc,
		"untranslated round-trip of word/document.xml must be byte-identical with the flag on vs off")

	// And the source descr survives the round-trip verbatim.
	assert.Contains(t, string(onDoc), `descr="Okapi2.jpg"`,
		"alt text must round-trip into the rewritten drawing")
}

// TestWMLTitleSurfacedWhenOn: a <wp:docPr> title= (object title) attribute
// surfaces as a Translatable:false RoleCaption block when the flag is on, while
// the graphic name= stays translatable. simple_chart.docx carries both.
func TestWMLTitleSurfacedWhenOn(t *testing.T) {
	dir := testdataDir(t)

	on := readFile(t, filepath.Join(dir, "simple_chart.docx"))
	title := blockWithElement(nonTranslatableBlocks(on), "drawing-title")
	require.NotNil(t, title, "title= should surface as a non-translatable block")
	assert.False(t, title.Translatable)
	assert.Equal(t, model.RoleCaption, title.SemanticRole())
	assert.Equal(t, "This is the title of the chart.", title.SourceText())

	// Off: no title block.
	off := readFileWithConfig(t, filepath.Join(dir, "simple_chart.docx"), func(c *Config) {
		c.SetExtractNonTranslatableContent(false)
	})
	assert.Nil(t, blockWithElement(nonTranslatableBlocks(off), "drawing-title"),
		"title= must not surface when the flag is off")

	// Byte-stable untranslated round-trip (flag on vs off).
	original := readBytes(t, filepath.Join(dir, "simple_chart.docx"))
	onZip := roundtripUntranslatedConfig(t, original, "chart.docx", func(c *Config) { c.SetExtractNonTranslatableContent(true) })
	offZip := roundtripUntranslatedConfig(t, original, "chart.docx", func(c *Config) { c.SetExtractNonTranslatableContent(false) })
	assert.Equal(t,
		zipPartBytes(t, offZip, "word/document.xml"),
		zipPartBytes(t, onZip, "word/document.xml"),
		"untranslated round-trip must be byte-identical with the flag on vs off")
}

// --- A: PPTX cNvPr alt-text (descr/title) ---

// TestPPTXAltTextSurfacedWhenOn: with the default flag, a <p:cNvPr> descr=
// attribute on a slide surfaces as a Translatable:false RoleCaption block.
func TestPPTXAltTextSurfacedWhenOn(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "1380-graphic-metadata.pptx"))

	descr := blockWithElement(nonTranslatableBlocks(parts), "drawing-descr")
	require.NotNil(t, descr, "PPTX cNvPr descr= alt text should surface as a non-translatable block")
	assert.False(t, descr.Translatable)
	assert.Equal(t, model.RoleCaption, descr.SemanticRole())
	assert.Equal(t, "Screen Clipping", descr.SourceText())
	assert.Equal(t, "property", descr.Type)
}

// TestPPTXAltTextHiddenWhenOff: with the flag off, the PPTX cNvPr descr/title
// stay in the opaque slide skeleton (no surfaced blocks).
func TestPPTXAltTextHiddenWhenOff(t *testing.T) {
	dir := testdataDir(t)
	parts := readFileWithConfig(t, filepath.Join(dir, "1380-graphic-metadata.pptx"), func(c *Config) {
		c.SetExtractNonTranslatableContent(false)
	})

	for _, b := range nonTranslatableBlocks(parts) {
		if b.Properties != nil {
			el := b.Properties["element"]
			assert.NotEqual(t, "drawing-descr", el, "PPTX descr must not surface when the flag is off")
			assert.NotEqual(t, "drawing-title", el, "PPTX title must not surface when the flag is off")
		}
	}
}

// TestPPTXAltTextRoundTripByteStable: an untranslated round-trip with the flag on
// produces every slide XML part byte-identical to the flag-off round-trip.
func TestPPTXAltTextRoundTripByteStable(t *testing.T) {
	dir := testdataDir(t)
	original := readBytes(t, filepath.Join(dir, "1380-graphic-metadata.pptx"))

	on := roundtripUntranslatedConfig(t, original, "deck.pptx", func(c *Config) {
		c.SetExtractNonTranslatableContent(true)
	})
	off := roundtripUntranslatedConfig(t, original, "deck.pptx", func(c *Config) {
		c.SetExtractNonTranslatableContent(false)
	})

	onSlide := zipPartBytes(t, on, "ppt/slides/slide1.xml")
	offSlide := zipPartBytes(t, off, "ppt/slides/slide1.xml")
	assert.Equal(t, offSlide, onSlide,
		"untranslated round-trip of slide1.xml must be byte-identical with the flag on vs off")
	assert.Contains(t, string(onSlide), `descr="Screen Clipping"`,
		"PPTX alt text must round-trip into the rewritten slide")
}

// --- B: PPTX comments ---

// commentTexts returns the text of every comment Data part.
func commentTexts(parts []*model.Part) []string {
	var texts []string
	for _, d := range dataPartsOf(parts) {
		if d.Name == "comment" {
			texts = append(texts, d.Properties["text"])
		}
	}
	return texts
}

// TestPPTXCommentsSurfacedWhenOn: with the default flag, legacy PowerPoint
// comment bodies (<p:text>) surface as informational Data parts, never as
// translatable blocks.
func TestPPTXCommentsSurfacedWhenOn(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "Comments.pptx"))

	texts := commentTexts(parts)
	assert.Contains(t, texts, "This is a comment on a slide body.")
	assert.Contains(t, texts, "Comment on the title slide")

	// Comments must not become translatable blocks.
	for _, b := range translatableBlocks(parts) {
		assert.NotContains(t, b.SourceText(), "This is a comment on a slide body.")
		assert.NotContains(t, b.SourceText(), "Comment on the title slide")
	}
}

// TestPPTXCommentsHiddenWhenOff: with the flag off, no comment Data parts are
// emitted, so the part stream stays byte-identical for parity.
func TestPPTXCommentsHiddenWhenOff(t *testing.T) {
	dir := testdataDir(t)
	parts := readFileWithConfig(t, filepath.Join(dir, "Comments.pptx"), func(c *Config) {
		c.SetExtractNonTranslatableContent(false)
	})
	assert.Empty(t, commentTexts(parts), "no comment Data parts when the flag is off")
}

// TestPPTXCommentsRoundTripByteStable: the comment part round-trips verbatim and
// is byte-identical whether the flag is on or off.
func TestPPTXCommentsRoundTripByteStable(t *testing.T) {
	dir := testdataDir(t)
	original := readBytes(t, filepath.Join(dir, "Comments.pptx"))

	on := roundtripUntranslatedConfig(t, original, "deck.pptx", func(c *Config) {
		c.SetExtractNonTranslatableContent(true)
	})
	off := roundtripUntranslatedConfig(t, original, "deck.pptx", func(c *Config) {
		c.SetExtractNonTranslatableContent(false)
	})

	onC := zipPartBytes(t, on, "ppt/comments/comment1.xml")
	offC := zipPartBytes(t, off, "ppt/comments/comment1.xml")
	assert.Equal(t, offC, onC, "comment part must be byte-identical with the flag on vs off")
	assert.Contains(t, string(onC), "<p:text>", "comment body must survive the round-trip")
}

// --- B: XLSX comments ---

// TestXLSXCommentsSurfacedWhenOn: with the default flag, Excel comment bodies
// (<comment><text>) surface as informational Data parts carrying the cell ref,
// never as translatable blocks.
func TestXLSXCommentsSurfacedWhenOn(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "982-1.xlsx"))

	var found *model.Data
	for _, d := range dataPartsOf(parts) {
		if d.Name == "comment" {
			found = d
			break
		}
	}
	require.NotNil(t, found, "Excel comment should surface as a Data part")
	assert.Contains(t, found.Properties["text"], "A comment with")
	assert.Equal(t, "A1", found.Properties["ref"], "comment should carry its cell reference")
}

// TestXLSXCommentsHiddenWhenOff: with the flag off, no comment Data parts.
func TestXLSXCommentsHiddenWhenOff(t *testing.T) {
	dir := testdataDir(t)
	parts := readFileWithConfig(t, filepath.Join(dir, "982-1.xlsx"), func(c *Config) {
		c.SetExtractNonTranslatableContent(false)
	})
	assert.Empty(t, commentTexts(parts), "no comment Data parts when the flag is off")
}

// TestXLSXCommentsRoundTripByteStable: the comment part is copied verbatim and is
// byte-identical whether the flag is on or off.
func TestXLSXCommentsRoundTripByteStable(t *testing.T) {
	dir := testdataDir(t)
	original := readBytes(t, filepath.Join(dir, "982-1.xlsx"))

	on := roundtripUntranslatedConfig(t, original, "book.xlsx", func(c *Config) {
		c.SetExtractNonTranslatableContent(true)
	})
	off := roundtripUntranslatedConfig(t, original, "book.xlsx", func(c *Config) {
		c.SetExtractNonTranslatableContent(false)
	})

	onC := zipPartBytes(t, on, "xl/comments1.xml")
	offC := zipPartBytes(t, off, "xl/comments1.xml")
	assert.Equal(t, offC, onC, "comment part must be byte-identical with the flag on vs off")
}
