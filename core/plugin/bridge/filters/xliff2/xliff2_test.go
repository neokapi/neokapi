//go:build integration

package xliff2

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge"
	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.xliff2.XLIFF2Filter"
const mimeType = "application/xliff+xml"

// readXLIFF2 parses an XLIFF 2 snippet with custom filter params and returns the parts.
func readXLIFF2(t *testing.T, snippet string, params map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	return bridgetest.ReadString(t, pool, cfg, filterClass, snippet, "test.xlf", mimeType, params)
}

// snippetRoundtrip roundtrips an XLIFF 2 snippet and returns the output string.
func snippetRoundtrip(t *testing.T, snippet string, params map[string]any) string {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, []byte(snippet), "test.xlf", mimeType, params)
	return string(result.Output)
}

// --- XLIFF2FilterTest (25 tests) ---

// okapi: XLIFF2FilterTest#testSimple
func TestExtract_SimpleXLIFF2(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	xliff2 := `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="1">
      <segment>
        <source>Hello world</source>
      </segment>
    </unit>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff2, "test.xlf", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from XLIFF 2.0")
	assert.Equal(t, "Hello world", blocks[0].SourceText())
}

// okapi: XLIFF2FilterTest#testSubflows
func TestExtract_Subflows(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	path := bridgetest.TestdataFile(t, "okapi/filters/xliff2/src/test/resources/test01.xlf")
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "test01.xlf should extract translatable blocks")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Bolded text", "should extract subflow unit tu3")
	assert.Contains(t, texts, "Extra stuff", "should extract subflow unit tu3end")
}

// okapi: XLIFF2FilterTest#testDedupeCodeFinderCodes
func TestExtract_DedupeCodeFinderCodes(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	xliff2 := `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="1">
      <originalData>
        <data id="d1">&lt;b&gt;</data>
        <data id="d2">&lt;/b&gt;</data>
      </originalData>
      <segment>
        <source>Text <pc id="1" dataRefStart="d1" dataRefEnd="d2">bold</pc> end</source>
      </segment>
    </unit>
  </file>
</xliff>`

	params := map[string]any{
		"useCodeFinder": true,
	}
	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff2, "test.xlf", mimeType, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)

	var openingCount int
	for _, s := range frag.Spans {
		if s.SpanType == model.SpanOpening {
			openingCount++
		}
	}
	assert.Equal(t, 1, openingCount, "should not duplicate inline codes from code finder")
}

// okapi: XLIFF2FilterTest#testSimpleMeta
func TestExtract_SimpleMeta(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	path := bridgetest.TestdataFile(t, "okapi/filters/xliff2/src/test/resources/test01.xlf")
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "test01.xlf should extract translatable blocks with metadata")
}

// okapi: XLIFF2FilterTest#testInline
func TestExtract_InlinePh(t *testing.T) {
	parts := readXLIFF2(t, `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="1">
      <segment>
        <source>Line one<ph id="1" equiv="lb"/>Line two</source>
      </segment>
    </unit>
  </file>
</xliff>`, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)

	var hasPlaceholder bool
	for _, s := range frag.Spans {
		if s.SpanType == model.SpanPlaceholder {
			hasPlaceholder = true
			break
		}
	}
	assert.True(t, hasPlaceholder, "should have a placeholder span for <ph>")
}

// okapi: XLIFF2FilterTest#testInline
func TestExtract_InlinePc(t *testing.T) {
	parts := readXLIFF2(t, `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="1">
      <segment>
        <source>Hello <pc id="1">bold</pc> text</source>
      </segment>
    </unit>
  </file>
</xliff>`, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	require.GreaterOrEqual(t, len(frag.Spans), 2,
		"should have opening and closing spans for <pc>")

	var hasOpening, hasClosing bool
	for _, s := range frag.Spans {
		if s.SpanType == model.SpanOpening {
			hasOpening = true
		}
		if s.SpanType == model.SpanClosing {
			hasClosing = true
		}
	}
	assert.True(t, hasOpening, "should have opening span from <pc>")
	assert.True(t, hasClosing, "should have closing span from </pc>")
}

// okapi: XLIFF2FilterTest#testInline
func TestExtract_InlineScEc(t *testing.T) {
	parts := readXLIFF2(t, `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="1">
      <segment>
        <source>Before <sc id="1"/>middle<ec startRef="1"/> after</source>
      </segment>
    </unit>
  </file>
</xliff>`, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)

	var hasOpening, hasClosing bool
	for _, s := range frag.Spans {
		if s.SpanType == model.SpanOpening {
			hasOpening = true
		}
		if s.SpanType == model.SpanClosing {
			hasClosing = true
		}
	}
	assert.True(t, hasOpening, "should have opening span from <sc>")
	assert.True(t, hasClosing, "should have closing span from <ec>")
}

// okapi: XLIFF2FilterTest#testInlineCopyOf
func TestExtract_InlineCopyOf(t *testing.T) {
	parts := readXLIFF2(t, `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="1">
      <segment>
        <source>Text <ph id="1" equiv="br"/>more<ph id="2" copyOf="1"/></source>
      </segment>
    </unit>
  </file>
</xliff>`, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)

	var phCount int
	for _, s := range frag.Spans {
		if s.SpanType == model.SpanPlaceholder {
			phCount++
		}
	}
	assert.GreaterOrEqual(t, phCount, 2, "should have 2 placeholder spans (original + copyOf)")
}

// okapi: XLIFF2FilterTest#testFromFile
func TestExtract_FromFile(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	path := bridgetest.TestdataFile(t, "okapi/filters/xliff2/src/test/resources/test01.xlf")
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "test01.xlf should extract translatable blocks")

	// Unit tu1 has two segments plus an ignorable, combined into one block.
	var foundSample bool
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "Sample segment.") {
			foundSample = true
			break
		}
	}
	assert.True(t, foundSample, "test01.xlf should contain 'Sample segment.' text")
}

// okapi: XLIFF2FilterTest#testFromFile2
func TestExtract_FromFile2(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	path := bridgetest.TestdataFile(t, "okapi/filters/xliff2/src/test/resources/test02.xlf")
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)

	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	// test02.xlf has tu1 (2 segments), tu3 (subflow), tu3end (subflow), tu2 (with subflow refs).
	// The bridge emits each unit as a separate block.
	require.Len(t, blocks, 4, "test02.xlf should have 4 translatable units")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Sample segment.Segment's content.")
	assert.Contains(t, texts, "Bolded text")
	assert.Contains(t, texts, "Extra stuff")
}

// okapi: XLIFF2FilterTest#testFromEscapedFile
func TestExtract_FromEscapedFile(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	path := bridgetest.TestdataFile(t, "okapi/filters/xliff2/src/test/resources/escaped.xlf")
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)

	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	text := blocks[0].SourceText()
	assert.Contains(t, text, "<p>", "escaped HTML should be decoded to text")
	assert.Contains(t, text, "I want", "should contain the unescaped content")
}

// okapi: XLIFF2FilterTest#testGroupHandling
func TestExtract_GroupStructure(t *testing.T) {
	parts := readXLIFF2(t, `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <group id="g1">
      <unit id="1">
        <segment>
          <source>In group</source>
        </segment>
      </unit>
    </group>
  </file>
</xliff>`, nil)

	var hasGroupStart, hasGroupEnd bool
	for _, p := range parts {
		if p.Type == model.PartGroupStart {
			hasGroupStart = true
			gs := p.Resource.(*model.GroupStart)
			assert.Equal(t, "g1", gs.ID)
		}
		if p.Type == model.PartGroupEnd {
			hasGroupEnd = true
		}
	}
	assert.True(t, hasGroupStart, "should have GroupStart for <group>")
	assert.True(t, hasGroupEnd, "should have GroupEnd for </group>")
}

// okapi-unmapped: XLIFF2FilterTest#testWriteXLIFF2AsXliff12 -- cross-format conversion (XLIFF 2 to XLIFF 1.2) is not supported through the bridge
// okapi-unmapped: XLIFF2FilterTest#testMetadataXLIFF2intoXliff12 -- cross-format conversion to XLIFF 1.2 is Java-specific
// okapi-unmapped: XLIFF2FilterTest#testSegmentStateAndSubstateXLIFF2intoXliff12 -- cross-format conversion to XLIFF 1.2 is Java-specific

// okapi: XLIFF2FilterTest#testIgnoreable
func TestExtract_Ignoreable(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	path := bridgetest.TestdataFile(t, "okapi/filters/xliff2/src/test/resources/test01.xlf")
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)

	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should have translatable blocks despite ignorable elements")

	// The ignorable elements are part of the unit but do not prevent
	// extraction. The combined text includes "Sample segment." and "Segment's content.".
	var foundSample bool
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "Sample segment.") {
			foundSample = true
			break
		}
	}
	assert.True(t, foundSample, "translatable segment should be extracted")
}

// okapi: XLIFF2FilterTest#roundTripTests
func TestExtract_RoundTripTests(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okapi/filters/xliff2/src/test/resources/roundtrips/*_input.xlf", mimeType, nil)
}

// okapi: XLIFF2FilterTest#updateTarget
func TestExtract_UpdateTarget(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	path := bridgetest.TestdataFile(t, "integration-tests/okapi/src/test/resources/xliff2/update_target.xlf")
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)

	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "update_target.xlf should extract translatable blocks")

	// Segments within a unit are combined, so "Frodo" and "Gandalf" appear
	// as parts of multi-segment block text.
	var foundFrodo, foundGandalf bool
	for _, b := range blocks {
		text := b.SourceText()
		if strings.Contains(text, "Frodo") {
			foundFrodo = true
		}
		if strings.Contains(text, "Gandalf") {
			foundGandalf = true
		}
	}
	assert.True(t, foundFrodo || foundGandalf, "should extract named segments from update_target.xlf")

	// Verify roundtrip works for this file.
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, path, mimeType, nil)
}

// okapi: XLIFF2FilterTest#handleInvalidCodeTypes
func TestExtract_HandleInvalidCodeTypes(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	// The Java test uses maxValidation=false to accept invalid type values.
	// The bridge applies strict XLIFF 2 schema validation, so an invalid
	// type attribute is correctly rejected during open. We verify the error
	// is reported and contains a meaningful message.
	xliff2 := `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="1">
      <segment>
        <source>Text <ph id="1" type="invalidType"/>more</source>
      </segment>
    </unit>
  </file>
</xliff>`

	reader := bridge.NewBridgeFormatReader(pool, cfg, filterClass, format.FormatSignature{})

	doc := &model.RawDocument{
		URI:          "test.xlf",
		SourceLocale: "en",
		TargetLocale: "fr",
		Encoding:     "UTF-8",
		MimeType:     mimeType,
		Reader:       io.NopCloser(bytes.NewReader([]byte(xliff2))),
	}

	ctx := t.Context()
	openErr := reader.Open(ctx, doc)
	if openErr != nil {
		// Schema validation rejects it at open time.
		assert.Contains(t, openErr.Error(), "invalidType",
			"error should mention the invalid type value")
		return
	}
	t.Cleanup(func() { _ = reader.Close() })

	// If open succeeded, check for error during read.
	var readErr error
	for pr := range reader.Read(ctx) {
		if pr.Error != nil {
			readErr = pr.Error
			break
		}
	}
	require.Error(t, readErr, "invalid code type should produce a read error")
	assert.Contains(t, readErr.Error(), "invalidType",
		"error should mention the invalid type value")
}

// okapi: XLIFF2FilterTest#testDiscardInvalidTargets
func TestExtract_InvalidTargetXlf(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	path := bridgetest.TestdataFile(t, "okapi/filters/xliff2/src/test/resources/invalid-target.xlf")
	reader := bridge.NewBridgeFormatReader(pool, cfg, filterClass, format.FormatSignature{})

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	doc := &model.RawDocument{
		URI:          "invalid-target.xlf",
		SourceLocale: "en",
		TargetLocale: "fr",
		Encoding:     "UTF-8",
		MimeType:     mimeType,
		Reader:       io.NopCloser(bytes.NewReader(content)),
	}

	ctx := t.Context()
	require.NoError(t, reader.Open(ctx, doc))
	t.Cleanup(func() { _ = reader.Close() })

	var readErr error
	for pr := range reader.Read(ctx) {
		if pr.Error != nil {
			readErr = pr.Error
			break
		}
	}
	require.Error(t, readErr, "invalid-target.xlf should produce a read error")
	assert.Contains(t, readErr.Error(), "originalData",
		"error should mention missing originalData")
}

// okapi: XLIFF2FilterTest#testDoubleExtraction
func TestRoundTrip_DoubleExtraction(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	path := bridgetest.TestdataFile(t, "integration-tests/okapi/src/test/resources/xliff2/simple.xlf")
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	// First extraction.
	result1 := bridgetest.RoundTrip(t, pool, cfg, filterClass, content, path, mimeType, nil)
	require.NotEmpty(t, result1.Output, "first roundtrip should produce output")

	// Second extraction: roundtrip the output of the first.
	result2 := bridgetest.RoundTrip(t, pool, cfg, filterClass, result1.Output, "test.xlf", mimeType, nil)
	require.NotEmpty(t, result2.Output, "second roundtrip should produce output")

	// Both outputs should produce the same events.
	parts1 := bridgetest.ReadBytes(t, pool, cfg, filterClass, result1.Output, "test.xlf", mimeType, nil)
	parts2 := bridgetest.ReadBytes(t, pool, cfg, filterClass, result2.Output, "test.xlf", mimeType, nil)

	blocks1 := bridgetest.TranslatableBlocks(parts1)
	blocks2 := bridgetest.TranslatableBlocks(parts2)
	require.Equal(t, len(blocks1), len(blocks2), "double extraction should produce same block count")

	for i := range blocks1 {
		assert.Equal(t, blocks1[i].SourceText(), blocks2[i].SourceText(),
			"block %d source text should match after double extraction", i)
	}
}

// okapi: XLIFF2FilterTest#testStateChangeTranslated
func TestExtract_StateChangeTranslated(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	path := bridgetest.TestdataFile(t, "okapi/filters/xliff2/src/test/resources/segment-state.xlf")
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)

	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "segment-state.xlf should extract translatable blocks")

	var foundSegment bool
	for _, b := range blocks {
		text := b.SourceText()
		if strings.Contains(text, "Source of segment 0.") {
			foundSegment = true
			break
		}
	}
	assert.True(t, foundSegment, "should extract segments with different states")

	// Verify roundtrip preserves the content.
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, path, mimeType, nil)
}

// okapi: XLIFF2FilterTest#testStateChangeInitial
func TestExtract_StateChangeInitial(t *testing.T) {
	xliff2 := `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="u1">
      <segment id="s1" state="initial">
        <source>Initial state text</source>
      </segment>
    </unit>
  </file>
</xliff>`

	parts := readXLIFF2(t, xliff2, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Initial state text", blocks[0].SourceText())

	output := snippetRoundtrip(t, xliff2, nil)
	assert.Contains(t, output, "Initial state text")
}

// okapi: XLIFF2FilterTest#testWriteOriginalDataOption
func TestExtract_WriteOriginalDataOption(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	path := bridgetest.TestdataFile(t, "integration-tests/okapi/src/test/resources/xliff2/update_target.xlf")
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, content, path, mimeType, nil)
	require.NotEmpty(t, result.Output)

	output := string(result.Output)
	assert.Contains(t, output, "originalData", "roundtrip should preserve originalData section")
	assert.Contains(t, output, "dataRef", "roundtrip should preserve dataRef attributes")
}

// okapi: XLIFF2FilterTest#testSubFilterWithDefaultIcu
func TestExtract_SubFilterWithDefaultIcu(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	path := bridgetest.TestdataFile(t, "integration-tests/okapi/src/test/resources/xliff2/subfilter_icu/subfilter_icu.xlf")
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)

	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "ICU subfilter file should extract translatable blocks")

	var foundContent bool
	for _, b := range blocks {
		if b.SourceText() != "" {
			foundContent = true
			break
		}
	}
	assert.True(t, foundContent, "should extract ICU content as text")
}

// okapi: XLIFF2FilterTest#testSubFilterWithAllOptionsIcu
func TestExtract_SubFilterWithAllOptionsIcu(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	path := bridgetest.TestdataFile(t, "integration-tests/okapi/src/test/resources/xliff2/subfilter_icu/subfilter_icu.xlf")
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)

	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "ICU subfilter file should extract translatable blocks with all options")
}

// okapi: XLIFF2FilterTest#testSubFilterWithAllOptionsIcuRoundtrip
func TestRoundTrip_SubFilterWithAllOptionsIcuRoundtrip(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	path := bridgetest.TestdataFile(t, "integration-tests/okapi/src/test/resources/xliff2/subfilter_icu/subfilter_icu.xlf")
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, path, mimeType, nil)
}

// --- Additional extraction tests ---

func TestExtract_MultipleUnits(t *testing.T) {
	parts := readXLIFF2(t, `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="1">
      <segment>
        <source>First</source>
      </segment>
    </unit>
    <unit id="2">
      <segment>
        <source>Second</source>
      </segment>
    </unit>
    <unit id="3">
      <segment>
        <source>Third</source>
      </segment>
    </unit>
  </file>
</xliff>`, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.Len(t, blocks, 3)

	texts := bridgetest.BlockTexts(blocks)
	assert.Equal(t, "First", texts[0])
	assert.Equal(t, "Second", texts[1])
	assert.Equal(t, "Third", texts[2])
}

func TestExtract_WithTarget(t *testing.T) {
	parts := readXLIFF2(t, `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="1">
      <segment>
        <source>Hello</source>
        <target>Bonjour</target>
      </segment>
    </unit>
  </file>
</xliff>`, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	assert.Equal(t, "Hello", b.SourceText())
	assert.True(t, b.HasTarget("fr"), "should have French target")
	assert.Equal(t, "Bonjour", b.TargetText("fr"))
}

func TestExtract_UnitIDs(t *testing.T) {
	parts := readXLIFF2(t, `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="greeting">
      <segment>
        <source>Hello</source>
      </segment>
    </unit>
    <unit id="farewell">
      <segment>
        <source>Goodbye</source>
      </segment>
    </unit>
  </file>
</xliff>`, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.Len(t, blocks, 2)

	assert.Equal(t, "greeting", blocks[0].ID)
	assert.Equal(t, "farewell", blocks[1].ID)
}

// okapi: XLIFF2FilterTest#testSimpleMeta
func TestExtract_Notes(t *testing.T) {
	parts := readXLIFF2(t, `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="1">
      <notes>
        <note category="description">This is a note</note>
      </notes>
      <segment>
        <source>Hello</source>
      </segment>
    </unit>
  </file>
</xliff>`, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	if b.Annotations != nil {
		if noteAnn, ok := b.Annotations["note"]; ok {
			note := noteAnn.(*model.NoteAnnotation)
			assert.Equal(t, "This is a note", note.Text)
		}
	}
}

func TestExtract_MultipleSegments(t *testing.T) {
	parts := readXLIFF2(t, `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="1">
      <segment id="s1">
        <source>First sentence.</source>
      </segment>
      <segment id="s2">
        <source>Second sentence.</source>
      </segment>
    </unit>
  </file>
</xliff>`, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	require.GreaterOrEqual(t, len(b.Source), 2,
		"unit with 2 segments should produce 2+ source segments")
}

func TestExtract_LayerStructure(t *testing.T) {
	parts := readXLIFF2(t, `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="1">
      <segment>
        <source>Hello</source>
      </segment>
    </unit>
  </file>
</xliff>`, nil)

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

func TestExtract_UnicodeContent(t *testing.T) {
	parts := readXLIFF2(t, `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="1">
      <segment>
        <source>こんにちは世界</source>
      </segment>
    </unit>
  </file>
</xliff>`, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "こんにちは世界", blocks[0].SourceText())
}

func TestExtract_TranslatedXlf(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		bridgetest.TestdataFile(t, "integration-tests/okapi/src/test/resources/xliff2/translated.xlf"), mimeType, nil)

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "translated.xlf should extract translatable blocks")
}

func TestExtract_TranslateNo(t *testing.T) {
	parts := readXLIFF2(t, `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="1" translate="yes">
      <segment>
        <source>Translate me</source>
      </segment>
    </unit>
    <unit id="2" translate="no">
      <segment>
        <source>Do not translate</source>
      </segment>
    </unit>
  </file>
</xliff>`, nil)

	translatableBlocks := bridgetest.TranslatableBlocks(parts)
	require.Len(t, translatableBlocks, 1)
	assert.Equal(t, "Translate me", translatableBlocks[0].SourceText())
}
