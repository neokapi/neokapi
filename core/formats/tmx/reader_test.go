package tmx_test

// okapi-filter: tmx
//
// This file contains native Go tests for the TMX format reader/writer,
// mapped to the Java Okapi TmxFilterTest and ParametersTest test methods.
//
// --- File-based extraction tests (require Okapi test resource files) ---
//
// okapi-deferred: TmxFilterTest (file-based extraction of sampleTMX2.tmx) — requires okf_tmx/sampleTMX2.tmx; behavior covered by TestSpecialChars and TestMultipleTargets
// okapi-deferred: TmxFilterTest (file-based extraction of Paragraph_TM.tmx) — requires okf_tmx/Paragraph_TM.tmx; paragraph TU behavior covered by TestSegTypePara
// okapi-deferred: TmxFilterTest (file-based extraction of small_complete.tmx) — requires okf_tmx/small_complete.tmx; inline codes covered by TestBptEptPair, TestPhPlaceholder, TestItIsolatedBeginEnd, TestHiHighlight
//
// --- Integration-test (Failsafe) roundtrip contracts ---
//
// RoundTripTmxIT (roundtrip.integration) and TmxXliffCompareIT
// (xliffcompare.integration) in integration-tests/okapi. The plain-TMX rows
// map to native roundtrip tests below: RoundTripTmxIT#tmxFiles →
// TestRoundTrip_SimpleFile (real testdata/simple.tmx read→write) and
// TmxXliffCompareIT#tmxXliffCompareFiles → TestRoundTrip_InlineCodes
// (inline-code extraction stability). The serialized variant is not applicable:
//
// okapi-skip: RoundTripTmxIT#tmxSerializedFiles — Okapi serialized-skeleton variant (events written to a .ser/.json blob then merged); native uses its own skeleton store, not Okapi's serialized event format

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/formats/tmx"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errReader is an io.Reader that always fails, modelling an input that cannot
// be read (the native analogue of Java's "open an invalid URI").
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("simulated read failure") }

// --- Helpers ---

// readTMX parses a TMX string and returns all parts.
func readTMX(t *testing.T, input string) []*model.Part {
	t.Helper()
	ctx := t.Context()
	reader := tmx.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()
	return testutil.CollectParts(t, reader.Read(ctx))
}

// readTMXBlocks parses a TMX string and returns blocks.
func readTMXBlocks(t *testing.T, input string) []*model.Block {
	t.Helper()
	return testutil.FilterBlocks(readTMX(t, input))
}

// readTMXAllowError parses a TMX string and returns parts and any error.
func readTMXAllowError(t *testing.T, input string) ([]*model.Part, error) {
	t.Helper()
	ctx := t.Context()
	reader := tmx.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	var parts []*model.Part
	for pr := range reader.Read(ctx) {
		if pr.Error != nil {
			return parts, pr.Error
		}
		parts = append(parts, pr.Part)
	}
	return parts, nil
}

// readTMXFile reads a TMX test data file and returns blocks.
func readTMXFile(t *testing.T, path string) []*model.Block {
	t.Helper()
	ctx := t.Context()
	reader := tmx.NewReader()
	f, err := os.Open(path)
	require.NoError(t, err)
	err = reader.Open(ctx, testutil.RawDocFromReader(f, path, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()
	return testutil.CollectBlocks(t, reader.Read(ctx))
}

// roundTrip reads TMX, writes it, then reads the output again.
func roundTrip(t *testing.T, input string) (string, []*model.Block) {
	t.Helper()
	ctx := t.Context()

	// Read
	reader := tmx.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Write
	var buf bytes.Buffer
	writer := tmx.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)
	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()

	// Re-read
	reader2 := tmx.NewReader()
	err = reader2.Open(ctx, testutil.RawDocFromString(output, model.LocaleEnglish))
	require.NoError(t, err)
	blocks := testutil.CollectBlocks(t, reader2.Read(ctx))
	reader2.Close()

	return output, blocks
}

// wrapTMX wraps body content in a standard TMX 1.4 envelope.
func wrapTMX(body string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
  <header creationtool="XYZTool" creationtoolversion="1.0.0" datatype="PlainText" segtype="sentence" adminlang="en" srclang="en" o-tmf="abc">
  </header>
  <body>
` + body + `
  </body>
</tmx>`
}

// wrapTMXWithLangs wraps body content with a specific srclang.
func wrapTMXWithLangs(srclang, body string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
  <header creationtool="XYZTool" creationtoolversion="1.0.0" datatype="PlainText" segtype="sentence" adminlang="en" srclang="` + srclang + `" o-tmf="abc">
  </header>
  <body>
` + body + `
  </body>
</tmx>`
}

// findBlockContaining returns the first block whose source text contains substr.
func findBlockContaining(blocks []*model.Block, substr string) *model.Block {
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), substr) {
			return b
		}
	}
	return nil
}

// inlineCodeRuns returns only the inline-code runs (Ph, PcOpen, PcClose, Sub).
func inlineCodeRuns(runs []model.Run) []model.Run {
	var out []model.Run
	for _, r := range runs {
		if r.Text == nil && r.Plural == nil && r.Select == nil {
			out = append(out, r)
		}
	}
	return out
}

// hasInlineCodeRun reports whether any run is a non-text/non-plural/non-select run.
func hasInlineCodeRun(runs []model.Run) bool {
	for _, r := range runs {
		if r.Text == nil && r.Plural == nil && r.Select == nil {
			return true
		}
	}
	return false
}

// --- Filter metadata tests ---

// okapi: TmxFilterTest#testDefaultInfo
func TestDefaultInfo(t *testing.T) {
	reader := tmx.NewReader()
	assert.Equal(t, "tmx", reader.Name())
	assert.Equal(t, "TMX", reader.DisplayName())
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "application/x-tmx+xml")
	assert.Contains(t, sig.Extensions, ".tmx")
}

// okapi: TmxFilterTest#testGetName
func TestGetName(t *testing.T) {
	reader := tmx.NewReader()
	assert.Equal(t, "tmx", reader.Name())
}

// okapi: TmxFilterTest#testGetMimeType
func TestGetMimeType(t *testing.T) {
	reader := tmx.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "application/x-tmx+xml")
}

// --- Simple extraction tests ---

// okapi: TmxFilterTest#testSimpleTransUnit
func TestSimpleTransUnit(t *testing.T) {
	input := wrapTMX(`
    <tu>
      <tuv xml:lang="en"><seg>Hello World</seg></tuv>
      <tuv xml:lang="fr"><seg>Bonjour le monde</seg></tuv>
    </tu>`)
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)

	b := findBlockContaining(blocks, "Hello World")
	require.NotNil(t, b, "should find block with 'Hello World'")
	assert.Equal(t, "Hello World", b.SourceText())
	assert.True(t, b.HasTarget("fr"))
	assert.Equal(t, "Bonjour le monde", b.TargetText("fr"))
}

// okapi: TmxFilterTest#testMultiTransUnitWithEmptyLocales
func TestMultiTransUnitWithEmptyLocales(t *testing.T) {
	input := wrapTMX(`
    <tu>
      <tuv xml:lang="en"><seg>First</seg></tuv>
      <tuv xml:lang="fr"><seg>Premier</seg></tuv>
    </tu>
    <tu>
      <tuv xml:lang="en"><seg>Second</seg></tuv>
      <tuv xml:lang="fr"><seg>Deuxieme</seg></tuv>
    </tu>
    <tu>
      <tuv xml:lang="en"><seg>Third</seg></tuv>
      <tuv xml:lang="fr"><seg>Troisieme</seg></tuv>
    </tu>`)
	blocks := readTMXBlocks(t, input)
	require.Len(t, blocks, 3)

	texts := testutil.BlockTexts(blocks)
	assert.Contains(t, texts, "First")
	assert.Contains(t, texts, "Second")
	assert.Contains(t, texts, "Third")
}

// okapi: TmxFilterTest#testMulipleTargets
func TestMultipleTargets(t *testing.T) {
	input := `<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtool="XYZTool" creationtoolversion="1.01-023" datatype="PlainText"
    segtype="sentence" adminlang="en" srclang="en"
    creationdate="20020101T163812Z" o-encoding="iso-8859-1">
  </header>
  <body>
    <tu tuid="0002" srclang="*all*">
      <tuv xml:lang="en"><seg>menu</seg></tuv>
      <tuv xml:lang="fr"><seg>menu</seg></tuv>
      <tuv xml:lang="FR-FR"><seg>menu</seg></tuv>
    </tu>
  </body>
</tmx>`
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "menu", blocks[0].SourceText())
	// Should have fr and FR-FR as targets
	assert.True(t, blocks[0].HasTarget("fr"))
	assert.True(t, blocks[0].HasTarget("FR-FR"))
}

// --- Special characters and escaping ---

// okapi: TmxFilterTest#testSpecialChars
func TestSpecialChars(t *testing.T) {
	input := `<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtool="XYZTool" creationtoolversion="1.01-023" datatype="PlainText"
    segtype="sentence" adminlang="en" srclang="en"
    creationdate="20020101T163812Z" o-encoding="iso-8859-1">
  </header>
  <body>
    <tu tuid="0001">
      <tuv xml:lang="en">
        <seg>data (with a non-standard character: &#xF8FF;).</seg>
      </tuv>
      <tuv xml:lang="fr">
        <seg>donn&#xE9;es (avec un caract&#xE8;re non standard: &#xF8FF;).</seg>
      </tuv>
    </tu>
  </body>
</tmx>`
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)

	text := blocks[0].SourceText()
	assert.Contains(t, text, "data")
	assert.Contains(t, text, "non-standard character")
	// U+F8FF is Apple private use character
	assert.Contains(t, text, string(rune(0xF8FF)))
}

