//go:build integration

package subtitles

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const ttmlFilterClass = "net.sf.okapi.filters.ttml.TTMLFilter"
const ttmlMimeType = "application/ttml+xml"

// readTTML parses a TTML snippet with custom filter params and returns the parts.
func readTTML(t *testing.T, snippet string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	return bridgetest.ReadString(t, pool, cfg, ttmlFilterClass, snippet, "test.ttml", ttmlMimeType, filterParams)
}

// readTTMLDefault parses a TTML snippet with default (nil) params.
func readTTMLDefault(t *testing.T, snippet string) []*model.Part {
	t.Helper()
	return readTTML(t, snippet, nil)
}

// allBlocks returns all blocks (translatable and non-translatable) from parts.
func ttmlAllBlocks(parts []*model.Part) []*model.Block {
	return bridgetest.FilterBlocks(parts)
}

// snippetRoundtrip roundtrips a TTML snippet and returns the output string.
func ttmlSnippetRoundtrip(t *testing.T, snippet string, filterParams map[string]any) string {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	result := bridgetest.RoundTrip(t, pool, cfg, ttmlFilterClass, []byte(snippet), "test.ttml", ttmlMimeType, filterParams)
	return string(result.Output)
}

// findBlockContaining finds a block whose source text contains the given substring.
func ttmlFindBlockContaining(blocks []*model.Block, substr string) *model.Block {
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), substr) {
			return b
		}
	}
	return nil
}

// twoSubtitleSnippet is the common TTML snippet used by many Java tests.
// It contains two <p> elements with <br/> line breaks.
const twoSubtitleSnippet = `<p xml:id="subtitle1" begin="00:00:00.897" end="00:00:05.263" region="bottom" tts:textAlign="center">Thanks everyone <br/>for joining us today.</p>
<p xml:id="subtitle2" begin="00:00:05.430" end="00:00:08.730" region="bottom" tts:textAlign="center">I am so excited<br/> to be with you.</p>`

// ---------------------------------------------------------------------------
// TTMLFilterTest.java — Extraction Tests
// ---------------------------------------------------------------------------

// okapi: TTMLFilterTest#testProcessTextUnit
// Verifies basic <p> element extraction: two subtitles with <br/> line breaks
// produce two text units with <br/> removed (default escapeBrMode=true).
func TestExtract_ProcessTextUnit(t *testing.T) {
	parts := readTTMLDefault(t, twoSubtitleSnippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2, "should extract at least 2 text units from two <p> elements")

	// First subtitle: "Thanks everyone for joining us today."
	// The <br/> is replaced with space in default escape mode.
	b1 := blocks[0]
	assert.Equal(t, "Thanks everyone for joining us today.", b1.SourceText(),
		"first subtitle should have <br/> replaced with space join")

	// Second subtitle: "I am so excited to be with you."
	b2 := blocks[1]
	assert.Equal(t, "I am so excited to be with you.", b2.SourceText(),
		"second subtitle should have <br/> replaced with space join")
}

// okapi: TTMLFilterTest#testMergeCaptions
// When adjacent captions end with a comma (trailing punctuation), the filter
// merges them into a single text unit.
func TestExtract_TTML_MergeCaptions(t *testing.T) {
	// Note: the merge snippet differs from the standard one — subtitle1 ends
	// with a comma after "today," and subtitle2 has no trailing space after <br/>.
	snippet := `<p xml:id="subtitle1" begin="00:00:00.897" end="00:00:05.263" region="bottom" tts:textAlign="center">Thanks everyone<br/>for joining us today,</p>
<p xml:id="subtitle2" begin="00:00:05.430" end="00:00:08.730" region="bottom" tts:textAlign="center">I am so excited<br/>to be with you.</p>`

	parts := readTTMLDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract at least one text unit")

	// When captions are merged, both subtitles become one text unit.
	merged := blocks[0]
	text := merged.SourceText()
	assert.Contains(t, text, "Thanks everyone")
	assert.Contains(t, text, "to be with you.")
	// The merged text should contain content from both subtitles.
	assert.Contains(t, text, "I am so excited",
		"merged caption should contain text from both subtitles")
}

// okapi: TTMLFilterTest#testDontMergeCaptions
// When mergeCaptions is disabled, each <p> element produces a separate text unit.
func TestExtract_DontMergeCaptions(t *testing.T) {
	// The "don't merge" snippet has trailing space after "everyone " before <br/> in subtitle1.
	snippet := `<p xml:id="subtitle1" begin="00:00:00.897" end="00:00:05.263" region="bottom" tts:textAlign="center">Thanks everyone <br/>for joining us today,</p>
<p xml:id="subtitle2" begin="00:00:05.430" end="00:00:08.730" region="bottom" tts:textAlign="center">I am so excited<br/> to be with you.</p>`

	params := map[string]any{
		"mergeCaptions": false,
	}
	parts := readTTML(t, snippet, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2, "should extract separate text units when merge is disabled")

	b1 := blocks[0]
	assert.Equal(t, "Thanks everyone for joining us today,", b1.SourceText())

	b2 := blocks[1]
	assert.Equal(t, "I am so excited to be with you.", b2.SourceText())
}

