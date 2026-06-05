package epub_test

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/epub"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const containerXML = `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`

const contentOPF = `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <manifest>
    <item id="ch1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
    <item id="ch2" href="chapter2.xhtml" media-type="application/xhtml+xml"/>
    <item id="css" href="style.css" media-type="text/css"/>
  </manifest>
  <spine>
    <itemref idref="ch1"/>
    <itemref idref="ch2"/>
  </spine>
</package>`

const chapter1XHTML = `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Chapter 1</title></head>
<body>
  <h1>Welcome</h1>
  <p>This is the first paragraph.</p>
  <p>This is the second paragraph.</p>
</body>
</html>`

const chapter2XHTML = `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Chapter 2</title></head>
<body>
  <h2>Conclusion</h2>
  <p>Final thoughts here.</p>
</body>
</html>`

func makeEPUB(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// mimetype must be first entry, uncompressed
	header := &zip.FileHeader{
		Name:   "mimetype",
		Method: zip.Store,
	}
	w, err := zw.CreateHeader(header)
	require.NoError(t, err)
	_, err = io.WriteString(w, "application/epub+zip")
	require.NoError(t, err)

	entries := map[string]string{
		"META-INF/container.xml": containerXML,
		"OEBPS/content.opf":      contentOPF,
		"OEBPS/chapter1.xhtml":   chapter1XHTML,
		"OEBPS/chapter2.xhtml":   chapter2XHTML,
		"OEBPS/style.css":        "body { font-family: serif; }",
	}

	for name, content := range entries {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = io.WriteString(w, content)
		require.NoError(t, err)
	}

	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func rawDocFromBytes(data []byte, locale model.LocaleID) *model.RawDocument {
	return &model.RawDocument{
		URI:          "test://book.epub",
		SourceLocale: locale,
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader(data)),
	}
}

