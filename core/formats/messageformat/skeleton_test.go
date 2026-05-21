package messageformat_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/messageformat"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func snippetRoundtripWithSkeleton(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := messageformat.NewReader()
	writer := messageformat.NewWriter()

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

func TestSkeletonStore_ByteExact_SimpleLine(t *testing.T) {
	input := "Hello world"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "simple line roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_MultipleLines(t *testing.T) {
	input := "Hello world\nGoodbye world"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "multiple lines should be byte-exact")
}

func TestSkeletonStore_ByteExact_TrailingNewline(t *testing.T) {
	input := "Hello world\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "trailing newline should be preserved")
}

func TestSkeletonStore_ByteExact_EmptyInput(t *testing.T) {
	input := ""
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "empty input should produce empty output")
}

func TestSkeletonStore_ByteExact_PluralPattern(t *testing.T) {
	input := "{count, plural, one {# item} other {# items}}"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "plural pattern should be byte-exact")
}

func TestSkeletonStore_ByteExact_CRLF(t *testing.T) {
	input := "Hello\r\nWorld"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "CRLF should be preserved")
}

func TestSkeletonStore_ByteExact_EmptyLines(t *testing.T) {
	input := "Hello\n\nWorld"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "empty lines should be preserved")
}

// okapi: MessageFormatFilterTest#testDeepEmbeddedSubfilterJson
// okapi: MessageFormatFilterTest#testDeepEmbeddedSubfilterYaml
func TestSkeletonStore_ByteExact_DeepEmbedded(t *testing.T) {
	// Okapi's testDeepEmbeddedSubfilterJson/Yaml run this deeply-nested
	// select→plural→plural pattern through the messageformat subfilter and
	// assert byte-exact identity roundtrip (no translation). The JSON/YAML
	// wrapper is the parent filter's concern; the messageformat-specific
	// behavior under test is that this pattern roundtrips unchanged. The
	// native analog feeds the inline pattern directly and checks byte-exact
	// skeleton roundtrip.
	input := "{gender, select, male {{num_apples, plural, one {He has {num_oranges, plural, one {an orange} other {# oranges}}} other {He has # apples}}} female {{num_apples, plural, one {She has an apple} other {She has # apples}}} other {{num_apples, plural, one {They have an apple} other {They have # apples}}}}"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "deeply nested select/plural pattern should roundtrip byte-exact")
}

func TestSkeletonStore_WithTranslation(t *testing.T) {
	input := "Hello World\nGoodbye"
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := messageformat.NewReader()
	writer := messageformat.NewWriter()

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
				b.Targets[locale] = []*model.Segment{{ID: "s1", Runs: []model.Run{{Text: &model.TextRun{Text: "Bonjour le monde"}}}}}
			case "Goodbye":
				b.Targets[locale] = []*model.Segment{{ID: "s1", Runs: []model.Run{{Text: &model.TextRun{Text: "Au revoir"}}}}}
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	assert.Equal(t, "Bonjour le monde\nAu revoir", buf.String())
}
