package formats_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/config"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterAllReaders(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	expectedFormats := []registry.FormatID{
		"plaintext", "html", "xml", "xliff", "xliff2",
		"yaml", "json", "klf", "po", "properties",
		"markdown", "csv", "tsv", "srt", "ttml", "vtt", "tmx", "openxml",
		"mosestext", "dtd", "ts", "wiki", "tex",
		"regex", "doxygen", "messageformat", "phpcontent",
		"icml", "idml", "fixedwidth",
		"transtable", "paraplaintext", "splicedlines", "versifiedtext", "vignette",
		"odf", "epub", "rtf", "mif", "ttx", "txml", "doclang", "docling", "xcstrings", "arb", "resx",
		"androidxml", "applestrings", "i18next", "designtokens", "mdx", "asciidoc",
		// "image" (PNG/JPEG) is read-only: text + structure come from OCR via the
		// kapi-vision plugin when installed, else just the image as Media.
		"image",
		// "audio"/"video" are read-only: speech → text via the kapi-asr plugin
		// (audio + video audio track) and on-screen text via kapi-vision OCR
		// (video frames). Demux is via the kapi-av plugin / ffmpeg.
		"audio", "video",
		// Note: "pdf" is absent on native builds — it is read out-of-core by
		// the kapi-pdfium plugin (registered at runtime), and only registered
		// in-core on js builds (the PDFium-wasm reader). See register_pdf_*.go.
		//
		// Declarative pseudo-format: registered so it shows up in the
		// format list / UI. Open() intentionally errors — actual exec
		// extraction runs via `kapi extract -p` (Framework AD-002).
		"exec", "mo",
		// Archive container (ZIP/TAR/TAR.GZ) — a folder of sub-documents.
		"archive",
	}

	for _, name := range expectedFormats {
		assert.True(t, reg.HasReader(name), "reader not registered: %s", name)
	}
	assert.Len(t, reg.ReaderNames(), len(expectedFormats))
}

func TestRegisterAllWriters(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	expectedFormats := []registry.FormatID{
		"plaintext", "html", "xml", "xliff", "xliff2",
		"yaml", "json", "klf", "po", "mo", "properties",
		"markdown", "csv", "tsv", "srt", "ttml", "vtt", "tmx", "openxml",
		"mosestext", "dtd", "ts", "wiki", "tex",
		"regex", "doxygen", "messageformat", "phpcontent",
		"icml", "idml", "fixedwidth",
		"transtable", "paraplaintext", "splicedlines", "versifiedtext", "vignette",
		"odf", "epub", "rtf", "mif", "ttx", "txml", "doclang", "xcstrings", "arb", "resx",
		"androidxml", "applestrings", "i18next", "designtokens", "mdx", "asciidoc",
		// "image" has a writer: it emits the (possibly localized, e.g.
		// pseudo-localized) image bytes — the whole-image localization sink.
		"image",
		// "audio"/"video" have passthrough writers: the whole-asset replace-asset
		// sink (AD-030) — a per-locale media file the user/connector supplies is
		// written out as-is. Extraction (ASR/OCR) Blocks carry no replacement
		// bytes and pass through.
		"audio", "video",
		// Archive container (ZIP/TAR/TAR.GZ) — reconstructs the container,
		// re-serialising sub-filtered entries and copying the rest byte-for-byte.
		"archive",
		// Note: "docling" is intentionally absent — it is read-only (extraction
		// only), so it registers a reader but no writer. "pdf" is absent on
		// native builds entirely (read out-of-core by the kapi-pdfium plugin).
	}

	for _, name := range expectedFormats {
		assert.True(t, reg.HasWriter(name), "writer not registered: %s", name)
	}
	assert.Len(t, reg.WriterNames(), len(expectedFormats))
	assert.False(t, reg.HasWriter("docling"), "docling must remain read-only (no writer)")
}

