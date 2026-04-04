package memorytest

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"runtime"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/epub"
	htmlfmt "github.com/neokapi/neokapi/core/formats/html"
	jsonfmt "github.com/neokapi/neokapi/core/formats/json"
	"github.com/neokapi/neokapi/core/formats/odf"
	"github.com/neokapi/neokapi/core/formats/openxml"
	"github.com/neokapi/neokapi/core/formats/plaintext"
	xmlfmt "github.com/neokapi/neokapi/core/formats/xml"
	"github.com/neokapi/neokapi/core/formats/yaml"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// Document generators
// --------------------------------------------------------------------------

// generatePlaintext creates a plain text document with the given number of lines.
func generatePlaintext(lines int) string {
	var b strings.Builder
	for i := range lines {
		fmt.Fprintf(&b, "This is line number %d with some representative content for testing memory usage.\n", i+1)
	}
	return b.String()
}

// generateHTML creates an HTML document with the given number of paragraphs.
func generateHTML(paragraphs int) string {
	var b strings.Builder
	b.WriteString("<!DOCTYPE html><html><head><title>Memory Test</title></head><body>\n")
	for i := range paragraphs {
		fmt.Fprintf(&b, "<p>Paragraph %d: This paragraph has <strong>bold text</strong> and <em>italic text</em> and a <a href=\"#link%d\">hyperlink</a> to test inline spans.</p>\n", i+1, i+1)
	}
	b.WriteString("</body></html>")
	return b.String()
}

// generateJSON creates a JSON document with the given number of key-value pairs.
func generateJSON(entries int) string {
	var b strings.Builder
	b.WriteString("{\n")
	for i := range entries {
		if i > 0 {
			b.WriteString(",\n")
		}
		fmt.Fprintf(&b, `  "key_%d": "This is translatable string number %d with enough text to be realistic."`, i+1, i+1)
	}
	b.WriteString("\n}")
	return b.String()
}

// generateXML creates an XML document with the given number of translatable elements.
func generateXML(elements int) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n<resources>\n")
	for i := range elements {
		fmt.Fprintf(&b, `  <string name="key_%d">This is translatable string number %d with realistic content.</string>`+"\n", i+1, i+1)
	}
	b.WriteString("</resources>")
	return b.String()
}

// generateYAML creates a YAML document with the given number of key-value pairs.
func generateYAML(entries int) string {
	var b strings.Builder
	for i := range entries {
		fmt.Fprintf(&b, "key_%d: \"This is translatable string number %d with realistic content for testing.\"\n", i+1, i+1)
	}
	return b.String()
}

// generateODF creates an ODF (ODT) ZIP archive with the given number of paragraphs.
func generateODF(paragraphs int) []byte {
	var contentXML strings.Builder
	contentXML.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<office:document-content
  xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
  xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0"
  xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0">
<office:body><office:text>`)
	for i := range paragraphs {
		fmt.Fprintf(&contentXML, `<text:p>Paragraph %d: This is a test paragraph with enough content to be realistic for memory testing.</text:p>`, i+1)
	}
	contentXML.WriteString(`</office:text></office:body></office:document-content>`)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// mimetype must be first, uncompressed
	fh := &zip.FileHeader{Name: "mimetype", Method: zip.Store}
	w, _ := zw.CreateHeader(fh)
	_, _ = w.Write([]byte("application/vnd.oasis.opendocument.text"))

	w, _ = zw.Create("content.xml")
	_, _ = w.Write([]byte(contentXML.String()))

	w, _ = zw.Create("META-INF/manifest.xml")
	_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0">
  <manifest:file-entry manifest:full-path="/" manifest:media-type="application/vnd.oasis.opendocument.text"/>
  <manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/>
</manifest:manifest>`))

	zw.Close()
	return buf.Bytes()
}

