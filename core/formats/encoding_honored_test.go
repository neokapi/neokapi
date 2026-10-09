package formats

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/encoding"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
)

// A declared document encoding selects the codec the engine reads and writes
// by. format.OpenDocument decodes the bytes to UTF-8 before a reader sees them,
// and the writer encodes its text back in the same charset, so a legacy
// single-byte file reaches the content model as valid UTF-8 and leaves as the
// bytes it arrived in. Tracked in https://github.com/neokapi/neokapi/issues/1714.

// encodingProbe pairs a charset with a word it can carry; the probe word is
// what a decoded block must read as.
type encodingProbe struct {
	charset string
	word    string
}

func encodingProbes() []encodingProbe {
	return []encodingProbe{
		{charset: "ISO-8859-1", word: "Grüße"},
		{charset: "windows-1252", word: "Grüße €"},
		{charset: "Shift_JIS", word: "こんにちは"},
	}
}

// encodeIn transcodes UTF-8 text to the named charset.
func encodeIn(t *testing.T, text, charset string) []byte {
	t.Helper()
	out, err := encoding.NewEncoderManager().Encode(text, charset)
	require.NoError(t, err)
	return out
}

// readDeclared opens src through the ingestion seam, declared as charset, and
// returns the root layer, the block texts and the parts the reader streamed.
func readDeclared(t *testing.T, reader format.DataFormatReader, src []byte, charset string) (*model.Layer, []string, []*model.Part) {
	t.Helper()
	doc := &model.RawDocument{
		URI:          "probe",
		SourceLocale: model.LocaleID("de"),
		Encoding:     charset,
		Reader:       io.NopCloser(bytes.NewReader(src)),
	}
	require.NoError(t, format.OpenDocument(context.Background(), reader, doc))
	var layer *model.Layer
	var texts []string
	var parts []*model.Part
	for pr := range reader.Read(context.Background()) {
		require.NoError(t, pr.Error)
		parts = append(parts, pr.Part)
		switch pr.Part.Type {
		case model.PartLayerStart:
			if layer == nil {
				layer, _ = pr.Part.Resource.(*model.Layer)
			}
		case model.PartBlock:
			if b, ok := pr.Part.Resource.(*model.Block); ok {
				texts = append(texts, model.RenderRunsWithData(b.SourceRuns()))
			}
		}
	}
	require.NotNil(t, layer)
	return layer, texts, parts
}

// roundTripDeclared reads src declared as charset and writes it back untouched
// in the same charset, returning the bytes the writer produced.
func roundTripDeclared(t *testing.T, reg *registry.FormatRegistry, id string, src []byte, charset string) (texts []string, out []byte) {
	t.Helper()
	reader, err := reg.NewReader(registry.FormatID(id))
	require.NoError(t, err)
	writer, err := reg.NewWriter(registry.FormatID(id))
	require.NoError(t, err)
	store, err := format.NewWiredSkeleton(reader, writer)
	require.NoError(t, err)
	if store != nil {
		defer store.Close()
	}
	_, texts, parts := readDeclared(t, reader, src, charset)
	require.NoError(t, reader.Close())
	out, err = spec.WritePartsIn(writer, parts, src, charset)
	require.NoError(t, err)
	return texts, out
}

// TestDeclaredEncodingSelectsTheCodec reads a legacy single-byte and a
// double-byte document through a line-oriented format and an XML one. The
// declaration decodes the bytes on the way in and encodes them on the way out.
func TestDeclaredEncodingSelectsTheCodec(t *testing.T) {
	reg := registry.NewFormatRegistry()
	RegisterAll(reg)

	families := []struct {
		id     string
		source func(word, charset string) string
	}{
		{id: "properties", source: func(word, _ string) string {
			return "greeting=" + word + "\n"
		}},
		{id: "xml", source: func(word, charset string) string {
			return `<?xml version="1.0" encoding="` + charset + `"?>` + "\n<root><p>" + word + "</p></root>\n"
		}},
	}
	for _, fam := range families {
		for _, probe := range encodingProbes() {
			t.Run(fam.id+"/"+probe.charset, func(t *testing.T) {
				text := fam.source(probe.word, probe.charset)
				src := encodeIn(t, text, probe.charset)
				require.NotEqual(t, []byte(text), src, "the probe must not be ASCII-only")

				reader, err := reg.NewReader(registry.FormatID(fam.id))
				require.NoError(t, err)
				layer, texts, _ := readDeclared(t, reader, src, probe.charset)
				require.NoError(t, reader.Close())
				require.Equal(t, []string{probe.word}, texts, "the declaration decodes the bytes")
				require.Equal(t, probe.charset, layer.Encoding, "the layer records the encoding the document was read as")

				_, out := roundTripDeclared(t, reg, fam.id, src, probe.charset)
				require.Equal(t, src, out, "an untouched document is written back in the bytes it arrived in")
			})
		}
	}
}