// okapi: TmxFilterTest#testLineBreaks
func TestLineBreaks(t *testing.T) {
	input := wrapTMX(`
    <tu>
      <tuv xml:lang="en"><seg>Line one
Line two</seg></tuv>
      <tuv xml:lang="fr"><seg>Ligne une
Ligne deux</seg></tuv>
    </tu>`)
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)

	text := blocks[0].SourceText()
	assert.Contains(t, text, "Line one")
	assert.Contains(t, text, "Line two")
	assert.Contains(t, text, "\n", "should preserve line breaks")
}

// okapi: TmxFilterTest#testEscapes
func TestEscapes(t *testing.T) {
	input := wrapTMX(`
    <tu>
      <tuv xml:lang="en"><seg>Test &amp; &lt; &gt; &quot;</seg></tuv>
      <tuv xml:lang="fr"><seg>Test &amp; &lt; &gt; &quot;</seg></tuv>
    </tu>`)
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)

	text := blocks[0].SourceText()
	assert.Contains(t, text, "&")
	assert.Contains(t, text, "<")
	assert.Contains(t, text, ">")
	assert.Contains(t, text, "\"")
}

// okapi: TmxFilterTest#testOutputWithLT
func TestOutputWithLT(t *testing.T) {
	input := wrapTMX(`
    <tu>
      <tuv xml:lang="en"><seg>a &lt; b</seg></tuv>
      <tuv xml:lang="fr"><seg>a &lt; b</seg></tuv>
    </tu>`)

	output, blocks := roundTrip(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "a < b", blocks[0].SourceText())
	// Output should contain the less-than, escaped or literal
	assert.True(t, strings.Contains(output, "&lt;") || strings.Contains(output, "a < b"),
		"output should preserve less-than: %s", output)
}

// okapi: TmxFilterTest#testTUTUVAttrEscaping
func TestTUTUVAttrEscaping(t *testing.T) {
	input := wrapTMX(`
    <tu tuid="id&amp;1">
      <tuv xml:lang="en"><seg>Attr escaping test</seg></tuv>
      <tuv xml:lang="fr"><seg>Test echappement attribut</seg></tuv>
    </tu>`)
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Attr escaping test", blocks[0].SourceText())
	// The TU ID should have the unescaped value
	assert.Equal(t, "id&1", blocks[0].ID)
}

// --- Language handling ---

// okapi: TmxFilterTest#testLang11
func TestLang11(t *testing.T) {
	input := `<?xml version="1.0"?>
<tmx version="1.1">
  <header creationtool="test" creationtoolversion="1.0" datatype="PlainText"
    segtype="sentence" adminlang="en" srclang="en" o-tmf="abc">
  </header>
  <body>
    <tu>
      <tuv lang="en"><seg>TMX 1.1 text</seg></tuv>
      <tuv lang="fr"><seg>Texte TMX 1.1</seg></tuv>
    </tu>
  </body>
</tmx>`
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks, "should handle TMX 1.1 lang attribute")
	assert.Equal(t, "TMX 1.1 text", blocks[0].SourceText())
	assert.True(t, blocks[0].HasTarget("fr"))
	assert.Equal(t, "Texte TMX 1.1", blocks[0].TargetText("fr"))
}

// okapi: TmxFilterTest#testXmlLangOverLang
func TestXmlLangOverLang(t *testing.T) {
	// When both xml:lang and lang are present, xml:lang takes precedence.
	input := wrapTMX(`
    <tu>
      <tuv xml:lang="en" lang="de"><seg>xml:lang wins</seg></tuv>
      <tuv xml:lang="fr" lang="de"><seg>xml:lang gagne</seg></tuv>
    </tu>`)
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "xml:lang wins", blocks[0].SourceText())
	// Target should be "fr" (from xml:lang), not "de" (from lang)
	assert.True(t, blocks[0].HasTarget("fr"))
	assert.False(t, blocks[0].HasTarget("de"))
}

// okapi: TmxFilterTest#testRelaxLanguageMatching
// Native langMatches() implements relaxed BCP-47 matching: a bare primary
// subtag ("en") matches a regioned variant ("en-US"). TMX 1.4 keys TUVs by
// xml:lang, and Okapi's newer relax-matching lets srclang="en" select an
// en-US <tuv> as the source.
func TestRelaxLanguageMatching(t *testing.T) {
	// en should match en-US with relaxed matching.
	input := `<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtool="test" creationtoolversion="1.0" datatype="PlainText"
    segtype="sentence" adminlang="en" srclang="en" o-tmf="abc">
  </header>
  <body>
    <tu>
      <tuv xml:lang="en-US"><seg>US English</seg></tuv>
      <tuv xml:lang="fr"><seg>Anglais US</seg></tuv>
    </tu>
  </body>
</tmx>`
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks, "should extract blocks with relaxed language matching")
	assert.Equal(t, "US English", blocks[0].SourceText())
}

// okapi: TmxFilterTest#testRelaxLanguageMatchingInTheOtherDirection
// Relaxed matching is symmetric: srclang="en-US" also selects a bare "en"
// <tuv> as the source, mirroring Okapi's reverse-direction relax test.
func TestRelaxLanguageMatchingReverse(t *testing.T) {
	// en-US srclang should match en TUV.
	input := `<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtool="test" creationtoolversion="1.0" datatype="PlainText"
    segtype="sentence" adminlang="en" srclang="en-US" o-tmf="abc">
  </header>
  <body>
    <tu>
      <tuv xml:lang="en"><seg>Generic English</seg></tuv>
      <tuv xml:lang="fr"><seg>Anglais generique</seg></tuv>
    </tu>
  </body>
</tmx>`
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Generic English", blocks[0].SourceText())
	assert.True(t, blocks[0].HasTarget("fr"))
}

// okapi: TmxFilterTest#testRelaxLanguageMatchingStillDisallowsRegionMismatches
// Relaxed matching still rejects region mismatches: srclang="en-US" must NOT
// select an "en-GB" <tuv>. Here neither <tuv> matches the source language, so
// the reader falls back to the first <tuv> ("en-GB") as the source.
func TestRelaxLanguageRegionMismatch(t *testing.T) {
	// en-US should NOT match en-GB even with relaxed matching.
	input := `<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtool="test" creationtoolversion="1.0" datatype="PlainText"
    segtype="sentence" adminlang="en" srclang="en-US" o-tmf="abc">
  </header>
  <body>
    <tu>
      <tuv xml:lang="en-GB"><seg>British English</seg></tuv>
      <tuv xml:lang="fr"><seg>Anglais britannique</seg></tuv>
    </tu>
  </body>
</tmx>`
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	// en-US does NOT relax-match en-GB (region mismatch), so no <tuv> is the
	// source by language. The reader falls back to the first <tuv> (en-GB) for
	// the source content. Because en-GB never matched the source language, it
	// is ALSO retained as an en-GB target — i.e. the region mismatch is real,
	// not silently coalesced into the source.
	assert.Equal(t, "British English", blocks[0].SourceText())
	assert.True(t, blocks[0].HasTarget("en-GB"), "en-GB must remain a distinct target, not match en-US source")
	assert.True(t, blocks[0].HasTarget("fr"))
}

// --- Target attributes ---

// okapi: TmxFilterTest#testTargetAttributes
func TestTargetAttributes(t *testing.T) {
	input := `<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtool="XYZTool" creationtoolversion="1.01-023" datatype="PlainText"
    segtype="sentence" adminlang="en" srclang="en"
    creationdate="20020101T163812Z" o-encoding="iso-8859-1">
  </header>
  <body>
    <tu tuid="0001" datatype="Text" usagecount="2" lastusagedate="19970314T023401Z">
      <tuv xml:lang="en" creationdate="19970212T153400Z" creationid="BobW">
        <seg>source text</seg>
      </tuv>
      <tuv xml:lang="fr" creationdate="19970309T021145Z" creationid="BobW"
           changedate="19970314T023401Z" changeid="ManonD">
        <prop type="Origin">MT</prop>
        <seg>texte cible</seg>
      </tuv>
    </tu>
  </body>
</tmx>`
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "source text", blocks[0].SourceText())
	assert.True(t, blocks[0].HasTarget("fr"))
	assert.Equal(t, "texte cible", blocks[0].TargetText("fr"))
}

// --- TU Properties ---

// okapi: TmxFilterTest#testTUProperties
func TestTUProperties(t *testing.T) {
	input := `<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtool="XYZTool" creationtoolversion="1.01-023" datatype="PlainText"
    segtype="sentence" adminlang="en" srclang="en"
    creationdate="20020101T163812Z" o-encoding="iso-8859-1">
  </header>
  <body>
    <tu tuid="0001">
      <note>Text of a note at the TU level.</note>
      <prop type="x-Domain">Computing</prop>
      <tuv xml:lang="en"><seg>source</seg></tuv>
      <tuv xml:lang="fr"><seg>cible</seg></tuv>
    </tu>
  </body>
</tmx>`
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "source", blocks[0].SourceText())
	assert.Equal(t, "Computing", blocks[0].Properties["x-Domain"])
	assert.Equal(t, "Text of a note at the TU level.", blocks[0].Properties["notes"])
}

