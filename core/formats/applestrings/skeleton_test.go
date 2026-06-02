package applestrings_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	applestrings "github.com/neokapi/neokapi/core/formats/applestrings"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// skeletonRoundtripAppleStrings reads input through the applestrings reader with a
// skeleton store wired, then writes it back through the writer fed the same
// store, with no translation applied. The skeleton path is exactly what
// `kapi merge` uses (the returning file's blocks are spliced into the
// source-captured skeleton; the synthetic layer carries no original bytes), so a
// byte-exact identity roundtrip here is the merge byte-exactness guarantee.
//
// The URI controls kind detection; pass a *.strings / *.stringsdict URI so the
// reader dispatches to the right sub-format (testutil.RawDocFromString uses a
// non-suffixed URI and relies on content sniffing, which also works here).
func skeletonRoundtripAppleStrings(t *testing.T, uri, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := applestrings.NewReader()
	writer := applestrings.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	doc := &model.RawDocument{
		URI:          uri,
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       nopReader([]byte(input)),
	}
	require.NoError(t, reader.Open(ctx, doc))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()
	require.Positive(t, store.EntriesWritten(), "reader must emit skeleton entries")

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()
	return buf.String()
}

// TestSkeletonStore_ByteExact_Strings locks the .strings skeleton emit/consume
// path the merge workflow relies on. Each snippet decodes→re-encodes to itself,
// so identity is byte-exact. Escapes outside the encoder's repertoire (\U…, \/,
// \a/\b/\f/\v/\0) are intentionally excluded — they decode to forms the writer
// re-emits canonically (UTF-8, '/', the raw control char), which is faithful but
// not byte-identical; see the documented limitation in the writer.
func TestSkeletonStore_ByteExact_Strings(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input string
	}{
		{"single_entry", "\"greeting\" = \"Hello\";\n"},
		{"multiple_entries", "\"a\" = \"one\";\n\"b\" = \"two\";\n"},
		{"block_comment", "/* Greeting shown on launch */\n\"greeting\" = \"Hello\";\n"},
		{"line_comment", "// inline note\n\"ok\" = \"OK\";\n"},
		{"escaped_quote", "\"share\" = \"He said \\\"hi\\\"\";\n"},
		{"escaped_newline", "\"summary\" = \"line one\\nline two\";\n"},
		{"escaped_tab", "\"summary\" = \"col1\\tcol2\";\n"},
		{"escaped_backslash", "\"path\" = \"a\\\\b\";\n"},
		{"placeholder_at", "\"greeting\" = \"Hello, %@!\";\n"},
		{"placeholder_d", "\"count\" = \"%d items\";\n"},
		{"positional", "\"share\" = \"%1$@ shared %2$@\";\n"},
		{"percent_literal", "\"progress\" = \"%d%% done\";\n"},
		{"empty_value", "\"empty\" = \"\";\n"},
		{"no_trailing_newline", "\"k\" = \"v\";"},
		{"extra_whitespace", "\n\n\"k\"  =  \"v\" ;\n\n"},
		{"unicode_text", "\"hi\" = \"你好世界\";\n"},
		{"multi_with_comments", "/* one */\n\"a\" = \"1\";\n/* two */\n\"b\" = \"2\";\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := skeletonRoundtripAppleStrings(t, "Localizable.strings", tc.input)
			assert.Equal(t, tc.input, out, "skeleton store roundtrip should be byte-exact")
		})
	}
}

// TestSkeletonStore_ByteExact_Stringsdict locks the .stringsdict skeleton path.
// The plist DOCTYPE, keys, whitespace and entity encoding all replay verbatim
// from skeleton text; only the translatable <string> leaves
// (NSStringLocalizedFormatKey + CLDR plural values) go through the re-encode
// step, and the snippets are chosen so that re-encode is identity (plain text +
// %#@var@ / %d placeholders; no &quot;/&apos;/numeric-ref-only values).
func TestSkeletonStore_ByteExact_Stringsdict(t *testing.T) {
	t.Parallel()
	const full = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>%d files selected</key>
	<dict>
		<key>NSStringLocalizedFormatKey</key>
		<string>%#@files@</string>
		<key>files</key>
		<dict>
			<key>NSStringFormatSpecTypeKey</key>
			<string>NSStringPluralRuleType</string>
			<key>NSStringFormatValueTypeKey</key>
			<string>d</string>
			<key>one</key>
			<string>%d file selected</string>
			<key>other</key>
			<string>%d files selected</string>
		</dict>
	</dict>
</dict>
</plist>
`
	const entityAmp = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>%d items</key>
	<dict>
		<key>NSStringLocalizedFormatKey</key>
		<string>%#@items@</string>
		<key>items</key>
		<dict>
			<key>NSStringFormatSpecTypeKey</key>
			<string>NSStringPluralRuleType</string>
			<key>NSStringFormatValueTypeKey</key>
			<string>d</string>
			<key>one</key>
			<string>%d item &amp; more</string>
			<key>other</key>
			<string>%d items &amp; more</string>
		</dict>
	</dict>
</dict>
</plist>
`
	cases := []struct {
		name  string
		input string
	}{
		{"full_plural_dict", full},
		{"amp_entity_value", entityAmp},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := skeletonRoundtripAppleStrings(t, "Localizable.stringsdict", tc.input)
			assert.Equal(t, tc.input, out, "skeleton store roundtrip should be byte-exact")
		})
	}
}

