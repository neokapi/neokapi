package transtable_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/transtable"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func snippetRoundtripWithSkeleton(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := transtable.NewReader()
	writer := transtable.NewWriter()

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

func TestSkeletonStore_ByteExact_SimpleEntry(t *testing.T) {
	input := "greeting\tHello"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "single entry roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_MultipleEntries(t *testing.T) {
	input := "greeting\tHello\nfarewell\tGoodbye"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "multiple entries should be byte-exact")
}

func TestSkeletonStore_ByteExact_CRLF(t *testing.T) {
	input := "greeting\tHello\r\nfarewell\tGoodbye"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "CRLF line endings should be preserved byte-exact")
}

func TestSkeletonStore_ByteExact_Comments(t *testing.T) {
	input := "# This is a comment\ngreeting\tHello"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "comments should be preserved byte-exact")
}

func TestSkeletonStore_ByteExact_EmptyLines(t *testing.T) {
	input := "greeting\tHello\n\nfarewell\tGoodbye"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "empty lines should be preserved byte-exact")
}

func TestSkeletonStore_ByteExact_TrailingNewline(t *testing.T) {
	input := "greeting\tHello\nfarewell\tGoodbye\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "trailing newline should be preserved")
}

func TestSkeletonStore_ByteExact_NoTrailingNewline(t *testing.T) {
	input := "greeting\tHello\nfarewell\tGoodbye"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "no trailing newline should be preserved")
}

func TestSkeletonStore_ByteExact_CommentAndEmptyLines(t *testing.T) {
	input := "# Header comment\n\ngreeting\tHello\n# Section\nfarewell\tGoodbye"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "mixed comments and empty lines should be byte-exact")
}

func TestSkeletonStore_ByteExact_CRLFTrailingNewline(t *testing.T) {
	input := "greeting\tHello\r\nfarewell\tGoodbye\r\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "CRLF with trailing newline should be byte-exact")
}

func TestSkeletonStore_WithTranslation(t *testing.T) {
	input := "greeting\tHello World\nfarewell\tGoodbye"
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := transtable.NewReader()
	writer := transtable.NewWriter()

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

	assert.Equal(t, "greeting\tBonjour le monde\nfarewell\tAu revoir", buf.String())
}

func TestSkeletonStore_WithTranslation_CRLF(t *testing.T) {
	input := "greeting\tHello\r\nfarewell\tWorld"
	ctx := t.Context()
	locale := model.LocaleID("de")

	reader := transtable.NewReader()
	writer := transtable.NewWriter()

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
				b.Targets[locale] = []*model.Segment{{ID: "s1", Content: model.NewFragment("Hallo")}}
			case "World":
				b.Targets[locale] = []*model.Segment{{ID: "s1", Content: model.NewFragment("Welt")}}
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	assert.Equal(t, "greeting\tHallo\r\nfarewell\tWelt", buf.String())
}