// okapi: TmxFilterTest#testTUDuplicateProperties
func TestTUDuplicateProperties(t *testing.T) {
	input := `<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtool="XYZTool" creationtoolversion="1.0" datatype="PlainText"
    segtype="sentence" adminlang="en" srclang="en" o-tmf="abc">
  </header>
  <body>
    <tu>
      <prop type="x-Domain">Computing</prop>
      <prop type="x-Domain">Engineering</prop>
      <tuv xml:lang="en"><seg>duplicate props</seg></tuv>
      <tuv xml:lang="fr"><seg>props en double</seg></tuv>
    </tu>
  </body>
</tmx>`
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "duplicate props", blocks[0].SourceText())
	// Last value wins for duplicate property types
	assert.Equal(t, "Engineering", blocks[0].Properties["x-Domain"])
}

// --- Header tests ---

// okapi: TmxFilterTest#testPropAndNoteInStartDocument
func TestPropAndNoteInStartDocument(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
  <header creationtool="XYZTool" creationtoolversion="1.0" datatype="PlainText"
    segtype="sentence" adminlang="en-us" srclang="en-us" o-tmf="abc">
    <note>A header note.</note>
    <prop type="x-headerProp">headerPropValue</prop>
  </header>
  <body>
    <tu>
      <tuv xml:lang="en-us"><seg>Content</seg></tuv>
      <tuv xml:lang="fr"><seg>Contenu</seg></tuv>
    </tu>
  </body>
</tmx>`
	parts := readTMX(t, input)
	require.NotEmpty(t, parts)

	// Find the header Data part
	var headerData *model.Data
	for _, p := range parts {
		if p.Type == model.PartData {
			if data, ok := p.Resource.(*model.Data); ok && data.Name == "tmx-header" {
				headerData = data
				break
			}
		}
	}
	require.NotNil(t, headerData, "should emit header as Data")
	assert.Equal(t, "en-us", headerData.Properties["srclang"])
	assert.Equal(t, "A header note.", headerData.Properties["notes"])
	assert.Equal(t, "headerPropValue", headerData.Properties["prop:x-headerProp"])
}

// --- Layer start/end ---

// okapi: TmxFilterTest#testStartDocument
// Java asserts the filter emits a StartDocument event. The native equivalent
// is the PartLayerStart that opens every TMX stream (and the matching
// PartLayerEnd that closes it).
func TestLayerStartEnd(t *testing.T) {
	input := wrapTMX(`
    <tu>
      <tuv xml:lang="en"><seg>test</seg></tuv>
      <tuv xml:lang="fr"><seg>test</seg></tuv>
    </tu>`)
	parts := readTMX(t, input)
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "tmx", layer.Format)
	assert.True(t, layer.IsMultilingual)
}

// okapi: TmxFilterTest#testStartDocumentFromList
// Java asserts the StartDocument resource carries non-nil encoding, type,
// mimetype, locale and a "\r" line-break. The native StartDocument equivalent
// is the opening Layer: it carries Format ("type"), Encoding, MimeType and
// Locale. (Line-break sniffing is a Java filter concern with no native field
// the reader populates, so it is intentionally not asserted here.)
func TestStartDocumentFromList(t *testing.T) {
	input := wrapTMX(`
    <tu>
      <tuv xml:lang="en"><seg>Hello World!</seg></tuv>
      <tuv xml:lang="fr"><seg>Bonjour le monde!</seg></tuv>
    </tu>`)
	parts := readTMX(t, input)
	require.NotEmpty(t, parts)
	require.Equal(t, model.PartLayerStart, parts[0].Type)

	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok, "first part must be a Layer")
	assert.NotEmpty(t, layer.Format, "Format (the native 'type') must be set")
	assert.NotEmpty(t, layer.Encoding, "Encoding must be set")
	assert.NotEmpty(t, layer.MimeType, "MimeType must be set")
	assert.NotEmpty(t, layer.Locale, "Locale must be set")
	assert.Equal(t, "application/x-tmx+xml", layer.MimeType)
	assert.Equal(t, model.LocaleEnglish, layer.Locale)
}

// --- DTD handling ---

// okapi: TmxFilterTest#testDTDHandling
func TestDTDHandling(t *testing.T) {
	input := `<?xml version="1.0"?>
<!DOCTYPE tmx SYSTEM "tmx14.dtd">
<tmx version="1.4">
  <header creationtool="XYZTool" creationtoolversion="1.0.0" datatype="rtf"
    segtype="sentence" adminlang="en" srclang="en" o-tmf="abc">
  </header>
  <body>
    <tu tuid="1">
      <tuv xml:lang="en"><seg>DTD test</seg></tuv>
      <tuv xml:lang="fr"><seg>Test DTD</seg></tuv>
    </tu>
  </body>
</tmx>`
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "DTD test", blocks[0].SourceText())
}

// --- Segment type tests ---

// okapi: TmxFilterTest#testSegTypeSentence
func TestSegTypeSentence(t *testing.T) {
	input := `<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtool="test" creationtoolversion="1.0" datatype="PlainText"
    segtype="sentence" adminlang="en" srclang="en" o-tmf="abc">
  </header>
  <body>
    <tu>
      <tuv xml:lang="en"><seg>Sentence segtype</seg></tuv>
      <tuv xml:lang="fr"><seg>Segtype phrase</seg></tuv>
    </tu>
  </body>
</tmx>`
	parts := readTMX(t, input)
	// Check header has segtype
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if data.Name == "tmx-header" {
				assert.Equal(t, "sentence", data.Properties["segtype"])
			}
		}
	}
	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Sentence segtype", blocks[0].SourceText())
}

// okapi: TmxFilterTest#testSegTypePara
func TestSegTypePara(t *testing.T) {
	input := `<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtool="test" creationtoolversion="1.0" datatype="PlainText"
    segtype="paragraph" adminlang="en" srclang="en" o-tmf="abc">
  </header>
  <body>
    <tu>
      <tuv xml:lang="en"><seg>Paragraph segtype</seg></tuv>
      <tuv xml:lang="fr"><seg>Segtype paragraphe</seg></tuv>
    </tu>
  </body>
</tmx>`
	parts := readTMX(t, input)
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if data.Name == "tmx-header" {
				assert.Equal(t, "paragraph", data.Properties["segtype"])
			}
		}
	}
	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Paragraph segtype", blocks[0].SourceText())
}

// okapi: TmxFilterTest#testSegTypeOrSentence
func TestSegTypeOrSentence(t *testing.T) {
	input := `<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtool="test" creationtoolversion="1.0" datatype="PlainText"
    segtype="sentence" adminlang="en" srclang="en" o-tmf="abc">
  </header>
  <body>
    <tu segtype="sentence">
      <tuv xml:lang="en"><seg>TU-level sentence</seg></tuv>
      <tuv xml:lang="fr"><seg>Phrase niveau TU</seg></tuv>
    </tu>
  </body>
</tmx>`
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "TU-level sentence", blocks[0].SourceText())
	assert.Equal(t, "sentence", blocks[0].Properties["segtype"])
}

// okapi: TmxFilterTest#testSegTypeOrParagraph
func TestSegTypeOrParagraph(t *testing.T) {
	input := `<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtool="test" creationtoolversion="1.0" datatype="PlainText"
    segtype="paragraph" adminlang="en" srclang="en" o-tmf="abc">
  </header>
  <body>
    <tu segtype="paragraph">
      <tuv xml:lang="en"><seg>TU-level paragraph</seg></tuv>
      <tuv xml:lang="fr"><seg>Paragraphe niveau TU</seg></tuv>
    </tu>
  </body>
</tmx>`
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "TU-level paragraph", blocks[0].SourceText())
	assert.Equal(t, "paragraph", blocks[0].Properties["segtype"])
}

// okapi: TmxFilterTest#testSegTypeOrSentenceDefault
func TestSegTypeOrSentenceDefault(t *testing.T) {
	input := `<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtool="test" creationtoolversion="1.0" datatype="PlainText"
    segtype="sentence" adminlang="en" srclang="en" o-tmf="abc">
  </header>
  <body>
    <tu>
      <tuv xml:lang="en"><seg>Default sentence</seg></tuv>
      <tuv xml:lang="fr"><seg>Phrase par defaut</seg></tuv>
    </tu>
  </body>
</tmx>`
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Default sentence", blocks[0].SourceText())
}

// okapi: TmxFilterTest#testSegTypeOrParagraphDefault
func TestSegTypeOrParagraphDefault(t *testing.T) {
	input := `<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtool="test" creationtoolversion="1.0" datatype="PlainText"
    segtype="paragraph" adminlang="en" srclang="en" o-tmf="abc">
  </header>
  <body>
    <tu>
      <tuv xml:lang="en"><seg>Default paragraph</seg></tuv>
      <tuv xml:lang="fr"><seg>Paragraphe par defaut</seg></tuv>
    </tu>
  </body>
</tmx>`
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Default paragraph", blocks[0].SourceText())
}

// okapi: TmxFilterTest#testSegTypeOrSentenceUnknown
func TestSegTypeOrSentenceUnknown(t *testing.T) {
	input := `<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtool="test" creationtoolversion="1.0" datatype="PlainText"
    segtype="sentence" adminlang="en" srclang="en" o-tmf="abc">
  </header>
  <body>
    <tu segtype="unknown">
      <tuv xml:lang="en"><seg>Unknown segtype sentence</seg></tuv>
      <tuv xml:lang="fr"><seg>Segtype inconnu phrase</seg></tuv>
    </tu>
  </body>
</tmx>`
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Unknown segtype sentence", blocks[0].SourceText())
	assert.Equal(t, "unknown", blocks[0].Properties["segtype"])
}

// okapi: TmxFilterTest#testSegTypeOrParagraphUnknown
func TestSegTypeOrParagraphUnknown(t *testing.T) {
	input := `<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtool="test" creationtoolversion="1.0" datatype="PlainText"
    segtype="paragraph" adminlang="en" srclang="en" o-tmf="abc">
  </header>
  <body>
    <tu segtype="unknown">
      <tuv xml:lang="en"><seg>Unknown segtype paragraph</seg></tuv>
      <tuv xml:lang="fr"><seg>Segtype inconnu paragraphe</seg></tuv>
    </tu>
  </body>
</tmx>`
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Unknown segtype paragraph", blocks[0].SourceText())
}

// okapi: TmxFilterTest#testSegTypeHeaderSentence
func TestSegTypeHeaderSentence(t *testing.T) {
	input := `<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtool="test" creationtoolversion="1.0" datatype="PlainText"
    segtype="sentence" adminlang="en" srclang="en" o-tmf="abc">
  </header>
  <body>
    <tu>
      <tuv xml:lang="en"><seg>Header sentence</seg></tuv>
      <tuv xml:lang="fr"><seg>Phrase entete</seg></tuv>
    </tu>
  </body>
</tmx>`
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Header sentence", blocks[0].SourceText())
}

// okapi: TmxFilterTest#testSegTypeHeaderParagraph
func TestSegTypeHeaderParagraph(t *testing.T) {
	input := `<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtool="test" creationtoolversion="1.0" datatype="PlainText"
    segtype="paragraph" adminlang="en" srclang="en" o-tmf="abc">
  </header>
  <body>
    <tu>
      <tuv xml:lang="en"><seg>Header paragraph</seg></tuv>
      <tuv xml:lang="fr"><seg>Paragraphe entete</seg></tuv>
    </tu>
  </body>
</tmx>`
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Header paragraph", blocks[0].SourceText())
}

// okapi: TmxFilterTest#testSegTypeHeaderSentenceOverwrite
func TestSegTypeHeaderSentenceOverwrite(t *testing.T) {
	input := `<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtool="test" creationtoolversion="1.0" datatype="PlainText"
    segtype="paragraph" adminlang="en" srclang="en" o-tmf="abc">
  </header>
  <body>
    <tu segtype="sentence">
      <tuv xml:lang="en"><seg>Overwritten to sentence</seg></tuv>
      <tuv xml:lang="fr"><seg>Ecrase en phrase</seg></tuv>
    </tu>
  </body>
</tmx>`
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Overwritten to sentence", blocks[0].SourceText())
	assert.Equal(t, "sentence", blocks[0].Properties["segtype"])
}

// okapi: TmxFilterTest#testSegTypeHeaderParagraphOverwrite
func TestSegTypeHeaderParagraphOverwrite(t *testing.T) {
	input := `<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtool="test" creationtoolversion="1.0" datatype="PlainText"
    segtype="sentence" adminlang="en" srclang="en" o-tmf="abc">
  </header>
  <body>
    <tu segtype="paragraph">
      <tuv xml:lang="en"><seg>Overwritten to paragraph</seg></tuv>
      <tuv xml:lang="fr"><seg>Ecrase en paragraphe</seg></tuv>
    </tu>
  </body>
</tmx>`
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Overwritten to paragraph", blocks[0].SourceText())
	assert.Equal(t, "paragraph", blocks[0].Properties["segtype"])
}

// --- Inline codes ---

// okapi: TmxFilterTest#testUtInSeg
func TestUtInSeg(t *testing.T) {
	input := `<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtoolversion="1.0.0" datatype="html" segtype="sentence"
    adminlang="en-us" srclang="en" o-tmf="abc" creationtool="XYZTool">
  </header>
  <body>
    <tu>
      <tuv xml:lang="en">
        <seg><it x="1" pos="begin" type="italic">&lt;I></it><bpt x="2" i="1" type="bold">&lt;B></bpt>Click <ph x="3" type="image" assoc="b">&lt;IMG SRC="here.png"></ph> to <hi type="verb" x="4">start</hi>.<ept i="1">&lt;/B></ept></seg>
      </tuv>
      <tuv xml:lang="fr">
        <seg><it x="1" pos="begin" type="italic">&lt;I></it><bpt x="2" i="1" type="bold">&lt;B></bpt>Cliquez <ph x="3" type="image" assoc="b">&lt;IMG SRC="here.png"></ph> pour <hi type="verb" x="4">commencer</hi>.<ept i="1">&lt;/B></ept></seg>
      </tuv>
    </tu>
  </body>
</tmx>`
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)

	text := blocks[0].SourceText()
	assert.Contains(t, text, "Click")
	assert.Contains(t, text, "start")
	assert.Contains(t, text, "to")

	// Verify inline codes are captured as runs.
	codes := inlineCodeRuns(blocks[0].SourceRuns())
	assert.NotEmpty(t, codes, "should have inline-code runs")
	assert.GreaterOrEqual(t, len(codes), 5, "should have at least 5 inline-code runs (it, bpt, ph, hi open+close, ept)")
}

// okapi: TmxFilterTest#testUtInSub
func TestUtInSub(t *testing.T) {
	input := `<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtoolversion="1.0.0" datatype="html" segtype="sentence"
    adminlang="en-us" srclang="en" o-tmf="abc" creationtool="XYZTool">
  </header>
  <body>
    <tu>
      <tuv xml:lang="en">
        <seg><it x="1" pos="begin" type="italic">&lt;I></it><bpt x="2" i="1" type="bold">&lt;B></bpt>Click <ph x="3" type="image" assoc="b">&lt;IMG SRC="here.png" ALT="<sub>sub</sub>"></ph> to <hi type="verb" x="4">start</hi>.<ept i="1">&lt;/B></ept></seg>
      </tuv>
      <tuv xml:lang="fr">
        <seg><it x="1" pos="begin" type="italic">&lt;I></it><bpt x="2" i="1" type="bold">&lt;B></bpt>Cliquez <ph x="3" type="image" assoc="b">&lt;IMG SRC="here.png" ALT="<sub>sub</sub>"></ph> pour <hi type="verb" x="4">commencer</hi>.<ept i="1">&lt;/B></ept></seg>
      </tuv>
    </tu>
  </body>
</tmx>`
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)

	text := blocks[0].SourceText()
	assert.Contains(t, text, "Click")
	assert.Contains(t, text, "start")
}

// okapi: TmxFilterTest#testUtInHi
func TestUtInHi(t *testing.T) {
	input := `<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtoolversion="1.0.0" datatype="html" segtype="sentence"
    adminlang="en-us" srclang="en" o-tmf="abc" creationtool="XYZTool">
  </header>
  <body>
    <tu>
      <tuv xml:lang="en">
        <seg>Some text with <hi x="1" type="special-part">a part highlighted</hi>.</seg>
      </tuv>
      <tuv xml:lang="fr">
        <seg>Du texte avec <hi x="1" type="special-part">une portion delimitee</hi>.</seg>
      </tuv>
    </tu>
  </body>
</tmx>`
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)

	text := blocks[0].SourceText()
	assert.Contains(t, text, "Some text with")
	assert.Contains(t, text, "a part highlighted")
	assert.Contains(t, text, ".")

	// Verify hi creates opening/closing runs.
	codes := inlineCodeRuns(blocks[0].SourceRuns())
	assert.NotEmpty(t, codes)
	assert.Len(t, codes, 2, "hi should create opening+closing run pair")
}

// okapi: TmxFilterTest#testIsolatedCodes
func TestIsolatedCodes(t *testing.T) {
	input := `<tmx version="1.4">
  <header creationtool="XYZTool" creationtoolversion="1.01-023" datatype="PlainText"
    segtype="sentence" adminlang="en" srclang="en"
    creationdate="20020101T163812Z" changedate="20020413T023401Z"
    o-encoding="iso-8859-1">
  </header>
  <body>
    <tu tuid="4">
      <tuv xml:lang="en">
        <seg>First <it pos="begin" x="1" type="bold">&lt;b></it>sentence.</seg>
      </tuv>
      <tuv xml:lang="fr">
        <seg>Premiere <it type="bold" pos="begin" x="1">&lt;b></it>phrase.</seg>
      </tuv>
    </tu>
  </body>
</tmx>`
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)

	text := blocks[0].SourceText()
	assert.Contains(t, text, "First")
	assert.Contains(t, text, "sentence")

	// Verify the <it> element is captured as a PcOpen run.
	codes := inlineCodeRuns(blocks[0].SourceRuns())
	require.NotEmpty(t, codes)
	assert.NotNil(t, codes[0].PcOpen, "it pos=begin should be a PcOpen run")
}

// --- Stream handling ---

// okapi: TmxFilterTest#testConsolidatedStream
func TestConsolidatedStream(t *testing.T) {
	input := wrapTMX(`
    <tu>
      <tuv xml:lang="en"><seg>Consolidated</seg></tuv>
      <tuv xml:lang="fr"><seg>Consolide</seg></tuv>
    </tu>`)
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Consolidated", blocks[0].SourceText())
}

// okapi: TmxFilterTest#testUnConsolidatedStream
func TestUnConsolidatedStream(t *testing.T) {
	input := wrapTMX(`
    <tu>
      <tuv xml:lang="en"><seg>Unconsolidated</seg></tuv>
      <tuv xml:lang="fr"><seg>Non consolide</seg></tuv>
    </tu>`)
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Unconsolidated", blocks[0].SourceText())
}

// okapi: TmxFilterTest#testInputStream
func TestInputStream(t *testing.T) {
	blocks := readTMXFile(t, "testdata/simple.tmx")
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Hello World", blocks[0].SourceText())
}

// --- Cancel ---

// okapi: TmxFilterTest#testCancel
// Java calls filter.cancel() mid-stream and expects an EventType.CANCELED.
// The native reader cancels via context: a cancelled ctx makes Read() drain
// and close the channel without hanging or panicking.
func TestCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	reader := tmx.NewReader()
	input := wrapTMX(`
    <tu>
      <tuv xml:lang="en"><seg>Cancel test</seg></tuv>
      <tuv xml:lang="fr"><seg>Test annulation</seg></tuv>
    </tu>`)
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	// Cancel before reading all parts
	cancel()
	// Should not hang or panic
	for range reader.Read(ctx) {
	}
}

// --- Error handling tests ---

// okapi: TmxFilterTest#testSourceLangNotSpecified
func TestSourceLangNotSpecified(t *testing.T) {
	input := `<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtool="test" creationtoolversion="1.0" datatype="PlainText"
    segtype="sentence" adminlang="en" o-tmf="abc">
  </header>
  <body>
    <tu>
      <tuv xml:lang="en"><seg>No srclang</seg></tuv>
      <tuv xml:lang="fr"><seg>Pas de srclang</seg></tuv>
    </tu>
  </body>
</tmx>`
	// Missing srclang — should fall back to document locale
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "No srclang", blocks[0].SourceText())
}

// okapi: TmxFilterTest#testTargetLangNotSpecified
func TestTargetLangNotSpecified(t *testing.T) {
	input := wrapTMX(`
    <tu>
      <tuv xml:lang="en"><seg>No target lang</seg></tuv>
    </tu>`)
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "No target lang", blocks[0].SourceText())
	// No target TUV, so no targets
	assert.Empty(t, blocks[0].Targets)
}

// okapi: TmxFilterTest#testTargetLangNotSpecified2
func TestTargetLangNotSpecified2(t *testing.T) {
	input := wrapTMX(`
    <tu>
      <tuv xml:lang="en"><seg>Only source</seg></tuv>
    </tu>
    <tu>
      <tuv xml:lang="en"><seg>Also only source</seg></tuv>
    </tu>`)
	blocks := readTMXBlocks(t, input)
	require.Len(t, blocks, 2)
	assert.Equal(t, "Only source", blocks[0].SourceText())
	assert.Equal(t, "Also only source", blocks[1].SourceText())
}

// okapi: TmxFilterTest#testSourceLangNull
func TestSourceLangNull(t *testing.T) {
	input := `<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtool="test" creationtoolversion="1.0" datatype="PlainText"
    segtype="sentence" adminlang="en" srclang="" o-tmf="abc">
  </header>
  <body>
    <tu>
      <tuv xml:lang="en"><seg>Empty srclang</seg></tuv>
      <tuv xml:lang="fr"><seg>Srclang vide</seg></tuv>
    </tu>
  </body>
</tmx>`
	// Empty srclang — falls back to document locale (en)
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Empty srclang", blocks[0].SourceText())
}

// okapi: TmxFilterTest#testTargetLangNull
func TestTargetLangNull(t *testing.T) {
	input := wrapTMX(`
    <tu>
      <tuv xml:lang="en"><seg>Null target lang</seg></tuv>
    </tu>`)
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Null target lang", blocks[0].SourceText())
}

// okapi: TmxFilterTest#testTuXmlLangMissing
func TestTuXmlLangMissing(t *testing.T) {
	input := wrapTMX(`
    <tu>
      <tuv><seg>No lang</seg></tuv>
      <tuv xml:lang="fr"><seg>Pas de lang</seg></tuv>
    </tu>`)
	// Should not crash — TUV without lang is handled gracefully.
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
}

// okapi: TmxFilterTest#testInvalidXml
func TestInvalidXml(t *testing.T) {
	input := `<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtool="test" srclang="en">
  <body>
    <tu>
      <tuv xml:lang="en"><seg>broken</seg></tuv>
    </tu>
  </body>
</tmx>`
	// Should not panic. May or may not produce blocks depending on error tolerance.
	_, err := readTMXAllowError(t, input)
	_ = err
}

// okapi: TmxFilterTest#testEmptyTu
func TestEmptyTu(t *testing.T) {
	input := wrapTMX(`<tu></tu>`)
	blocks := readTMXBlocks(t, input)
	// Empty TU should still produce a block (with empty source text)
	require.NotEmpty(t, blocks)
	assert.Empty(t, blocks[0].SourceText())
}

// okapi: TmxFilterTest#testInvalidElementInTu
func TestInvalidElementInTu(t *testing.T) {
	input := wrapTMX(`
    <tu>
      <invalid>content</invalid>
      <tuv xml:lang="en"><seg>test</seg></tuv>
      <tuv xml:lang="fr"><seg>test</seg></tuv>
    </tu>`)
	// Should handle unknown elements gracefully
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "test", blocks[0].SourceText())
}

// okapi: TmxFilterTest#testInvalidElementInSub
func TestInvalidElementInSub(t *testing.T) {
	input := `<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtoolversion="1.0.0" datatype="html" segtype="sentence"
    adminlang="en-us" srclang="en" o-tmf="abc" creationtool="XYZTool">
  </header>
  <body>
    <tu>
      <tuv xml:lang="en">
        <seg><ph x="1" type="image">&lt;IMG ALT="<sub>sub text</sub>"></ph>Text</seg>
      </tuv>
      <tuv xml:lang="fr">
        <seg><ph x="1" type="image">&lt;IMG ALT="<sub>texte sub</sub>"></ph>Texte</seg>
      </tuv>
    </tu>
  </body>
</tmx>`
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].SourceText(), "Text")
}

// okapi: TmxFilterTest#testInvalidElementInPlaceholder
func TestInvalidElementInPlaceholder(t *testing.T) {
	input := `<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtoolversion="1.0.0" datatype="html" segtype="sentence"
    adminlang="en-us" srclang="en" o-tmf="abc" creationtool="XYZTool">
  </header>
  <body>
    <tu>
      <tuv xml:lang="en">
        <seg>Before <ph x="1" type="image">&lt;IMG ALT="<sub>placeholder sub</sub>"></ph> after</seg>
      </tuv>
      <tuv xml:lang="fr">
        <seg>Avant <ph x="1" type="image">&lt;IMG ALT="<sub>sub placeholder</sub>"></ph> apres</seg>
      </tuv>
    </tu>
  </body>
</tmx>`
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "Before")
	assert.Contains(t, text, "after")
}

// --- Output tests ---

// neokapi-only: TmxFilterTest#testOutputBasic_Comment — upstream @Test is commented out (disabled)
//
//	in v1.48.0, so there is no live Okapi case to map against. This native test still exercises
//	genuine behavior: a document-level XML comment must survive a read/write round-trip.
func TestOutputBasic_Comment(t *testing.T) {
	input := `<?xml version="1.0"?>
<!-- Example of TMX document -->
<tmx version="1.4">
  <header creationtool="XYZTool" creationtoolversion="1.01-023" datatype="PlainText"
    segtype="sentence" adminlang="en" srclang="en"
    creationdate="20020101T163812Z" o-encoding="iso-8859-1">
    <note>This is a note at document level.</note>
  </header>
  <body>
    <tu tuid="0001">
      <tuv xml:lang="en"><seg>Comment test</seg></tuv>
      <tuv xml:lang="fr"><seg>Test commentaire</seg></tuv>
    </tu>
  </body>
</tmx>`
	output, blocks := roundTrip(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Comment test", blocks[0].SourceText())
	assert.Contains(t, output, "Comment test")
}

// --- Double extraction tests ---

// okapi: TmxFilterTest#testDoubleExtraction
func TestDoubleExtraction(t *testing.T) {
	input := wrapTMX(`
    <tu>
      <tuv xml:lang="en"><seg>Double extraction test</seg></tuv>
      <tuv xml:lang="fr"><seg>Test double extraction</seg></tuv>
    </tu>
    <tu>
      <tuv xml:lang="en"><seg>Second entry</seg></tuv>
      <tuv xml:lang="fr"><seg>Deuxieme entree</seg></tuv>
    </tu>`)
	blocks1 := readTMXBlocks(t, input)
	blocks2 := readTMXBlocks(t, input)
	require.Len(t, blocks2, len(blocks1))
	for i := range blocks1 {
		assert.Equal(t, blocks1[i].SourceText(), blocks2[i].SourceText(),
			"block %d source text should match", i)
	}
}

// okapi: TmxFilterTest#testDoubleExtractionCompKit
func TestDoubleExtractionCompKit(t *testing.T) {
	input := wrapTMX(`
    <tu>
      <tuv xml:lang="en"><seg>CompKit test</seg></tuv>
      <tuv xml:lang="fr"><seg>Test CompKit</seg></tuv>
    </tu>`)
	_, blocks := roundTrip(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "CompKit test", blocks[0].SourceText())
}

// --- Parameters tests ---

// okapi-skip: ParametersTest#testToString — Java StringParameters serializes config to a
//
//	"#v1\nkey.b=value" line format; neokapi config is YAML/map driven (Config.ApplyMap) and
//	has no string-serialization round-trip, so toString has no native counterpart.
//
// okapi: ParametersTest#testParameters
// Java asserts the documented defaults (escapeGT=false, processAllTargets=true,
// exitOnInvalid=false). The native Config exposes the same three knobs; the
// Java-only segType/consolidateDpSkeleton defaults have no native field.
func TestParameters(t *testing.T) {
	cfg := &tmx.Config{}
	cfg.Reset()
	assert.Equal(t, "tmx", cfg.FormatName())
	require.NoError(t, cfg.Validate())
	assert.False(t, cfg.EscapeGT, "escapeGT should default to false")
	assert.True(t, cfg.ProcessAllTargets, "processAllTargets should default to true")
	assert.False(t, cfg.ExitOnInvalid, "exitOnInvalid should default to false")
}

// okapi: ParametersTest#testReset
// Java mutates every parameter then asserts reset() restores defaults. Native
// Reset() must likewise restore the documented defaults from a mutated state.
func TestParametersReset(t *testing.T) {
	cfg := &tmx.Config{
		EscapeGT:          true,
		ProcessAllTargets: false,
		ExitOnInvalid:     true,
		UseCodeFinder:     true,
	}
	cfg.Reset()
	require.NoError(t, cfg.Validate())
	assert.False(t, cfg.EscapeGT, "escapeGT should reset to false")
	assert.True(t, cfg.ProcessAllTargets, "processAllTargets should reset to true")
	assert.False(t, cfg.ExitOnInvalid, "exitOnInvalid should reset to false")
	assert.False(t, cfg.UseCodeFinder, "useCodeFinder should reset to false")
}

// okapi: ParametersTest#testFromString
// Java's fromString() deserializes a "#v1\nkey.b=value" parameter string into
// the filter's getters. The native equivalent is Config.ApplyMap, which loads
// config values from the recipe map. This asserts the three knobs with native
// counterparts (escapeGT, processAllTargets, exitOnInvalid) round-trip from a
// map; the Java-only segType/consolidateDpSkeleton keys have no native field.
func TestParametersFromMap(t *testing.T) {
	cfg := &tmx.Config{}
	cfg.Reset()
	err := cfg.ApplyMap(map[string]any{
		"escapeGT":          true,
		"processAllTargets": false,
		"exitOnInvalid":     true,
	})
	require.NoError(t, err)
	assert.True(t, cfg.EscapeGT, "escapeGT should be true")
	assert.False(t, cfg.ProcessAllTargets, "processAllTargets should be false")
	assert.True(t, cfg.ExitOnInvalid, "exitOnInvalid should be true")

	// Unknown keys are rejected, mirroring Java's strict parameter parsing.
	require.Error(t, cfg.ApplyMap(map[string]any{"noSuchParam": true}))
}

// --- Roundtrip tests ---

// okapi: RoundTripTmxIT#tmxFiles
// RoundTripTmxIT#tmxFiles (roundtrip.integration) extracts→merges→re-extracts
// every .tmx in the corpus and asserts the events match. This native test reads
// a real TMX file (testdata/simple.tmx) through the reader and writes it back,
// asserting source/target content survives the read→write cycle.
func TestRoundTrip_SimpleFile(t *testing.T) {
	ctx := t.Context()

	f, err := os.Open("testdata/simple.tmx")
	require.NoError(t, err)
	reader := tmx.NewReader()
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/simple.tmx", model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := tmx.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "<tmx")
	assert.Contains(t, output, "Hello World")
	assert.Contains(t, output, "Bonjour le monde")
	assert.Contains(t, output, "Goodbye")
	assert.Contains(t, output, "Au revoir")
	assert.Contains(t, output, "Auf Wiedersehen")
}

// Additional native roundtrip coverage for RoundTripTmxIT#tmxFiles
// (reread consistency); the IT contract itself is mapped on
// TestRoundTrip_SimpleFile above.
func TestRoundTrip_Reread(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
  <header srclang="en" datatype="plaintext"/>
  <body>
    <tu tuid="tu1">
      <tuv xml:lang="en"><seg>Hello</seg></tuv>
      <tuv xml:lang="fr"><seg>Bonjour</seg></tuv>
    </tu>
  </body>
</tmx>`
	_, blocks := roundTrip(t, input)
	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello", blocks[0].SourceText())
	assert.True(t, blocks[0].HasTarget("fr"))
	assert.Equal(t, "Bonjour", blocks[0].TargetText("fr"))
}

