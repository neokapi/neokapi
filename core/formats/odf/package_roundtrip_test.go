package odf_test

import (
	"archive/zip"
	"bytes"
	encxml "encoding/xml"
	"errors"
	"io"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The ODF reader and writer a production run gets come from the format
// registry, which hands every SubfilterAware reader and writer the registry
// itself as a resolver. These tests take the pair from the registry rather than
// constructing it directly, so what they exercise is the pair a `kapi run`
// builds — the configuration the package's other round-trip tests, which
// construct the pair themselves, do not reach.

// registryPair returns the ODF reader/writer the format registry constructs,
// with the skeleton store core/flow's FileRunner wires between them.
func registryPair(t *testing.T) (format.DataFormatReader, format.DataFormatWriter, *format.SkeletonStore) {
	t.Helper()
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	reader, err := reg.NewReader("odf")
	require.NoError(t, err)
	writer, err := reg.NewWriter("odf")
	require.NoError(t, err)

	skel, err := format.NewWiredSkeleton(reader, writer)
	require.NoError(t, err)
	require.NotNil(t, skel, "odf reader and writer are a skeleton pair")
	t.Cleanup(func() { _ = skel.Close() })

	return reader, writer, skel
}

// zipEntries reads a ZIP archive into a name → bytes map, and reports the
// entry order and each entry's compression method — the ODF package
// requirements (OpenDocument 1.3 part 3 §3.3) that a plain content comparison
// would miss.
func zipEntries(t *testing.T, data []byte) (map[string][]byte, []string, map[string]uint16) {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err, "output is not a readable ZIP archive")

	entries := make(map[string][]byte, len(zr.File))
	methods := make(map[string]uint16, len(zr.File))
	order := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		rc, err := f.Open()
		require.NoError(t, err)
		body, err := io.ReadAll(rc)
		rc.Close()
		require.NoError(t, err)
		entries[f.Name] = body
		methods[f.Name] = f.Method
		order = append(order, f.Name)
	}
	return entries, order, methods
}

// requireODFPackage asserts the bytes are a well-formed OpenDocument package:
// `mimetype` first and stored uncompressed, a manifest, and XML streams that
// parse and carry the OpenDocument root element the stream is defined to have.
func requireODFPackage(t *testing.T, data []byte, wantMime string) map[string][]byte {
	t.Helper()
	entries, order, methods := zipEntries(t, data)

	require.NotEmpty(t, order)
	assert.Equal(t, "mimetype", order[0], "mimetype must be the first entry")
	assert.Equal(t, zip.Store, methods["mimetype"], "mimetype must be stored uncompressed")
	assert.Equal(t, wantMime, string(entries["mimetype"]))
	assert.Contains(t, entries, "META-INF/manifest.xml")

	roots := map[string]string{
		"content.xml": "document-content",
		"styles.xml":  "document-styles",
		"meta.xml":    "document-meta",
	}
	for name, wantRoot := range roots {
		body, ok := entries[name]
		if !ok {
			continue
		}
		d := encxml.NewDecoder(bytes.NewReader(body))
		var root encxml.Name
		for {
			tok, err := d.Token()
			if errors.Is(err, io.EOF) {
				break
			}
			require.NoErrorf(t, err, "%s does not parse as XML: %q", name, string(body))
			if se, ok := tok.(encxml.StartElement); ok && root.Local == "" {
				root = se.Name
			}
		}
		require.NotEmptyf(t, root.Local, "%s has no elements at all", name)
		assert.Equalf(t, wantRoot, root.Local, "%s root element", name)
		assert.Equalf(t, nsOffice, root.Space, "%s root namespace", name)
	}
	return entries
}

// nsOffice is the OpenDocument office namespace every package stream's root
// element is in (OpenDocument 1.3 part 3 §3.1).
const nsOffice = "urn:oasis:names:tc:opendocument:xmlns:office:1.0"