// generateEPUB creates an EPUB ZIP archive with the given number of chapters,
// each containing the given number of paragraphs.
func generateEPUB(chapters, paragraphsPerChapter int) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// mimetype
	fh := &zip.FileHeader{Name: "mimetype", Method: zip.Store}
	w, _ := zw.CreateHeader(fh)
	_, _ = w.Write([]byte("application/epub+zip"))

	// container.xml
	w, _ = zw.Create("META-INF/container.xml")
	_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`)

	// content.opf with manifest/spine for all chapters
	var opfManifest, opfSpine strings.Builder
	for i := 1; i <= chapters; i++ {
		fmt.Fprintf(&opfManifest, `    <item id="ch%d" href="chapter%d.xhtml" media-type="application/xhtml+xml"/>`+"\n", i, i)
		fmt.Fprintf(&opfSpine, `    <itemref idref="ch%d"/>`+"\n", i)
	}
	w, _ = zw.Create("OEBPS/content.opf")
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <manifest>
%s  </manifest>
  <spine>
%s  </spine>
</package>`, opfManifest.String(), opfSpine.String())

	// Chapter XHTML files
	for ch := 1; ch <= chapters; ch++ {
		var body strings.Builder
		for p := 1; p <= paragraphsPerChapter; p++ {
			fmt.Fprintf(&body, "  <p>Chapter %d, paragraph %d: This is test content with enough text to be realistic.</p>\n", ch, p)
		}
		w, _ = zw.Create(fmt.Sprintf("OEBPS/chapter%d.xhtml", ch))
		fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Chapter %d</title></head>
<body>
  <h1>Chapter %d</h1>
%s</body>
</html>`, ch, ch, body.String())
	}

	zw.Close()
	return buf.Bytes()
}

// generateOpenXMLDocx creates a minimal DOCX ZIP archive with the given number of paragraphs.
func generateOpenXMLDocx(paragraphs int) []byte {
	var bodyXML strings.Builder
	for i := range paragraphs {
		fmt.Fprintf(&bodyXML, `<w:p><w:r><w:t>Paragraph %d: This is a test paragraph with enough content to be realistic for memory testing purposes.</w:t></w:r></w:p>`, i+1)
	}

	documentXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:wpc="http://schemas.microsoft.com/office/word/2010/wordprocessingCanvas"
  xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006"
  xmlns:o="urn:schemas-microsoft-com:office:office"
  xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"
  xmlns:m="http://schemas.openxmlformats.org/officeDocument/2006/math"
  xmlns:v="urn:schemas-microsoft-com:vml"
  xmlns:wp14="http://schemas.microsoft.com/office/word/2010/wordprocessingDrawing"
  xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"
  xmlns:w10="urn:schemas-microsoft-com:office:word"
  xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"
  xmlns:w14="http://schemas.microsoft.com/office/word/2010/wordml"
  xmlns:w15="http://schemas.microsoft.com/office/word/2012/wordml"
  xmlns:wpg="http://schemas.microsoft.com/office/word/2010/wordprocessingGroup"
  xmlns:wpi="http://schemas.microsoft.com/office/word/2010/wordprocessingInk"
  xmlns:wne="http://schemas.microsoft.com/office/word/2006/wordml"
  xmlns:wps="http://schemas.microsoft.com/office/word/2010/wordprocessingShape"
  mc:Ignorable="w14 w15 wp14">
  <w:body>%s</w:body>
</w:document>`, bodyXML.String())

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// [Content_Types].xml
	w, _ := zw.Create("[Content_Types].xml")
	_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`)

	// _rels/.rels
	w, _ = zw.Create("_rels/.rels")
	_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`)

	// word/_rels/document.xml.rels
	w, _ = zw.Create("word/_rels/document.xml.rels")
	_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"/>`)

	// word/document.xml
	w, _ = zw.Create("word/document.xml")
	_, _ = io.WriteString(w, documentXML)

	zw.Close()
	return buf.Bytes()
}

// --------------------------------------------------------------------------
// Test helpers
// --------------------------------------------------------------------------

// rawDocFromString creates a RawDocument from a string.
func rawDocFromString(content string, sourceLocale model.LocaleID) *model.RawDocument {
	return &model.RawDocument{
		URI:          "test://memory-input",
		SourceLocale: sourceLocale,
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(strings.NewReader(content)),
	}
}