// okapi: EpubFilterTests#testSimpleReadWrite — extracts text from EPUB chapters.
func TestReadEPUBContent(t *testing.T) {
	ctx := t.Context()
	data := makeEPUB(t)

	reader := epub.NewReader()
	err := reader.Open(ctx, rawDocFromBytes(data, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	texts := testutil.BlockTexts(blocks)
	assert.Contains(t, texts, "Welcome")
	assert.Contains(t, texts, "This is the first paragraph.")
	assert.Contains(t, texts, "This is the second paragraph.")
	assert.Contains(t, texts, "Conclusion")
	assert.Contains(t, texts, "Final thoughts here.")
}

// okapi: EpubFilterTests#testSimpleReadWrite — verifies sub-document layer structure per chapter.
func TestReadEPUBLayerStructure(t *testing.T) {
	ctx := t.Context()
	data := makeEPUB(t)

	reader := epub.NewReader()
	err := reader.Open(ctx, rawDocFromBytes(data, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	// Must start and end with root layer
	require.GreaterOrEqual(t, len(parts), 2)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	rootLayer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "epub", rootLayer.Format)
	assert.True(t, rootLayer.IsRoot())

	// Count child layers (one per chapter)
	childLayerCount := 0
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			l := p.Resource.(*model.Layer)
			if l.ParentID != "" {
				childLayerCount++
			}
		}
	}
	assert.Equal(t, 2, childLayerCount, "should have 2 child layers for 2 chapters")
}

func TestReadEPUBNonContentAsData(t *testing.T) {
	ctx := t.Context()
	data := makeEPUB(t)

	reader := epub.NewReader()
	err := reader.Open(ctx, rawDocFromBytes(data, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	// style.css should be emitted as Data
	var hasCSS bool
	for _, p := range parts {
		if p.Type == model.PartData {
			d := p.Resource.(*model.Data)
			if d.Properties["entry"] == "OEBPS/style.css" {
				hasCSS = true
			}
		}
	}
	assert.True(t, hasCSS, "CSS file should be emitted as Data")
}

// okapi: EpubFilterTests#testInformation — title elements are extracted as translatable content.
func TestReadEPUBTitleExtraction(t *testing.T) {
	ctx := t.Context()
	data := makeEPUB(t)

	reader := epub.NewReader()
	err := reader.Open(ctx, rawDocFromBytes(data, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	texts := testutil.BlockTexts(blocks)

	// <title> tags should be extracted
	assert.Contains(t, texts, "Chapter 1")
	assert.Contains(t, texts, "Chapter 2")
}

func TestReadNilDocument(t *testing.T) {
	ctx := t.Context()
	reader := epub.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}

// okapi: EpubFilterTests#testInformation — verifies EPUB MIME type and file signature.
func TestReaderSignature(t *testing.T) {
	reader := epub.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "application/epub+zip")
	assert.Contains(t, sig.Extensions, ".epub")
}

func TestReaderMetadata(t *testing.T) {
	reader := epub.NewReader()
	assert.Equal(t, "epub", reader.Name())
	assert.Equal(t, "EPUB E-Book", reader.DisplayName())
}

func TestRoundTrip(t *testing.T) {
	ctx := t.Context()
	data := makeEPUB(t)

	reader := epub.NewReader()
	err := reader.Open(ctx, rawDocFromBytes(data, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := epub.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetOriginalContent(data)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	// Read back the output
	reader2 := epub.NewReader()
	err = reader2.Open(ctx, rawDocFromBytes(buf.Bytes(), model.LocaleEnglish))
	require.NoError(t, err)
	defer reader2.Close()

	blocks := testutil.CollectBlocks(t, reader2.Read(ctx))
	texts := testutil.BlockTexts(blocks)
	assert.Contains(t, texts, "Welcome")
	assert.Contains(t, texts, "This is the first paragraph.")
	assert.Contains(t, texts, "Final thoughts here.")
}

func TestRoundTripWithTranslation(t *testing.T) {
	ctx := t.Context()
	data := makeEPUB(t)

	reader := epub.NewReader()
	err := reader.Open(ctx, rawDocFromBytes(data, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Set translations
	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			switch block.SourceText() {
			case "Welcome":
				block.SetTargetText("fr", "Bienvenue")
			case "This is the first paragraph.":
				block.SetTargetText("fr", "Ceci est le premier paragraphe.")
			case "Final thoughts here.":
				block.SetTargetText("fr", "Pensees finales ici.")
			}
		}
	}

	var buf bytes.Buffer
	writer := epub.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetOriginalContent(data)
	writer.SetLocale("fr")

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	// Read back and verify translations
	reader2 := epub.NewReader()
	err = reader2.Open(ctx, rawDocFromBytes(buf.Bytes(), model.LocaleEnglish))
	require.NoError(t, err)
	defer reader2.Close()

	blocks := testutil.CollectBlocks(t, reader2.Read(ctx))
	texts := testutil.BlockTexts(blocks)
	assert.Contains(t, texts, "Bienvenue")
	assert.Contains(t, texts, "Ceci est le premier paragraphe.")
	assert.Contains(t, texts, "Pensees finales ici.")
}

func TestConfigFormatName(t *testing.T) {
	cfg := &epub.Config{}
	assert.Equal(t, "epub", cfg.FormatName())
}

func TestConfigApplyMapUnknown(t *testing.T) {
	cfg := &epub.Config{}
	err := cfg.ApplyMap(map[string]any{
		"unknown": "value",
	})
	require.Error(t, err)
}

func TestConfigValidate(t *testing.T) {
	cfg := &epub.Config{}
	require.NoError(t, cfg.Validate())
}

func TestSkeletonRoundTrip(t *testing.T) {
	ctx := t.Context()
	data := makeEPUB(t)

	// Read with skeleton store
	skelStore, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer skelStore.Close()

	reader := epub.NewReader()
	reader.SetSkeletonStore(skelStore)
	err = reader.Open(ctx, rawDocFromBytes(data, model.LocaleEnglish))
	require.NoError(t, err)

	parts1 := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	blocks1 := collectBlockTexts(parts1)

	// Write with skeleton store
	var buf bytes.Buffer
	writer := epub.NewWriter()
	writer.SetOriginalContent(data)
	writer.SetSkeletonStore(skelStore)
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts1)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	require.Greater(t, buf.Len(), 0, "output should not be empty")

	// Re-read and compare
	reader2 := epub.NewReader()
	err = reader2.Open(ctx, rawDocFromBytes(buf.Bytes(), model.LocaleEnglish))
	require.NoError(t, err)
	defer reader2.Close()

	parts2 := testutil.CollectParts(t, reader2.Read(ctx))
	blocks2 := collectBlockTexts(parts2)

	assert.Len(t, blocks2, len(blocks1), "block count should match")
	for i := range blocks1 {
		assert.Equal(t, blocks1[i], blocks2[i], "block[%d] text mismatch", i)
	}
}

func TestSkeletonRoundTripWithTranslation(t *testing.T) {
	ctx := t.Context()
	data := makeEPUB(t)

	// Read with skeleton store
	skelStore, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer skelStore.Close()

	reader := epub.NewReader()
	reader.SetSkeletonStore(skelStore)
	err = reader.Open(ctx, rawDocFromBytes(data, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Set translations on all blocks
	frFR := model.LocaleID("fr-FR")
	for _, p := range parts {
		if p.Type == model.PartBlock {
			if b, ok := p.Resource.(*model.Block); ok {
				b.SetTargetText(frFR, "FR: "+b.SourceText())
			}
		}
	}

	// Write with skeleton + locale
	var buf bytes.Buffer
	writer := epub.NewWriter()
	writer.SetOriginalContent(data)
	writer.SetSkeletonStore(skelStore)
	writer.SetLocale(frFR)
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	require.Greater(t, buf.Len(), 0, "output should not be empty")

	// Re-read and verify translations appear
	reader2 := epub.NewReader()
	err = reader2.Open(ctx, rawDocFromBytes(buf.Bytes(), model.LocaleEnglish))
	require.NoError(t, err)
	defer reader2.Close()

	parts2 := testutil.CollectParts(t, reader2.Read(ctx))
	blocks2 := collectBlockTexts(parts2)

	for _, text := range blocks2 {
		assert.True(t, strings.HasPrefix(text, "FR: "),
			"translated text should start with 'FR: ', got: %q", text)
	}
}

func TestSkeletonStoreEmitterInterface(t *testing.T) {
	reader := epub.NewReader()
	var _ format.SkeletonStoreEmitter = reader
}

func TestSkeletonStoreConsumerInterface(t *testing.T) {
	writer := epub.NewWriter()
	var _ format.SkeletonStoreConsumer = writer
}

func TestOriginalContentSetterInterface(t *testing.T) {
	writer := epub.NewWriter()
	var _ format.OriginalContentSetter = writer
}

// collectBlockTexts extracts source texts from block parts.
func collectBlockTexts(parts []*model.Part) []string {
	var texts []string
	for _, p := range parts {
		if p.Type == model.PartBlock {
			if b, ok := p.Resource.(*model.Block); ok {
				texts = append(texts, b.SourceText())
			}
		}
	}
	return texts
}