// TestSkeletonStore_WithTranslation_Strings exercises the .strings re-encode
// path the byte-exact (untranslated) test skips: every other byte replays from
// the skeleton verbatim and only the translated values change.
func TestSkeletonStore_WithTranslation_Strings(t *testing.T) {
	t.Parallel()
	input := "/* Greeting */\n\"greeting\" = \"Hello\";\n\"farewell\" = \"Goodbye\";\n"
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := applestrings.NewReader()
	writer := applestrings.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	doc := &model.RawDocument{URI: "Localizable.strings", SourceLocale: model.LocaleEnglish, Encoding: "UTF-8", Reader: nopReader([]byte(input))}
	require.NoError(t, reader.Open(ctx, doc))
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

	assert.Equal(t, "/* Greeting */\n\"greeting\" = \"Bonjour\";\n\"farewell\" = \"Au revoir\";\n", buf.String())
}

// TestSkeletonStore_ByteExact_StringsUTF8BOM verifies a leading UTF-8 BOM is
// preserved across the skeleton roundtrip. The BOM is kept in the decoded
// content (the lexer skips it) and replays as the leading skeleton text; the
// final UTF-8 encode pass leaves it untouched.
func TestSkeletonStore_ByteExact_StringsUTF8BOM(t *testing.T) {
	t.Parallel()
	input := "\uFEFF\"greeting\" = \"Hello\";\n" // \uFEFF encodes to the UTF-8 BOM bytes
	out := skeletonRoundtripAppleStrings(t, "Localizable.strings", input)
	assert.Equal(t, input, out, "leading UTF-8 BOM must survive the skeleton roundtrip")
}

// TestSkeletonStore_ByteExact_StringsUTF16LE verifies a UTF-16LE (BOM) .strings
// input round-trips byte-for-byte through the skeleton path: the reader
// transcodes to UTF-8 for the skeleton, and the writer's final encode step
// re-encodes the UTF-8 output back to UTF-16LE with its BOM, so the bytes match.
func TestSkeletonStore_ByteExact_StringsUTF16LE(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	src := "\"hello\" = \"world\";\n"
	utf16le := []byte{0xFF, 0xFE}
	for _, r := range src {
		utf16le = append(utf16le, byte(r), 0x00)
	}

	reader := applestrings.NewReader()
	writer := applestrings.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	doc := &model.RawDocument{URI: "u.strings", SourceLocale: model.LocaleEnglish, Encoding: "UTF-8", Reader: nopReader(utf16le)}
	require.NoError(t, reader.Open(ctx, doc))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()

	assert.Equal(t, utf16le, buf.Bytes(), "UTF-16LE input must round-trip byte-for-byte via the skeleton path")
}

// TestSkeletonStore_WithTranslation_Stringsdict exercises the .stringsdict
// re-encode path: only the changed plural <string> value is rewritten; the
// DOCTYPE, keys, whitespace and untouched siblings replay verbatim.
func TestSkeletonStore_WithTranslation_Stringsdict(t *testing.T) {
	t.Parallel()
	const input = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>%d files selected</key>
	<dict>
		<key>NSStringLocalizedFormatKey</key>
		<string>%#@files@</string>
		<key>files</key>
		<dict>
			<key>NSStringFormatSpecTypeKey</key>
			<string>NSStringPluralRuleType</string>
			<key>NSStringFormatValueTypeKey</key>
			<string>d</string>
			<key>one</key>
			<string>%d file selected</string>
			<key>other</key>
			<string>%d files selected</string>
		</dict>
	</dict>
</dict>
</plist>
`
	const want = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>%d files selected</key>
	<dict>
		<key>NSStringLocalizedFormatKey</key>
		<string>%#@files@</string>
		<key>files</key>
		<dict>
			<key>NSStringFormatSpecTypeKey</key>
			<string>NSStringPluralRuleType</string>
			<key>NSStringFormatValueTypeKey</key>
			<string>d</string>
			<key>one</key>
			<string>%d fichier sélectionné</string>
			<key>other</key>
			<string>%d files selected</string>
		</dict>
	</dict>
</dict>
</plist>
`
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := applestrings.NewReader()
	writer := applestrings.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	doc := &model.RawDocument{URI: "Localizable.stringsdict", SourceLocale: model.LocaleEnglish, Encoding: "UTF-8", Reader: nopReader([]byte(input))}
	require.NoError(t, reader.Open(ctx, doc))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			if b.Name == "%d files selected/files/one" {
				b.SetTargetText(locale, "%d fichier sélectionné")
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()

	assert.Equal(t, want, buf.String())
}