// okapi: TTMLFilterTest#testQuoteCaptions
// Verifies quote and punctuation handling: single-quoted caption text is
// preserved with quotes intact.
func TestExtract_QuoteCaptions(t *testing.T) {
	snippet := `<p xml:id="subtitle1" begin="00:00:00.897" end="00:00:05.263" region="bottom" tts:textAlign="center">'Thanks everyone <br/>for joining us today.'</p>
<p xml:id="subtitle2" begin="00:00:05.430" end="00:00:08.730" region="bottom" tts:textAlign="center">'I am so excited<br/> to be with you.'</p>`

	parts := readTTMLDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 1, "should extract text units from quoted captions")

	// Java test expects: "'Thanks everyone for joining us today.'"
	b1 := blocks[0]
	text := b1.SourceText()
	assert.Contains(t, text, "'Thanks everyone")
	assert.Contains(t, text, "today.'")
}

// okapi: TTMLFilterTest#testEmptyCaptions
// Empty <p> elements still produce text units (with empty text).
func TestExtract_EmptyCaptions(t *testing.T) {
	snippet := `<p xml:id="subtitle1" begin="00:00:00.897" end="00:00:05.263" region="bottom" tts:textAlign="center"></p>
<p xml:id="subtitle2" begin="00:00:05.430" end="00:00:08.730" region="bottom" tts:textAlign="center">I am so excited<br/> to be with you.</p>`

	parts := readTTMLDefault(t, snippet)

	blocks := ttmlAllBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 1, "should extract at least one block")

	// The non-empty subtitle should be extractable.
	found := ttmlFindBlockContaining(blocks, "I am so excited")
	require.NotNil(t, found, "should find the non-empty subtitle block")
	assert.Equal(t, "I am so excited to be with you.", found.SourceText())
}

// okapi: TTMLFilterTest#testReadMaxCharMaxLine
// When <metadata> contains okp:maximum_character_count and okp:maximum_line_count,
// the filter reads those values. We verify the text extraction still works
// correctly (the max char/line values are internal annotations).
func TestExtract_ReadMaxCharMaxLine(t *testing.T) {
	snippet := `<metadata>
<okp:maximum_character_count>20</okp:maximum_character_count>
<okp:maximum_line_count>3</okp:maximum_line_count>
</metadata>
<p xml:id="subtitle1" begin="00:00:00.897" end="00:00:05.263" region="bottom" tts:textAlign="center">Thanks everyone <br/>for joining us today.</p>
<p xml:id="subtitle2" begin="00:00:05.430" end="00:00:08.730" region="bottom" tts:textAlign="center">I am so excited<br/> to be with you.</p>`

	parts := readTTMLDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2, "should extract text units even with custom max char/line metadata")

	b1 := blocks[0]
	assert.Equal(t, "Thanks everyone for joining us today.", b1.SourceText())

	b2 := blocks[1]
	assert.Equal(t, "I am so excited to be with you.", b2.SourceText())
}

// okapi: TTMLFilterTest#testCodeFinder
// When useCodeFinder is enabled with HTML tag rules, <span> tags become
// inline codes and the text is extracted without markup.
func TestExtract_CodeFinder(t *testing.T) {
	snippet := `<p xml:id="subtitle1" begin="00:00:00.897" end="00:00:05.263" region="bottom" tts:textAlign="center"><span ns2:fontStyle="italic">Thanks everyone <br/>for joining us today.</span></p>
<p xml:id="subtitle2" begin="00:00:05.430" end="00:00:08.730" region="bottom" tts:textAlign="center">I am so <span ns2:fontStyle="italic">excited</span><br/> to be with you.</p>`

	params := map[string]any{
		"mergeCaptions": false,
		"useCodeFinder": true,
		"codeFinderRules": "#v1\ncount.i=1\n" +
			"rule0=</?([A-Z0-9a-z]*)\\b[^>]*>\n" +
			"sample=&name; <tag></at><tag/> <tag attr='val'> </tag=\"val\">\n" +
			"useAllRulesWhenTesting.b=true",
	}
	parts := readTTML(t, snippet, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2, "should extract text units with code finder enabled")

	// First subtitle: text should be "Thanks everyone for joining us today."
	// with <span> tags as inline codes.
	b1 := blocks[0]
	assert.Equal(t, "Thanks everyone for joining us today.", b1.SourceText())
	// Verify spans are created for the inline codes.
	if b1.Source[0].Content != nil {
		assert.NotEmpty(t, b1.Source[0].Content.Spans, "should have spans for <span> inline codes")
		// Look for span data containing the opening and closing tags.
		foundOpen := false
		foundClose := false
		for _, s := range b1.Source[0].Content.Spans {
			if strings.Contains(s.Data, "<span") {
				foundOpen = true
			}
			if strings.Contains(s.Data, "</span>") {
				foundClose = true
			}
		}
		assert.True(t, foundOpen, "should find opening <span> tag as inline code")
		assert.True(t, foundClose, "should find closing </span> tag as inline code")
	}

	// Second subtitle: "I am so excited to be with you."
	b2 := blocks[1]
	assert.Equal(t, "I am so excited to be with you.", b2.SourceText())
	if b2.Source[0].Content != nil {
		assert.NotEmpty(t, b2.Source[0].Content.Spans, "should have spans for <span> inline codes in second subtitle")
	}
}

