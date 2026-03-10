package wiki_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/formats/wiki"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func snippetRoundtripWithSkeleton(t *testing.T, input string) string {
	t.Helper()
	ctx := context.Background()

	reader := wiki.NewReader()
	writer := wiki.NewWriter()

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

func TestSkeletonStore_ByteExact_SimpleParagraph(t *testing.T) {
	input := "Hello world"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "simple paragraph roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_MultipleParagraphs(t *testing.T) {
	input := "First paragraph\n\nSecond paragraph"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "multiple paragraphs should be byte-exact")
}

func TestSkeletonStore_ByteExact_TrailingNewline(t *testing.T) {
	input := "Hello world\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "trailing newline should be preserved")
}

func TestSkeletonStore_ByteExact_Header(t *testing.T) {
	input := "== My Header ==\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "header should be byte-exact")
}

func TestSkeletonStore_ByteExact_HeaderAndParagraph(t *testing.T) {
	input := "== Title ==\nSome text here\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "header and paragraph should be byte-exact")
}

func TestSkeletonStore_ByteExact_EmptyInput(t *testing.T) {
	input := ""
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "empty input should produce empty output")
}

func TestSkeletonStore_ByteExact_BlankLines(t *testing.T) {
	input := "First\n\n\nSecond"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "multiple blank lines should be preserved")
}

func TestSkeletonStore_WithTranslation(t *testing.T) {
	input := "Hello World\n\nGoodbye\n"
	ctx := context.Background()
	locale := model.LocaleID("fr")

	reader := wiki.NewReader()
	writer := wiki.NewWriter()

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

	assert.Equal(t, "Bonjour le monde\n\nAu revoir\n", buf.String())
}
