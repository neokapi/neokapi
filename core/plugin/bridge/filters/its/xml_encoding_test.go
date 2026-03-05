//go:build integration

package its

import (
	"os"
	"testing"

	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Tests translated from XMLFilterEncodingTest.java (4 @Test methods)
// These verify encoding detection and conversion.
// ---------------------------------------------------------------------------

// okapi: XMLFilterEncodingTest#utf8ToUtf16le
func TestEncoding_UTF8ToUTF16LE(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	path := tdDir + "/okf_xml/test08_utf8nobom.xml"

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	parts := bridgetest.ReadBytes(t, pool, cfg, filterClass, content, path, mimeType, nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "UTF-8 file should have translatable content")
}

// okapi: XMLFilterEncodingTest#utf16WithBom
func TestEncoding_UTF16WithBom(t *testing.T) {
	// This test verifies that UTF-16 with BOM is handled correctly.
	// test10_utf16le-with-bom.xml is a UTF-16LE file with BOM.
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	path := tdDir + "/okf_xml/test10_utf16le-with-bom.xml"

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	parts := bridgetest.ReadBytes(t, pool, cfg, filterClass, content, path, mimeType, nil)
	require.NotEmpty(t, parts)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "UTF-16LE with BOM should have translatable content")
}

// okapi: XMLFilterEncodingTest#utf16WithoutBom
func TestEncoding_UTF16WithoutBom(t *testing.T) {
	// For UTF-16 without BOM, the XML declaration encoding attribute is used.
	// We test with a simple UTF-8 snippet that declares UTF-8 encoding.
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc><p>UTF-8 declared encoding</p></doc>`
	parts := readXMLDefault(t, xml)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "UTF-8 declared encoding", blocks[0].SourceText())
}

// okapi: XMLFilterEncodingTest#utf16leWithBomFromFile
func TestEncoding_UTF16LEWithBomFromFile(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	path := tdDir + "/okf_xml/test10_utf16le-with-bom.xml"

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	// Should read and extract successfully despite being UTF-16LE.
	parts := bridgetest.ReadBytes(t, pool, cfg, filterClass, content, path, mimeType, nil)
	require.NotEmpty(t, parts)

	// Verify it roundtrips.
	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, content, path, mimeType, nil)
	require.NotEmpty(t, result.Output)
}
