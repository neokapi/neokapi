package dtd_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/dtd"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func snippetRoundtripWithSkeleton(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := dtd.NewReader()
	writer := dtd.NewWriter()

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

func TestSkeletonStore_ByteExact_SimpleEntity(t *testing.T) {
	input := `<!ENTITY greeting "Hello world">`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "single entity roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_MultipleEntities(t *testing.T) {
	input := "<!ENTITY greeting \"Hello\">\n<!ENTITY farewell \"Goodbye\">\n<!ENTITY thanks \"Thank you\">\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "multiple entities with newlines should be byte-exact")
}

func TestSkeletonStore_ByteExact_WithComments(t *testing.T) {
	input := "<!-- Window title -->\n<!ENTITY findWindow.title \"Find Files\">\n<!-- File menu -->\n<!ENTITY fileMenu.label \"File\">\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "entities with comments should be byte-exact")
}

func TestSkeletonStore_ByteExact_SingleQuoted(t *testing.T) {
	input := "<!ENTITY greeting 'Hello world'>"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "single-quoted entity should be byte-exact")
}

func TestSkeletonStore_ByteExact_WithEscapes(t *testing.T) {
	// `>` does not require escaping inside a quoted entity value, so the
	// writer leaves it bare (matching okapi's DTDFilter output and the
	// XML 1.0 spec's allowed-character set). Input `&gt;` round-trips to
	// `>` after the reader resolves the entity reference.
	input := "<!ENTITY escaped \"Text with &amp; ampersand and &lt;angle brackets&gt;\">\n"
	expected := "<!ENTITY escaped \"Text with &amp; ampersand and &lt;angle brackets>\">\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, expected, output, "entities round-trip with `>` left bare")
}

func TestSkeletonStore_ByteExact_WithNCRs(t *testing.T) {
	// NCRs (&#65;) are resolved to characters (A) during reading.
	// The skeleton preserves the structural bytes around the value, but
	// the value itself is re-encoded from the resolved block content.
	// Characters that don't need DTD escaping (like A, B) are written as-is.
	input := "<!ENTITY ncr \"Char &#65; and hex &#x42;\">\n"
	expected := "<!ENTITY ncr \"Char A and hex B\">\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, expected, output, "NCRs should be resolved and re-encoded")
}

func TestSkeletonStore_ByteExact_EmptyEntity(t *testing.T) {
	input := `<!ENTITY empty "">`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "empty entity should be byte-exact")
}

func TestSkeletonStore_ByteExact_CRLF(t *testing.T) {
	input := "<!ENTITY greeting \"Hello\">\r\n<!ENTITY farewell \"Goodbye\">\r\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "CRLF line endings should be preserved")
}

func TestSkeletonStore_ByteExact_ParameterEntity(t *testing.T) {
	// Parameter entities are not translatable, they should be preserved as skeleton text
	input := "<!ENTITY % pent \"value\">\n<!ENTITY greeting \"Hello\">\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "parameter entities should be preserved as skeleton text")
}

func TestSkeletonStore_ByteExact_ComplexFile(t *testing.T) {
	data, err := os.ReadFile("testdata/complex.dtd")
	require.NoError(t, err)
	input := string(data)
	output := snippetRoundtripWithSkeleton(t, input)
	// NCRs (&#65; -> A, &#x42; -> B) are resolved during reading, so the
	// ncr entity value will differ. Everything else should be byte-exact.
	assert.Contains(t, output, "<!-- Window title -->\n<!ENTITY findWindow.title \"Find Files\">\n")
	assert.Contains(t, output, "<!-- File menu -->\n<!ENTITY fileMenu.label \"File\">\n")
	assert.Contains(t, output, "<!ENTITY editMenu.label \"Edit\">\n")
	assert.Contains(t, output, "<!ENTITY escaped \"Text with &amp; ampersand and &lt;angle brackets>\">\n")
	assert.Contains(t, output, "<!ENTITY ncr \"Char A and hex B\">\n")
	assert.Contains(t, output, "<!ENTITY unicode \"\xe3\x81\x93\xe3\x82\x93\xe3\x81\xab\xe3\x81\xa1\xe3\x81\xaf\xe4\xb8\x96\xe7\x95\x8c\">\n")
}

func TestSkeletonStore_ByteExact_SimpleFile(t *testing.T) {
	data, err := os.ReadFile("testdata/simple.dtd")
	require.NoError(t, err)
	input := string(data)
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "simple file roundtrip should be byte-exact")
}

// okapi: DTDFilterTest#testDoubleExtraction
// Okapi's testDoubleExtraction runs RoundTripComparison over Test01.dtd and
// Test02.dtd (the upstream fixtures, no translation). RoundTripComparison
// asserts EVENT stability — it extracts, regenerates, re-extracts, and
// compares the text units — not byte equality (okapi's DTDEncoder, like the
// native writer, escapes `<`/`&`/`"` in entity values, so the regenerated
// bytes legitimately differ from a source that used bare `<` or `&amp;`).
// The native analog asserts the same idempotency: re-extracting the
// skeleton-store output yields the same source text units. Test01.dtd in
// particular exercises the escaped-ampersand case `&amp;test1;`, which the
// reader resolves to the literal text "&test1;" and the writer re-escapes to
// `&amp;test1;` (rather than silently re-emitting the bare, semantically
// different reference `&test1;`).
//
// The same extract→write→re-extract idempotency is the contract Okapi's
// integration-test suite enforces over its DTD file corpus and gold XLIFF:
// okapi: RoundTripDtdIT#dtdFiles
// okapi: DtdXliffCompareIT#dtdXliffCompareFiles
// okapi-skip: RoundTripDtdIT#dtdFilesSerialized — Okapi serialized-skeleton roundtrip variant; native uses its own skeleton store (no serialized-skeleton mode)
func TestDoubleExtraction(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"testdata/Test01.dtd", "testdata/Test02.dtd"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile(name)
			require.NoError(t, err)
			input := string(data)
			first := dtdSourceTexts(t, input)
			output := snippetRoundtripWithSkeleton(t, input)
			second := dtdSourceTexts(t, output)
			assert.Equal(t, first, second,
				"%s text units must be stable across an extract→write→re-extract roundtrip", name)
		})
	}
}