// TestKLFFormatIDAndJSXAlias asserts the user-facing id is "klf" while
// the legacy "jsx" id keeps resolving as a back-compat alias (issue
// #717). `kapi formats` and detection surface "klf"; `--format jsx`
// still works.
func TestKLFFormatIDAndJSXAlias(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	// Canonical id is registered and listed.
	require.True(t, reg.HasReader("klf"), "klf reader must be registered")
	require.True(t, reg.HasWriter("klf"), "klf writer must be registered")

	listed := map[registry.FormatID]bool{}
	for _, name := range reg.ReaderNames() {
		listed[name] = true
	}
	assert.True(t, listed["klf"], "klf must be in ReaderNames")
	assert.False(t, listed["jsx"], "jsx must NOT be in ReaderNames — it is a name-only alias")

	// `kapi formats` lists klf, never jsx.
	var sawKLF, sawJSX bool
	for _, info := range reg.FormatInfos() {
		switch info.Name {
		case "klf":
			sawKLF = true
		case "jsx":
			sawJSX = true
		}
	}
	assert.True(t, sawKLF, "FormatInfos must include klf")
	assert.False(t, sawJSX, "FormatInfos must NOT include jsx")

	// The alias resolves to the klf reader/writer.
	r, err := reg.NewReader("jsx")
	require.NoError(t, err, "--format jsx must still resolve")
	assert.Equal(t, "klf", r.Name())
	w, err := reg.NewWriter("jsx")
	require.NoError(t, err)
	assert.Equal(t, "klf", w.Name())

	// Detection by extension / MIME returns the canonical id.
	byExt, err := reg.DetectByExtension(".klf")
	require.NoError(t, err)
	assert.Equal(t, registry.FormatID("klf"), byExt)
	assert.Equal(t, registry.FormatID("klf"),
		reg.ResolveFormat("application/vnd.neokapi.klf+json"))
}

func TestRegistryCreateInstances(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	for _, name := range reg.ReaderNames() {
		reader, err := reg.NewReader(name)
		require.NoError(t, err, "failed to create reader: %s", name)
		assert.Equal(t, string(name), reader.Name())
	}

	for _, name := range reg.WriterNames() {
		writer, err := reg.NewWriter(name)
		require.NoError(t, err, "failed to create writer: %s", name)
		assert.Equal(t, string(name), writer.Name())
	}
}

func TestCollectNativeDecoders(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	configReg := config.NewRegistry()
	reg.CollectNativeDecoders(configReg)

	// All native formats should have decoders registered
	expectedFormats := []string{
		"plaintext", "html", "xml", "xliff", "xliff2",
		"yaml", "json", "po", "properties",
		"markdown", "csv", "tsv", "srt", "ttml", "vtt", "tmx",
		"ts", "fixedwidth", "phpcontent", "asciidoc",
	}
	for _, name := range expectedFormats {
		kind := config.FormatConfigKind(name)
		assert.True(t, configReg.Has(kind), "decoder not registered for %s", kind)
	}

	// HTML and JSON should use their declared kind (same pattern)
	assert.True(t, configReg.Has(config.FormatConfigKind("html")))
	assert.True(t, configReg.Has(config.FormatConfigKind("json")))

	// Test that a decoder can decode a spec
	env := &config.Envelope{
		APIVersion: "v1",
		Kind:       config.FormatConfigKind("json"),
		Spec:       map[string]any{"extractAllPairs": false, "useFullKeyPath": true},
	}
	result, err := configReg.Decode(env)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestFormatDetection(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	detector := reg.Detector()

	tests := []struct {
		mime      string
		expected  string
		extension string
	}{
		{"text/html", "html", ""},
		{"application/json", "json", ""},
		{"application/yaml", "yaml", ""},
		{"text/xml", "xml", ""},
		{"", "yaml", ".yaml"},
		{"", "yaml", ".yml"},
		{"", "json", ".json"},
		{"", "properties", ".properties"},
		{"", "po", ".po"},
		{"", "plaintext", ".txt"},
	}

	for _, tt := range tests {
		var name string
		var err error
		if tt.mime != "" {
			name, err = detector.DetectByMIME(tt.mime)
		} else {
			name, err = detector.DetectByExtension(tt.extension)
		}
		require.NoError(t, err, "detection failed for mime=%q ext=%q", tt.mime, tt.extension)
		assert.Equal(t, tt.expected, name, "wrong format for mime=%q ext=%q", tt.mime, tt.extension)
	}
}

// ftypBoxBytes builds a 32-byte ISOBMFF header with the given major brand.
func ftypBoxBytes(brand string) []byte {
	b := make([]byte, 32)
	b[3] = 0x20
	copy(b[4:8], "ftyp")
	copy(b[8:12], brand)
	return b
}

// TestImageFormatDetection covers the widened raster set: every new extension
// and MIME resolves to "image", and content detection routes WebP and the
// ISOBMFF still images (HEIC/AVIF) to "image" via the Sniff hook without
// stealing the shared RIFF/ftyp containers from audio and video.
func TestImageFormatDetection(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	detector := reg.Detector()

	for _, ext := range []string{
		".png", ".jpg", ".jpeg", ".gif", ".bmp", ".tif", ".tiff",
		".webp", ".heic", ".heif", ".avif",
	} {
		name, err := detector.DetectByExtension(ext)
		require.NoError(t, err, "ext %s", ext)
		assert.Equal(t, "image", name, "ext %s should detect as image", ext)
	}

	for _, mime := range []string{
		"image/png", "image/jpeg", "image/gif", "image/bmp", "image/tiff",
		"image/webp", "image/heic", "image/heif", "image/avif",
	} {
		name, err := detector.DetectByMIME(mime)
		require.NoError(t, err, "mime %s", mime)
		assert.Equal(t, "image", name, "mime %s should detect as image", mime)
	}

	// Content detection: WebP (RIFF container, "WEBP" at offset 8) and the
	// ISOBMFF still images resolve to image via Sniff.
	webp := append([]byte("RIFF\x10\x00\x00\x00WEBPVP8 "), make([]byte, 16)...)
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"webp", webp},
		{"heic", ftypBoxBytes("heic")},
		{"avif", ftypBoxBytes("avif")},
		{"gif", []byte("GIF89a\x01\x00\x01\x00\x00\x00\x00")},
	} {
		name, err := detector.DetectByContent(bytes.NewReader(tc.data))
		require.NoError(t, err, "content %s", tc.name)
		assert.Equal(t, "image", name, "%s content should detect as image", tc.name)
	}

	// Collision guards: a real WAV (RIFF…WAVE) stays audio, and an MP4 ftyp box
	// (brand isom) is not claimed by image.
	wav := append([]byte("RIFF\x24\x00\x00\x00WAVEfmt "), make([]byte, 16)...)
	name, err := detector.DetectByContent(bytes.NewReader(wav))
	require.NoError(t, err)
	assert.Equal(t, "audio", name, "WAV must not be claimed by image")

	// An MP4 ftyp box (brand isom) must not be claimed by image; whatever it
	// resolves to (video, or nothing), it is not "image".
	if mp4, derr := detector.DetectByContent(bytes.NewReader(ftypBoxBytes("isom"))); derr == nil {
		assert.NotEqual(t, "image", mp4, "an MP4 ftyp box must not detect as image")
	}
}