// odtBody wraps raw markup in an office:document-content shell, for the cases
// simpleODTContent's paragraph list cannot spell.
func odtBody(inner string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<office:document-content
  xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
  xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0"
  xmlns:style="urn:oasis:names:tc:opendocument:xmlns:style:1.0"
  xmlns:xlink="http://www.w3.org/1999/xlink">
<office:body><office:text>` + inner + `</office:text></office:body></office:document-content>`
}

// masterPageStyles builds a styles.xml whose master page carries one
// paragraph — the translatable text a header or footer holds, and the reason
// styles.xml is a translatable stream and not only a stylesheet.
func masterPageStyles(paragraph string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<office:document-styles
  xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
  xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0">
<office:master-styles>
<text:p>` + paragraph + `</text:p>
</office:master-styles>
</office:document-styles>`
}

// A run that translates nothing hands back the package it was given: every
// stream byte for byte, through the reader and writer a registry builds.
//
// The cases are the ones a real .odt puts in the way of a skeleton rebuilt
// from parsed tokens rather than sliced from the source — a decoder reports
// `<a/>` and `<a></a>` the same way, hands back `'` for `&apos;`, resolves
// `<text:s>` and `<text:tab/>` into characters, and folds a CRLF inside text
// to LF (XML 1.0 §2.11).
func TestRegistryPair_UntranslatedPackageIsByteExact(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		name    string
		content string
		styles  string
	}{
		{
			name:    "paragraphs",
			content: simpleODTContent("Hello, World!", "Second paragraph"),
			styles:  masterPageStyles("Help"),
		},
		{
			name: "empty elements keep their self-closing form",
			content: odtBody(
				`<office:scripts/><text:p>Body</text:p><text:p text:style-name="P1"/>`),
			styles: masterPageStyles("Header"),
		},
		{
			name: "attribute entities and quoting are the document's own",
			content: odtBody(
				`<text:p text:style-name='Quoted &apos;name&apos;'>` +
					`Ampersand &amp; less-than &lt; apostrophe &apos; here</text:p>`),
			styles: masterPageStyles("Head &amp; foot"),
		},
		{
			name: "resolved space and tab markup comes back as markup",
			content: odtBody(
				`<text:p>Spaces [ <text:s text:c="4"/>] and a tab [<text:tab/>] here</text:p>`),
			styles: masterPageStyles("Help"),
		},
		{
			name:    "a CRLF inside extracted text stays a CRLF",
			content: odtBody("<text:p>First line\r\n\tsecond line</text:p>"),
			styles:  masterPageStyles("Help"),
		},
		{
			name: "inline markup inside a paragraph is the document's own",
			content: odtBody(
				`<text:p>Plain <text:span text:style-name='T1'>bold &amp; bright</text:span>` +
					` and <text:a xlink:href="a.html?x=1&amp;y=2">a link</text:a>.</text:p>`),
			styles: masterPageStyles("Help"),
		},
		{
			name: "a translatable attribute leaves the rest of its tag alone",
			content: odtBody(
				`<text:list-style style:name='L1'>` +
					`<text:list-level-style-number text:level="1" style:num-prefix="Before &gt;" style:num-suffix="&lt; After"/>` +
					`</text:list-style><text:p>Body</text:p>`),
			styles: masterPageStyles("Help"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := makeODFZipWithStyles(mimeODT, tt.content, tt.styles)
			reader, writer, _ := registryPair(t)

			doc := testutil.RawDocFromReader(bytes.NewReader(source), "test.odt", model.LocaleEnglish)
			require.NoError(t, reader.Open(ctx, doc))
			parts := testutil.CollectParts(t, reader.Read(ctx))
			require.NoError(t, reader.Close())

			var buf bytes.Buffer
			require.NoError(t, writer.SetOutputWriter(&buf))
			writer.(format.OriginalContentSetter).SetOriginalContent(source)
			require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
			require.NoError(t, writer.Close())

			got := requireODFPackage(t, buf.Bytes(), mimeODT)
			want, _, _ := zipEntries(t, source)
			for name, wantBody := range want {
				assert.Equalf(t, string(wantBody), string(got[name]),
					"entry %q changed although nothing was translated", name)
			}
		})
	}
}

