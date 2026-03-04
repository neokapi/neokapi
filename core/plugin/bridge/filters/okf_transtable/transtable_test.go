//go:build integration

package okf_transtable

import (
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.transtable.TransTableFilter"
const mimeType = "text/x-transtable"

// readTransTable parses a TransTable snippet with custom filter params and returns the parts.
func readTransTable(t *testing.T, snippet string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	return bridgetest.ReadString(t, pool, cfg, filterClass, snippet, "test.txt", mimeType, filterParams)
}

// readTransTableDefault parses a TransTable snippet with default (nil) params.
func readTransTableDefault(t *testing.T, snippet string) []*model.Part {
	t.Helper()
	return readTransTable(t, snippet, nil)
}

// findBlockByID finds a block whose ID matches the given string.
func findBlockByID(blocks []*model.Block, id string) *model.Block {
	for _, b := range blocks {
		if b.ID == id {
			return b
		}
	}
	return nil
}

// okapi: TransTableFilterTest#testStartDocument
// surefire: testStartDocument (0.104s, PASS)
func TestExtract_StartDocument(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	// The Java test uses test01.xml.txt and verifies the StartDocument event.
	// We verify the LayerStart part is produced from a minimal TransTable input.
	snippet := "TransTableV1\ten\tfr\n" +
		"\"okpCtx:tu=1\"\t\"source\"\n"
	parts := readTransTableDefault(t, snippet)

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")
}

// okapi: TransTableFilterTest#testMinimalInput
// surefire: testMinimalInput (0.0s, PASS)
func TestExtract_MinimalInput(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	snippet := "TransTableV1\ten\tfr\n" +
		"\"okpCtx:tu=1\"\t\"source\""
	parts := bridgetest.ReadString(t, pool, cfg, filterClass, snippet, "test.txt", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract at least one translatable block")

	b := blocks[0]
	assert.Equal(t, "source", b.SourceText())
	assert.Equal(t, "1", b.ID)
}

// okapi: TransTableFilterTest#testMinimalSourceTarget
// surefire: testMinimalSourceTarget (0.001s, PASS)
func TestExtract_MinimalSourceTarget(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	snippet := "TransTableV1\ten\tfr\n" +
		"\"okpCtx:tu=1\"\t\"source\"\t\"target\""
	parts := bridgetest.ReadString(t, pool, cfg, filterClass, snippet, "test.txt", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract at least one translatable block")

	b := blocks[0]
	assert.Equal(t, "source", b.SourceText())
	assert.True(t, b.HasTarget("fr"), "should have French target")
	assert.Equal(t, "target", b.TargetText("fr"))
	assert.Equal(t, "1", b.ID)
}

// okapi: TransTableFilterTest#testQuotesInput
// surefire: testQuotesInput (0.0s, PASS)
func TestExtract_QuotesInput(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	// Header fields are quoted, context field is unquoted.
	snippet := "\"TransTableV1\"\t\"en\"\t\"fr\"\n" +
		"okpCtx:tu=1\tsource"
	parts := bridgetest.ReadString(t, pool, cfg, filterClass, snippet, "test.txt", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract at least one translatable block")

	b := blocks[0]
	assert.Equal(t, "source", b.SourceText())
	assert.Equal(t, "1", b.ID)
}

// okapi: TransTableFilterTest#testUnSegmented
// surefire: testUnSegmented (0.001s, PASS)
func TestExtract_UnSegmented(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	snippet := "\"TransTableV1\"\t\"en\"\t\"fr\"\n" +
		"okpCtx:tu=1:s=0\tsource1\n" +
		"okpCtx:tu=2\tsource2"
	parts := bridgetest.ReadString(t, pool, cfg, filterClass, snippet, "test.txt", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2, "should extract at least 2 blocks")

	b1 := findBlockByID(blocks, "1")
	require.NotNil(t, b1, "should find block with ID 1")
	assert.Equal(t, "source1", b1.SourceText())

	b2 := findBlockByID(blocks, "2")
	require.NotNil(t, b2, "should find block with ID 2")
	assert.Equal(t, "source2", b2.SourceText())
}

// okapi: TransTableFilterTest#testSegmented
// surefire: testSegmented (0.005s, PASS)
func TestExtract_Segmented(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	snippet := "\"TransTableV1\"\t\"en\"\t\"fr\"\n" +
		"okpCtx:tu=1:s=0\tsource1\n" +
		"okpCtx:tu=2:s=0\tsrc2-seg0\n" +
		"okpCtx:tu=2:s=1\tsrc2-seg1\n" +
		"okpCtx:tu=2:s=2\tsrc2-seg2\n" +
		"okpCtx:tu=3:s=ZZZ\tsrc3-segZZZ\n" +
		"okpCtx:tu=4\tsrc4-seg0"
	parts := bridgetest.ReadString(t, pool, cfg, filterClass, snippet, "test.txt", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)

	// TU 1: single unsegmented entry.
	b1 := findBlockByID(blocks, "1")
	require.NotNil(t, b1, "should find block with ID 1")
	assert.Equal(t, "source1", b1.SourceText())

	// TU 2: three segments merged into one text unit.
	b2 := findBlockByID(blocks, "2")
	require.NotNil(t, b2, "should find block with ID 2")
	assert.Equal(t, "src2-seg0src2-seg1src2-seg2", b2.SourceText())

	// TU 3: single segment with custom ID "ZZZ".
	b3 := findBlockByID(blocks, "3")
	require.NotNil(t, b3, "should find block with ID 3")
	assert.Equal(t, "src3-segZZZ", b3.SourceText())
}

// okapi: TransTableFilterTest#testSegmentedWithTarget
// surefire: testSegmentedWithTarget (0.001s, PASS)
func TestExtract_SegmentedWithTarget(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	snippet := "\"TransTableV1\"\t\"en\"\t\"fr\"\n" +
		"okpCtx:tu=1:s=0\tsource1\ttarget1\n" +
		"okpCtx:tu=2:s=0\tsrc2-seg0\n" +
		"\n  \n\n" +
		"okpCtx:tu=2:s=1\tsrc2-seg1\ttrg2-seg1\n" +
		"okpCtx:tu=2:s=2\tsrc2-seg2\n" +
		"okpCtx:tu=3:s=ZZZ\tsrc3-segZZZ\n" +
		"okpCtx:tu=4\tsrc4-seg0\ttrg4-seg0\n"
	parts := bridgetest.ReadString(t, pool, cfg, filterClass, snippet, "test.txt", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)

	// TU 1: source and target.
	b1 := findBlockByID(blocks, "1")
	require.NotNil(t, b1, "should find block with ID 1")
	assert.Equal(t, "source1", b1.SourceText())
	assert.True(t, b1.HasTarget("fr"), "block 1 should have French target")
	assert.Equal(t, "target1", b1.TargetText("fr"))

	// TU 2: segments merged, partial target (only seg1 has target).
	b2 := findBlockByID(blocks, "2")
	require.NotNil(t, b2, "should find block with ID 2")
	assert.Equal(t, "src2-seg0src2-seg1src2-seg2", b2.SourceText())
	if b2.HasTarget("fr") {
		assert.Contains(t, b2.TargetText("fr"), "trg2-seg1")
	}

	// TU 3: single segment with custom ID.
	b3 := findBlockByID(blocks, "3")
	require.NotNil(t, b3, "should find block with ID 3")
	assert.Equal(t, "src3-segZZZ", b3.SourceText())

	// TU 4: unsegmented with target.
	b4 := findBlockByID(blocks, "4")
	require.NotNil(t, b4, "should find block with ID 4")
	assert.Equal(t, "src4-seg0", b4.SourceText())
	assert.True(t, b4.HasTarget("fr"), "block 4 should have French target")
	assert.Equal(t, "trg4-seg0", b4.TargetText("fr"))
}

// --- Full-file extraction test ---

// TestExtract_TestFile reads the test01.xml.txt file from testdata and verifies extraction.
func TestExtract_TestFile(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	path := tdDir + "/okf_transtable/test01.xml.txt"
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 6, "test01.xml.txt should produce at least 6 translatable blocks")

	// Verify specific entries from the test file.
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Description of the first record")
	assert.Contains(t, texts, "Path of the file to process")
	assert.Contains(t, texts, "Text of the third record")
}

// --- Layer structure test ---

func TestExtract_LayerStructure(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	snippet := "TransTableV1\ten\tfr\n" +
		"\"okpCtx:tu=1\"\t\"source\"\n"
	parts := bridgetest.ReadString(t, pool, cfg, filterClass, snippet, "test.txt", mimeType, nil)

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type, "last part should be LayerEnd")
}

// --- Block ID uniqueness test ---

func TestExtract_BlockIDsUnique(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	snippet := "TransTableV1\ten\tfr\n" +
		"\"okpCtx:tu=1\"\t\"first\"\n" +
		"\"okpCtx:tu=2\"\t\"second\"\n" +
		"\"okpCtx:tu=3\"\t\"third\"\n"
	parts := bridgetest.ReadString(t, pool, cfg, filterClass, snippet, "test.txt", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 3)

	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID, "block should have an ID")
		assert.False(t, ids[b.ID], "block IDs should be unique, got duplicate: %s", b.ID)
		ids[b.ID] = true
	}
}

