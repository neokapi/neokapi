package properties_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/formats/properties"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- PropertiesFilterTest — implemented native tests ----

// okapi: PropertiesFilterTest#testDefaultInfo
func TestRead_DefaultInfo(t *testing.T) {
	// Verifies the reader has correct name, display name, and signature.
	reader := properties.NewReader()
	assert.Equal(t, "properties", reader.Name())
	assert.Equal(t, "Java Properties", reader.DisplayName())
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "text/x-java-properties")
	assert.Contains(t, sig.Extensions, ".properties")
}

// okapi: PropertiesFilterTest#testStartDocument
func TestRead_StartDocument(t *testing.T) {
	// Verifies that the first part emitted is a PartLayerStart with correct format.
	ctx := t.Context()
	reader := properties.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("key=value", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "properties", layer.Format)
	assert.Equal(t, "text/x-java-properties", layer.MimeType)
	assert.NotEmpty(t, layer.ID)
}

// okapi: PropertiesFilterTest#testLineBreaks_CR
func TestRead_LineBreaksCr(t *testing.T) {
	// Verifies that bare-CR line breaks produce separate entries, matching
	// Okapi's PropertiesFilter (BufferedReader.readLine() recognises LF, CR
	// and CRLF). readPhysicalLine splits on bare CR so each line is its own
	// entry rather than collapsing into a single logical line.
	ctx := t.Context()
	reader := properties.NewReader()
	input := "Key1=Text1\rKey2=Text2"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 2)
	assert.Equal(t, "Key1", blocks[0].Name)
	assert.Equal(t, "Text1", blocks[0].SourceText())
	assert.Equal(t, "Key2", blocks[1].Name)
	assert.Equal(t, "Text2", blocks[1].SourceText())
}

// okapi: PropertiesFilterTest#testineBreaks_CRLF
func TestRead_LineBreaksCrlf(t *testing.T) {
	// Verifies that CRLF line breaks produce separate entries.
	ctx := t.Context()
	reader := properties.NewReader()
	input := "Key1=Text1\r\nKey2=Text2"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 2)
	assert.Equal(t, "Key1", blocks[0].Name)
	assert.Equal(t, "Text1", blocks[0].SourceText())
	assert.Equal(t, "Key2", blocks[1].Name)
	assert.Equal(t, "Text2", blocks[1].SourceText())
}

// okapi: PropertiesFilterTest#testLineBreaks_LF
func TestRead_LineBreaksLf(t *testing.T) {
	// Verifies that LF line breaks produce separate entries, including blanks.
	ctx := t.Context()
	reader := properties.NewReader()
	input := "Key1=Text1\n\n\nKey2=Text2"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 2)
	assert.Equal(t, "Key1", blocks[0].Name)
	assert.Equal(t, "Text1", blocks[0].SourceText())
	assert.Equal(t, "Key2", blocks[1].Name)
	assert.Equal(t, "Text2", blocks[1].SourceText())

	// Verify blank lines are preserved as data parts
	var blankCount int
	for _, p := range parts {
		if p.Type == model.PartData {
			d := p.Resource.(*model.Data)
			if d.Name == "blank" {
				blankCount++
			}
		}
	}
	assert.Equal(t, 2, blankCount)
}

// okapi: PropertiesFilterTest#testSpecialCharsInKey
func TestRead_SpecialCharsInKey(t *testing.T) {
	// Verifies that escaped special characters in keys are preserved.
	ctx := t.Context()
	reader := properties.NewReader()
	// Key is "Key \:\\" (escaped space, colon, backslash) with colon separator
	input := "Key\\ \\:\\\\:Text1"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Key\\ \\:\\\\", blocks[0].Name)
	assert.Equal(t, "Text1", blocks[0].SourceText())
}