// A run that does translate hands back a package too — the translation lands
// inside the element it came from, and every other byte of the stream is where
// it was.
func TestRegistryPair_TranslatedPackageStaysAPackage(t *testing.T) {
	ctx := t.Context()
	source := makeODFZipWithStyles(mimeODT,
		simpleODTContent("File", "Edit"),
		masterPageStyles("Help"))

	reader, writer, _ := registryPair(t)

	doc := testutil.RawDocFromReader(bytes.NewReader(source), "test.odt", model.LocaleEnglish)
	require.NoError(t, reader.Open(ctx, doc))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	require.NoError(t, reader.Close())

	const target = model.LocaleID("nb-NO")
	translations := map[string]string{"File": "Fil", "Edit": "Rediger", "Help": "Hjelp"}
	translated := 0
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b, ok := p.Resource.(*model.Block)
		if !ok {
			continue
		}
		if tr, ok := translations[b.SourceText()]; ok {
			b.SetTargetText(target, tr)
			translated++
		}
	}
	require.Equal(t, len(translations), translated,
		"every paragraph of content.xml and styles.xml should be extracted exactly once")

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	writer.SetLocale(target)
	writer.(format.OriginalContentSetter).SetOriginalContent(source)
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	require.NoError(t, writer.Close())

	entries := requireODFPackage(t, buf.Bytes(), mimeODT)
	assert.Contains(t, string(entries["content.xml"]), "<text:p>Fil</text:p>")
	assert.Contains(t, string(entries["content.xml"]), "<text:p>Rediger</text:p>")
	assert.Contains(t, string(entries["styles.xml"]), "<text:p>Hjelp</text:p>")
	assert.NotContains(t, string(entries["content.xml"]), ">File<")

	// The output is a document the reader reads back as the same document.
	reader2, _, _ := registryPair(t)
	doc2 := testutil.RawDocFromReader(bytes.NewReader(buf.Bytes()), "test.odt", model.LocaleEnglish)
	require.NoError(t, reader2.Open(ctx, doc2))
	blocks := testutil.CollectBlocks(t, reader2.Read(ctx))
	require.NoError(t, reader2.Close())
	texts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		texts = append(texts, b.SourceText())
	}
	assert.Equal(t, []string{"Fil", "Rediger", "Hjelp"}, texts)
}

// Extraction follows ODF's own rules whether or not a resolver is present: the
// paragraphs of content.xml and styles.xml, and nothing else the package
// happens to spell as XML text.
func TestRegistryPair_ExtractionFollowsODFRules(t *testing.T) {
	ctx := t.Context()
	source := makeODFZipWithStyles(mimeODT,
		simpleODTContent("Hello", "World"),
		masterPageStyles("Header text"))

	read := func(r format.DataFormatReader) []string {
		t.Helper()
		doc := testutil.RawDocFromReader(bytes.NewReader(source), "test.odt", model.LocaleEnglish)
		require.NoError(t, r.Open(ctx, doc))
		blocks := testutil.CollectBlocks(t, r.Read(ctx))
		require.NoError(t, r.Close())
		out := make([]string, 0, len(blocks))
		for _, b := range blocks {
			out = append(out, b.SourceText())
		}
		return out
	}

	fromRegistry, _, _ := registryPair(t)
	want := []string{"Hello", "World", "Header text"}
	assert.Equal(t, want, read(fromRegistry))

	// Style names, page-layout names and the rest of the package's XML text are
	// not translatable content, so nothing but the paragraphs is extracted.
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	direct, err := reg.NewReader("odf")
	require.NoError(t, err)
	assert.Equal(t, want, read(direct))
}
