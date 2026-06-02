package arb_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/arb"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// arbRoundtripWithSkeleton reads input through the ARB reader with a skeleton
// store wired, then writes it back through the writer fed the same store, with
// no translation applied. The skeleton path is what `kapi merge` uses (the
// returning file's blocks are spliced into the source-captured skeleton), so a
// byte-exact identity roundtrip here is the merge byte-exactness guarantee.
func arbRoundtripWithSkeleton(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := arb.NewReader()
	writer := arb.NewWriter()

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

// TestSkeletonStore_ByteExact_ARB locks the skeleton emit/consume path the ARB
// reader/writer perform for `kapi merge`. Real-fixture coverage of the
// non-skeleton (original-bytes) path lives in TestByteFaithfulRoundTrip and
// TestCorpusByteFaithfulRoundTrip; these controlled snippets pin the merge path.
//
// Byte-exactness limitations (deliberately excluded from this identity table):
//
//   - Values with JSON escapes whose canonical form differs from Dart's
//     JsonEncoder — e.g. an escaped forward slash ("\/"), an uppercase-hex
//     "é", or a "A"-style escape for a printable character. On the
//     skeleton path the value is re-encoded via encodeJSONString (Dart-faithful:
//     no slash escaping, lowercase \uXXXX only for control chars, literal UTF-8
//     otherwise), so such an input round-trips to its canonical form, not its
//     original bytes. The original-bytes writer path copies these verbatim; the
//     merge path intentionally normalizes them. All snippets below use already-
//     canonical encodings so identity holds.
func TestSkeletonStore_ByteExact_ARB(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input string
	}{
		{
			"plain_message",
			"{\n  \"appTitle\": \"Flutter Gallery\"\n}\n",
		},
		{
			"locale_and_description",
			"{\n  \"@@locale\": \"en\",\n  \"appTitle\": \"Flutter Gallery\",\n  \"@appTitle\": {\n    \"description\": \"The application title\"\n  }\n}\n",
		},
		{
			"multiple_globals",
			"{\n  \"@@locale\": \"en\",\n  \"@@last_modified\": \"2024-01-15T10:30:00.000Z\",\n  \"@@author\": \"Flutter Team\",\n  \"greeting\": \"Hello\"\n}\n",
		},
		{
			"icu_plural",
			"{\n  \"itemCount\": \"{count, plural, =0{No items} =1{One item} other{{count} items}}\",\n  \"@itemCount\": {\n    \"placeholders\": {\n      \"count\": {\n        \"type\": \"int\"\n      }\n    }\n  }\n}\n",
		},
		{
			"icu_select",
			"{\n  \"pronoun\": \"{gender, select, male{He} female{She} other{They}}\"\n}\n",
		},
		{
			"placeholder_interpolation",
			"{\n  \"welcome\": \"Hello {name}, welcome back!\",\n  \"@welcome\": {\n    \"placeholders\": {\n      \"name\": {\n        \"type\": \"String\"\n      }\n    }\n  }\n}\n",
		},
		{
			"multiple_keys",
			"{\n  \"@@locale\": \"en\",\n  \"hello\": \"Hello\",\n  \"goodbye\": \"Goodbye\",\n  \"yes\": \"Yes\",\n  \"no\": \"No\"\n}\n",
		},
		{
			"compact_single_line",
			"{\"@@locale\":\"en\",\"a\":\"x\",\"b\":\"y\"}",
		},
		{
			"unicode_literal",
			"{\n  \"greeting\": \"你好世界\",\n  \"emoji\": \"Hi \U0001F44B\"\n}\n",
		},
		{
			"standard_escapes",
			"{\n  \"tabbed\": \"col1\\tcol2\",\n  \"multiline\": \"line1\\nline2\",\n  \"quoted\": \"say \\\"hi\\\"\"\n}\n",
		},
		{
			"icu_quoted_brace",
			"{\n  \"price\": \"It costs '{'5'}' dollars\"\n}\n",
		},
		// NOTE: a leading UTF-8 BOM is intentionally NOT covered here. The ARB
		// reader's parseCatalog drives encoding/json.Decoder, which rejects a
		// leading BOM ("invalid character '\u00EF' looking for beginning of value")
		// before the skeleton path ever runs. That is a pre-existing reader
		// limitation orthogonal to skeleton support \u2014 a BOM-prefixed .arb cannot
		// be extracted today regardless of the writer path. (The whitespace-
		// preserving scanner does keep a BOM in a token prefix, so once the
		// reader tolerates a BOM the skeleton path would round-trip it.)
		{
			"leading_whitespace",
			"  \n{\n    \"appTitle\": \"Flutter Gallery\"\n}\n",
		},
		{
			"empty_message_value",
			"{\n  \"blank\": \"\",\n  \"filled\": \"text\"\n}\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.input, arbRoundtripWithSkeleton(t, tc.input),
				"skeleton store roundtrip should be byte-exact")
		})
	}
}

// TestSkeletonStore_WithTranslation_ARB exercises the re-encode path the
// byte-exact (untranslated) test skips: every other byte is replayed from the
// skeleton verbatim and only the translated message values change. Metadata
// (@@locale, @-attribute objects) and structure are untouched.
func TestSkeletonStore_WithTranslation_ARB(t *testing.T) {
	t.Parallel()
	input := "{\n  \"@@locale\": \"en\",\n  \"hello\": \"Hello\",\n  \"@hello\": {\n    \"description\": \"A greeting\"\n  },\n  \"goodbye\": \"Goodbye\"\n}\n"
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := arb.NewReader()
	writer := arb.NewWriter()

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

	want := "{\n  \"@@locale\": \"en\",\n  \"hello\": \"Bonjour\",\n  \"@hello\": {\n    \"description\": \"A greeting\"\n  },\n  \"goodbye\": \"Au revoir\"\n}\n"
	assert.Equal(t, want, buf.String(),
		"only message values change; metadata and structure replay verbatim")
}

// TestSkeletonStore_WithTranslation_PreservesICU verifies that translating the
// literal text around an ICU plural/placeholder construct leaves the protected
// ICU syntax byte-identical while replaying the rest of the document from the
// skeleton.
func TestSkeletonStore_WithTranslation_PreservesICU(t *testing.T) {
	t.Parallel()
	input := "{\n  \"itemCount\": \"You have {count, plural, =0{no items} other{{count} items}}\"\n}\n"
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := arb.NewReader()
	writer := arb.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b := p.Resource.(*model.Block)
		// Translate only the leading literal text run, preserving the ICU
		// placeholder run (which re-emits its captured Data verbatim).
		runs := make([]model.Run, 0, len(b.Source))
		for _, r := range b.Source {
			if r.Text != nil {
				runs = append(runs, model.Run{Text: &model.TextRun{Text: "Vous avez "}})
			} else {
				runs = append(runs, r)
			}
		}
		b.SetTargetRuns(locale, runs)
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()

	want := "{\n  \"itemCount\": \"Vous avez {count, plural, =0{no items} other{{count} items}}\"\n}\n"
	assert.Equal(t, want, buf.String(),
		"ICU construct must survive translation of surrounding literal text")
}
