package archive_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildRegistry returns a registry with all built-in formats registered, so the
// archive reader/writer can detect and resolve real sub-formats exactly as in
// production.
func buildRegistry(t *testing.T) *registry.FormatRegistry {
	t.Helper()
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	return reg
}

// entry is a single file to place in a test archive.
type entry struct {
	name string
	data []byte
}

func makeZip(t *testing.T, entries []entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		w, err := zw.Create(e.name)
		require.NoError(t, err)
		_, err = w.Write(e.data)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func makeTar(t *testing.T, entries []entry, gzipped bool) []byte {
	t.Helper()
	var buf bytes.Buffer
	var out io.Writer = &buf
	var gz *gzip.Writer
	if gzipped {
		gz = gzip.NewWriter(&buf)
		out = gz
	}
	tw := tar.NewWriter(out)
	for _, e := range entries {
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name: e.name,
			Mode: 0o644,
			Size: int64(len(e.data)),
		}))
		_, err := tw.Write(e.data)
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	if gz != nil {
		require.NoError(t, gz.Close())
	}
	return buf.Bytes()
}

// readArchive runs the archive reader over data and returns the part stream.
func readArchive(t *testing.T, reg *registry.FormatRegistry, uri string, data []byte) []*model.Part {
	t.Helper()
	reader, err := reg.NewReader("archive")
	require.NoError(t, err)
	doc := &model.RawDocument{URI: uri, Reader: io.NopCloser(bytes.NewReader(data))}
	require.NoError(t, reader.Open(context.Background(), doc))
	var parts []*model.Part
	for pr := range reader.Read(context.Background()) {
		require.NoError(t, pr.Error)
		parts = append(parts, pr.Part)
	}
	require.NoError(t, reader.Close())
	return parts
}

// writeArchive runs the archive writer over parts and returns the rebuilt bytes.
func writeArchive(t *testing.T, reg *registry.FormatRegistry, original []byte, parts []*model.Part, targetLang string) []byte {
	t.Helper()
	writer, err := reg.NewWriter("archive")
	require.NoError(t, err)
	if ocs, ok := writer.(interface{ SetOriginalContent([]byte) }); ok {
		ocs.SetOriginalContent(original)
	}
	var out bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&out))
	if targetLang != "" {
		writer.SetLocale(model.LocaleID(targetLang))
	}
	ch := make(chan *model.Part, len(parts))
	for _, p := range parts {
		ch <- p
	}
	close(ch)
	require.NoError(t, writer.Write(context.Background(), ch))
	require.NoError(t, writer.Close())
	return out.Bytes()
}

// zipEntries reads a zip into a name→bytes map.
func zipEntries(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	out := make(map[string][]byte)
	for _, f := range zr.File {
		rc, err := f.Open()
		require.NoError(t, err)
		b, err := io.ReadAll(rc)
		rc.Close()
		require.NoError(t, err)
		out[f.Name] = b
	}
	return out
}

func tarEntries(t *testing.T, data []byte, gzipped bool) map[string][]byte {
	t.Helper()
	var src io.Reader = bytes.NewReader(data)
	if gzipped {
		gz, err := gzip.NewReader(bytes.NewReader(data))
		require.NoError(t, err)
		defer gz.Close()
		src = gz
	}
	tr := tar.NewReader(src)
	out := make(map[string][]byte)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		b, err := io.ReadAll(tr)
		require.NoError(t, err)
		out[hdr.Name] = b
	}
	return out
}

// blockTexts returns the source text of every block in the stream.
func blockTexts(parts []*model.Part) []string {
	var out []string
	for _, p := range parts {
		if p.Type == model.PartBlock {
			if b, ok := p.Resource.(*model.Block); ok {
				out = append(out, b.SourceText())
			}
		}
	}
	return out
}

