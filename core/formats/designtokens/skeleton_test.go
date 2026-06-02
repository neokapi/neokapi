package designtokens_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/designtokens"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// roundtripWithSkeleton reads input through the design-tokens reader with a
// skeleton store wired, then writes it back through the writer fed the same
// store, with no translation applied. The skeleton path is what `kapi merge`
// uses, so a byte-exact identity roundtrip here is the merge guarantee. Only
// $description values are translatable; token $value/$type/structure are
// replayed from the skeleton verbatim.
func roundtripWithSkeleton(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := designtokens.NewReader()
	writer := designtokens.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()
	require.Positive(t, store.EntriesWritten(), "reader must emit skeleton entries")

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()
	return buf.String()
}

// TestSkeletonStore_ByteExact_DesignTokens locks the skeleton emit/consume path
// once the design-tokens wrapper forwards the store to the inner JSON
// reader/writer. Non-translatable token values (colors, dimensions, references,
// arrays) and the entire structure must replay byte-for-byte; only $description
// is a translatable block.
func TestSkeletonStore_ByteExact_DesignTokens(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input string
	}{
		{
			"token_with_description",
			"{\n  \"color\": {\n    \"primary\": {\n      \"$value\": \"#0d6efd\",\n      \"$type\": \"color\",\n      \"$description\": \"Primary brand color\"\n    }\n  }\n}\n",
		},
		{
			"group_and_token_description",
			"{\n  \"color\": {\n    \"$description\": \"Brand palette\",\n    \"primary\": {\n      \"$value\": \"#0d6efd\",\n      \"$type\": \"color\",\n      \"$description\": \"Primary brand color\"\n    }\n  }\n}\n",
		},
		{
			"no_description",
			"{\n  \"spacing\": {\n    \"small\": {\n      \"$value\": \"4px\",\n      \"$type\": \"dimension\"\n    }\n  }\n}\n",
		},
		{
			"reference_and_array_values",
			"{\n  \"font\": {\n    \"family\": {\n      \"$value\": [\"Inter\", \"sans-serif\"],\n      \"$type\": \"fontFamily\",\n      \"$description\": \"Body font stack\"\n    }\n  },\n  \"color\": {\n    \"text\": {\n      \"$value\": \"{color.primary}\",\n      \"$type\": \"color\"\n    }\n  }\n}\n",
		},
		{
			"compact",
			"{\"t\":{\"$value\":\"4px\",\"$type\":\"dimension\",\"$description\":\"Spacer\"}}",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input),
				"skeleton store roundtrip should be byte-exact")
		})
	}
}

// TestSkeletonStore_WithTranslation_DesignTokens exercises the re-encode path:
// only the $description value changes; the token $value/$type and all structure
// are replayed from the skeleton verbatim.
func TestSkeletonStore_WithTranslation_DesignTokens(t *testing.T) {
	t.Parallel()
	input := "{\n  \"color\": {\n    \"primary\": {\n      \"$value\": \"#0d6efd\",\n      \"$type\": \"color\",\n      \"$description\": \"Primary brand color\"\n    }\n  }\n}\n"
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := designtokens.NewReader()
	writer := designtokens.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			if b.SourceText() == "Primary brand color" {
				b.SetTargetText(locale, "Couleur de marque principale")
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()

	assert.Equal(t,
		"{\n  \"color\": {\n    \"primary\": {\n      \"$value\": \"#0d6efd\",\n      \"$type\": \"color\",\n      \"$description\": \"Couleur de marque principale\"\n    }\n  }\n}\n",
		buf.String())
}
