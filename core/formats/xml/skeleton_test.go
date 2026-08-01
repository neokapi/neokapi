package xml_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	xmlfmt "github.com/neokapi/neokapi/core/formats/xml"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func xmlSkeletonRoundtrip(t *testing.T, input string) string {
	t.Helper()
	return xmlSkeletonRoundtripCfg(t, input, nil)
}

// xmlSkeletonRoundtripCfg is xmlSkeletonRoundtrip with an explicit reader
// config (used by the #928 non-translatable-content round-trip tests).
func xmlSkeletonRoundtripCfg(t *testing.T, input string, cfg *xmlfmt.Config) string {
	t.Helper()
	ctx := t.Context()

	reader := xmlfmt.NewReader()
	writer := xmlfmt.NewWriter()
	if cfg != nil {
		require.NoError(t, reader.SetConfig(cfg))
	}

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

// ---------------------------------------------------------------------------
// Byte-exact roundtrip tests
// ---------------------------------------------------------------------------

func TestSkeletonStore_ByteExact_SimpleXML(t *testing.T) {
	input := `<?xml version="1.0"?><root><message>Hello World</message></root>`
	output := xmlSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "simple XML roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_MultipleElements(t *testing.T) {
	input := `<?xml version="1.0"?>
<resources>
  <string>Title</string>
  <string>Description</string>
</resources>`
	output := xmlSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "multiple elements roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_Attributes(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<root xmlns:custom="http://example.com" xml:lang="en">
  <item id="123" class="primary">First item</item>
  <item id="456" class="secondary">Second item</item>
</root>`
	output := xmlSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "attributes roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_NestedElements(t *testing.T) {
	input := `<?xml version="1.0"?>
<root>
  <parent>
    <child>Nested content</child>
  </parent>
  <other>Sibling content</other>
</root>`
	output := xmlSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "nested elements roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_Comments(t *testing.T) {
	input := `<?xml version="1.0"?>
<!-- This is a file-level comment -->
<root>
  <!-- Element comment -->
  <message>Hello</message>
</root>`
	output := xmlSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "comments roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_ProcessingInstructions(t *testing.T) {
	input := `<?xml version="1.0"?>
<?xml-stylesheet type="text/xsl" href="style.xsl"?>
<root>
  <message>Hello</message>
</root>`
	output := xmlSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "processing instructions roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_Namespaces(t *testing.T) {
	input := `<?xml version="1.0"?>
<root xmlns="http://example.com/default" xmlns:ns="http://example.com/ns">
  <message>Default namespace</message>
  <ns:item>Prefixed namespace</ns:item>
</root>`
	output := xmlSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "namespace preservation roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_Entities(t *testing.T) {
	input := `<?xml version="1.0"?>
<root>
  <message>Cats &amp; Dogs</message>
  <note>x &lt; y &gt; z</note>
</root>`
	output := xmlSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "entities roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_MultiLineContent(t *testing.T) {
	input := `<?xml version="1.0"?>
<root>
  <description>This is a
multi-line
description text.</description>
</root>`
	output := xmlSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "multi-line content roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_EmptyElements(t *testing.T) {
	input := `<?xml version="1.0"?>
<root>
  <empty/>
  <message>Between empties</message>
  <also-empty></also-empty>
</root>`
	output := xmlSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "empty elements roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_Whitespace(t *testing.T) {
	// Unusual whitespace patterns
	input := `<?xml version="1.0"?>
<root>
	<message>Tab indented</message>
    <message>Space indented</message>
</root>
`
	output := xmlSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "whitespace patterns roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_XMLDeclarationVariants(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<root><message>Hello</message></root>`
	output := xmlSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "XML declaration variants roundtrip should be byte-exact")
}

// ---------------------------------------------------------------------------
// Translation test
// ---------------------------------------------------------------------------

func TestSkeletonStore_WithTranslation(t *testing.T) {
	input := `<?xml version="1.0"?>
<root>
  <greeting>Hello</greeting>
  <farewell>Goodbye</farewell>
</root>`
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := xmlfmt.NewReader()
	writer := xmlfmt.NewWriter()

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
				b.SetTargetText(locale, "Bonjour")
			case "Goodbye":
				b.SetTargetText(locale, "Au revoir")
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	expected := `<?xml version="1.0"?>
<root>
  <greeting>Bonjour</greeting>
  <farewell>Au revoir</farewell>
</root>`
	assert.Equal(t, expected, buf.String())
}

