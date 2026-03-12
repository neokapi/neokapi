//go:build integration

package mif

import (
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// okapi: DocumentTest#iteratesThroughTheStatementsOfASample
func TestDocument_IteratesThroughStatementsOfASample(t *testing.T) {
	// Verify that the filter can iterate through all statements of a sample MIF file
	// without errors, producing a valid part sequence.
	parts := readMIFDefault(t, "Test01.mif")

	require.NotEmpty(t, parts, "should produce parts from Test01.mif")

	// Verify the document has proper structure: starts with LayerStart, ends with LayerEnd.
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type, "last part should be LayerEnd")

	// Should have at least some blocks between the layer boundaries.
	blocks := allBlocks(parts)
	assert.NotEmpty(t, blocks, "should extract blocks from sample MIF file")
}

// okapi: DocumentTest#iteratesThroughTheStatementsOfEveryResourceUnderTest
func TestDocument_IteratesThroughStatementsOfEveryResource(t *testing.T) {
	// Iterate through every .mif test file and verify that it can be parsed
	// without errors. This mirrors the Java test that processes all MIF resources.
	//
	// Ch13_Safety.mif triggers a Java-side StringIndexOutOfBoundsException
	// in the Okapi MIF filter (start -1, end 21, length 21), so we skip it.
	knownFailing := map[string]bool{
		"Ch13_Safety.mif": true,
	}

	files := mifTestFiles(t)
	require.NotEmpty(t, files, "should find MIF test files")

	for _, f := range files {
		t.Run(f, func(t *testing.T) {
			if knownFailing[f] {
				t.Skipf("known failing file: %s (Java-side StringIndexOutOfBoundsException)", f)
			}

			parts := readMIFDefault(t, f)
			require.NotEmpty(t, parts, "should produce parts from %s", f)

			// Every valid MIF document should start with LayerStart and end with LayerEnd.
			assert.Equal(t, model.PartLayerStart, parts[0].Type,
				"%s: first part should be LayerStart", f)
			assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type,
				"%s: last part should be LayerEnd", f)
		})
	}
}

// okapi: ExtractsTest#gathersExtractsFromEveryResourceUnderTest
func TestExtracts_GathersExtractsFromEveryResource(t *testing.T) {
	// Gather translatable extracts from every MIF test file.
	// This mirrors the Java ExtractsTest that reads all resources and collects
	// translatable content, verifying extraction succeeds without error.
	//
	// Ch13_Safety.mif triggers a Java-side StringIndexOutOfBoundsException
	// in the Okapi MIF filter, so we skip it.
	knownFailing := map[string]bool{
		"Ch13_Safety.mif": true,
	}

	pool, cfg := bridgetest.SharedBridge(t)
	files := mifTestFiles(t)
	require.NotEmpty(t, files, "should find MIF test files")

	for _, f := range files {
		t.Run(f, func(t *testing.T) {
			if knownFailing[f] {
				t.Skipf("known failing file: %s (Java-side StringIndexOutOfBoundsException)", f)
			}

			path := bridgetest.TestdataFile(t, "okf_mif/"+f)
			parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)
			require.NotEmpty(t, parts, "should produce parts from %s", f)

			// Verify extraction produces a valid part list (no panics, no errors).
			blocks := bridgetest.TranslatableBlocks(parts)
			t.Logf("%s: extracted %d translatable blocks", f, len(blocks))
		})
	}
}