// okapi: PropertiesFilterTest#testDoubleExtraction
// okapi: RoundTripPropertyIT#propertiesFiles — native extract→write→re-extract over real .properties inputs; Okapi's propertiesFiles does extract→merge→compare-events over a corpus.
// okapi: PropertyXliffCompareIT#propertiesXliffCompareFiles — same double-extraction verifies extracted content is stable; Okapi's propertiesXliffCompareFiles extracts to XLIFF and compares against a gold XLIFF corpus.
// okapi-skip: RoundTripPropertyIT#propertiesSerializedFiles — Okapi serialized-skeleton variant; native uses its own skeleton store, not Okapi's serialized event/skeleton format.
func TestRoundTrip_DoubleExtraction(t *testing.T) {
	// Double-extraction roundtrip: read → write → read → compare.
	tests := []struct {
		name  string
		input string
	}{
		{"basic", "key1=value1\nkey2=value2"},
		{"comments", "# comment\nkey=value"},
		{"unicode", "key=\\u3053\\u3093\\u306B\\u3061\\u306F"},
		{"continuation", "key=line1 \\\n    line2"},
		{"mixed_separators", "key1=value1\nkey2:value2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()

			// First extraction
			reader1 := properties.NewReader()
			err := reader1.Open(ctx, testutil.RawDocFromString(tt.input, model.LocaleEnglish))
			require.NoError(t, err)
			parts1 := testutil.CollectParts(t, reader1.Read(ctx))
			reader1.Close()

			// Write
			var buf bytes.Buffer
			writer := properties.NewWriter()
			err = writer.SetOutputWriter(&buf)
			require.NoError(t, err)
			writer.SetLocale(model.LocaleEnglish)
			ch := testutil.PartsToChannel(parts1)
			err = writer.Write(ctx, ch)
			require.NoError(t, err)
			writer.Close()

			output1 := buf.String()

			// Second extraction from written output
			reader2 := properties.NewReader()
			err = reader2.Open(ctx, testutil.RawDocFromString(output1, model.LocaleEnglish))
			require.NoError(t, err)
			parts2 := testutil.CollectParts(t, reader2.Read(ctx))
			reader2.Close()

			// Compare blocks from both extractions
			blocks1 := testutil.FilterBlocks(parts1)
			blocks2 := testutil.FilterBlocks(parts2)
			require.Equal(t, len(blocks1), len(blocks2), "block count mismatch")
			for i := range blocks1 {
				assert.Equal(t, blocks1[i].Name, blocks2[i].Name, "block %d name mismatch", i)
				assert.Equal(t, blocks1[i].SourceText(), blocks2[i].SourceText(), "block %d text mismatch", i)
			}
		})
	}
}

// okapi: PropertiesFilterTest#testHtmlOutput
func TestRead_HtmlOutput(t *testing.T) {
	// Without a subfilter, HTML-like content is treated as plain text.
	ctx := t.Context()
	reader := properties.NewReader()
	input := "Key1=<b>Text with &amp;=amp test</b>"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "<b>Text with &amp;=amp test</b>", blocks[0].SourceText())

	// Roundtrip preserves HTML content
	var buf bytes.Buffer
	writer := properties.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)
	ch := testutil.PartsToChannel(testutil.CollectParts(t, func() <-chan model.PartResult {
		r := properties.NewReader()
		_ = r.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
		return r.Read(ctx)
	}()))
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()
	assert.Equal(t, input, buf.String())
}

// okapi: PropertiesFilterTest#testJavaEscapeChars
func TestRead_JavaEscapeChars(t *testing.T) {
	// With useJavaEscapes, \: \= \# \! are decoded to their literal characters.
	ctx := t.Context()

	reader := properties.NewReader()
	err := reader.Config().ApplyMap(map[string]any{"useJavaEscapes": true})
	require.NoError(t, err)
	input := "key1=Listen\\: here's the \\#1 thing to remember\\: a \\!\\= b \\\\ c"
	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Listen: here's the #1 thing to remember: a != b \\ c", blocks[0].SourceText())
}

// okapi: PropertiesFilterTest#testIdGeneration_defaultConfig
func TestRead_IdGenerationDefaultConfig(t *testing.T) {
	// Verifies that block IDs are unique and based on a counter.
	ctx := t.Context()
	reader := properties.NewReader()
	input := "key1=value1\nkey2=value2\nkey3=value3"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 3)

	// Verify IDs are unique
	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.False(t, ids[b.ID], "duplicate block ID: %s", b.ID)
		ids[b.ID] = true
	}
	// Verify names are the keys
	assert.Equal(t, "key1", blocks[0].Name)
	assert.Equal(t, "key2", blocks[1].Name)
	assert.Equal(t, "key3", blocks[2].Name)
}

// okapi: PropertiesFilterTest#testMessagePlaceholders
func TestRead_MessagePlaceholders(t *testing.T) {
	// The native reader treats {0}, {1} etc. as plain text (no inline code spans).
	// Java message format placeholder detection is not implemented in the native reader.
	ctx := t.Context()
	reader := properties.NewReader()
	input := "Key1={1}Text1{2}"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	// Placeholders are part of the plain text
	assert.Equal(t, "{1}Text1{2}", blocks[0].SourceText())
}