// TestByteOrderMarkWinsOverTheDeclaration: a mark is evidence in the file. A
// UTF-8 mark keeps the bytes UTF-8 whatever was declared, on the way in and on
// the way out; a UTF-16 mark selects that decoder.
func TestByteOrderMarkWinsOverTheDeclaration(t *testing.T) {
	reg := registry.NewFormatRegistry()
	RegisterAll(reg)

	t.Run("utf-8 mark declared ISO-8859-1", func(t *testing.T) {
		src := []byte(format.UTF8BOM + "greeting=Grüße\n")
		reader, err := reg.NewReader("properties")
		require.NoError(t, err)
		layer, texts, _ := readDeclared(t, reader, src, "ISO-8859-1")
		require.NoError(t, reader.Close())
		require.Equal(t, []string{"Grüße"}, texts, "the marked file is read as UTF-8")
		require.Equal(t, "UTF-8", layer.Encoding)

		_, out := roundTripDeclared(t, reg, "properties", src, "ISO-8859-1")
		require.Equal(t, src, out, "the mark keeps the output UTF-8: ISO-8859-1 cannot carry it")
	})

	t.Run("utf-16 mark declared ISO-8859-1", func(t *testing.T) {
		src := encodeIn(t, format.UTF8BOM+"Grüße\n", "UTF-16LE")
		require.Equal(t, []byte{0xFF, 0xFE}, src[:2])
		reader, err := reg.NewReader("plaintext")
		require.NoError(t, err)
		layer, texts, _ := readDeclared(t, reader, src, "ISO-8859-1")
		require.NoError(t, reader.Close())
		require.Equal(t, []string{"Grüße"}, texts)
		require.Equal(t, "UTF-16LE", layer.Encoding)

		_, out := roundTripDeclared(t, reg, "plaintext", src, "UTF-16LE")
		require.Equal(t, src, out, "a UTF-16 document written back as UTF-16 keeps its bytes")
	})
}

// TestUndeclaredDocumentStaysUTF8: with neither a mark nor a declaration the
// bytes are UTF-8, as before, and nothing marks the document settled.
func TestUndeclaredDocumentStaysUTF8(t *testing.T) {
	reg := registry.NewFormatRegistry()
	RegisterAll(reg)
	for _, declared := range []string{"", "UTF-8", "utf8"} {
		t.Run("declared "+declared, func(t *testing.T) {
			src := []byte("greeting=Grüße\n")
			reader, err := reg.NewReader("properties")
			require.NoError(t, err)
			doc := &model.RawDocument{Encoding: declared, Reader: io.NopCloser(bytes.NewReader(src))}
			require.NoError(t, format.OpenDocument(context.Background(), reader, doc))
			require.False(t, doc.EncodingSettled)
			require.Equal(t, declared, doc.Encoding, "an unsettled declaration is left as it was")
			var texts []string
			for pr := range reader.Read(context.Background()) {
				require.NoError(t, pr.Error)
				if b, ok := pr.Part.Resource.(*model.Block); ok {
					texts = append(texts, model.RenderRunsWithData(b.SourceRuns()))
				}
			}
			require.NoError(t, reader.Close())
			require.Equal(t, []string{"Grüße"}, texts)
		})
	}
}

// TestDeclaredEncodingRoundTrip is the encoding axis of the round-trip
// conformance sweep: every text format that reads and writes takes an
// untouched document in a legacy charset and writes it back byte for byte.
func TestDeclaredEncodingRoundTrip(t *testing.T) {
	reg := registry.NewFormatRegistry()
	RegisterAll(reg)

	for _, tc := range bomSweepFormats() {
		for _, probe := range encodingProbes() {
			t.Run(tc.id+"/"+probe.charset, func(t *testing.T) {
				text := strings.ReplaceAll(tc.source, "Hello", probe.word)
				text = strings.ReplaceAll(text, `encoding="utf-8"`, `encoding="`+probe.charset+`"`)
				text = strings.ReplaceAll(text, `encoding="UTF-8"`, `encoding="`+probe.charset+`"`)
				src := encodeIn(t, text, probe.charset)
				texts, out := roundTripDeclared(t, reg, tc.id, src, probe.charset)
				require.Contains(t, strings.Join(texts, "\n"), probe.word, "the declaration decodes the bytes")
				if !bytes.Equal(out, src) {
					t.Fatalf("%s: untouched round-trip in %s changed the document\nwant %q\ngot  %q",
						tc.id, probe.charset, string(src), string(out))
				}
			})
		}
	}
}

// TestBinaryFormatsKeepTheirBytes: a declaration is a charset for text; a
// container, an image or a compiled catalog has none, and OpenDocument hands
// such a reader the bytes as they are.
func TestBinaryFormatsKeepTheirBytes(t *testing.T) {
	reg := registry.NewFormatRegistry()
	RegisterAll(reg)
	for _, id := range []string{"openxml", "odf", "epub", "archive", "image", "audio", "video", "mo"} {
		t.Run(id, func(t *testing.T) {
			reader, err := reg.NewReader(registry.FormatID(id))
			require.NoError(t, err)
			require.True(t, reader.Signature().Binary, "%s declares Binary", id)
			if w, err := reg.NewWriter(registry.FormatID(id)); err == nil {
				bw, ok := w.(interface{ EffectiveOutputOptions() format.OutputOptions })
				require.True(t, ok)
				w.SetEncoding("ISO-8859-1")
				require.True(t, bw.EffectiveOutputOptions().IsZero(), "%s writer applies no charset", id)
			}
		})
	}
}