func TestSkeletonStore_TranslationWithEntities(t *testing.T) {
	input := `<?xml version="1.0"?>
<root>
  <message>Cats &amp; Dogs</message>
</root>`
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := xmlfmt.NewReader()
	writer := xmlfmt.NewWriter()

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
			if b.SourceText() == "Cats & Dogs" {
				b.SetTargetText(locale, "Chats & Chiens")
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	// The writer should XML-escape the ampersand in the translated text
	expected := `<?xml version="1.0"?>
<root>
  <message>Chats &amp; Chiens</message>
</root>`
	assert.Equal(t, expected, buf.String())
}

func TestSkeletonStore_TranslatableAttributes(t *testing.T) {
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := xmlfmt.NewReader()
	writer := xmlfmt.NewWriter()

	cfg := &xmlfmt.Config{}
	cfg.ElementRules = append(cfg.ElementRules, &xmlfmt.ElementRule{
		Name:      "item",
		RuleTypes: []xmlfmt.RuleType{xmlfmt.RuleAttributeTrans},
		TranslatableAttributes: map[string]*xmlfmt.TranslatableAttrCondition{
			"label": {},
		},
	})
	require.NoError(t, reader.SetConfig(cfg))

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	input := `<?xml version="1.0"?>
<root>
  <item label="Hello" id="1">Content</item>
</root>`

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// First verify roundtrip
	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()
	assert.Equal(t, input, buf.String(), "translatable attributes roundtrip should be byte-exact")

	// Now test with translation
	store2, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store2.Close()

	reader2 := xmlfmt.NewReader()
	require.NoError(t, reader2.SetConfig(cfg))
	reader2.SetSkeletonStore(store2)

	writer2 := xmlfmt.NewWriter()
	writer2.SetSkeletonStore(store2)

	err = reader2.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts2 := testutil.CollectParts(t, reader2.Read(ctx))
	reader2.Close()

	for _, p := range parts2 {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			switch b.SourceText() {
			case "Hello":
				b.SetTargetText(locale, "Bonjour")
			case "Content":
				b.SetTargetText(locale, "Contenu")
			}
		}
	}

	var buf2 bytes.Buffer
	writer2.SetLocale(locale)
	require.NoError(t, writer2.SetOutputWriter(&buf2))

	ch2 := testutil.PartsToChannel(parts2)
	require.NoError(t, writer2.Write(ctx, ch2))
	writer2.Close()

	expected := `<?xml version="1.0"?>
<root>
  <item label="Bonjour" id="1">Contenu</item>
</root>`
	assert.Equal(t, expected, buf2.String())
}

// ---------------------------------------------------------------------------
// Mixed content (inline + block)
// ---------------------------------------------------------------------------

func TestSkeletonStore_ByteExact_MixedContent(t *testing.T) {
	ctx := t.Context()

	reader := xmlfmt.NewReader()
	writer := xmlfmt.NewWriter()

	cfg := &xmlfmt.Config{}
	cfg.ElementRules = append(cfg.ElementRules, &xmlfmt.ElementRule{
		Name:      "b",
		RuleTypes: []xmlfmt.RuleType{xmlfmt.RuleInline},
	})
	require.NoError(t, reader.SetConfig(cfg))

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	input := `<?xml version="1.0"?>
<root>
  <message>Hello <b>World</b> today</message>
</root>`

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	assert.Equal(t, input, buf.String(), "mixed content roundtrip should be byte-exact")
}

// TestSkeletonStore_MixedContent_Idempotent pins repeated passes over mixed
// content, which is the property a convergence loop depends on and which
// single-pass byte-exactness does not imply.
//
// A parent's text nodes are non-contiguous — separated by its element children
// — but the reader concatenates them into one block while the skeleton reserves
// a ref for only the last. The bytes of the earlier runs stayed in the skeleton
// as literal text AND inside the block, so each pass emitted the separator
// twice and the next pass read the doubled result as the new block text. One
// byte per whitespace-only separator, per pass, without bound: a project that
// converges daily grew a byte a day, the source diff showed a whitespace change
// every run, and nothing in the recipe pointed at a cause (#1605).
func TestSkeletonStore_MixedContent_Idempotent(t *testing.T) {
	t.Parallel()
	for name, input := range map[string]string{
		// The filed reproducer: element-first, whitespace-only separator,
		// trailing non-whitespace text.
		"separator then trailing text": `<r><a>x</a> <b>y</b> tail</r>`,
		"two separators":               `<r><a>x</a> <b>y</b> <c>z</c> tail</r>`,
		"tab separator":                "<r><a>x</a>\t<b>y</b> tail</r>",
		"newline separator":            "<r><a>x</a>\n<b>y</b> tail</r>",

		// Shapes the issue probed as stable; they must stay stable.
		"non-whitespace separator": `<r><a>x</a>M<b>y</b>N</r>`,
		"no separator":             `<r><a>x</a><b>y</b> tail</r>`,
		"no trailing text":         `<r><a>x</a> <b>y</b></r>`,
		"pretty printed":           "<r>\n  <a>x</a>\n  <b>y</b>\n</r>",
		"leading text":             `<para>See <emphasis>a</emphasis> <emphasis>b</emphasis> now.</para>`,
		"ordinary prose":           `<p>Click <b>Save</b> <i>now</i> to continue.</p>`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// A single pass is byte-exact on an untouched document …
			pass1 := xmlSkeletonRoundtrip(t, input)
			assert.Equal(t, input, pass1, "single pass must be byte-exact")

			// … and so is every pass after it. Four, because the growth is
			// one byte per separator per pass: a fix that merely halved it
			// would still pass a two-pass check.
			prev := pass1
			for i := 2; i <= 5; i++ {
				next := xmlSkeletonRoundtrip(t, prev)
				assert.Equal(t, prev, next, "pass %d changed the document", i)
				prev = next
			}
		})
	}
}
