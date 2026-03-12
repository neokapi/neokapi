//go:build integration

package html

import (
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// okapi: ExtractionComparisionTest#testStartDocument
func TestExtraction_StartDocument(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	path := bridgetest.TestdataFile(t, "okf_html/324.html")
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)

	require.NotEmpty(t, parts, "should produce at least one part")
	assert.Equal(t, model.PartLayerStart, parts[0].Type,
		"first part should be LayerStart (equivalent to Java StartDocument)")

	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok, "first part resource should be a Layer")
	assert.NotEmpty(t, layer.ID, "layer should have an ID")
}

// okapi: ExtractionComparisionTest#testOpenTwice
func TestExtraction_OpenTwice(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	path := bridgetest.TestdataFile(t, "okf_html/324.html")

	// First read.
	parts1 := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)
	blocks1 := bridgetest.FilterBlocks(parts1)

	// Second read of the same file — verify the filter can be re-opened.
	parts2 := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)
	blocks2 := bridgetest.FilterBlocks(parts2)

	require.NotEmpty(t, blocks1, "first read should produce blocks")
	require.Equal(t, len(blocks1), len(blocks2),
		"opening and reading the same file twice should produce the same number of blocks")

	texts1 := bridgetest.BlockTexts(blocks1)
	texts2 := bridgetest.BlockTexts(blocks2)
	assert.Equal(t, texts1, texts2,
		"opening and reading the same file twice should produce the same block texts")
}

// okapi: ExtractionComparisionTest#testDoubleExtractionSingle
func TestExtraction_DoubleExtractionSingle(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	// Java uses a single file (test.html) with RoundTripComparison:
	// read → write → re-read, then compare events from both reads.
	path := bridgetest.TestdataFile(t, "okf_html/test.html")
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
		content, path, mimeType, nil)
}

// okapi: ExtractionComparisionTest#testDoubleExtraction
func TestExtraction_DoubleExtraction(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	// The Java test iterates over all HTML test files returned by
	// HtmlUtils.getHtmlTestFiles() and runs a RoundTripComparison on each.
	// We replicate this by running event-level roundtrip on all okf_html
	// testdata files.
	tdDir := bridgetest.TestdataDir(t)
	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okf_html/*.html", mimeType, nil)
}

// okapi: ExtractionComparisionTest#testDoubleExtraction2
func TestExtraction_DoubleExtraction2(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	path := bridgetest.TestdataFile(t, "okf_html/test.asp")
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
		content, path, mimeType, nil)
}

// okapi: ExtractionComparisionTest#testReconstructFile
func TestExtraction_ReconstructFile(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	path := bridgetest.TestdataFile(t, "okf_html/324.html")
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	// Read and reconstruct the file through a full roundtrip, then verify
	// the reconstructed output produces the same events when re-read.
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
		content, path, mimeType, nil)
}
