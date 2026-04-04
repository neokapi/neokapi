package doxygen_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/doxygen"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func snippetRoundtripWithSkeleton(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

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

func TestSkeletonStore_ByteExact_MultiLineComment(t *testing.T) {
	input := "/// First line\n/// Second line\n/// Third line\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "multi-line /// comment should be byte-exact")
}

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
	input := "/// Hello world"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "no trailing newline should be preserved")
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
				b.Targets[locale] = []*model.Segment{{ID: "s1", Content: model.NewFragment("Bonjour le monde")}}
			case "Goodbye":
				b.Targets[locale] = []*model.Segment{{ID: "s1", Content: model.NewFragment("Au revoir")}}
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