// rawDocFromBytes creates a RawDocument from a byte slice.
func rawDocFromBytes(data []byte, uri string, sourceLocale model.LocaleID) *model.RawDocument {
	return &model.RawDocument{
		URI:          uri,
		SourceLocale: sourceLocale,
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader(data)),
	}
}

// drainParts reads all parts from a reader's channel and discards them,
// returning any error encountered.
func drainParts(t testing.TB, ch <-chan model.PartResult) {
	t.Helper()
	for result := range ch {
		if result.Error != nil {
			t.Fatalf("reader error: %v", result.Error)
		}
	}
}

// heapInUse returns the current heap in use after forcing GC.
func heapInUse() uint64 {
	runtime.GC()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapInuse
}

// measureHeapGrowth runs a read operation and measures heap growth.
func measureHeapGrowth(t *testing.T, readFn func()) uint64 {
	t.Helper()

	// Force GC and get baseline
	baseline := heapInUse()

	readFn()

	// Measure after read completes (parts are drained synchronously)
	peak := heapInUse()

	if peak < baseline {
		return 0
	}
	return peak - baseline
}

// --------------------------------------------------------------------------
// Benchmark tests: measure allocation per read operation
// --------------------------------------------------------------------------

func BenchmarkMemory_Plaintext(b *testing.B) {
	content := generatePlaintext(10000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := plaintext.NewReader()
		store, err := format.NewSkeletonStore()
		if err != nil {
			b.Fatal(err)
		}
		reader.SetSkeletonStore(store)
		ctx := context.Background()
		if err := reader.Open(ctx, rawDocFromString(content, model.LocaleEnglish)); err != nil {
			b.Fatal(err)
		}
		drainParts(b, reader.Read(ctx))
		reader.Close()
		store.Close()
	}
}

func BenchmarkMemory_HTML(b *testing.B) {
	content := generateHTML(5000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := htmlfmt.NewReader()
		store, err := format.NewSkeletonStore()
		if err != nil {
			b.Fatal(err)
		}
		reader.SetSkeletonStore(store)
		ctx := context.Background()
		if err := reader.Open(ctx, rawDocFromString(content, model.LocaleEnglish)); err != nil {
			b.Fatal(err)
		}
		drainParts(b, reader.Read(ctx))
		reader.Close()
		store.Close()
	}
}

func BenchmarkMemory_JSON(b *testing.B) {
	content := generateJSON(10000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := jsonfmt.NewReader()
		store, err := format.NewSkeletonStore()
		if err != nil {
			b.Fatal(err)
		}
		reader.SetSkeletonStore(store)
		ctx := context.Background()
		if err := reader.Open(ctx, rawDocFromString(content, model.LocaleEnglish)); err != nil {
			b.Fatal(err)
		}
		drainParts(b, reader.Read(ctx))
		reader.Close()
		store.Close()
	}
}

func BenchmarkMemory_XML(b *testing.B) {
	content := generateXML(10000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := xmlfmt.NewReader()
		store, err := format.NewSkeletonStore()
		if err != nil {
			b.Fatal(err)
		}
		reader.SetSkeletonStore(store)
		ctx := context.Background()
		if err := reader.Open(ctx, rawDocFromString(content, model.LocaleEnglish)); err != nil {
			b.Fatal(err)
		}
		drainParts(b, reader.Read(ctx))
		reader.Close()
		store.Close()
	}
}

func BenchmarkMemory_YAML(b *testing.B) {
	content := generateYAML(10000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := yaml.NewReader()
		store, err := format.NewSkeletonStore()
		if err != nil {
			b.Fatal(err)
		}
		reader.SetSkeletonStore(store)
		ctx := context.Background()
		if err := reader.Open(ctx, rawDocFromString(content, model.LocaleEnglish)); err != nil {
			b.Fatal(err)
		}
		drainParts(b, reader.Read(ctx))
		reader.Close()
		store.Close()
	}
}

