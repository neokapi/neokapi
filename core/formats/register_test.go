package formats_test

import (
	"bytes"
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
		"yaml", "json", "jsx", "po", "properties",
		"markdown", "csv", "tsv", "srt", "ttml", "vtt", "tmx", "openxml",
		"mosestext", "dtd", "ts", "wiki", "tex",
		"regex", "doxygen", "messageformat", "phpcontent",
		"icml", "idml", "fixedwidth",
		"transtable", "paraplaintext", "splicedlines", "versifiedtext", "vignette",
		"odf", "epub", "rtf", "mif", "ttx", "txml", "pdf", "xcstrings", "arb", "resx",
		"androidxml", "applestrings", "i18next", "designtokens", "mdx",
		// Declarative pseudo-format: registered so it shows up in the
		// format list / UI. Open() intentionally errors — actual exec
		// extraction runs via `kapi extract -p` (Framework AD-002).
		"exec", "mo",
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
		"yaml", "json", "jsx", "po", "mo", "properties",
		"markdown", "csv", "tsv", "srt", "ttml", "vtt", "tmx", "openxml",
		"mosestext", "dtd", "ts", "wiki", "tex",
		"regex", "doxygen", "messageformat", "phpcontent",
		"icml", "idml", "fixedwidth",
		"transtable", "paraplaintext", "splicedlines", "versifiedtext", "vignette",
		"odf", "epub", "rtf", "mif", "ttx", "txml", "pdf", "xcstrings", "arb", "resx",
		"androidxml", "applestrings", "i18next", "designtokens", "mdx",
	}

	for _, name := range expectedFormats {
		assert.True(t, reg.HasWriter(name), "writer not registered: %s", name)
	}
	assert.Len(t, reg.WriterNames(), len(expectedFormats))
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
		"ts", "fixedwidth", "phpcontent",
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
		Translate: func(v tool.TargetView) error {
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