// --- Write phase tests ---
//
// The TransTable writer produces output that omits the header row required for
// re-parsing. This means event-level roundtrip (read -> write -> re-read) fails
// with "Unexpected header." This matches Java's known failing roundtrip behavior
// (RoundTripTranstableIT marks test01.xml.txt as knownFailing, and the
// testDoubleExtraction is commented out in TransTableFilterTest.java).
//
// The writer outputs target text where available, falling back to source text
// when no target exists. These tests verify only that the write phase completes
// without error and that the output contains the expected content.

func TestWrite_MinimalSnippet(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	snippet := "TransTableV1\ten\tfr\n" +
		"\"okpCtx:tu=1\"\t\"source\"\n"
	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, []byte(snippet), "test.txt", mimeType, nil)
	assert.NotEmpty(t, result.Output, "write phase should produce output")
	// No target provided, so the writer outputs the source text.
	assert.Contains(t, string(result.Output), "source", "output should contain source text")
}

func TestWrite_SourceTarget(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	snippet := "TransTableV1\ten\tfr\n" +
		"\"okpCtx:tu=1\"\t\"source\"\t\"target\"\n"
	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, []byte(snippet), "test.txt", mimeType, nil)
	assert.NotEmpty(t, result.Output, "write phase should produce output")
	// The writer outputs target text when available.
	assert.Contains(t, string(result.Output), "target", "output should contain target text")
}

func TestWrite_MultipleEntries(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	snippet := "TransTableV1\ten\tfr\n" +
		"\"okpCtx:tu=1\"\t\"first\"\n" +
		"\"okpCtx:tu=2\"\t\"second\"\t\"deuxieme\"\n" +
		"\"okpCtx:tu=3\"\t\"third\"\n"
	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, []byte(snippet), "test.txt", mimeType, nil)
	assert.NotEmpty(t, result.Output, "write phase should produce output")
	// Entries without targets output source text; entries with targets output target text.
	assert.Contains(t, string(result.Output), "first", "output should contain first entry (source, no target)")
	assert.Contains(t, string(result.Output), "deuxieme", "output should contain second entry target")
	assert.Contains(t, string(result.Output), "third", "output should contain third entry (source, no target)")
}