// TestDoclingContentDetection asserts the DoclingDocument JSON reader wins by
// content sniff over the generic JSON reader, while a plain JSON object still
// resolves to "json" (docling's below-default priority loses the fallback).
func TestDoclingContentDetection(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	detector := reg.Detector()

	docling := `{"schema_name": "DoclingDocument", "version": "1.2.0", "body": {"children": []}, "texts": []}`
	name, err := detector.DetectByContent(strings.NewReader(docling))
	require.NoError(t, err)
	assert.Equal(t, "docling", name, "DoclingDocument JSON should detect as docling")

	plain := `{"greeting": "hello", "items": [1, 2, 3], "nested": {"a": true}}`
	name, err = detector.DetectByContent(strings.NewReader(plain))
	require.NoError(t, err)
	assert.Equal(t, "json", name, "plain JSON must not be claimed by docling")
}

// TestDoclangContentDetection asserts DocLang XML wins by content sniff over the
// generic XML reader (a .dclg.xml file's extension is just ".xml"), while a
// plain .xml document still resolves to "xml".
func TestDoclangContentDetection(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	detector := reg.Detector()

	doclang := `<?xml version="1.0"?><doclang xmlns="https://www.doclang.ai/ns/v0" version="0.6"><heading level="1">Hi</heading></doclang>`
	name, err := detector.DetectByContent(strings.NewReader(doclang))
	require.NoError(t, err)
	assert.Equal(t, "doclang", name, "DocLang XML should detect as doclang")

	plain := `<?xml version="1.0"?><root><item>value</item></root>`
	name, err = detector.DetectByContent(strings.NewReader(plain))
	require.NoError(t, err)
	assert.Equal(t, "xml", name, "plain XML must not be claimed by doclang")

	// A .dclg.xml path (filepath.Ext == ".xml") resolves via the shared
	// extension + content sniff.
	dir := t.TempDir()
	p := filepath.Join(dir, "report.dclg.xml")
	require.NoError(t, os.WriteFile(p, []byte(doclang), 0o644))
	det, err := reg.DetectFile(p, nil)
	require.NoError(t, err)
	assert.Equal(t, registry.FormatID("doclang"), det, ".dclg.xml file should detect as doclang")
}