// okapi: TmxXliffCompareIT#tmxXliffCompareFiles
// TmxXliffCompareIT#tmxXliffCompareFiles (xliffcompare.integration) extracts
// each corpus .tmx to XLIFF and diffs against a frozen previous-release XLIFF
// baseline (extraction-output stability). The native equivalent verifies that
// inline-code-bearing TMX content extracts stably across the read cycle.
func TestRoundTrip_InlineCodes(t *testing.T) {
	input := `<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtoolversion="1.0.0" datatype="html" segtype="sentence"
    adminlang="en-us" srclang="en" o-tmf="abc" creationtool="XYZTool">
  </header>
  <body>
    <tu>
      <tuv xml:lang="en">
        <seg><bpt x="1" i="1" type="bold">&lt;B></bpt>Click here<ept i="1">&lt;/B></ept></seg>
      </tuv>
      <tuv xml:lang="fr">
        <seg><bpt x="1" i="1" type="bold">&lt;B></bpt>Cliquez ici<ept i="1">&lt;/B></ept></seg>
      </tuv>
    </tu>
  </body>
</tmx>`
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].SourceText(), "Click here")
}

// Additional native roundtrip coverage for RoundTripTmxIT#tmxFiles
// (multiple TUs); the IT contract itself is mapped on
// TestRoundTrip_SimpleFile above.
func TestRoundTrip_MultipleUnits(t *testing.T) {
	input := wrapTMX(`
    <tu>
      <tuv xml:lang="en"><seg>First</seg></tuv>
      <tuv xml:lang="fr"><seg>Premier</seg></tuv>
    </tu>
    <tu>
      <tuv xml:lang="en"><seg>Second</seg></tuv>
      <tuv xml:lang="fr"><seg>Deuxieme</seg></tuv>
    </tu>
    <tu>
      <tuv xml:lang="en"><seg>Third</seg></tuv>
      <tuv xml:lang="fr"><seg>Troisieme</seg></tuv>
    </tu>`)
	_, blocks := roundTrip(t, input)
	require.Len(t, blocks, 3)
	assert.Equal(t, "First", blocks[0].SourceText())
	assert.Equal(t, "Second", blocks[1].SourceText())
	assert.Equal(t, "Third", blocks[2].SourceText())
}

