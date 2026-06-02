package i18next_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/i18next"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// roundtripWithSkeleton reads input through the i18next reader with a skeleton
// store wired, then writes it back through the writer fed the same store, with
// no translation applied. The skeleton path is what `kapi merge` uses (the
// returning file's blocks are spliced into the source-captured skeleton), so a
// byte-exact identity roundtrip here is the merge byte-exactness guarantee.
func roundtripWithSkeleton(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := i18next.NewReader()
	writer := i18next.NewWriter()

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

// TestSkeletonStore_ByteExact_I18next locks the skeleton emit/consume path the
// inner JSON reader/writer perform once the i18next wrapper forwards the store.
// Real-fixture coverage of the non-skeleton path lives in
// TestCorpusByteFaithfulRoundTrip; these controlled snippets pin the merge path.
func TestSkeletonStore_ByteExact_I18next(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input string
	}{
		{"simple", "{\n  \"greeting\": \"Hello\"\n}\n"},
		{"nested_namespace", "{\n  \"common\": {\n    \"ok\": \"OK\",\n    \"cancel\": \"Cancel\"\n  }\n}\n"},
		{"plurals_v4", "{\n  \"item_one\": \"{{count}} item\",\n  \"item_other\": \"{{count}} items\"\n}\n"},
		{"interpolation", "{\n  \"welcome\": \"Hello {{name}}\"\n}\n"},
		{"context", "{\n  \"friend_male\": \"A boyfriend\",\n  \"friend_female\": \"A girlfriend\"\n}\n"},
		{"compact", "{\"a\":\"x\",\"b\":\"y\"}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input),
				"skeleton store roundtrip should be byte-exact")
		})
	}
}

// TestSkeletonStore_WithTranslation_I18next exercises the re-encode path the
// byte-exact (untranslated) test skips: every other byte is replayed from the
// skeleton verbatim and only the translated values change.
func TestSkeletonStore_WithTranslation_I18next(t *testing.T) {
	t.Parallel()
	input := "{\n  \"greeting\": \"Hello\",\n  \"farewell\": \"Goodbye\"\n}\n"
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := i18next.NewReader()
	writer := i18next.NewWriter()

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
			switch b.SourceText() {
			case "Hello":
				b.SetTargetText(locale, "Bonjour")
			case "Goodbye":
				b.SetTargetText(locale, "Au revoir")
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()

	assert.Equal(t, "{\n  \"greeting\": \"Bonjour\",\n  \"farewell\": \"Au revoir\"\n}\n", buf.String())
}
