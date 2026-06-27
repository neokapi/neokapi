package fixedwidth_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/fixedwidth"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fwSkeletonRoundtrip(t *testing.T, input string, cols []fixedwidth.ColumnDef, cfgFn func(*fixedwidth.Config)) string {
	t.Helper()
	ctx := t.Context()

	reader := fixedwidth.NewReader()
	cfg := reader.Config().(*fixedwidth.Config)
	cfg.Columns = cols
	if cfgFn != nil {
		cfgFn(cfg)
	}

	writer := fixedwidth.NewWriter()
	writer.SetColumns(cols)

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

// fwStreamingRoundtrip drives a concurrent streaming round-trip via a
// NewStreamingSkeletonStore, forwarding the reader's Parts into the writer while
// the reader is still producing. Output must match the buffered skeleton path.
func fwStreamingRoundtrip(t *testing.T, input string, cols []fixedwidth.ColumnDef, cfgFn func(*fixedwidth.Config)) string {
	t.Helper()
	ctx := t.Context()

	reader := fixedwidth.NewReader()
	cfg := reader.Config().(*fixedwidth.Config)
	cfg.Columns = cols
	if cfgFn != nil {
		cfgFn(cfg)
	}
	writer := fixedwidth.NewWriter()
	writer.SetColumns(cols)

	store := format.NewStreamingSkeletonStore()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))

	partsCh := make(chan *model.Part, 64)
	readCh := reader.Read(ctx)
	go func() {
		defer close(partsCh)
		for res := range readCh {
			if res.Error == nil && res.Part != nil {
				partsCh <- res.Part
			}
		}
		store.CloseWrite()
		reader.Close()
	}()

	require.NoError(t, writer.Write(ctx, partsCh))
	writer.Close()
	return buf.String()
}

// TestStreamingMatchesBuffered asserts the streaming skeleton path is
// byte-identical to the buffered path.
func TestStreamingMatchesBuffered(t *testing.T) {
	assert.Equal(t,
		fwSkeletonRoundtrip(t, "id001Hello World    \nid002Goodbye World  \n", twoCols, nil),
		fwStreamingRoundtrip(t, "id001Hello World    \nid002Goodbye World  \n", twoCols, nil))
	header := func(cfg *fixedwidth.Config) { cfg.HasHeader = true }
	assert.Equal(t,
		fwSkeletonRoundtrip(t, "ID   Text           \nid001Hello World    \n", twoCols, header),
		fwStreamingRoundtrip(t, "ID   Text           \nid001Hello World    \n", twoCols, header))
	assert.Equal(t,
		fwSkeletonRoundtrip(t, "id001Hello World    \r\nid002Goodbye World  \r\n", twoCols, nil),
		fwStreamingRoundtrip(t, "id001Hello World    \r\nid002Goodbye World  \r\n", twoCols, nil))
}

func TestSkeletonStore_ByteExact_BasicTwoColumns(t *testing.T) {
	input := "id001Hello World    \nid002Goodbye World  \n"
	output := fwSkeletonRoundtrip(t, input, twoCols, nil)
	assert.Equal(t, input, output, "basic two-column roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_WithHeader(t *testing.T) {
	input := "ID   Text           \nid001Hello World    \n"
	output := fwSkeletonRoundtrip(t, input, twoCols, func(cfg *fixedwidth.Config) {
		cfg.HasHeader = true
	})
	assert.Equal(t, input, output, "header row roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_CRLF(t *testing.T) {
	input := "id001Hello World    \r\nid002Goodbye World  \r\n"
	output := fwSkeletonRoundtrip(t, input, twoCols, nil)
	assert.Equal(t, input, output, "CRLF line endings should be preserved")
}

func TestSkeletonStore_ByteExact_MultipleRows(t *testing.T) {
	input := "id001Hello World    \nid002Foo Bar         \nid003Baz Qux        \n"
	output := fwSkeletonRoundtrip(t, input, twoCols, nil)
	assert.Equal(t, input, output, "multiple rows should be byte-exact")
}

func TestSkeletonStore_ByteExact_AllTranslatable(t *testing.T) {
	cols := []fixedwidth.ColumnDef{
		{Name: "first", Start: 0, Width: 5, Translatable: true},
		{Name: "second", Start: 5, Width: 5, Translatable: true},
	}
	input := "AAAAABBBBB\n"
	output := fwSkeletonRoundtrip(t, input, cols, nil)
	assert.Equal(t, input, output, "all translatable columns should be byte-exact")
}

func TestSkeletonStore_ByteExact_AllNonTranslatable(t *testing.T) {
	cols := []fixedwidth.ColumnDef{
		{Name: "first", Start: 0, Width: 5, Translatable: false},
		{Name: "second", Start: 5, Width: 5, Translatable: false},
	}
	input := "AAAAABBBBB\n"
	output := fwSkeletonRoundtrip(t, input, cols, nil)
	assert.Equal(t, input, output, "all non-translatable columns should be byte-exact")
}

func TestSkeletonStore_ByteExact_NoTrailingNewline(t *testing.T) {
	input := "id001Hello World    "
	output := fwSkeletonRoundtrip(t, input, twoCols, nil)
	assert.Equal(t, input, output, "no trailing newline should be preserved")
}

func TestSkeletonStore_ByteExact_EmptyInput(t *testing.T) {
	input := ""
	output := fwSkeletonRoundtrip(t, input, twoCols, nil)
	assert.Equal(t, input, output, "empty input should produce empty output")
}

func TestSkeletonStore_WithTranslation(t *testing.T) {
	input := "id001Hello World    \n"
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := fixedwidth.NewReader()
	cfg := reader.Config().(*fixedwidth.Config)
	cfg.Columns = twoCols

	writer := fixedwidth.NewWriter()
	writer.SetColumns(twoCols)

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
			if b.SourceText() == "Hello World    " {
				b.SetTargetRuns(locale, []model.Run{{Text: &model.TextRun{Text: "Bonjour Monde  "}}})
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	// The skeleton preserves non-translatable "id001" and line structure
	assert.Contains(t, buf.String(), "id001")
	assert.Contains(t, buf.String(), "Bonjour Monde")
}

func TestSkeletonStore_ByteExact_ThreeTranslatable(t *testing.T) {
	input := "Hello     World     Goodbye   \n"
	output := fwSkeletonRoundtrip(t, input, threeTranslatableCols, nil)
	assert.Equal(t, input, output, "three translatable columns should be byte-exact")
}
