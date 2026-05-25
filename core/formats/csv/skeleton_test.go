package csv_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	csvfmt "github.com/neokapi/neokapi/core/formats/csv"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func skeletonRoundtrip(t *testing.T, input string, cfgFn func(*csvfmt.Config)) string {
	t.Helper()
	ctx := t.Context()

	reader := csvfmt.NewReader()
	if cfgFn != nil {
		cfgFn(reader.Config().(*csvfmt.Config))
	}
	writer := csvfmt.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	return buf.String()
}

func skeletonTSVRoundtrip(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := csvfmt.NewTSVReader()
	writer := csvfmt.NewTSVWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	return buf.String()
}

func TestSkeletonStore_ByteExact_SimpleCSV(t *testing.T) {
	t.Parallel()
	input := "name,value\nhello,world\nfoo,bar"
	output := skeletonRoundtrip(t, input, nil)
	assert.Equal(t, input, output, "simple CSV roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_QuotedFields(t *testing.T) {
	t.Parallel()
	input := "name,value\n\"hello, world\",simple\nplain,\"quoted value\""
	output := skeletonRoundtrip(t, input, nil)
	assert.Equal(t, input, output, "quoted fields should be preserved byte-exact")
}

func TestSkeletonStore_ByteExact_CRLF(t *testing.T) {
	t.Parallel()
	input := "name,value\r\nhello,world\r\nfoo,bar"
	output := skeletonRoundtrip(t, input, nil)
	assert.Equal(t, input, output, "CRLF line endings should be preserved byte-exact")
}

func TestSkeletonStore_ByteExact_CRLFTrailingNewline(t *testing.T) {
	t.Parallel()
	input := "name,value\r\nhello,world\r\nfoo,bar\r\n"
	output := skeletonRoundtrip(t, input, nil)
	assert.Equal(t, input, output, "CRLF with trailing newline should be preserved")
}

func TestSkeletonStore_ByteExact_MixedQuoting(t *testing.T) {
	t.Parallel()
	input := "a,b,c\nunquoted,\"quoted\",unquoted\n\"all quoted\",\"also quoted\",\"quoted too\""
	output := skeletonRoundtrip(t, input, nil)
	assert.Equal(t, input, output, "mixed quoting styles should be preserved byte-exact")
}

func TestSkeletonStore_ByteExact_EscapedQuotes(t *testing.T) {
	t.Parallel()
	input := "name,value\n\"has \"\"escaped\"\" quotes\",plain"
	output := skeletonRoundtrip(t, input, nil)
	assert.Equal(t, input, output, "escaped quotes should be preserved byte-exact")
}

func TestSkeletonStore_ByteExact_TrailingNewline(t *testing.T) {
	t.Parallel()
	input := "name,value\nhello,world\n"
	output := skeletonRoundtrip(t, input, nil)
	assert.Equal(t, input, output, "trailing newline should be preserved")
}

func TestSkeletonStore_ByteExact_NoHeader(t *testing.T) {
	t.Parallel()
	input := "hello,world\nfoo,bar"
	output := skeletonRoundtrip(t, input, func(cfg *csvfmt.Config) {
		cfg.HasHeader = false
	})
	assert.Equal(t, input, output, "no-header CSV should roundtrip byte-exact")
}

func TestSkeletonStore_ByteExact_HeaderPreserved(t *testing.T) {
	t.Parallel()
	input := "Name,Description,Count\nAlice,Developer,1\nBob,Designer,2"
	output := skeletonRoundtrip(t, input, nil)
	assert.Equal(t, input, output, "header row should be preserved byte-exact")
}

func TestSkeletonStore_ByteExact_EmptyCells(t *testing.T) {
	t.Parallel()
	input := "a,b,c\n,hello,\nfoo,,bar"
	output := skeletonRoundtrip(t, input, nil)
	assert.Equal(t, input, output, "empty cells should be preserved byte-exact")
}

func TestSkeletonStore_ByteExact_TSV(t *testing.T) {
	t.Parallel()
	input := "name\tvalue\nhello\tworld\nfoo\tbar"
	output := skeletonTSVRoundtrip(t, input)
	assert.Equal(t, input, output, "TSV roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_TSV_CRLF(t *testing.T) {
	t.Parallel()
	input := "name\tvalue\r\nhello\tworld\r\nfoo\tbar"
	output := skeletonTSVRoundtrip(t, input)
	assert.Equal(t, input, output, "TSV with CRLF should be byte-exact")
}

func TestSkeletonStore_WithTranslation(t *testing.T) {
	t.Parallel()
	input := "key,text\ngreeting,Hello World\nfarewell,Goodbye"
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := csvfmt.NewReader()
	writer := csvfmt.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			switch b.SourceText() {
			case "Hello World":
				b.SetTargetText(locale, "Bonjour le monde")
			case "Goodbye":
				b.SetTargetText(locale, "Au revoir")
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	assert.Equal(t, "key,text\ngreeting,Bonjour le monde\nfarewell,Au revoir", buf.String())
}

func TestSkeletonStore_WithTranslation_QuotedField(t *testing.T) {
	t.Parallel()
	input := "key,text\ngreeting,\"Hello, World\"\nfarewell,Goodbye"
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := csvfmt.NewReader()
	writer := csvfmt.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			switch b.SourceText() {
			case "Hello, World":
				b.SetTargetText(locale, "Bonjour le monde")
			case "Goodbye":
				b.SetTargetText(locale, "Au revoir")
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	// The quote wrapper is preserved from skeleton, translated value replaces content.
	assert.Equal(t, "key,text\ngreeting,\"Bonjour le monde\"\nfarewell,Au revoir", buf.String())
}

func TestSkeletonStore_WithTranslation_CRLF(t *testing.T) {
	t.Parallel()
	input := "key,text\r\ngreeting,Hello\r\nfarewell,Goodbye"
	ctx := t.Context()
	locale := model.LocaleID("de")

	reader := csvfmt.NewReader()
	writer := csvfmt.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			switch b.SourceText() {
			case "Hello":
				b.SetTargetText(locale, "Hallo")
			case "Goodbye":
				b.SetTargetText(locale, "Tschuss")
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	// CRLF line endings should be preserved even with translation.
	assert.Equal(t, "key,text\r\ngreeting,Hallo\r\nfarewell,Tschuss", buf.String())
}

func TestSkeletonStore_ByteExact_NonTranslatableColumns(t *testing.T) {
	t.Parallel()
	input := "id,name,count\n1,Alice,10\n2,Bob,20"
	output := skeletonRoundtrip(t, input, func(cfg *csvfmt.Config) {
		cfg.TranslatableColumns = []int{1} // only "name" column is translatable
	})
	assert.Equal(t, input, output, "non-translatable columns should be preserved byte-exact")
}

func TestSkeletonStore_ByteExact_SingleColumn(t *testing.T) {
	t.Parallel()
	input := "value\nhello\nworld"
	output := skeletonRoundtrip(t, input, nil)
	assert.Equal(t, input, output, "single column CSV should roundtrip byte-exact")
}