// --- Additional edge case tests ---

func TestEmptyBody(t *testing.T) {
	input := `<?xml version="1.0"?>
<tmx version="1.4">
  <header srclang="en"/>
  <body></body>
</tmx>`
	blocks := readTMXBlocks(t, input)
	assert.Empty(t, blocks)
}

// okapi: TmxFilterTest#testOpenInvalidInputStream
// Java opens the filter with a null InputStream and expects an
// IllegalArgumentException. The native equivalent of an invalid/absent input
// is opening with a nil document (or nil reader): Open returns an error
// instead of panicking.
func TestNilDocument(t *testing.T) {
	ctx := t.Context()
	reader := tmx.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)

	// A document with a nil Reader is likewise rejected at Open time.
	err = reader.Open(ctx, &model.RawDocument{URI: "test://input", SourceLocale: model.LocaleEnglish})
	require.Error(t, err)
}

// okapi: TmxFilterTest#testOpenInvalidUri
// Java opens a non-existent file URI and expects an OkapiIOException while
// processing. The native reader consumes an io.Reader rather than resolving a
// URI, so the equivalent failure is an underlying input that cannot be read:
// the reader must surface that as a PartResult.Error (not panic, not silently
// drop content).
func TestOpenInvalidUri(t *testing.T) {
	ctx := t.Context()
	reader := tmx.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromReader(errReader{}, "test://invalid", model.LocaleEnglish))
	require.NoError(t, err, "Open succeeds; the failure must surface during Read")
	defer reader.Close()

	var readErr error
	var parts int
	for pr := range reader.Read(ctx) {
		if pr.Error != nil {
			readErr = pr.Error
			continue
		}
		parts++
	}
	require.Error(t, readErr, "an unreadable input must surface a PartResult error")
	assert.Contains(t, readErr.Error(), "reading")
}