// okapi: PropertiesFilterTest#testMessagePlaceholdersEscaped
func TestRead_MessagePlaceholdersEscaped(t *testing.T) {
	// Escaped message placeholders (''{0}'') are treated as plain text.
	ctx := t.Context()
	reader := properties.NewReader()
	input := "Key1=''{0}''Text1''{2}''"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Contains(t, blocks[0].SourceText(), "{0}")
	assert.Contains(t, blocks[0].SourceText(), "{2}")
}

// ---- Escape roundtrip tests ----

// okapi: PropertiesFilterTest#testEscapes (extended)
func TestRead_EscapeSequences(t *testing.T) {
	// Verifies that standard Java property escape sequences are decoded.
	ctx := t.Context()
	reader := properties.NewReader()
	input := "key=Text\\n=lf, \\t=tab, \\r=cr, \\\\=bs"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "Text\n=lf")
	assert.Contains(t, text, "\t=tab")
	assert.Contains(t, text, "\r=cr")
	assert.Contains(t, text, "\\=bs")
}

func TestRoundTrip_EscapeSequences(t *testing.T) {
	// Verifies escape sequences survive roundtrip: read → write → compare.
	ctx := t.Context()
	input := "key=Text\\n=lf, \\t=tab, \\r=cr, \\\\=bs"

	reader := properties.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := properties.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)
	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	assert.Equal(t, input, buf.String())
}

// ---- Localization directives ----
//
// The native reader honors Okapi-compatible localization directives in
// comment lines: single-entry `#_skip` / `#_text` and block
// `#_bskip` / `#_eskip`, `#_btext` / `#_etext`. These are on by default
// (matching Okapi's LocalizationDirectives.reset() → setOptions(true, true)).
// See reader.go directiveKind / directiveStack / nextOverride.

// okapi: PropertiesFilterTest#testLocDirectives_Skip
func TestRead_LocDirectivesSkip(t *testing.T) {
	// `#_skip` suppresses extraction of the single entry that follows;
	// the entry after that is extracted normally.
	//   #_skip
	//   Key1:Text1   → skipped
	//   Key2:Text2   → extracted
	// Okapi: TU#1 == "Text2".
	ctx := t.Context()
	reader := properties.NewReader()
	input := "#_skip\nKey1:Text1\nKey2:Text2"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1, "only the entry after the skipped one is extracted")
	assert.Equal(t, "Key2", blocks[0].Name)
	assert.Equal(t, "Text2", blocks[0].SourceText())
}

// okapi: PropertiesFilterTest#testLocDirectives_Group
func TestRead_LocDirectivesGroup(t *testing.T) {
	// `#_bskip` opens a skip block; a single-entry `#_text` overrides it
	// for exactly the next entry; subsequent entries fall back to the
	// surrounding skip block.
	//   #_bskip
	//   Key1:Text1   → skipped (block)
	//   #_text
	//   Key2:Text2   → extracted (single override)
	//   Key2:Text3   → skipped (block resumes)
	// Okapi: TU#1 == "Text2", TU#2 == null.
	ctx := t.Context()
	reader := properties.NewReader()
	input := "#_bskip\nKey1:Text1\n#_text\nKey2:Text2\nKey2:Text3"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1, "only the single _text override is extracted inside the _bskip block")
	assert.Equal(t, "Key2", blocks[0].Name)
	assert.Equal(t, "Text2", blocks[0].SourceText())
}

// ---- Subfilter tests (not applicable to the native reader) ----
//
// All of these Okapi @Test methods set `params.setSubfilter("okf_html")`
// and assert on the result of dispatching each property VALUE through the
// HTML subfilter (inner-text extraction, HTML-entity decoding, `<p>`/`<br>`
// splitting into multiple TUs, START_SUBFILTER events, and code-finder
// rules being propagated into the embedded HTML filter). The neokapi
// native properties reader extracts each value as a single opaque Block;
// embedded-format dispatch is a flow/recipe concern handled by composing a
// downstream HTML tool or the okapi-bridge plugin, not behavior of the
// native reader/writer. See TestRead_HtmlOutput for the native (no-subfilter)
// treatment of HTML-bearing values.

