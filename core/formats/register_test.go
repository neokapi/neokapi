package formats_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/gokapi/gokapi/core/config"
	"github.com/gokapi/gokapi/core/flow"
	"github.com/gokapi/gokapi/core/formats"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/registry"
	"github.com/gokapi/gokapi/core/testutil"
	"github.com/gokapi/gokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterAllReaders(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	expectedFormats := []string{
		"plaintext", "html", "xml", "xliff", "xliff2",
		"yaml", "json", "po", "properties",
		"markdown", "csv", "srt", "vtt", "tmx",
	}

	for _, name := range expectedFormats {
		assert.True(t, reg.HasReader(name), "reader not registered: %s", name)
	}
	assert.Len(t, reg.ReaderNames(), len(expectedFormats))
}

func TestRegisterAllWriters(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	expectedFormats := []string{
		"plaintext", "html", "xml", "xliff", "xliff2",
		"yaml", "json", "po", "properties",
		"markdown", "csv", "srt", "vtt", "tmx",
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
		assert.Equal(t, name, reader.Name())
	}

	for _, name := range reg.WriterNames() {
		writer, err := reg.NewWriter(name)
		require.NoError(t, err, "failed to create writer: %s", name)
		assert.Equal(t, name, writer.Name())
	}
}

func TestCollectNativeDecoders(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	configReg := config.NewRegistry()
	reg.CollectNativeDecoders(configReg)

	// All 14 native formats should have decoders registered
	expectedFormats := []string{
		"plaintext", "html", "xml", "xliff", "xliff2",
		"yaml", "json", "po", "properties",
		"markdown", "csv", "srt", "vtt", "tmx",
	}
	for _, name := range expectedFormats {
		apiVersion := "gokapi/" + name + "-v1"
		assert.True(t, configReg.Has(apiVersion), "decoder not registered for %s", apiVersion)
	}

	// HTML and JSON should use their declared apiVersion (same pattern)
	assert.True(t, configReg.Has("gokapi/html-v1"))
	assert.True(t, configReg.Has("gokapi/json-v1"))

	// Test that a decoder can decode a spec
	env := &config.Envelope{
		APIVersion: "gokapi/json-v1",
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
	ctx := context.Background()
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
	ctx := context.Background()
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
	ctx := context.Background()
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
	ctx := context.Background()
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
		HandleBlockFn: func(part *model.Part) (*model.Part, error) {
			block := part.Resource.(*model.Block)
			upper := strings.ToUpper(block.SourceText())
			block.SetTargetText(model.LocaleFrench, upper)
			return part, nil
		},
	}

	// Build and execute a flow
	f := flow.NewFlow("test-pipeline").
		AddTool(uppercaseTool).
		Build()

	executor := flow.NewFlowExecutor()
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
	ctx := context.Background()
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	// Test that all formats can read simple content without errors
	testCases := []struct {
		format string
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
		t.Run(tc.format, func(t *testing.T) {
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