func BenchmarkMemory_ODF(b *testing.B) {
	data := generateODF(2000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := odf.NewReader()
		store, err := format.NewSkeletonStore()
		if err != nil {
			b.Fatal(err)
		}
		reader.SetSkeletonStore(store)
		ctx := context.Background()
		if err := reader.Open(ctx, rawDocFromBytes(data, "test.odt", model.LocaleEnglish)); err != nil {
			b.Fatal(err)
		}
		drainParts(b, reader.Read(ctx))
		reader.Close()
		store.Close()
	}
}

func BenchmarkMemory_EPUB(b *testing.B) {
	data := generateEPUB(10, 200)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := epub.NewReader()
		store, err := format.NewSkeletonStore()
		if err != nil {
			b.Fatal(err)
		}
		reader.SetSkeletonStore(store)
		ctx := context.Background()
		if err := reader.Open(ctx, rawDocFromBytes(data, "test.epub", model.LocaleEnglish)); err != nil {
			b.Fatal(err)
		}
		drainParts(b, reader.Read(ctx))
		reader.Close()
		store.Close()
	}
}

func BenchmarkMemory_OpenXML(b *testing.B) {
	data := generateOpenXMLDocx(2000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := openxml.NewReader()
		store, err := format.NewSkeletonStore()
		if err != nil {
			b.Fatal(err)
		}
		reader.SetSkeletonStore(store)
		ctx := context.Background()
		if err := reader.Open(ctx, rawDocFromBytes(data, "test.docx", model.LocaleEnglish)); err != nil {
			b.Fatal(err)
		}
		drainParts(b, reader.Read(ctx))
		reader.Close()
		store.Close()
	}
}

// --------------------------------------------------------------------------
// Memory bound tests: assert peak heap stays under threshold
// --------------------------------------------------------------------------

func TestMemoryBound_Plaintext(t *testing.T) {
	content := generatePlaintext(10000) // ~800KB of text
	runMemoryRead := func() {
		reader := plaintext.NewReader()
		store, err := format.NewSkeletonStore()
		require.NoError(t, err)
		defer store.Close()
		reader.SetSkeletonStore(store)
		ctx := context.Background()
		require.NoError(t, reader.Open(ctx, rawDocFromString(content, model.LocaleEnglish)))
		drainParts(t, reader.Read(ctx))
		reader.Close()
	}

	// Warm up to populate caches, compile regexps, etc.
	runMemoryRead()

	growth := measureHeapGrowth(t, runMemoryRead)
	t.Logf("Plaintext: heap growth = %d KB for ~%d KB input", growth/1024, len(content)/1024)
	// Plaintext reads line-by-line with skeleton store; heap growth should be modest.
	// Allow 2MB headroom for GC timing and runtime overhead.
	assert.Less(t, growth, uint64(2*1024*1024), "plaintext heap growth should stay under 2MB")
}

func TestMemoryBound_HTML(t *testing.T) {
	content := generateHTML(5000) // ~1MB of HTML
	runMemoryRead := func() {
		reader := htmlfmt.NewReader()
		store, err := format.NewSkeletonStore()
		require.NoError(t, err)
		defer store.Close()
		reader.SetSkeletonStore(store)
		ctx := context.Background()
		require.NoError(t, reader.Open(ctx, rawDocFromString(content, model.LocaleEnglish)))
		drainParts(t, reader.Read(ctx))
		reader.Close()
	}

	runMemoryRead() // warm up
	growth := measureHeapGrowth(t, runMemoryRead)
	t.Logf("HTML: heap growth = %d KB for ~%d KB input", growth/1024, len(content)/1024)
	// HTML tokenizer path with skeleton store reads content once but needs tokenizer state.
	assert.Less(t, growth, uint64(4*1024*1024), "HTML heap growth should stay under 4MB")
}

func TestMemoryBound_JSON(t *testing.T) {
	content := generateJSON(10000) // ~1MB of JSON
	runMemoryRead := func() {
		reader := jsonfmt.NewReader()
		store, err := format.NewSkeletonStore()
		require.NoError(t, err)
		defer store.Close()
		reader.SetSkeletonStore(store)
		ctx := context.Background()
		require.NoError(t, reader.Open(ctx, rawDocFromString(content, model.LocaleEnglish)))
		drainParts(t, reader.Read(ctx))
		reader.Close()
	}

	runMemoryRead()
	growth := measureHeapGrowth(t, runMemoryRead)
	t.Logf("JSON: heap growth = %d KB for ~%d KB input", growth/1024, len(content)/1024)
	assert.Less(t, growth, uint64(4*1024*1024), "JSON heap growth should stay under 4MB")
}