// okapi-skip: PropertiesFilterTest#testWithSubfilter — subfilter dispatch (okf_html inner-text extraction) is a flow/recipe concern, not native reader behavior
// okapi-skip: PropertiesFilterTest#testWithSubfilterTwoParas — subfilter dispatch (okf_html splits value into multiple TUs) is a flow/recipe concern, not native reader behavior
// okapi-skip: PropertiesFilterTest#testWithSubfilterWithEmbeddedMessagePH — code-finder rules propagated into the okf_html subfilter; subfilter dispatch not native reader behavior
// okapi-skip: PropertiesFilterTest#testWithSubfilterWithEmbeddedEscapedMessagePH — code-finder rules propagated into the okf_html subfilter; subfilter dispatch not native reader behavior
// okapi-skip: PropertiesFilterTest#testWithSubfilterWithHTMLEscapes — HTML-entity decoding via okf_html subfilter is a flow/recipe concern, not native reader behavior
// okapi-skip: PropertiesFilterTest#testWithSubfilterOutput — round-trip through the okf_html subfilter; subfilter dispatch not native reader behavior
// okapi-skip: PropertiesFilterTest#testWithSubfilterOutputEscapeExtended — escapeExtendedChars output is exercised natively in TestWrite_EscapeExtendedChars; this case is subfilter-coupled (okf_html), not native reader behavior
// okapi-skip: PropertiesFilterTest#testWithSubfilterOutputDoNotEscapeExtended — escapeExtendedChars=false output is exercised natively in TestWrite_DoNotEscapeExtendedChars; this case is subfilter-coupled (okf_html), not native reader behavior
// okapi-skip: PropertiesFilterTest#testDoubleExtractionSubFilter — round-trip comparison with the okf_html subfilter; subfilter dispatch not native reader behavior
// okapi-skip: PropertiesFilterTest#testIdGeneration_subfiltersConfig — asserts START_SUBFILTER events and subfilter-derived TU id/name generation; subfilter dispatch not native reader behavior
func TestRead_SubfilterValueIsOpaque(t *testing.T) {
	// Documents the native (no-subfilter) contract that justifies the
	// okapi-skip annotations above: a value containing HTML markup is
	// extracted verbatim as a single Block. There is no inner-text
	// extraction and no splitting into multiple TUs.
	ctx := t.Context()
	reader := properties.NewReader()
	input := "Key1=<b>Text with more</b> <p> test"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1, "native reader does not split HTML values via a subfilter")
	assert.Equal(t, "<b>Text with more</b> <p> test", blocks[0].SourceText())
}

// ---- escapeExtendedChars writer knob (no-subfilter native equivalent) ----
//
// These exercise the escapeExtendedChars output behavior that the Okapi
// testWithSubfilterOutput{,DoNot}EscapeExtended tests assert, minus the
// okf_html subfilter dispatch those tests couple it with. They are the
// native carriers for the writer knob; the subfilter-coupled Okapi tests
// stay skip-classified above.

// writeValue runs a single value through the native reader+writer with the
// given escapeExtendedChars setting on the writer, returning the output.
func writeValue(t *testing.T, value string, escapeExtended bool) string {
	t.Helper()
	ctx := t.Context()

	reader := properties.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString("key="+value, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// The reader stored the raw value for byte-exact skeleton-less output.
	// Drop it so the writer re-encodes the source through encodePropertyValue,
	// which is where the escapeExtendedChars knob takes effect.
	for _, p := range parts {
		if p.Type == model.PartBlock {
			delete(p.Resource.(*model.Block).Properties, "rawValue")
		}
	}

	cfg := &properties.Config{}
	cfg.Reset()
	cfg.EscapeExtendedChars = escapeExtended

	var buf bytes.Buffer
	writer := properties.NewWriter()
	writer.SetConfig(cfg)
	require.NoError(t, writer.SetOutputWriter(&buf))
	writer.SetLocale(model.LocaleEnglish)
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()

	return buf.String()
}

// neokapi-only: covers the escapeExtendedChars=true (default) writer path
// independently of the okf_html subfilter that
// PropertiesFilterTest#testWithSubfilterOutputEscapeExtended couples it with.
func TestWrite_EscapeExtendedChars(t *testing.T) {
	// Latin-1 supplement chars are escaped to \uXXXX (lowercase hex).
	got := writeValue(t, "vältüé wîth html", true)
	assert.Equal(t, "key=v\\u00e4lt\\u00fc\\u00e9 w\\u00eeth html", got)
}

// neokapi-only: covers the escapeExtendedChars=false writer path
// independently of the okf_html subfilter that
// PropertiesFilterTest#testWithSubfilterOutputDoNotEscapeExtended couples it
// with. Before this fix the writer ignored the flag and always escaped.
func TestWrite_DoNotEscapeExtendedChars(t *testing.T) {
	// With escapeExtendedChars=false the non-ASCII bytes pass through verbatim.
	got := writeValue(t, "vältüé wîth html", false)
	assert.Equal(t, "key=vältüé wîth html", got)
}
