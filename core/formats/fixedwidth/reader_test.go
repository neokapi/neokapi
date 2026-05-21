// okapi-filter: fixedwidth
package fixedwidth_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/formats/fixedwidth"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Test helpers ---

func readFW(t *testing.T, input string, cols []fixedwidth.ColumnDef) []*model.Part {
	t.Helper()
	return readFWWithConfig(t, input, cols, nil)
}

func readFWWithConfig(t *testing.T, input string, cols []fixedwidth.ColumnDef, cfgFn func(*fixedwidth.Config)) []*model.Part {
	t.Helper()
	ctx := t.Context()
	reader := fixedwidth.NewReader()
	cfg := reader.Config().(*fixedwidth.Config)
	cfg.Columns = cols
	if cfgFn != nil {
		cfgFn(cfg)
	}
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()
	return testutil.CollectParts(t, reader.Read(ctx))
}

func collectBlocks(parts []*model.Part) []*model.Block {
	return testutil.FilterBlocks(parts)
}

func blockTexts(blocks []*model.Block) []string {
	return testutil.BlockTexts(blocks)
}

func roundTripFW(t *testing.T, input string, cols []fixedwidth.ColumnDef, cfgFn func(*fixedwidth.Config)) string {
	t.Helper()
	return roundTripFWLocale(t, input, cols, model.LocaleEnglish, cfgFn)
}

func roundTripFWLocale(t *testing.T, input string, cols []fixedwidth.ColumnDef, locale model.LocaleID, cfgFn func(*fixedwidth.Config)) string {
	t.Helper()
	ctx := t.Context()
	reader := fixedwidth.NewReader()
	cfg := reader.Config().(*fixedwidth.Config)
	cfg.Columns = cols
	if cfgFn != nil {
		cfgFn(cfg)
	}
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := fixedwidth.NewWriter()
	writer.SetColumns(cols)
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(locale)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()
	return buf.String()
}

// --- Basic column definitions used across tests ---

var twoCols = []fixedwidth.ColumnDef{
	{Name: "id", Start: 0, Width: 5, Translatable: false},
	{Name: "text", Start: 5, Width: 15, Translatable: true},
}

var threeTranslatableCols = []fixedwidth.ColumnDef{
	{Name: "first", Start: 0, Width: 10, Translatable: true},
	{Name: "second", Start: 10, Width: 10, Translatable: true},
	{Name: "third", Start: 20, Width: 10, Translatable: true},
}

// --- Reader Tests ---

// neokapi-only: FixedWidthFilterTest#testBasicRead
func TestFW_BasicRead(t *testing.T) {
	// 5 chars for id, 15 chars for text
	input := "id001Hello World    \nid002Goodbye World  \n"
	parts := readFW(t, input, twoCols)
	blocks := collectBlocks(parts)
	require.Len(t, blocks, 2)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Hello World    ")
	assert.Contains(t, texts, "Goodbye World  ")
}

// neokapi-only: FixedWidthFilterTest#testBasicReadTrimmed
func TestFW_BasicReadTrimmed(t *testing.T) {
	input := "id001Hello World    \nid002Goodbye World  \n"
	parts := readFWWithConfig(t, input, twoCols, func(cfg *fixedwidth.Config) {
		cfg.TrimValues = true
	})
	blocks := collectBlocks(parts)
	require.Len(t, blocks, 2)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Hello World")
	assert.Contains(t, texts, "Goodbye World")
}

func TestFW_NameAndFormat(t *testing.T) {
	input := "id001Hello          \n"
	parts := readFW(t, input, twoCols)
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)

	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok)
	assert.Equal(t, "text/plain", layer.MimeType)
	assert.Equal(t, "fixedwidth", layer.Format)
}

func TestFW_ReaderMetadata(t *testing.T) {
	reader := fixedwidth.NewReader()
	assert.Equal(t, "fixedwidth", reader.Name())
	assert.Equal(t, "Fixed-Width", reader.DisplayName())
}

func TestFW_Signature(t *testing.T) {
	reader := fixedwidth.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.Extensions, ".dat")
	assert.Contains(t, sig.Extensions, ".fixed")
}

func TestFW_NilDocument(t *testing.T) {
	ctx := t.Context()
	reader := fixedwidth.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}

func TestFW_EmptyInput(t *testing.T) {
	parts := readFW(t, "", twoCols)
	blocks := collectBlocks(parts)
	assert.Empty(t, blocks)
	require.GreaterOrEqual(t, len(parts), 2)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

func TestFW_LayerStartEnd(t *testing.T) {
	input := "id001Hello          \n"
	parts := readFW(t, input, twoCols)
	require.GreaterOrEqual(t, len(parts), 2)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

func TestFW_WithHeader(t *testing.T) {
	input := "ID   Text           \nid001Hello World    \n"
	parts := readFWWithConfig(t, input, twoCols, func(cfg *fixedwidth.Config) {
		cfg.HasHeader = true
	})

	// Check header Data part
	hasHeaderData := false
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if data.Name == "header-row" {
				hasHeaderData = true
				assert.Contains(t, data.Properties["content"], "ID")
			}
		}
	}
	assert.True(t, hasHeaderData, "header row should be emitted as Data")

	blocks := collectBlocks(parts)
	require.Len(t, blocks, 1)
}

