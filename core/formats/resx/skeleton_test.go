package resx_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/resx"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// roundtripSkeletonRESX reads input through the RESX reader with a skeleton
// store wired, then writes it back through the writer fed the same store, with
// no translation applied. The skeleton path is what `kapi merge` uses: at merge
// time the source is re-read (no skeleton), the writer opens the
// extract-captured skeleton, and the returning blocks are spliced into it. A
// byte-exact identity round-trip here is therefore the merge byte-exactness
// guarantee.
func roundtripSkeletonRESX(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := resx.NewReader()
	writer := resx.NewWriter()

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

// resxHeader is the standard ResX 2.0 prolog: comment + embedded xsd schema +
// the four resheaders. Reused across the byte-exact snippets so each case can
// add just the <data> entries it exercises while keeping a realistic document
// shape (the header bytes must all round-trip verbatim through the skeleton).
const resxHeader = `<?xml version="1.0" encoding="utf-8"?>
<root>
  <!--
    Microsoft ResX Schema
  -->
  <xsd:schema id="root" xmlns="" xmlns:xsd="http://www.w3.org/2001/XMLSchema" xmlns:msdata="urn:schemas-microsoft-com:xml-msdata">
    <xsd:element name="root" msdata:IsDataSet="true" />
  </xsd:schema>
  <resheader name="resmimetype">
    <value>text/microsoft-resx</value>
  </resheader>
  <resheader name="version">
    <value>2.0</value>
  </resheader>
  <resheader name="reader">
    <value>System.Resources.ResXResourceReader, System.Windows.Forms, Version=4.0.0.0, Culture=neutral, PublicKeyToken=b77a5c561934e089</value>
  </resheader>
  <resheader name="writer">
    <value>System.Resources.ResXResourceWriter, System.Windows.Forms, Version=4.0.0.0, Culture=neutral, PublicKeyToken=b77a5c561934e089</value>
  </resheader>
`

// TestSkeletonStore_ByteExact_RESX locks the skeleton emit/consume path that
// `kapi merge` relies on for RESX. Each snippet is fed through the reader (with
// a skeleton store) and the writer (fed the same store) with no translation; the
// output must be byte-identical to the input. The header boilerplate, the
// embedded schema, typed/binary <data>, comments, entities, name-reference
// entries, and whitespace must all replay verbatim — only translatable <value>
// inner content is mediated by a ref, and an untranslated ref re-encodes to the
// original bytes.
func TestSkeletonStore_ByteExact_RESX(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input string
	}{
		{
			name: "full_document_with_header_and_schema",
			input: resxHeader + `  <data name="GreetingText" xml:space="preserve">
    <value>Hello, world!</value>
    <comment>Shown on the welcome screen.</comment>
  </data>
</root>
`,
		},
		{
			name: "string_with_comment",
			input: resxHeader + `  <data name="SaveButton" xml:space="preserve">
    <value>Save</value>
    <comment>Button on the toolbar.</comment>
  </data>
</root>
`,
		},
		{
			name: "typed_non_translatable_data",
			input: resxHeader + `  <data name="AppIcon" type="System.Drawing.Bitmap, System.Drawing" mimetype="application/x-microsoft.net.object.bytearray.base64">
    <value>iVBORw0KGgoAAAANSU==</value>
  </data>
  <data name="WindowTitle" xml:space="preserve">
    <value>Main Window</value>
  </data>
</root>
`,
		},
		{
			name: "value_needing_entity_escaping",
			input: resxHeader + `  <data name="WithEntities" xml:space="preserve">
    <value>Fish &amp; Chips &lt;tasty&gt;</value>
  </data>
</root>
`,
		},
		{
			name: "name_reference_entry",
			input: resxHeader + `  <data name="&gt;&gt;AppIcon.Name" xml:space="preserve">
    <value>AppIcon</value>
  </data>
  <data name="CloseButton.Text" xml:space="preserve">
    <value>Close</value>
  </data>
</root>
`,
		},
		{
			name: "dotnet_placeholder",
			input: resxHeader + `  <data name="ItemCount" xml:space="preserve">
    <value>You have {0} items in your cart.</value>
    <comment>{0} is replaced with the number of items.</comment>
  </data>
</root>
`,
		},
		{
			name: "multiline_value",
			input: resxHeader + `  <data name="MultiLine" xml:space="preserve">
    <value>First line
Second line</value>
  </data>
</root>
`,
		},
		{
			name: "multiple_entries",
			input: resxHeader + `  <data name="One" xml:space="preserve">
    <value>First</value>
  </data>
  <data name="Two" xml:space="preserve">
    <value>Second</value>
    <comment>The second one.</comment>
  </data>
  <data name="Three" xml:space="preserve">
    <value>Third &amp; final</value>
  </data>
</root>
`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.input, roundtripSkeletonRESX(t, tc.input),
				"skeleton store round-trip should be byte-exact")
		})
	}
}

// TestSkeletonStore_WithTranslation_RESX exercises the re-encode path the
// byte-exact (untranslated) test skips: every other byte is replayed from the
// skeleton verbatim and only the translated <value> inner content changes. It
// also confirms a value needing escaping is XML-encoded on write and that a
// .NET placeholder re-emits verbatim through the ref.
func TestSkeletonStore_WithTranslation_RESX(t *testing.T) {
	t.Parallel()
	input := resxHeader + `  <data name="GreetingText" xml:space="preserve">
    <value>Hello, world!</value>
    <comment>Shown on the welcome screen.</comment>
  </data>
  <data name="SaveButton" xml:space="preserve">
    <value>Save</value>
  </data>
  <data name="ItemCount" xml:space="preserve">
    <value>You have {0} items in your cart.</value>
  </data>
</root>
`
	ctx := t.Context()
	locale := model.LocaleID("de")

	reader := resx.NewReader()
	writer := resx.NewWriter()

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
		switch b.Name {
		case "GreetingText":
			// A translation needing entity escaping on write.
			b.SetTargetText(locale, "Hallo & <Welt>")
		case "ItemCount":
			// Keep the {0} placeholder so it re-emits verbatim.
			b.SetTargetText(locale, "Sie haben {0} Artikel im Warenkorb.")
		}
		// SaveButton left untranslated → re-encodes to its source value.
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()

	want := resxHeader + `  <data name="GreetingText" xml:space="preserve">
    <value>Hallo &amp; &lt;Welt&gt;</value>
    <comment>Shown on the welcome screen.</comment>
  </data>
  <data name="SaveButton" xml:space="preserve">
    <value>Save</value>
  </data>
  <data name="ItemCount" xml:space="preserve">
    <value>Sie haben {0} Artikel im Warenkorb.</value>
  </data>
</root>
`
	assert.Equal(t, want, buf.String(),
		"only the translated <value> inner content should change")
}
