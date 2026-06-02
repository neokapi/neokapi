package androidxml_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/androidxml"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// skeletonRoundtripAndroid reads input through the androidxml reader with a
// skeleton store wired, then writes it back through the writer fed the same
// store, with no translation applied. This is exactly the path `kapi merge`
// drives: the source is re-read and the captured skeleton is handed to the
// WRITER, which splices each block's runs into the SkeletonRef slots. A
// byte-exact identity roundtrip here is the merge byte-exactness guarantee for
// Android string resources — independent of the androidxml.original layer
// property (which merge discards).
func skeletonRoundtripAndroid(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := androidxml.NewReader()
	writer := androidxml.NewWriter()

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

// TestSkeletonStore_ByteExact_Android pins the skeleton emit/consume path the
// reader/writer use on `kapi merge`. Each snippet exercises a distinct part of
// the Android resource surface; an untranslated roundtrip must reproduce the
// source bytes exactly (prolog, comments, whitespace, attribute order, entity
// encoding, Android backslash escapes, CDATA, and xliff:g markup). Real-fixture
// coverage of the non-skeleton path lives in TestByteFaithfulRoundTrip; these
// controlled snippets lock the merge path.
func TestSkeletonStore_ByteExact_Android(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input string
	}{
		{
			"plain_string",
			"<resources>\n    <string name=\"greeting\">Hello, world!</string>\n</resources>\n",
		},
		{
			"xliff_g_placeholder",
			"<resources xmlns:xliff=\"urn:oasis:names:tc:xliff:document:1.2\">\n" +
				"    <string name=\"file_size\">Used <xliff:g id=\"size\" example=\"1.2 GB\">%1$s</xliff:g> of space</string>\n" +
				"</resources>\n",
		},
		{
			"printf_placeholders",
			"<resources>\n    <string name=\"items\">You have %1$d items (%2$.2f total)</string>\n</resources>\n",
		},
		{
			// Android backslash escapes (\' \") survive verbatim because the
			// reader keeps them in the block text (see entities.go). The XML
			// entities &amp; and &lt; round-trip because the writer's encodeText
			// re-escapes '&' and '<'. NOTE: a source &gt; does NOT round-trip on
			// the skeleton (re-render) path — encodeText leaves '>' bare per XML
			// 1.0 §2.4 (a bare '>' is legal in character data and Android files
			// routinely carry one), so the limitation is intentional and shared
			// with the generic xml writer. &gt; is therefore omitted here.
			"escaped_apostrophe_and_entity",
			"<resources>\n" +
				"    <string name=\"escaped\">It\\'s a \\\"quoted\\\" word</string>\n" +
				"    <string name=\"entities\">Fish &amp; Chips &lt;tasty</string>\n" +
				"</resources>\n",
		},
		{
			"cdata_html",
			"<resources>\n    <string name=\"rich_html\"><![CDATA[Read our <a href=\"https://example.com\">terms &amp; conditions</a> first.]]></string>\n</resources>\n",
		},
		{
			"string_array",
			"<resources>\n    <string-array name=\"weekdays\">\n" +
				"        <item>Monday</item>\n        <item>Tuesday</item>\n        <item>Wednesday</item>\n" +
				"    </string-array>\n</resources>\n",
		},
		{
			"plurals",
			"<resources>\n    <plurals name=\"cart_items\">\n" +
				"        <item quantity=\"one\">%1$d item</item>\n        <item quantity=\"other\">%1$d items</item>\n" +
				"    </plurals>\n</resources>\n",
		},
		{
			"non_translatable_entry",
			"<resources>\n    <string name=\"debug\" translatable=\"false\">DEBUG_BUILD_MARKER</string>\n</resources>\n",
		},
		{
			"non_translatable_array",
			"<resources>\n    <string-array name=\"flags\" translatable=\"false\">\n" +
				"        <item>internal_flag_a</item>\n        <item>internal_flag_b</item>\n" +
				"    </string-array>\n</resources>\n",
		},
		{
			"resource_reference",
			"<resources>\n    <string name=\"alias\">@string/app_name</string>\n</resources>\n",
		},
		{
			"comment_and_prolog",
			"<?xml version=\"1.0\" encoding=\"utf-8\"?>\n" +
				"<!-- Shown on the welcome screen. -->\n" +
				"<resources>\n    <!-- A greeting. -->\n    <string name=\"greeting\">Hello</string>\n</resources>\n",
		},
		{
			"mixed_full_document",
			"<?xml version=\"1.0\" encoding=\"utf-8\"?>\n" +
				"<resources xmlns:xliff=\"urn:oasis:names:tc:xliff:document:1.2\">\n\n" +
				"    <!-- greeting -->\n    <string name=\"greeting\">Hello, world!</string>\n\n" +
				"    <string name=\"debug\" translatable=\"false\">MARKER</string>\n\n" +
				"    <plurals name=\"cart\">\n        <item quantity=\"one\">%1$d item</item>\n        <item quantity=\"other\">%1$d items</item>\n    </plurals>\n\n" +
				"    <string-array name=\"weekdays\">\n        <item>Monday</item>\n        <item>Tuesday</item>\n    </string-array>\n\n" +
				"</resources>\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.input, skeletonRoundtripAndroid(t, tc.input),
				"skeleton store roundtrip should be byte-exact")
		})
	}
}

// TestSkeletonStore_WithTranslation_Android exercises the re-encode path the
// byte-exact (untranslated) test skips: every byte outside a translatable value
// replays from the skeleton verbatim and only the value content of translated
// blocks changes. It covers a plain <string>, an array item, and a plurals item
// so each value-bearing element type's splice is verified.
func TestSkeletonStore_WithTranslation_Android(t *testing.T) {
	t.Parallel()
	input := "<resources>\n" +
		"    <string name=\"greeting\">Hello</string>\n" +
		"    <string name=\"farewell\">Goodbye</string>\n" +
		"    <string-array name=\"weekdays\">\n        <item>Monday</item>\n        <item>Tuesday</item>\n    </string-array>\n" +
		"    <plurals name=\"cart\">\n        <item quantity=\"one\">%1$d item</item>\n        <item quantity=\"other\">%1$d items</item>\n    </plurals>\n" +
		"</resources>\n"
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := androidxml.NewReader()
	writer := androidxml.NewWriter()

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
		switch b.SourceText() {
		case "Hello":
			b.SetTargetText(locale, "Bonjour")
		case "Goodbye":
			b.SetTargetText(locale, "Au revoir")
		case "Monday":
			b.SetTargetText(locale, "Lundi")
		case "Tuesday":
			b.SetTargetText(locale, "Mardi")
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()

	want := "<resources>\n" +
		"    <string name=\"greeting\">Bonjour</string>\n" +
		"    <string name=\"farewell\">Au revoir</string>\n" +
		"    <string-array name=\"weekdays\">\n        <item>Lundi</item>\n        <item>Mardi</item>\n    </string-array>\n" +
		// Plurals were not translated, so their source content replays verbatim
		// (only changed values differ; everything else is byte-exact skeleton).
		"    <plurals name=\"cart\">\n        <item quantity=\"one\">%1$d item</item>\n        <item quantity=\"other\">%1$d items</item>\n    </plurals>\n" +
		"</resources>\n"
	assert.Equal(t, want, buf.String(), "only translated inner content should change")
}