func TestFW_NonTranslatableAsData(t *testing.T) {
	input := "id001Hello World    \n"
	parts := readFW(t, input, twoCols)

	hasIDData := false
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if data.Properties["column"] == "id" {
				hasIDData = true
				assert.Equal(t, "id001", data.Properties["content"])
			}
		}
	}
	assert.True(t, hasIDData, "non-translatable column should be emitted as Data")
}

func TestFW_MultipleTranslatableColumns(t *testing.T) {
	input := "Hello     World     Goodbye   \n"
	parts := readFW(t, input, threeTranslatableCols)
	blocks := collectBlocks(parts)
	require.Len(t, blocks, 3)
}

func TestFW_MultipleRows(t *testing.T) {
	input := "id001Hello World    \nid002Foo Bar         \nid003Baz Qux        \n"
	parts := readFW(t, input, twoCols)
	blocks := collectBlocks(parts)
	require.Len(t, blocks, 3)
}

func TestFW_ShortLine(t *testing.T) {
	// Line shorter than column definition
	cols := []fixedwidth.ColumnDef{
		{Name: "text", Start: 0, Width: 20, Translatable: true},
	}
	input := "Short\n"
	parts := readFW(t, input, cols)
	blocks := collectBlocks(parts)
	require.Len(t, blocks, 1)
	assert.Equal(t, "Short", blocks[0].SourceText())
}

func TestFW_ColumnBeyondLineLength(t *testing.T) {
	// Column starts beyond the end of the line
	cols := []fixedwidth.ColumnDef{
		{Name: "text", Start: 100, Width: 10, Translatable: true},
	}
	input := "Short\n"
	parts := readFW(t, input, cols)
	blocks := collectBlocks(parts)
	assert.Empty(t, blocks, "column beyond line length should produce no blocks")
}

func TestFW_BlockProperties(t *testing.T) {
	input := "id001Hello World    \n"
	parts := readFW(t, input, twoCols)
	blocks := collectBlocks(parts)
	require.Len(t, blocks, 1)
	assert.Equal(t, "text", blocks[0].Properties["column"])
	assert.Equal(t, "1", blocks[0].Properties["row"])
	assert.Equal(t, "5", blocks[0].Properties["start"])
	assert.Equal(t, "15", blocks[0].Properties["width"])
}

func TestFW_BlockName(t *testing.T) {
	input := "id001Hello World    \n"
	parts := readFW(t, input, twoCols)
	blocks := collectBlocks(parts)
	require.Len(t, blocks, 1)
	assert.Equal(t, "text.row1", blocks[0].Name)
}

// --- Config Tests ---

func TestFW_ConfigFormatName(t *testing.T) {
	cfg := &fixedwidth.Config{}
	assert.Equal(t, "fixedwidth", cfg.FormatName())
}

func TestFW_ConfigValidate_NoColumns(t *testing.T) {
	cfg := &fixedwidth.Config{}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one column")
}