// okapi: TTMLFilterTest#testProcessTextUnitNonEscapeBrMode
// When escapeBrMode is disabled, <br/> tags are preserved as literal text
// rather than being replaced with spaces.
func TestExtract_ProcessTextUnitNonEscapeBrMode(t *testing.T) {
	params := map[string]any{
		"escapeBrMode":  false,
		"useCodeFinder": false,
	}
	parts := readTTML(t, twoSubtitleSnippet, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2, "should extract text units in non-escape br mode")

	// In non-escape mode, <br/> is preserved in the text.
	b1 := blocks[0]
	assert.Equal(t, "Thanks everyone <br/>for joining us today.", b1.SourceText(),
		"first subtitle should preserve <br/> in non-escape mode")

	b2 := blocks[1]
	assert.Equal(t, "I am so excited<br/> to be with you.", b2.SourceText(),
		"second subtitle should preserve <br/> in non-escape mode")
}

// ---------------------------------------------------------------------------
// TTMLSkeletonWriterTest.java — Writer Tests (via roundtrip)
// ---------------------------------------------------------------------------

// okapi-unmapped: TTMLSkeletonWriterTest#testProcessTextUnit — Java-only skeleton writer unit test
// (constructs TextUnit + CaptionAnnotation + TTMLSkeletonPart manually; not exercisable via bridge roundtrip)

// okapi-unmapped: TTMLSkeletonWriterTest#testProcessTextUnitSplitLines — Java-only skeleton writer unit test
// (tests maxCharsPerLine=10 line splitting with manual skeleton construction)

// okapi-unmapped: TTMLSkeletonWriterTest#testProcessTextUnitSplitCaptionsLines — Java-only skeleton writer unit test
// (tests caption splitting across multiple <p> elements with manual skeleton construction)

// okapi-unmapped: TTMLSkeletonWriterTest#testProcessTextUnitSplitCaptionsLineOverflow — Java-only skeleton writer unit test
// (tests overflow line splitting behavior with maxCharsPerLine=5)

// okapi-unmapped: TTMLSkeletonWriterTest#testProcessTextUnitSplitCaptionsCaptionOverflow — Java-only skeleton writer unit test
// (tests caption overflow with "hippopotamus" across 3 captions)

// okapi-unmapped: TTMLSkeletonWriterTest#testProcessTextUnitWithCodes — Java-only skeleton writer unit test
// (tests code finder integration during write with manual TextUnit + code processing)

// okapi-unmapped: TTMLSkeletonWriterTest#testProcessTextUnitNonEscapeBrMode — Java-only skeleton writer unit test
// (tests non-escape br mode write with manual skeleton construction)

// ---------------------------------------------------------------------------
// Snippet Roundtrip Tests
// ---------------------------------------------------------------------------

// TestRoundTrip_BasicSnippet verifies that a basic TTML snippet survives a
// read-write cycle. This exercises the skeleton writer end-to-end through the
// bridge (unlike the Java TTMLSkeletonWriterTest tests which construct skeletons
// manually).
func TestRoundTrip_BasicSnippet(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	snippet := `<?xml version="1.0" encoding="UTF-8"?>
<tt xmlns="http://www.w3.org/ns/ttml" xmlns:tts="http://www.w3.org/ns/ttml#styling" xml:lang="EN">
  <body>
    <div>
      <p xml:id="subtitle1" begin="00:00:00.430" end="00:00:02.497" region="bottom" tts:textAlign="center">Hello world.</p>
    </div>
  </body>
</tt>`

	bridgetest.AssertRoundTripEvents(t, pool, cfg, ttmlFilterClass, []byte(snippet), "test.ttml", ttmlMimeType, nil)
}

