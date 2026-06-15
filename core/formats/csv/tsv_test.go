// okapi-filter: tsv
package csv_test

import (
	"bytes"
	"testing"

	csvfmt "github.com/neokapi/neokapi/core/formats/csv"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- TSV test helpers ---

func readTSV(t *testing.T, input string) []*model.Part {
	t.Helper()
	ctx := t.Context()
	reader := csvfmt.NewTSVReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()
	return testutil.CollectParts(t, reader.Read(ctx))
}

func roundTripTSV(t *testing.T, input string) string {
	t.Helper()
	return roundTripTSVLocale(t, input, model.LocaleEnglish)
}

func roundTripTSVLocale(t *testing.T, input string, locale model.LocaleID) string {
	t.Helper()
	ctx := t.Context()
	reader := csvfmt.NewTSVReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := csvfmt.NewTSVWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(locale)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()
	return buf.String()
}

// --- TSV Reader Tests ---

// neokapi-only: TableFilterTest#testTSV — no such method in v1.48.0 TableFilterTest (no TSV-specific @Test); native tab-separated reading is neokapi's own coverage.
func TestTSV_BasicRead(t *testing.T) {
	t.Parallel()
	input := "name\tvalue\ngreeting\tHello\nfarewell\tGoodbye\n"
	parts := readTSV(t, input)
	blocks := collectBlocks(parts)
	// Both columns are translatable by default (name + value for each row)
	require.Len(t, blocks, 4)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Hello")
	assert.Contains(t, texts, "Goodbye")
	assert.Contains(t, texts, "greeting")
	assert.Contains(t, texts, "farewell")
}

func TestTSV_NameAndMimeType(t *testing.T) {
	t.Parallel()
	input := "a\tb\n1\t2\n"
	parts := readTSV(t, input)
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)

	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok)
	assert.Equal(t, "text/tab-separated-values", layer.MimeType)
	assert.Equal(t, "tsv", layer.Format)
}

func TestTSV_ReaderMetadata(t *testing.T) {
	t.Parallel()
	reader := csvfmt.NewTSVReader()
	assert.Equal(t, "tsv", reader.Name())
	assert.Equal(t, "TSV", reader.DisplayName())
}

func TestTSV_Signature(t *testing.T) {
	t.Parallel()
	reader := csvfmt.NewTSVReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "text/tab-separated-values")
	assert.Contains(t, sig.Extensions, ".tsv")
	assert.NotContains(t, sig.Extensions, ".csv")
}

func TestTSV_EmptyInput(t *testing.T) {
	t.Parallel()
	parts := readTSV(t, "")
	blocks := collectBlocks(parts)
	assert.Empty(t, blocks)
	require.GreaterOrEqual(t, len(parts), 2)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

func TestTSV_NilDocument(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := csvfmt.NewTSVReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}

func TestTSV_HeaderRow(t *testing.T) {
	t.Parallel()
	input := "key\tvalue\nid1\tHello\n"
	parts := readTSV(t, input)

	// The header row is emitted as a table-row of non-translatable header
	// cells (RoleTableHeader) so cross-format writers can render a table header.
	var headerTexts []string
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		block := p.Resource.(*model.Block)
		if block.SemanticRole() == model.RoleTableHeader {
			assert.False(t, block.Translatable, "header cells should be non-translatable")
			headerTexts = append(headerTexts, block.SourceText())
		}
	}
	assert.Equal(t, []string{"key", "value"}, headerTexts, "header row should be emitted as header cells")
}

func TestTSV_NoHeader(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := csvfmt.NewTSVReader()
	cfg := reader.Config().(*csvfmt.Config)
	cfg.HasHeader = false

	input := "Hello\tWorld\nFoo\tBar\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := collectBlocks(parts)
	require.Len(t, blocks, 4)
}

func TestTSV_MultipleColumns(t *testing.T) {
	t.Parallel()
	input := "col1\tcol2\tcol3\na\tb\tc\n"
	parts := readTSV(t, input)
	blocks := collectBlocks(parts)
	require.Len(t, blocks, 3)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "a")
	assert.Contains(t, texts, "b")
	assert.Contains(t, texts, "c")
}

func TestTSV_EmbeddedCommas(t *testing.T) {
	t.Parallel()
	// Tabs as separator means commas in values are fine without quoting
	ctx := t.Context()
	reader := csvfmt.NewTSVReader()
	cfg := reader.Config().(*csvfmt.Config)
	cfg.HasHeader = false

	input := "Hello, World\tGoodbye, World\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := collectBlocks(parts)
	require.Len(t, blocks, 2)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Hello, World")
	assert.Contains(t, texts, "Goodbye, World")
}