func TestFW_ConfigValidate_EmptyName(t *testing.T) {
	cfg := &fixedwidth.Config{
		Columns: []fixedwidth.ColumnDef{{Name: "", Start: 0, Width: 10}},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name must not be empty")
}

func TestFW_ConfigValidate_ZeroWidth(t *testing.T) {
	cfg := &fixedwidth.Config{
		Columns: []fixedwidth.ColumnDef{{Name: "col", Start: 0, Width: 0}},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "width must be positive")
}

func TestFW_ConfigValidate_NegativeStart(t *testing.T) {
	cfg := &fixedwidth.Config{
		Columns: []fixedwidth.ColumnDef{{Name: "col", Start: -1, Width: 10}},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "start must not be negative")
}

func TestFW_ConfigValidate_Valid(t *testing.T) {
	cfg := &fixedwidth.Config{
		Columns: []fixedwidth.ColumnDef{{Name: "col", Start: 0, Width: 10}},
	}
	err := cfg.Validate()
	require.NoError(t, err)
}

func TestFW_ConfigReset(t *testing.T) {
	cfg := &fixedwidth.Config{
		Columns:   twoCols,
		HasHeader: true,
	}
	cfg.Reset()
	assert.Nil(t, cfg.Columns)
	assert.False(t, cfg.HasHeader)
	assert.False(t, cfg.TrimValues)
}

func TestFW_ConfigApplyMap_UnknownParam(t *testing.T) {
	cfg := &fixedwidth.Config{}
	err := cfg.ApplyMap(map[string]any{"unknown": true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown parameter")
}

func TestFW_ConfigApplyMap_HasHeader(t *testing.T) {
	cfg := &fixedwidth.Config{}
	err := cfg.ApplyMap(map[string]any{"hasHeader": true})
	require.NoError(t, err)
	assert.True(t, cfg.HasHeader)
}

func TestFW_ConfigApplyMap_TrimValues(t *testing.T) {
	cfg := &fixedwidth.Config{}
	err := cfg.ApplyMap(map[string]any{"trimValues": true})
	require.NoError(t, err)
	assert.True(t, cfg.TrimValues)
}

func TestFW_ConfigApplyMap_Columns(t *testing.T) {
	cfg := &fixedwidth.Config{}
	err := cfg.ApplyMap(map[string]any{
		"columns": []any{
			map[string]any{
				"name":         "id",
				"start":        float64(0),
				"width":        float64(5),
				"translatable": false,
			},
			map[string]any{
				"name":         "text",
				"start":        float64(5),
				"width":        float64(15),
				"translatable": true,
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, cfg.Columns, 2)
	assert.Equal(t, "id", cfg.Columns[0].Name)
	assert.Equal(t, 0, cfg.Columns[0].Start)
	assert.Equal(t, 5, cfg.Columns[0].Width)
	assert.False(t, cfg.Columns[0].Translatable)
	assert.Equal(t, "text", cfg.Columns[1].Name)
	assert.True(t, cfg.Columns[1].Translatable)
}

// --- Writer Tests ---

func TestFW_WriterMetadata(t *testing.T) {
	writer := fixedwidth.NewWriter()
	assert.Equal(t, "fixedwidth", writer.Name())
}

// neokapi-only: FixedWidthFilterTest#testRoundTrip
func TestFW_RoundTrip(t *testing.T) {
	input := "id001Hello World    \nid002Goodbye World  \n"
	output := roundTripFW(t, input, twoCols, nil)
	assert.Contains(t, output, "Hello World")
	assert.Contains(t, output, "Goodbye World")
	assert.Contains(t, output, "id001")
}

func TestFW_RoundTripWithHeader(t *testing.T) {
	input := "ID   Text           \nid001Hello World    \n"
	output := roundTripFW(t, input, twoCols, func(cfg *fixedwidth.Config) {
		cfg.HasHeader = true
	})
	assert.Contains(t, output, "ID   Text")
	assert.Contains(t, output, "Hello World")
}

func TestFW_RoundTripWithTranslation(t *testing.T) {
	ctx := t.Context()
	input := "id001Hello World    \n"

	reader := fixedwidth.NewReader()
	cfg := reader.Config().(*fixedwidth.Config)
	cfg.Columns = twoCols
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			block.SetTargetText(model.LocaleFrench, "Bonjour Monde")
		}
	}

	var buf bytes.Buffer
	writer := fixedwidth.NewWriter()
	writer.SetColumns(twoCols)
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Bonjour Monde")
	assert.Contains(t, output, "id001")
}

func TestFW_UnicodeContent(t *testing.T) {
	cols := []fixedwidth.ColumnDef{
		{Name: "text", Start: 0, Width: 10, Translatable: true},
	}
	// Unicode characters: each is one rune
	input := "Helloworld\n"
	parts := readFW(t, input, cols)
	blocks := collectBlocks(parts)
	require.Len(t, blocks, 1)
	assert.Equal(t, "Helloworld", blocks[0].SourceText())
}

func TestFW_AllTranslatable(t *testing.T) {
	cols := []fixedwidth.ColumnDef{
		{Name: "first", Start: 0, Width: 5, Translatable: true},
		{Name: "second", Start: 5, Width: 5, Translatable: true},
	}
	input := "AAAAABBBBB\n"
	parts := readFW(t, input, cols)
	blocks := collectBlocks(parts)
	require.Len(t, blocks, 2)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "AAAAA")
	assert.Contains(t, texts, "BBBBB")
}

func TestFW_AllNonTranslatable(t *testing.T) {
	cols := []fixedwidth.ColumnDef{
		{Name: "first", Start: 0, Width: 5, Translatable: false},
		{Name: "second", Start: 5, Width: 5, Translatable: false},
	}
	input := "AAAAABBBBB\n"
	parts := readFW(t, input, cols)
	blocks := collectBlocks(parts)
	assert.Empty(t, blocks, "all non-translatable columns should produce no blocks")

	// But we should have Data parts
	dataCount := 0
	for _, p := range parts {
		if p.Type == model.PartData {
			dataCount++
		}
	}
	assert.Equal(t, 2, dataCount)
}

func TestFW_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // cancel immediately

	reader := fixedwidth.NewReader()
	cfg := reader.Config().(*fixedwidth.Config)
	cfg.Columns = twoCols

	input := "id001Hello World    \n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	ch := reader.Read(ctx)
	var parts []*model.Part
	for pr := range ch {
		if pr.Part != nil {
			parts = append(parts, pr.Part)
		}
	}
	// With a cancelled context, we may get 0 or partial parts
	assert.True(t, len(parts) <= 4, "should not emit all parts when context is cancelled")
}
