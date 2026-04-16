//go:build integration

package xliff

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- XLIFFFilterBalancingTest ---
// These tests verify that XLIFF files with various g/bx/ex tag structures
// are read correctly and their text content is extracted.
//
// Note: The bridge does not yet populate Spans for XLIFF inline codes
// (g, bx, ex elements). The Java filter processes these correctly and the
// text content is extracted, but the Go-side proto-to-model conversion
// does not map them to model.Fragment.Spans. Once Span mapping is
// implemented in the bridge, these tests should be updated to also
// verify Span structure.

// okapi: XLIFFFilterBalancingTest#testValidBalancingWithCTypesAfterJoinAll
func TestBalancing_WithCTypes(t *testing.T) {
	parts := readXLIFFFile(t, "okapi/filters/xliff/src/test/resources/Balancing/WithCTypes.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	b := blocks[0]
	runs := b.SourceRuns()
	require.NotEmpty(t, runs)
	assert.NotEmpty(t, model.RunsPlainText(runs), "should extract text content from XLIFF with ctypes")
}

// okapi: XLIFFFilterBalancingTest#testValidBalancingOverMultipleSegmentsAfterJoinAll
func TestBalancing_OverMultipleSegments(t *testing.T) {
	parts := readXLIFFFile(t, "okapi/filters/xliff/src/test/resources/Balancing/MultipleSegments.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.NotEmpty(t, blocks[0].SourceText())
}

// okapi: XLIFFFilterBalancingTest#testValidBalancingBetweenSegmentsAfterJoinAll
func TestBalancing_BetweenSegments(t *testing.T) {
	parts := readXLIFFFile(t, "okapi/filters/xliff/src/test/resources/Balancing/BetweenSegments.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.NotEmpty(t, blocks[0].SourceText())
}

// okapi: XLIFFFilterBalancingTest#testValidBalancingWithBxAndGTagsAfterJoinAll
func TestBalancing_WithBxAndGTags(t *testing.T) {
	parts := readXLIFFFile(t, "okapi/filters/xliff/src/test/resources/Balancing/DifferentTags.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	b := blocks[0]
	runs := b.SourceRuns()
	require.NotEmpty(t, runs)
	assert.NotEmpty(t, model.RunsPlainText(runs), "should extract text content from XLIFF with mixed bx/g tags")
}

// okapi: XLIFFFilterBalancingTest#testValidBalancingWithNestedGTagsAfterJoinAll
func TestBalancing_WithNestedGTags(t *testing.T) {
	parts := readXLIFFFile(t, "okapi/filters/xliff/src/test/resources/Balancing/2LevelGTags.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	b := blocks[0]
	runs := b.SourceRuns()
	require.NotEmpty(t, runs)
	assert.NotEmpty(t, model.RunsPlainText(runs), "should extract text content from XLIFF with nested g tags")
}

// okapi: XLIFFFilterBalancingTest#testValidBalancingWithNestedGTagsOnThreeLevelsAfterJoinAll
func TestBalancing_WithNestedGTagsOnThreeLevels(t *testing.T) {
	parts := readXLIFFFile(t, "okapi/filters/xliff/src/test/resources/Balancing/3LevelGTags.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	b := blocks[0]
	runs := b.SourceRuns()
	require.NotEmpty(t, runs)
	assert.NotEmpty(t, model.RunsPlainText(runs), "should extract text content from XLIFF with 3-level nested g tags")
}

// okapi: XLIFFFilterBalancingTest#testValidBalancingWithNestedGTagsOnThreeLevelsAfterJoinAllWithNamespaces
func TestBalancing_WithNestedGTagsOnThreeLevelsWithNamespaces(t *testing.T) {
	parts := readXLIFFFile(t, "okapi/filters/xliff/src/test/resources/Balancing/3LevelGTagsWithNamespaces.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	b := blocks[0]
	runs := b.SourceRuns()
	require.NotEmpty(t, runs)
	assert.NotEmpty(t, model.RunsPlainText(runs), "should extract text content from XLIFF with 3-level nested g tags with namespaces")
}

// okapi: XLIFFFilterBalancingTest#testDifferentCTypes
func TestBalancing_DifferentCTypes(t *testing.T) {
	parts := readXLIFFFile(t, "okapi/filters/xliff/src/test/resources/Balancing/DifferentCTypes.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	b := blocks[0]
	runs := b.SourceRuns()
	require.NotEmpty(t, runs)
	assert.NotEmpty(t, model.RunsPlainText(runs), "should extract text content from XLIFF with different ctypes")
}

// okapi: XLIFFFilterBalancingTest#testDifferentCTypesWithBreakingMrk
func TestBalancing_DifferentCTypesWithBreakingMrk(t *testing.T) {
	parts := readXLIFFFile(t, "okapi/filters/xliff/src/test/resources/Balancing/DifferentCTypesWithBreakingMrk.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	b := blocks[0]
	runs := b.SourceRuns()
	require.NotEmpty(t, runs)
	assert.NotEmpty(t, model.RunsPlainText(runs), "should extract text content from XLIFF with different ctypes and breaking mrk")
}