func TestTSV_TranslatableColumns(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := csvfmt.NewTSVReader()
	cfg := reader.Config().(*csvfmt.Config)
	cfg.TranslatableColumns = []int{1}

	input := "id\ttext\nk1\tHello\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := collectBlocks(parts)
	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello", blocks[0].SourceText())
}

func TestTSV_TrimValues(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := csvfmt.NewTSVReader()
	cfg := reader.Config().(*csvfmt.Config)
	cfg.TrimValues = true
	cfg.HasHeader = false

	input := "  Hello  \t  World  \n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := collectBlocks(parts)
	require.Len(t, blocks, 2)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Hello")
	assert.Contains(t, texts, "World")
}

// --- TSV Writer Tests ---

func TestTSV_WriterMetadata(t *testing.T) {
	t.Parallel()
	writer := csvfmt.NewTSVWriter()
	assert.Equal(t, "tsv", writer.Name())
}

func TestTSV_RoundTrip(t *testing.T) {
	t.Parallel()
	input := "key\tvalue\ngreeting\tHello\nfarewell\tGoodbye\n"
	output := roundTripTSV(t, input)
	assert.Contains(t, output, "key\tvalue")
	assert.Contains(t, output, "Hello")
	assert.Contains(t, output, "Goodbye")
}

func TestTSV_RoundTripWithTranslation(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	input := "key\tvalue\ngreeting\tHello\n"

	reader := csvfmt.NewTSVReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			if block.SourceText() == "Hello" {
				block.SetTargetText(model.LocaleFrench, "Bonjour")
			}
		}
	}

	var buf bytes.Buffer
	writer := csvfmt.NewTSVWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Bonjour")
	assert.NotContains(t, output, "Hello")
}

func TestTSV_KeyColumns(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := csvfmt.NewTSVReader()
	cfg := reader.Config().(*csvfmt.Config)
	cfg.KeyColumns = []int{0}
	cfg.TranslatableColumns = []int{1}

	input := "id\ttext\ngreeting\tHello\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := collectBlocks(parts)
	require.Len(t, blocks, 1)
	assert.Equal(t, "greeting", blocks[0].ID)
}

func TestTSV_CommentColumns(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := csvfmt.NewTSVReader()
	cfg := reader.Config().(*csvfmt.Config)
	cfg.CommentColumns = []int{2}
	cfg.TranslatableColumns = []int{1}

	input := "id\ttext\tnote\nk1\tHello\tA greeting\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := collectBlocks(parts)
	require.Len(t, blocks, 1)
	assert.Equal(t, "A greeting", blocks[0].Properties["comment"])
}

func TestTSV_ConfigFormatName(t *testing.T) {
	t.Parallel()
	cfg := &csvfmt.Config{Separator: '\t'}
	assert.Equal(t, "tsv", cfg.FormatName())

	cfg2 := &csvfmt.Config{Separator: ','}
	assert.Equal(t, "csv", cfg2.FormatName())
}

func TestTSV_MultipleRows(t *testing.T) {
	t.Parallel()
	input := "h1\th2\nrow1a\trow1b\nrow2a\trow2b\nrow3a\trow3b\n"
	parts := readTSV(t, input)
	blocks := collectBlocks(parts)
	require.Len(t, blocks, 6)
}

func TestTSV_EmptyRows(t *testing.T) {
	t.Parallel()
	// Rows with empty cells should still produce blocks for non-empty cells
	input := "h1\th2\n\tworld\nhello\t\n"
	parts := readTSV(t, input)
	blocks := collectBlocks(parts)
	// "world" from row 1 and "hello" from row 2 (empty cells are skipped)
	require.Len(t, blocks, 2)
}

func TestTSV_LayerStartEnd(t *testing.T) {
	t.Parallel()
	input := "col\ndata\n"
	parts := readTSV(t, input)
	require.GreaterOrEqual(t, len(parts), 2)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

func TestTSV_CSVReaderDoesNotClaimTSV(t *testing.T) {
	t.Parallel()
	reader := csvfmt.NewReader()
	sig := reader.Signature()
	assert.NotContains(t, sig.Extensions, ".tsv")
	assert.NotContains(t, sig.MIMETypes, "text/tab-separated-values")
}