func TestDetectsPlainZipAsArchive(t *testing.T) {
	reg := buildRegistry(t)
	data := makeZip(t, []entry{
		{"messages.json", []byte(`{"greeting":"Hello"}`)},
		{"notes.txt", []byte("a note")},
	})
	name, err := reg.Detector().Detect("bundle.zip", bytes.NewReader(data), "")
	require.NoError(t, err)
	assert.Equal(t, "archive", name)
}

func TestDetectsTarByExtension(t *testing.T) {
	reg := buildRegistry(t)
	data := makeTar(t, []entry{{"a.json", []byte(`{"k":"v"}`)}}, false)
	name, err := reg.Detector().Detect("bundle.tar", bytes.NewReader(data), "")
	require.NoError(t, err)
	assert.Equal(t, "archive", name)
}

func TestEpubSniffWinsOverArchive(t *testing.T) {
	// A ZIP whose leading bytes carry the EPUB mimetype marker must still be
	// detected as epub, not stolen by the generic archive format.
	reg := buildRegistry(t)
	data := makeZip(t, []entry{
		{"mimetype", []byte("application/epub+zip")},
		{"content.opf", []byte("<package/>")},
	})
	name, err := reg.Detector().Detect("book.epub", bytes.NewReader(data), "")
	require.NoError(t, err)
	assert.Equal(t, "epub", name)
}

func TestZipReadEmitsChildLayersAndData(t *testing.T) {
	reg := buildRegistry(t)
	data := makeZip(t, []entry{
		{"locales/en.json", []byte(`{"greeting":"Hello","farewell":"Bye"}`)},
		{"README.md", []byte("# Title\n\nSome prose.\n")},
		{"logo.png", []byte("\x89PNG\r\n\x1a\nbinary-bytes")},
		{"data.bin", []byte("\x00\x01\x02opaque")},
	})
	parts := readArchive(t, reg, "bundle.zip", data)

	// Child layers for the JSON + Markdown entries; Data for the binaries.
	var childFormats, dataEntries []string
	for _, p := range parts {
		switch p.Type {
		case model.PartLayerStart:
			if l, ok := p.Resource.(*model.Layer); ok && l.Properties["subfilter.source"] == "archive" {
				childFormats = append(childFormats, l.Format)
			}
		case model.PartData:
			if d, ok := p.Resource.(*model.Data); ok {
				dataEntries = append(dataEntries, d.Name)
			}
		}
	}
	assert.ElementsMatch(t, []string{"json", "markdown"}, childFormats)
	assert.ElementsMatch(t, []string{"logo.png", "data.bin"}, dataEntries)

	// The JSON values are surfaced as translatable blocks.
	assert.Contains(t, blockTexts(parts), "Hello")
	assert.Contains(t, blockTexts(parts), "Bye")
}

func TestZipRoundTripPreservesUntouchedEntries(t *testing.T) {
	reg := buildRegistry(t)
	jsonSrc := []byte(`{"greeting":"Hello","count":3}`)
	pngSrc := []byte("\x89PNG\r\n\x1a\nbinary-bytes-1234")
	binSrc := []byte("\x00\x01\x02opaque payload")
	data := makeZip(t, []entry{
		{"locales/en.json", jsonSrc},
		{"logo.png", pngSrc},
		{"data.bin", binSrc},
	})

	parts := readArchive(t, reg, "bundle.zip", data)
	out := writeArchive(t, reg, data, parts, "")

	got := zipEntries(t, out)
	// Binary entries are byte-for-byte identical.
	assert.Equal(t, pngSrc, got["logo.png"])
	assert.Equal(t, binSrc, got["data.bin"])
	// JSON reconstructs byte-for-byte from json.original when untranslated.
	assert.Equal(t, jsonSrc, got["locales/en.json"])
}

func TestZipRoundTripAppliesTranslation(t *testing.T) {
	reg := buildRegistry(t)
	data := makeZip(t, []entry{
		{"en.json", []byte(`{"greeting":"Hello"}`)},
	})
	parts := readArchive(t, reg, "bundle.zip", data)

	// Simulate a translation: every block gets an uppercased French target.
	for _, p := range parts {
		if p.Type == model.PartBlock {
			if b, ok := p.Resource.(*model.Block); ok {
				b.SetTargetText("fr", strings.ToUpper(b.SourceText()))
			}
		}
	}

	out := writeArchive(t, reg, data, parts, "fr")
	got := zipEntries(t, out)
	assert.Contains(t, string(got["en.json"]), "HELLO")
	assert.Contains(t, string(got["en.json"]), "greeting")
}

