//go:build integration

package okf_wsxzpackage

import (
	"os"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// okapi-skip: WsxzPackageFilterTests#testInformation — Java-only API test (IFilter.getName/getMimeType)

// okapi: WsxzPackageFilterTests#testSimpleRead
func TestSimpleRead(t *testing.T) {
	parts := readFile(t, "okf_wsxzpackage/test1.wsxz")
	require.NotEmpty(t, parts)

	// WSXZ packages contain XLIFF sub-documents.
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type, "last part should be LayerEnd")

	// Should have nested layers for the embedded XLIFF content.
	layerStarts := countPartsByType(parts, model.PartLayerStart)
	assert.GreaterOrEqual(t, layerStarts, 1, "should have at least 1 layer")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from WSXZ package")

	// Verify blocks have valid structure.
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID, "block should have an ID")
		require.NotEmpty(t, b.Source, "block should have source segments")
		for _, seg := range b.Source {
			assert.NotNil(t, seg.Content, "segment should have content")
		}
	}
}

// okapi: WsxzPackageFilterTests#testSimpleReadWrite
func TestSimpleReadWrite(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, "okf_wsxzpackage/test1.wsxz")

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, content, path, mimeType, nil)
	require.NotEmpty(t, result.Parts, "roundtrip should produce parts")
	require.NotEmpty(t, result.Output, "roundtrip should produce output")

	blocks := bridgetest.TranslatableBlocks(result.Parts)
	require.NotEmpty(t, blocks, "roundtrip should extract translatable blocks")
}

// TestLayerBalance verifies that layer start/end parts are balanced.
func TestLayerBalance(t *testing.T) {
	parts := readFile(t, "okf_wsxzpackage/test1.wsxz")

	starts := countPartsByType(parts, model.PartLayerStart)
	ends := countPartsByType(parts, model.PartLayerEnd)
	assert.Equal(t, starts, ends,
		"layer starts (%d) and ends (%d) should be balanced", starts, ends)
}