// dtdSourceTexts reads the DTD content and returns the source text of every
// translatable block, in order.
func dtdSourceTexts(t *testing.T, input string) []string {
	t.Helper()
	ctx := t.Context()
	reader := dtd.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	defer reader.Close()
	var out []string
	for _, b := range testutil.FilterBlocks(testutil.CollectParts(t, reader.Read(ctx))) {
		out = append(out, b.SourceText())
	}
	return out
}

func TestSkeletonStore_WithTranslation(t *testing.T) {
	input := "<!ENTITY greeting \"Hello\">\n<!ENTITY farewell \"Goodbye\">\n"
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := dtd.NewReader()
	writer := dtd.NewWriter()

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

	expected := "<!ENTITY greeting \"Bonjour\">\n<!ENTITY farewell \"Au revoir\">\n"
	assert.Equal(t, expected, buf.String())
}

func TestSkeletonStore_WithTranslation_Escaping(t *testing.T) {
	input := "<!ENTITY greeting \"Hello\">\n"
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := dtd.NewReader()
	writer := dtd.NewWriter()

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
			// Translation contains characters that need DTD escaping
			b.SetTargetText(locale, "A & B < C")
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	expected := "<!ENTITY greeting \"A &amp; B &lt; C\">\n"
	assert.Equal(t, expected, buf.String())
}

// TestCodeFinderMarkupEscapedOnWrite reproduces the Test01.dtd parity
// divergence: with the code finder enabled (okapi's DTDFilter default),
// HTML markup inside an entity value is lifted into inline-code runs, but
// those codes still carry literal `<` and `"` bytes. The writer MUST
// entity-escape them so the round-tripped entity value remains valid DTD —
// a bare `"` would prematurely close the quoted value (XML 1.0 §2.3
// EntityValue, §4.2). Mirrors okapi's DTDFilter.java:297-305, which
// re-encodes each code-finder code via encoder.encode(..., TEXT).
func TestCodeFinderMarkupEscapedOnWrite(t *testing.T) {
	// Source entity already escapes the markup (`&lt;`, `&quot;`); the
	// reader decodes it to literal `<`/`"`, then the code finder captures
	// the resulting tags as inline codes. Round-trip must re-escape.
	input := `<!ENTITY test3 "Text with &lt;i>HTML&lt;/i> &lt;a name=&quot;aaa&quot;>codes&lt;/a>.">` + "\n"
	ctx := t.Context()

	reader := dtd.NewReader()
	writer := dtd.NewWriter()

	// okapi's DTDFilter default: useCodeFinder=true with the HTML-tag rule.
	require.NoError(t, reader.Config().ApplyMap(map[string]any{
		"useCodeFinder": true,
		"codeFinderRules": []any{
			`</?([A-Z0-9a-z]*)\b[^>]*>`,
		},
	}))

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
	writer.SetLocale(model.LocaleEnglish)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	out := buf.String()
	// The markup must be re-escaped, byte-identical to the source.
	assert.Equal(t, input, out)
	// Explicit guards against the regression: no raw markup leaks into the
	// quoted entity value.
	assert.NotContains(t, out, `<i>`, "raw `<i>` markup must be escaped")
	assert.NotContains(t, out, `name="aaa"`, "raw attribute quotes must be escaped")
	assert.Contains(t, out, `&lt;i>`)
	assert.Contains(t, out, `name=&quot;aaa&quot;`)
}

// TestCodeFinderMarkupVsStructuralRefs verifies the writer keeps the two
// inline-code categories distinct: code-finder markup is escaped, while a
// structural named-entity reference (`&test1;`) — which the reader also
// captures as an inline code — is emitted verbatim, not double-escaped.
func TestCodeFinderMarkupVsStructuralRefs(t *testing.T) {
	input := `<!ENTITY mixed "See &lt;b>&test1;&lt;/b>">` + "\n"
	ctx := t.Context()

	reader := dtd.NewReader()
	writer := dtd.NewWriter()

	require.NoError(t, reader.Config().ApplyMap(map[string]any{
		"useCodeFinder": true,
		"codeFinderRules": []any{
			`</?([A-Z0-9a-z]*)\b[^>]*>`,
		},
	}))

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
	writer.SetLocale(model.LocaleEnglish)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	out := buf.String()
	// Markup `<b>`/`</b>` escaped; structural ref `&test1;` left verbatim
	// (NOT `&amp;test1;`).
	assert.Equal(t, input, out)
	assert.Contains(t, out, `&test1;`)
	assert.NotContains(t, out, `&amp;test1;`)
}
