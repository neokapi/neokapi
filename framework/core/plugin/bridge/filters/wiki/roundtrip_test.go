//go:build integration

package wiki

import (
	"testing"

	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// okapi: RoundTripWikiIT (simple inline snippet)
func TestRoundTrip_Simple(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	input := []byte("== Title ==\nSimple text here.\n")
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, input, "test.txt", mimeType, nil)
}

// okapi: RoundTripWikiIT (multi-line snippet)
func TestRoundTrip_MultiLine(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	input := []byte("== Title ==\nFirst line.\nSecond line.\n")
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, input, "test.txt", mimeType, nil)
}

// okapi: RoundTripWikiIT (table snippet)
func TestRoundTrip_Table(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	input := []byte("{|\n|-\n| Cell 1 || Cell 2\n|-\n| Cell 3 || Cell 4\n|}")
	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, input, "test.txt", mimeType, nil)
	require.NotEmpty(t, result.Output, "roundtrip should produce output")
	assert.Contains(t, string(result.Output), "Cell 1")
}

// okapi: RoundTripWikiIT#testWikiFiles (*.wiki files)
func TestRoundTrip_WikiFiles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	// Test all .wiki files from okf_wiki testdata.
	// The Java RoundTripWikiIT iterates over wiki files in the wikitext/ directory.
	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okf_wiki/*.wiki", mimeType, nil)
}

// okapi: RoundTripWikiIT#testWikiFiles (*.txt DokuWiki files)
func TestRoundTrip_DokuWikiTxtFiles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	// DokuWiki uses .txt extension.
	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okf_wiki/*.txt", mimeType, nil)
}

// okapi: RoundTripWikiIT (header roundtrip)
func TestRoundTrip_Header(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	input := []byte("== Header ==\nBody text.\n")
	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, input, "test.txt", mimeType, nil)
	require.NotEmpty(t, result.Output, "roundtrip should produce output")
	assert.Contains(t, string(result.Output), "Header")
	assert.Contains(t, string(result.Output), "Body text")
}

// okapi: RoundTripWikiIT (image caption roundtrip)
func TestRoundTrip_ImageCaption(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	input := []byte("[[File:Example.jpg|thumb|A caption for the image]]")
	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, input, "test.txt", mimeType, nil)
	require.NotEmpty(t, result.Output, "roundtrip should produce output")
}

// okapi: RoundTripWikiIT (wiki markup formatting roundtrip)
func TestRoundTrip_WikiFormatting(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	tests := []struct {
		name  string
		input string
	}{
		{"bold", "'''Bold text''' here."},
		{"italic", "''Italic text'' here."},
		{"link", "A [[link|link text]] in text."},
		{"header_l2", "== Header 2 =="},
		{"header_l3", "=== Header 3 ==="},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := bridgetest.RoundTrip(t, pool, cfg, filterClass,
				[]byte(tt.input), "test.txt", mimeType, nil)
			require.NotEmpty(t, result.Output, "roundtrip should produce output for %s", tt.name)
		})
	}
}
