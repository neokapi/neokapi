package doxygen_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/doxygen"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func snippetRoundtripWithSkeleton(t *testing.T, input string) string {
	t.Helper()
	return snippetRoundtripWithSkeletonConfig(t, input, nil)
}

func snippetRoundtripWithSkeletonConfig(t *testing.T, input string, configure func(*doxygen.Config)) string {
	t.Helper()
	ctx := t.Context()

	reader := doxygen.NewReader()
	if configure != nil {
		configure(reader.Config().(*doxygen.Config))
	}
	writer := doxygen.NewWriter()

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

func TestSkeletonStore_ByteExact_SimpleComment(t *testing.T) {
	input := "/// Hello world\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "simple /// comment roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_CodeAndComment(t *testing.T) {
	input := "/// Hello world\nint x;\n/// Goodbye world\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "code between comments should be byte-exact")
}

func TestSkeletonStore_OkapiPattern_MultiLineComment(t *testing.T) {
	// Mirrors okapi DoxygenFilter behaviour: consecutive `///` prose
	// lines collapse into a single TextUnit with whitespace flattened
	// to single spaces (WhitespaceAdjustingEventBuilder.collapseWhitespace
	// + parsePlainText). On roundtrip the joined text emits on the
	// first line, with bare `///` padding lines accumulated at the end
	// to preserve the original line count. Byte-exact against the okapi
	// reference, NOT against the source — this is the same trade-off
	// the parity harness verifies.
	input := "/// First line\n/// Second line\n/// Third line\n"
	expected := "/// First line Second line Third line\n///\n///\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, expected, output, "consecutive /// prose lines collapse to one line with bare-marker padding (okapi parity)")
}

// okapi: DoxygenWriterTest#testOutputJavadocComment
// Okapi's writer collapses a multi-line Javadoc comment into a single fluid
// TextUnit and re-emits the prose on the first body line, padding the
// remaining body lines with bare ` * ` markers and preserving the closing
// ` */`. The native skeleton writer reproduces the same layout byte-for-byte,
// including okapi's WHITESPACE_COLLAPSE folding the trailing space of " * This
// is " into the joined "This is a test." (no trailing space).
func TestWriter_OutputJavadocComment(t *testing.T) {
	input := "/**\n * This is \n * a test.\n */\nbaz baz baz"
	expected := "/**\n * This is a test.\n * \n */\nbaz baz baz\n"
	assert.Equal(t, expected, snippetRoundtripWithSkeleton(t, input))
}

// DoxygenWriterTest#testOutputMultilineComment and
// DoxygenFilterTest#testDoubleExtractionLists exercise doxygen reader/writer
// behaviors the native filter does not model (#611):
//
// okapi-skip: DoxygenWriterTest#testOutputMultilineComment — okapi extracts and merges trailing `///` comments that follow code on the same line (`foo foo foo /// This is`); the native reader only extracts leading `///` and `///<` trailing comments, so it emits no translatable text for this layout and cannot reproduce okapi's multi-line `///` reflow
// okapi-skip: DoxygenFilterTest#testDoubleExtractionLists — okapi's RoundTripComparison over lists.h is event-stable; the native reader/writer reflow of HTML (`<ul>/<li>`) and `-#`/`.` doxygen lists is not roundtrip-idempotent for lists.h (paragraph-break and list structure shift on re-extraction), so a faithful roundtrip cannot be asserted. Simpler fixtures (sample.h, qt-style.h, javadoc-style.h) do roundtrip and are covered by TestDoubleExtraction_Sample, TestDoubleExtraction_QtStyle, and TestDoubleExtraction_JavadocStyle

func TestSkeletonStore_ByteExact_JavadocSingleLine(t *testing.T) {
	input := "/** A Javadoc comment */\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "single-line javadoc comment should be byte-exact")
}

func TestSkeletonStore_ByteExact_CodeOnly(t *testing.T) {
	input := "int x = 0;\nint y = 1;\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "code-only content should be byte-exact")
}

func TestSkeletonStore_ByteExact_NoTrailingNewline(t *testing.T) {
	// Mirrors okapi's DoxygenFilter behaviour: the upstream filter
	// reads the source line-by-line and unconditionally appends
	// `linebreak` to every line, so the merged output always
	// terminates with a newline even when the source did not. Native
	// matches that for parity (closes qt-style.h byte-equal against
	// the okapi reference); without it the writer would emit fewer
	// bytes than okapi does.
	input := "/// Hello world"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input+"\n", output, "writer adds trailing newline to mirror okapi DoxygenFilter")
}

func TestSkeletonStore_ByteExact_EmptyInput(t *testing.T) {
	input := ""
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "empty input should produce empty output")
}

func TestSkeletonStore_ByteExact_TrailingComment(t *testing.T) {
	input := "int x; ///< A trailing comment\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "trailing ///< comment should be byte-exact")
}

func TestSkeletonStore_ByteExact_QtBlockComment(t *testing.T) {
	input := "/*! A Qt comment */\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "single-line Qt comment should be byte-exact")
}

func TestSkeletonStore_WithTranslation(t *testing.T) {
	input := "/// Hello World\nint x;\n/// Goodbye\n"
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := doxygen.NewReader()
	writer := doxygen.NewWriter()

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
				b.SetTargetRuns(locale, []model.Run{{Text: &model.TextRun{Text: "Bonjour le monde"}}})
			case "Goodbye":
				b.SetTargetRuns(locale, []model.Run{{Text: &model.TextRun{Text: "Au revoir"}}})
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Bonjour le monde")
	assert.Contains(t, output, "Au revoir")
	assert.Contains(t, output, "int x;")
	assert.NotContains(t, output, "Hello World")
	assert.NotContains(t, output, "Goodbye")
}