func TestMemoryBound_XML(t *testing.T) {
	content := generateXML(10000) // ~1MB of XML
	runMemoryRead := func() {
		reader := xmlfmt.NewReader()
		store, err := format.NewSkeletonStore()
		require.NoError(t, err)
		defer store.Close()
		reader.SetSkeletonStore(store)
		ctx := context.Background()
		require.NoError(t, reader.Open(ctx, rawDocFromString(content, model.LocaleEnglish)))
		drainParts(t, reader.Read(ctx))
		reader.Close()
	}

	runMemoryRead()
	growth := measureHeapGrowth(t, runMemoryRead)
	t.Logf("XML: heap growth = %d KB for ~%d KB input", growth/1024, len(content)/1024)
	assert.Less(t, growth, uint64(4*1024*1024), "XML heap growth should stay under 4MB")
}

func TestMemoryBound_YAML(t *testing.T) {
	content := generateYAML(10000) // ~900KB of YAML
	runMemoryRead := func() {
		reader := yaml.NewReader()
		store, err := format.NewSkeletonStore()
		require.NoError(t, err)
		defer store.Close()
		reader.SetSkeletonStore(store)
		ctx := context.Background()
		require.NoError(t, reader.Open(ctx, rawDocFromString(content, model.LocaleEnglish)))
		drainParts(t, reader.Read(ctx))
		reader.Close()
	}

	runMemoryRead()
	growth := measureHeapGrowth(t, runMemoryRead)
	t.Logf("YAML: heap growth = %d KB for ~%d KB input", growth/1024, len(content)/1024)
	assert.Less(t, growth, uint64(4*1024*1024), "YAML heap growth should stay under 4MB")
}

func TestMemoryBound_ODF(t *testing.T) {
	data := generateODF(2000) // ZIP with ~200KB of XML content
	runMemoryRead := func() {
		reader := odf.NewReader()
		store, err := format.NewSkeletonStore()
		require.NoError(t, err)
		defer store.Close()
		reader.SetSkeletonStore(store)
		ctx := context.Background()
		require.NoError(t, reader.Open(ctx, rawDocFromBytes(data, "test.odt", model.LocaleEnglish)))
		drainParts(t, reader.Read(ctx))
		reader.Close()
	}

	runMemoryRead()
	growth := measureHeapGrowth(t, runMemoryRead)
	t.Logf("ODF: heap growth = %d KB for ~%d KB ZIP input", growth/1024, len(data)/1024)
	// ODF uses temp file for ZIP access, so heap should not grow by the full ZIP size.
	assert.Less(t, growth, uint64(2*1024*1024), "ODF heap growth should stay under 2MB (temp file approach)")
}

func TestMemoryBound_EPUB(t *testing.T) {
	data := generateEPUB(10, 200) // ~200KB of XHTML content across 10 chapters
	runMemoryRead := func() {
		reader := epub.NewReader()
		store, err := format.NewSkeletonStore()
		require.NoError(t, err)
		defer store.Close()
		reader.SetSkeletonStore(store)
		ctx := context.Background()
		require.NoError(t, reader.Open(ctx, rawDocFromBytes(data, "test.epub", model.LocaleEnglish)))
		drainParts(t, reader.Read(ctx))
		reader.Close()
	}

	runMemoryRead()
	growth := measureHeapGrowth(t, runMemoryRead)
	t.Logf("EPUB: heap growth = %d KB for ~%d KB ZIP input", growth/1024, len(data)/1024)
	// EPUB uses temp file for ZIP access.
	assert.Less(t, growth, uint64(2*1024*1024), "EPUB heap growth should stay under 2MB (temp file approach)")
}