func TestEndToEndPlaintextRoundTrip(t *testing.T) {
	ctx := t.Context()
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	input := "Hello World\nThis is a test"

	reader, err := reg.NewReader("plaintext")
	require.NoError(t, err)
	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer, err := reg.NewWriter("plaintext")
	require.NoError(t, err)
	_ = writer.SetOutputWriter(&buf)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	assert.Equal(t, input, buf.String())
}

func TestEndToEndPropertiesRoundTrip(t *testing.T) {
	ctx := t.Context()
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	input := "greeting=Hello\nfarewell=Goodbye"

	reader, err := reg.NewReader("properties")
	require.NoError(t, err)
	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer, err := reg.NewWriter("properties")
	require.NoError(t, err)
	_ = writer.SetOutputWriter(&buf)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	assert.Equal(t, input, buf.String())
}

func TestEndToEndWithTranslation(t *testing.T) {
	ctx := t.Context()
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	input := "greeting=Hello\nfarewell=Goodbye"

	reader, err := reg.NewReader("properties")
	require.NoError(t, err)
	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Simulate translation by setting target texts
	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			switch block.SourceText() {
			case "Hello":
				block.SetTargetText(model.LocaleFrench, "Bonjour")
			case "Goodbye":
				block.SetTargetText(model.LocaleFrench, "Au revoir")
			}
		}
	}

	var buf bytes.Buffer
	writer, err := reg.NewWriter("properties")
	require.NoError(t, err)
	_ = writer.SetOutputWriter(&buf)
	writer.SetLocale(model.LocaleFrench)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	assert.Equal(t, "greeting=Bonjour\nfarewell=Au revoir", buf.String())
}

func TestEndToEndFlowPipeline(t *testing.T) {
	ctx := t.Context()
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	input := "title: Hello World\ndescription: A test"

	// Read YAML
	reader, err := reg.NewReader("yaml")
	require.NoError(t, err)
	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Create an uppercase tool to transform block text
	uppercaseTool := &tool.BaseTool{
		ToolName:        "uppercase",
		ToolDescription: "Converts source text to uppercase",
		Produce: func(v tool.VariantView) error {
			upper := strings.ToUpper(v.SourceText())
			v.SetTargetText(model.LocaleFrench, upper)
			return nil
		},
	}

	// Build and execute a flow
	f, err := flow.NewFlow("test-pipeline").
		AddTool(uppercaseTool).
		Build()
	require.NoError(t, err)

	executor := flow.NewExecutor()
	inCh, outCh, wait := executor.ExecuteWithChannels(ctx, f)

	// Feed parts
	go func() {
		for _, p := range parts {
			inCh <- p
		}
		close(inCh)
	}()

	// Collect output
	var outputParts []*model.Part
	for p := range outCh {
		outputParts = append(outputParts, p)
	}
	require.NoError(t, wait())

	// Verify blocks were transformed
	blocks := testutil.FilterBlocks(outputParts)
	require.Len(t, blocks, 2)

	var texts []string
	for _, b := range blocks {
		texts = append(texts, b.TargetText(model.LocaleFrench))
	}
	assert.Contains(t, texts, "HELLO WORLD")
	assert.Contains(t, texts, "A TEST")
}

func TestEndToEndMultiFormatRead(t *testing.T) {
	ctx := t.Context()
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	// Test that all formats can read simple content without errors
	testCases := []struct {
		format registry.FormatID
		input  string
	}{
		{"plaintext", "Hello World"},
		{"json", `{"key": "value"}`},
		{"yaml", "key: value"},
		{"properties", "key=value"},
		{"xml", "<root><item>text</item></root>"},
		{"html", "<html><body><p>Hello</p></body></html>"},
	}

	for _, tc := range testCases {
		t.Run(string(tc.format), func(t *testing.T) {
			reader, err := reg.NewReader(tc.format)
			require.NoError(t, err)
			err = reader.Open(ctx, testutil.RawDocFromString(tc.input, model.LocaleEnglish))
			require.NoError(t, err)
			defer reader.Close()

			parts := testutil.CollectParts(t, reader.Read(ctx))
			blocks := testutil.FilterBlocks(parts)

			require.NotEmpty(t, parts, "format %s produced no parts", tc.format)
			require.NotEmpty(t, blocks, "format %s produced no blocks", tc.format)

			// Verify LayerStart and LayerEnd bookends
			assert.Equal(t, model.PartLayerStart, parts[0].Type, "format %s missing LayerStart", tc.format)
			assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type, "format %s missing LayerEnd", tc.format)
		})
	}
}