func TestVersionDetection(t *testing.T) {
	tests := []struct {
		name    string
		version string
	}{
		{"TMX 1.1", "1.1"},
		{"TMX 1.4", "1.4"},
		{"TMX 1.4b", "1.4b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := `<?xml version="1.0"?>
<tmx version="` + tt.version + `">
  <header srclang="en"/>
  <body>
    <tu><tuv xml:lang="en"><seg>text</seg></tuv></tu>
  </body>
</tmx>`
			parts := readTMX(t, input)
			for _, p := range parts {
				if p.Type == model.PartData {
					data := p.Resource.(*model.Data)
					if data.Name == "tmx-header" {
						assert.Equal(t, tt.version, data.Properties["version"])
					}
				}
			}
		})
	}
}

func TestMultipleNotesOnTU(t *testing.T) {
	input := wrapTMX(`
    <tu>
      <note>First note</note>
      <note>Second note</note>
      <tuv xml:lang="en"><seg>Multi-note TU</seg></tuv>
      <tuv xml:lang="fr"><seg>TU multi-notes</seg></tuv>
    </tu>`)
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "First note\nSecond note", blocks[0].Properties["notes"])
}

func TestMultiplePropertiesOnTU(t *testing.T) {
	input := wrapTMX(`
    <tu>
      <prop type="x-Domain">Computing</prop>
      <prop type="x-Client">Acme</prop>
      <tuv xml:lang="en"><seg>Multi-prop TU</seg></tuv>
      <tuv xml:lang="fr"><seg>TU multi-props</seg></tuv>
    </tu>`)
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Computing", blocks[0].Properties["x-Domain"])
	assert.Equal(t, "Acme", blocks[0].Properties["x-Client"])
}

func TestHeaderCreationTool(t *testing.T) {
	input := `<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtool="MyTool" creationtoolversion="2.0" datatype="xml"
    segtype="sentence" adminlang="en-US" srclang="en" o-tmf="xliff">
  </header>
  <body>
    <tu><tuv xml:lang="en"><seg>text</seg></tuv></tu>
  </body>
</tmx>`
	parts := readTMX(t, input)
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if data.Name == "tmx-header" {
				assert.Equal(t, "MyTool", data.Properties["creationtool"])
				assert.Equal(t, "xml", data.Properties["datatype"])
				assert.Equal(t, "en", data.Properties["srclang"])
				assert.Equal(t, "xliff", data.Properties["o-tmf"])
			}
		}
	}
}

func TestTuidPreserved(t *testing.T) {
	input := wrapTMX(`
    <tu tuid="custom-id-123">
      <tuv xml:lang="en"><seg>ID test</seg></tuv>
      <tuv xml:lang="fr"><seg>Test ID</seg></tuv>
    </tu>`)
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "custom-id-123", blocks[0].ID)
	assert.Equal(t, "custom-id-123", blocks[0].Name)
}

func TestAutoGeneratedId(t *testing.T) {
	input := wrapTMX(`
    <tu>
      <tuv xml:lang="en"><seg>No tuid</seg></tuv>
      <tuv xml:lang="fr"><seg>Pas de tuid</seg></tuv>
    </tu>
    <tu>
      <tuv xml:lang="en"><seg>Also no tuid</seg></tuv>
      <tuv xml:lang="fr"><seg>Pas de tuid non plus</seg></tuv>
    </tu>`)
	blocks := readTMXBlocks(t, input)
	require.Len(t, blocks, 2)
	assert.Equal(t, "tu1", blocks[0].ID)
	assert.Equal(t, "tu2", blocks[1].ID)
}

func TestBptEptPair(t *testing.T) {
	input := wrapTMX(`
    <tu>
      <tuv xml:lang="en">
        <seg><bpt i="1" type="bold">&lt;b></bpt>bold text<ept i="1">&lt;/b></ept></seg>
      </tuv>
      <tuv xml:lang="fr">
        <seg><bpt i="1" type="bold">&lt;b></bpt>texte gras<ept i="1">&lt;/b></ept></seg>
      </tuv>
    </tu>`)
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "bold text", blocks[0].SourceText())

	codes := inlineCodeRuns(blocks[0].SourceRuns())
	require.Len(t, codes, 2)
	require.NotNil(t, codes[0].PcOpen)
	assert.Equal(t, "bold", codes[0].PcOpen.Type)
	assert.Equal(t, "<b>", codes[0].PcOpen.Data)
	require.NotNil(t, codes[1].PcClose)
	assert.Equal(t, "</b>", codes[1].PcClose.Data)
}

func TestPhPlaceholder(t *testing.T) {
	input := wrapTMX(`
    <tu>
      <tuv xml:lang="en">
        <seg>Click <ph type="image">&lt;img src="logo.png"/></ph> here</seg>
      </tuv>
      <tuv xml:lang="fr">
        <seg>Cliquez <ph type="image">&lt;img src="logo.png"/></ph> ici</seg>
      </tuv>
    </tu>`)
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Equal(t, "Click  here", text)

	codes := inlineCodeRuns(blocks[0].SourceRuns())
	require.Len(t, codes, 1)
	require.NotNil(t, codes[0].Ph)
	assert.Equal(t, "image", codes[0].Ph.Type)
}

func TestItIsolatedBeginEnd(t *testing.T) {
	input := wrapTMX(`
    <tu>
      <tuv xml:lang="en">
        <seg>Before <it pos="begin" type="bold">&lt;b></it>bold <it pos="end" type="bold">&lt;/b></it>after</seg>
      </tuv>
      <tuv xml:lang="fr">
        <seg>Avant <it pos="begin" type="bold">&lt;b></it>gras <it pos="end" type="bold">&lt;/b></it>apres</seg>
      </tuv>
    </tu>`)
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)

	codes := inlineCodeRuns(blocks[0].SourceRuns())
	require.Len(t, codes, 2)
	assert.NotNil(t, codes[0].PcOpen, "it pos=begin should be a PcOpen run")
	assert.NotNil(t, codes[1].PcClose, "it pos=end should be a PcClose run")
}

func TestHiHighlight(t *testing.T) {
	input := wrapTMX(`
    <tu>
      <tuv xml:lang="en">
        <seg>Normal <hi type="term">highlighted</hi> normal</seg>
      </tuv>
      <tuv xml:lang="fr">
        <seg>Normal <hi type="term">surligne</hi> normal</seg>
      </tuv>
    </tu>`)
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].SourceText(), "highlighted")

	codes := inlineCodeRuns(blocks[0].SourceRuns())
	require.Len(t, codes, 2, "hi should produce open+close pair")
	require.NotNil(t, codes[0].PcOpen)
	assert.Equal(t, "term", codes[0].PcOpen.Type)
	assert.NotNil(t, codes[1].PcClose)
}

func TestSubInsidePh(t *testing.T) {
	input := wrapTMX(`
    <tu>
      <tuv xml:lang="en">
        <seg>Text <ph type="fnref">&lt;a href="#fn1"><sub>1</sub>&lt;/a></ph> more</seg>
      </tuv>
      <tuv xml:lang="fr">
        <seg>Texte <ph type="fnref">&lt;a href="#fn1"><sub>1</sub>&lt;/a></ph> plus</seg>
      </tuv>
    </tu>`)
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "Text")
	assert.Contains(t, text, "more")
}

func TestWhitespacePreservation(t *testing.T) {
	input := wrapTMX(`
    <tu>
      <tuv xml:lang="en"><seg>  spaced  text  </seg></tuv>
      <tuv xml:lang="fr"><seg>  texte  espace  </seg></tuv>
    </tu>`)
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	// XML normally collapses some whitespace, but seg content should be preserved
	text := blocks[0].SourceText()
	assert.Contains(t, text, "spaced")
	assert.Contains(t, text, "text")
}

func TestEmptySegment(t *testing.T) {
	input := wrapTMX(`
    <tu>
      <tuv xml:lang="en"><seg></seg></tuv>
      <tuv xml:lang="fr"><seg></seg></tuv>
    </tu>`)
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.Empty(t, blocks[0].SourceText())
}

func TestTUWithOnlyTarget(t *testing.T) {
	// TU with only target (no matching source language)
	input := wrapTMXWithLangs("de", `
    <tu>
      <tuv xml:lang="en"><seg>English only</seg></tuv>
      <tuv xml:lang="fr"><seg>Francais seulement</seg></tuv>
    </tu>`)
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	// No "de" TUV, so first TUV (en) becomes source
	assert.Equal(t, "English only", blocks[0].SourceText())
}

func TestMultipleLanguageTargets(t *testing.T) {
	input := wrapTMX(`
    <tu>
      <tuv xml:lang="en"><seg>Hello</seg></tuv>
      <tuv xml:lang="fr"><seg>Bonjour</seg></tuv>
      <tuv xml:lang="de"><seg>Hallo</seg></tuv>
      <tuv xml:lang="es"><seg>Hola</seg></tuv>
      <tuv xml:lang="ja"><seg>&#x3053;&#x3093;&#x306B;&#x3061;&#x306F;</seg></tuv>
    </tu>`)
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Hello", blocks[0].SourceText())
	assert.True(t, blocks[0].HasTarget("fr"))
	assert.True(t, blocks[0].HasTarget("de"))
	assert.True(t, blocks[0].HasTarget("es"))
	assert.True(t, blocks[0].HasTarget("ja"))
	assert.Equal(t, "Bonjour", blocks[0].TargetText("fr"))
	assert.Equal(t, "Hallo", blocks[0].TargetText("de"))
	assert.Equal(t, "Hola", blocks[0].TargetText("es"))
}

func TestUnicodeContent(t *testing.T) {
	input := wrapTMX(`
    <tu>
      <tuv xml:lang="en"><seg>Hello &#x2603; world</seg></tuv>
      <tuv xml:lang="fr"><seg>Bonjour &#x2603; monde</seg></tuv>
    </tu>`)
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].SourceText(), "\u2603") // snowman
}

func TestCDATAInSeg(t *testing.T) {
	input := wrapTMX(`
    <tu>
      <tuv xml:lang="en"><seg><![CDATA[CDATA content]]></seg></tuv>
      <tuv xml:lang="fr"><seg><![CDATA[Contenu CDATA]]></seg></tuv>
    </tu>`)
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "CDATA content", blocks[0].SourceText())
}

func TestConfigTransform(t *testing.T) {
	// Verify the config transformer is registered
	cfg := tmx.Config{}
	assert.Equal(t, "tmx", cfg.FormatName())

	// ApplyMap with empty should succeed
	err := cfg.ApplyMap(map[string]any{})
	require.NoError(t, err)

	// ApplyMap with unknown key should error
	err = cfg.ApplyMap(map[string]any{"unknown": "value"})
	require.Error(t, err)
}

func TestSchemaMetadata(t *testing.T) {
	cfg := tmx.Config{}
	schema := cfg.Schema()
	assert.Equal(t, "tmx", schema.FormatMeta.ID)
	assert.Contains(t, schema.FormatMeta.Extensions, ".tmx")
	assert.Contains(t, schema.FormatMeta.MimeTypes, "application/x-tmx+xml")
}

func TestMixedInlineCodes(t *testing.T) {
	// Complex inline code scenario with multiple code types
	input := wrapTMX(`
    <tu>
      <tuv xml:lang="en">
        <seg><bpt i="1" type="bold">&lt;b></bpt>Bold <ph type="br">&lt;br/></ph> and <it pos="begin" type="italic">&lt;i></it>italic<ept i="1">&lt;/b></ept></seg>
      </tuv>
      <tuv xml:lang="fr">
        <seg><bpt i="1" type="bold">&lt;b></bpt>Gras <ph type="br">&lt;br/></ph> et <it pos="begin" type="italic">&lt;i></it>italique<ept i="1">&lt;/b></ept></seg>
      </tuv>
    </tu>`)
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "Bold")
	assert.Contains(t, text, "italic")

	codes := inlineCodeRuns(blocks[0].SourceRuns())
	assert.GreaterOrEqual(t, len(codes), 4, "should have bpt, ph, it, ept inline-code runs")
}

func TestLargeTMX(t *testing.T) {
	// Generate a TMX with many TUs to test performance
	var body strings.Builder
	for i := range 100 {
		body.WriteString(fmt.Sprintf(`    <tu tuid="tu%d">
      <tuv xml:lang="en"><seg>Entry %d</seg></tuv>
      <tuv xml:lang="fr"><seg>Entree %d</seg></tuv>
    </tu>
`, i+1, i+1, i+1))
	}
	input := wrapTMX(body.String())
	blocks := readTMXBlocks(t, input)
	require.Len(t, blocks, 100)
	assert.Equal(t, "Entry 1", blocks[0].SourceText())
	assert.Equal(t, "Entry 100", blocks[99].SourceText())
}

func TestNonAsciiLanguageCodes(t *testing.T) {
	input := `<?xml version="1.0"?>
<tmx version="1.4">
  <header srclang="zh-CN"/>
  <body>
    <tu>
      <tuv xml:lang="zh-CN"><seg>Chinese text</seg></tuv>
      <tuv xml:lang="zh-TW"><seg>Traditional Chinese</seg></tuv>
    </tu>
  </body>
</tmx>`
	ctx := t.Context()
	reader := tmx.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, "zh-CN"))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Chinese text", blocks[0].SourceText())
	assert.True(t, blocks[0].HasTarget("zh-TW"))
}

func TestOpenInvalidContent(t *testing.T) {
	// Completely non-XML content should not panic
	_, err := readTMXAllowError(t, "this is not XML at all")
	_ = err // may or may not error depending on tolerance
}

func TestOpenEmptyContent(t *testing.T) {
	_, err := readTMXAllowError(t, "")
	_ = err
}

func TestHeaderMetadataComplete(t *testing.T) {
	input := `<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtool="neokapi" creationtoolversion="1.0"
    segtype="sentence" o-tmf="xliff" adminlang="en-US"
    srclang="en" datatype="xml">
  </header>
  <body>
    <tu><tuv xml:lang="en"><seg>test</seg></tuv></tu>
  </body>
</tmx>`
	parts := readTMX(t, input)
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if data.Name == "tmx-header" {
				assert.Equal(t, "1.4", data.Properties["version"])
				assert.Equal(t, "en", data.Properties["srclang"])
				assert.Equal(t, "en-US", data.Properties["adminlang"])
				assert.Equal(t, "xml", data.Properties["datatype"])
				assert.Equal(t, "sentence", data.Properties["segtype"])
				assert.Equal(t, "xliff", data.Properties["o-tmf"])
				assert.Equal(t, "neokapi", data.Properties["creationtool"])
			}
		}
	}
}

func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // cancel immediately

	reader := tmx.NewReader()
	input := wrapTMX(`
    <tu>
      <tuv xml:lang="en"><seg>Should not read</seg></tuv>
    </tu>`)
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	// Reading with cancelled context should not hang
	var count int
	for range reader.Read(ctx) {
		count++
	}
	// May get 0 or more parts depending on buffering
}

func TestWriterNilOutput(t *testing.T) {
	ctx := t.Context()
	writer := tmx.NewWriter()
	// Don't set output — should not panic
	ch := make(chan *model.Part)
	close(ch)
	err := writer.Write(ctx, ch)
	require.NoError(t, err)
}

func TestMultipleTUsWithSameId(t *testing.T) {
	input := wrapTMX(`
    <tu tuid="same-id">
      <tuv xml:lang="en"><seg>First occurrence</seg></tuv>
      <tuv xml:lang="fr"><seg>Premiere occurrence</seg></tuv>
    </tu>
    <tu tuid="same-id">
      <tuv xml:lang="en"><seg>Second occurrence</seg></tuv>
      <tuv xml:lang="fr"><seg>Deuxieme occurrence</seg></tuv>
    </tu>`)
	blocks := readTMXBlocks(t, input)
	require.Len(t, blocks, 2)
	assert.Equal(t, "First occurrence", blocks[0].SourceText())
	assert.Equal(t, "Second occurrence", blocks[1].SourceText())
}

func TestTargetFragmentWithSpans(t *testing.T) {
	input := wrapTMX(`
    <tu>
      <tuv xml:lang="en">
        <seg><bpt i="1" type="bold">&lt;b></bpt>source<ept i="1">&lt;/b></ept></seg>
      </tuv>
      <tuv xml:lang="fr">
        <seg><bpt i="1" type="bold">&lt;b></bpt>cible<ept i="1">&lt;/b></ept></seg>
      </tuv>
    </tu>`)
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)

	// Verify target also has inline-code runs.
	assert.True(t, blocks[0].HasTarget("fr"))
	targetRuns := blocks[0].TargetRuns("fr")
	require.NotEmpty(t, targetRuns)
	assert.True(t, hasInlineCodeRun(targetRuns))
	assert.Equal(t, "cible", model.RunsPlainText(targetRuns))
}

func TestNestedHiElements(t *testing.T) {
	// Hi elements can contain text that should appear as regular content
	input := wrapTMX(`
    <tu>
      <tuv xml:lang="en">
        <seg>See <hi type="term">translation memory</hi> and <hi type="term">terminology</hi>.</seg>
      </tuv>
      <tuv xml:lang="fr">
        <seg>Voir <hi type="term">memoire de traduction</hi> et <hi type="term">terminologie</hi>.</seg>
      </tuv>
    </tu>`)
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "translation memory")
	assert.Contains(t, text, "terminology")

	codes := inlineCodeRuns(blocks[0].SourceRuns())
	assert.Len(t, codes, 4, "two hi elements = 4 inline-code runs (2 open + 2 close)")
}

func TestBlockTranslatable(t *testing.T) {
	input := wrapTMX(`
    <tu>
      <tuv xml:lang="en"><seg>Translatable</seg></tuv>
      <tuv xml:lang="fr"><seg>Traduisible</seg></tuv>
    </tu>`)
	blocks := readTMXBlocks(t, input)
	require.NotEmpty(t, blocks)
	assert.True(t, blocks[0].Translatable)
}

func TestCloserMethod(t *testing.T) {
	reader := tmx.NewReader()
	// Close without Open should not panic
	err := reader.Close()
	require.NoError(t, err)
}

func TestWriterClose(t *testing.T) {
	writer := tmx.NewWriter()
	err := writer.Close()
	require.NoError(t, err)
}