// TestRoundTrip_MultipleSubtitles verifies roundtrip with multiple <p> elements.
func TestRoundTrip_MultipleSubtitles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	snippet := `<?xml version="1.0" encoding="UTF-8"?>
<tt xmlns="http://www.w3.org/ns/ttml" xmlns:tts="http://www.w3.org/ns/ttml#styling" xml:lang="EN">
  <body>
    <div>
      <p xml:id="subtitle1" begin="00:00:00.430" end="00:00:02.497" region="bottom" tts:textAlign="center">First subtitle.</p>
      <p xml:id="subtitle2" begin="00:00:02.497" end="00:00:06.363" region="bottom" tts:textAlign="center">Second subtitle.</p>
    </div>
  </body>
</tt>`

	bridgetest.AssertRoundTripEvents(t, pool, cfg, ttmlFilterClass, []byte(snippet), "test.ttml", ttmlMimeType, nil)
}

// TestRoundTrip_WithLineBreaks verifies <br/> elements survive roundtrip.
func TestRoundTrip_WithLineBreaks(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	snippet := `<?xml version="1.0" encoding="UTF-8"?>
<tt xmlns="http://www.w3.org/ns/ttml" xmlns:tts="http://www.w3.org/ns/ttml#styling" xml:lang="EN">
  <body>
    <div>
      <p xml:id="subtitle1" begin="00:00:00.430" end="00:00:02.497" region="bottom" tts:textAlign="center">First line<br/>second line.</p>
    </div>
  </body>
</tt>`

	bridgetest.AssertRoundTripEvents(t, pool, cfg, ttmlFilterClass, []byte(snippet), "test.ttml", ttmlMimeType, nil)
}

// ---------------------------------------------------------------------------
// Additional extraction tests
// ---------------------------------------------------------------------------

// TestExtract_LayerStructure verifies the part stream starts with LayerStart and
// ends with LayerEnd.
func TestExtract_TTML_LayerStructure(t *testing.T) {
	snippet := `<?xml version="1.0" encoding="UTF-8"?>
<tt xmlns="http://www.w3.org/ns/ttml" xml:lang="EN">
  <body>
    <div>
      <p xml:id="subtitle1" begin="00:00:00.430" end="00:00:02.497">Hello.</p>
    </div>
  </body>
</tt>`

	parts := readTTMLDefault(t, snippet)
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type, "last part should be LayerEnd")
}

// TestExtract_BlockIDs verifies that each extracted block has a unique ID.
func TestExtract_TTML_BlockIDs(t *testing.T) {
	snippet := `<?xml version="1.0" encoding="UTF-8"?>
<tt xmlns="http://www.w3.org/ns/ttml" xmlns:tts="http://www.w3.org/ns/ttml#styling" xml:lang="EN">
  <body>
    <div>
      <p xml:id="subtitle1" begin="00:00:00.430" end="00:00:02.497" region="bottom" tts:textAlign="center">First.</p>
      <p xml:id="subtitle2" begin="00:00:02.497" end="00:00:06.363" region="bottom" tts:textAlign="center">Second.</p>
      <p xml:id="subtitle3" begin="00:00:06.830" end="00:00:09.963" region="bottom" tts:textAlign="center">Third.</p>
    </div>
  </body>
</tt>`

	parts := readTTMLDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 3)

	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID, "block should have an ID")
		assert.False(t, ids[b.ID], "block IDs should be unique, got duplicate: %s", b.ID)
		ids[b.ID] = true
	}
}

// TestExtract_EmptyDocument verifies that a TTML document with only empty
// subtitles does not crash.
func TestExtract_EmptyDocument(t *testing.T) {
	snippet := `<?xml version="1.0" encoding="UTF-8"?>
<tt xmlns="http://www.w3.org/ns/ttml" xml:lang="EN">
  <body>
    <div>
      <p xml:id="subtitle1" begin="00:00:00.430" end="00:00:02.497"></p>
    </div>
  </body>
</tt>`

	parts := readTTMLDefault(t, snippet)
	require.NotEmpty(t, parts, "should produce parts even for empty subtitles")
}

// TestExtract_FullDocument verifies extraction from a complete TTML document
// with head, metadata, layout, and body sections.
func TestExtract_FullDocument(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, "integration-tests/okapi/src/test/resources/ttml/example1.ttml")
	parts := bridgetest.ReadFile(t, pool, cfg, ttmlFilterClass, path, ttmlMimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from example1.ttml")

	// example1.ttml has subtitles with Lorem ipsum content.
	texts := bridgetest.BlockTexts(blocks)
	found := false
	for _, txt := range texts {
		if strings.Contains(txt, "Lorem ipsum") {
			found = true
			break
		}
	}
	assert.True(t, found, "should find 'Lorem ipsum' in extracted blocks from example1.ttml")
}
