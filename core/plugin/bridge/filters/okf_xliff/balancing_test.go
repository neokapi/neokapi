//go:build integration

package okf_xliff

import (
	"testing"

	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- XLIFFFilterBalancingTest ---
// These tests verify that inline code balancing is preserved after reading
// XLIFF files with various g/bx/ex tag structures.

// okapi: XLIFFFilterBalancingTest#testValidBalancingWithCTypesAfterJoinAll
func TestBalancing_WithCTypes(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/Balancing/WithCTypes.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	b := blocks[0]
	frag := b.FirstFragment()
	require.NotNil(t, frag)
	assert.GreaterOrEqual(t, len(frag.Spans), 2, "should have at least 2 inline codes")
}

// okapi: XLIFFFilterBalancingTest#testValidBalancingOverMultipleSegmentsAfterJoinAll
func TestBalancing_OverMultipleSegments(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/Balancing/MultipleSegments.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.NotEmpty(t, blocks[0].SourceText())
}

// okapi: XLIFFFilterBalancingTest#testValidBalancingBetweenSegmentsAfterJoinAll
func TestBalancing_BetweenSegments(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/Balancing/BetweenSegments.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.NotEmpty(t, blocks[0].SourceText())
}

// okapi: XLIFFFilterBalancingTest#testValidBalancingWithBxAndGTagsAfterJoinAll
func TestBalancing_WithBxAndGTags(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/Balancing/DifferentTags.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	b := blocks[0]
	frag := b.FirstFragment()
	require.NotNil(t, frag)
	assert.GreaterOrEqual(t, len(frag.Spans), 4, "should have at least 4 inline codes for mixed bx/g tags")
}

// okapi: XLIFFFilterBalancingTest#testValidBalancingWithNestedGTagsAfterJoinAll
func TestBalancing_WithNestedGTags(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/Balancing/2LevelGTags.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	b := blocks[0]
	frag := b.FirstFragment()
	require.NotNil(t, frag)
	assert.GreaterOrEqual(t, len(frag.Spans), 4, "should have at least 4 inline codes for nested g tags")
}

// okapi: XLIFFFilterBalancingTest#testValidBalancingWithNestedGTagsOnThreeLevelsAfterJoinAll
func TestBalancing_WithNestedGTagsOnThreeLevels(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/Balancing/3LevelGTags.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	b := blocks[0]
	frag := b.FirstFragment()
	require.NotNil(t, frag)
	assert.GreaterOrEqual(t, len(frag.Spans), 6, "should have at least 6 inline codes for 3-level nested g tags")
}

// okapi: XLIFFFilterBalancingTest#testValidBalancingWithNestedGTagsOnThreeLevelsAfterJoinAllWithNamespaces
func TestBalancing_WithNestedGTagsOnThreeLevelsWithNamespaces(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/Balancing/3LevelGTagsWithNamespaces.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	b := blocks[0]
	frag := b.FirstFragment()
	require.NotNil(t, frag)
	assert.GreaterOrEqual(t, len(frag.Spans), 6, "should have at least 6 inline codes for 3-level nested g tags with namespaces")
}

// okapi: XLIFFFilterBalancingTest#testDifferentCTypes
func TestBalancing_DifferentCTypes(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/Balancing/DifferentCTypes.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	b := blocks[0]
	frag := b.FirstFragment()
	require.NotNil(t, frag)
	assert.GreaterOrEqual(t, len(frag.Spans), 4, "should have at least 4 inline codes for different ctypes")
}

// okapi: XLIFFFilterBalancingTest#testDifferentCTypesWithBreakingMrk
func TestBalancing_DifferentCTypesWithBreakingMrk(t *testing.T) {
	parts := readXLIFFFile(t, "okf_xliff/Balancing/DifferentCTypesWithBreakingMrk.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	b := blocks[0]
	frag := b.FirstFragment()
	require.NotNil(t, frag)
	assert.GreaterOrEqual(t, len(frag.Spans), 4, "should have at least 4 inline codes for different ctypes with breaking mrk")
}