func TestTarRoundTrip(t *testing.T) {
	for _, gz := range []bool{false, true} {
		name := "tar"
		if gz {
			name = "tar.gz"
		}
		t.Run(name, func(t *testing.T) {
			reg := buildRegistry(t)
			jsonSrc := []byte(`{"greeting":"Hello"}`)
			binSrc := []byte("\x00\x01opaque")
			data := makeTar(t, []entry{
				{"en.json", jsonSrc},
				{"data.bin", binSrc},
			}, gz)

			uri := "bundle." + name
			parts := readArchive(t, reg, uri, data)
			assert.Contains(t, blockTexts(parts), "Hello")

			out := writeArchive(t, reg, data, parts, "")
			got := tarEntries(t, out, gz)
			assert.Equal(t, binSrc, got["data.bin"])
			assert.Equal(t, jsonSrc, got["en.json"])
		})
	}
}

func TestNestedArchivePassesThrough(t *testing.T) {
	reg := buildRegistry(t)
	inner := makeZip(t, []entry{{"inner.json", []byte(`{"k":"v"}`)}})
	data := makeZip(t, []entry{
		{"nested.zip", inner},
		{"top.json", []byte(`{"greeting":"Hi"}`)},
	})
	parts := readArchive(t, reg, "bundle.zip", data)

	// The nested archive must be a Data part (not recursed), the top JSON a child.
	var dataNames, childNames []string
	for _, p := range parts {
		switch p.Type {
		case model.PartData:
			if d, ok := p.Resource.(*model.Data); ok {
				dataNames = append(dataNames, d.Name)
			}
		case model.PartLayerStart:
			if l, ok := p.Resource.(*model.Layer); ok && l.Properties["subfilter.source"] == "archive" {
				childNames = append(childNames, l.Name)
			}
		}
	}
	assert.Contains(t, dataNames, "nested.zip")
	assert.Contains(t, childNames, "top.json")

	// And the nested archive survives the round-trip byte-for-byte.
	out := writeArchive(t, reg, data, parts, "")
	assert.Equal(t, inner, zipEntries(t, out)["nested.zip"])
}

func TestExcludeGlobPassesEntryThrough(t *testing.T) {
	reg := buildRegistry(t)
	reader, err := reg.NewReader("archive")
	require.NoError(t, err)
	cfg := reader.Config()
	require.NotNil(t, cfg)
	require.NoError(t, cfg.ApplyMap(map[string]any{"exclude": []any{"vendor/**"}}))

	data := makeZip(t, []entry{
		{"app.json", []byte(`{"greeting":"Hi"}`)},
		{"vendor/lib.json", []byte(`{"x":"y"}`)},
	})
	doc := &model.RawDocument{URI: "bundle.zip", Reader: io.NopCloser(bytes.NewReader(data))}
	require.NoError(t, reader.Open(context.Background(), doc))

	var childNames, dataNames []string
	for pr := range reader.Read(context.Background()) {
		require.NoError(t, pr.Error)
		switch pr.Part.Type {
		case model.PartLayerStart:
			if l, ok := pr.Part.Resource.(*model.Layer); ok && l.Properties["subfilter.source"] == "archive" {
				childNames = append(childNames, l.Name)
			}
		case model.PartData:
			if d, ok := pr.Part.Resource.(*model.Data); ok {
				dataNames = append(dataNames, d.Name)
			}
		}
	}
	require.NoError(t, reader.Close())

	assert.Contains(t, childNames, "app.json")
	assert.Contains(t, dataNames, "vendor/lib.json")
	assert.NotContains(t, childNames, "vendor/lib.json")
}