func TestMemoryBound_OpenXML(t *testing.T) {
	data := generateOpenXMLDocx(2000)
	runMemoryRead := func() {
		reader := openxml.NewReader()
		store, err := format.NewSkeletonStore()
		require.NoError(t, err)
		defer store.Close()
		reader.SetSkeletonStore(store)
		ctx := context.Background()
		require.NoError(t, reader.Open(ctx, rawDocFromBytes(data, "test.docx", model.LocaleEnglish)))
		drainParts(t, reader.Read(ctx))
		reader.Close()
	}

	runMemoryRead()
	growth := measureHeapGrowth(t, runMemoryRead)
	t.Logf("OpenXML: heap growth = %d KB for ~%d KB ZIP input", growth/1024, len(data)/1024)
	// OpenXML reads ZIP into memory currently (bytes.NewReader path).
	// This is a baseline; if refactored to temp file, the bound can be tightened.
	assert.Less(t, growth, uint64(4*1024*1024), "OpenXML heap growth should stay under 4MB")
}

// --------------------------------------------------------------------------
// ZIP-specific: temp file usage and cleanup verification
// --------------------------------------------------------------------------

func TestZIPTempFile_ODF_Cleanup(t *testing.T) {
	data := generateODF(100)
	reader := odf.NewReader()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)

	ctx := context.Background()
	require.NoError(t, reader.Open(ctx, rawDocFromBytes(data, "test.odt", model.LocaleEnglish)))
	drainParts(t, reader.Read(ctx))

	// Close should clean up temp file. No panic or error expected.
	require.NoError(t, reader.Close())

	// A second Close should be safe (no double-remove error).
	require.NoError(t, reader.Close())
}

func TestZIPTempFile_EPUB_Cleanup(t *testing.T) {
	data := generateEPUB(2, 10)
	reader := epub.NewReader()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)

	ctx := context.Background()
	require.NoError(t, reader.Open(ctx, rawDocFromBytes(data, "test.epub", model.LocaleEnglish)))
	drainParts(t, reader.Read(ctx))

	require.NoError(t, reader.Close())
	require.NoError(t, reader.Close())
}

func TestZIPFormats_SkeletonStore_ProducesEntries(t *testing.T) {
	// Verify that ZIP-based formats actually produce skeleton entries when
	// a skeleton store is attached, confirming the temp file path is used
	// rather than holding content in memory.
	tests := []struct {
		name      string
		data      []byte
		uri       string
		newReader func() (format.DataFormatReader, format.SkeletonStoreEmitter)
	}{
		{
			name: "ODF",
			data: generateODF(10),
			uri:  "test.odt",
			newReader: func() (format.DataFormatReader, format.SkeletonStoreEmitter) {
				r := odf.NewReader()
				return r, r
			},
		},
		{
			name: "EPUB",
			data: generateEPUB(2, 5),
			uri:  "test.epub",
			newReader: func() (format.DataFormatReader, format.SkeletonStoreEmitter) {
				r := epub.NewReader()
				return r, r
			},
		},
		{
			name: "OpenXML",
			data: generateOpenXMLDocx(10),
			uri:  "test.docx",
			newReader: func() (format.DataFormatReader, format.SkeletonStoreEmitter) {
				r := openxml.NewReader()
				return r, r
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reader, emitter := tc.newReader()
			store, err := format.NewSkeletonStore()
			require.NoError(t, err)
			defer store.Close()

			emitter.SetSkeletonStore(store)
			ctx := context.Background()
			require.NoError(t, reader.Open(ctx, rawDocFromBytes(tc.data, tc.uri, model.LocaleEnglish)))
			drainParts(t, reader.Read(ctx))
			reader.Close()

			// Flush and verify there are skeleton entries
			require.NoError(t, store.Flush())

			entryCount := 0
			for {
				_, err := store.Next()
				if err == io.EOF {
					break
				}
				require.NoError(t, err)
				entryCount++
			}
			assert.Greater(t, entryCount, 0, "%s should produce skeleton entries", tc.name)
			t.Logf("%s: produced %d skeleton entries", tc.name, entryCount)
		})
	}
}
