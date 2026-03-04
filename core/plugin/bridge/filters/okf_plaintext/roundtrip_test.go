//go:build integration

package okf_plaintext

import (
	"os"
	"testing"

	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/require"
)

// readTestFile reads a file from disk and returns the content bytes.
func readTestFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// okapi: RoundTripPlainTextIT
func TestRoundTrip_Simple(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	input := []byte("Hello world\nThis is a test.\n")
	bridgetest.AssertRoundTrip(t, pool, cfg, filterClass, input, "test.txt", mimeType, nil)
}

// okapi: RoundTripPlainTextIT#testPlainTextFiles
func TestRoundTrip_TestFiles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okf_plaintext/*.txt", mimeType, nil)
}

// okapi: RoundTripPlainTextIT#testPlainTextFiles (paragraph mode)
func TestRoundTrip_ParagraphMode(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	params := map[string]any{
		"parametersClass": "net.sf.okapi.filters.plaintext.paragraphs.Parameters",
	}

	tests := []struct {
		name  string
		input string
	}{
		{"single_line", "Hello world"},
		{"two_lines", "Line 1\nLine 2"},
		{"paragraphs", "Para 1 line 1\nPara 1 line 2\n\nPara 2"},
		{"trailing_newline", "Content\n"},
		{"leading_newline", "\nContent"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
				[]byte(tt.input), "test.txt", mimeType, params)
		})
	}
}

// okapi: RoundTripPlainTextIT#testPlainTextFiles (spliced mode)
func TestRoundTrip_SplicedMode(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	params := map[string]any{
		"parametersClass": "net.sf.okapi.filters.plaintext.spliced.Parameters",
	}

	tests := []struct {
		name  string
		input string
	}{
		{"simple_lines", "Line 1\nLine 2"},
		{"backslash_continuation", "Line 1 \\\nLine 2"},
		{"trailing_newline", "Content\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
				[]byte(tt.input), "test.txt", mimeType, params)
		})
	}
}

// okapi: RoundTripPlainTextIT#testPlainTextFiles (line ending variants)
func TestRoundTrip_LineEndings(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	tests := []struct {
		name  string
		input string
	}{
		{"lf", "Line 1\nLine 2\nLine 3"},
		{"crlf", "Line 1\r\nLine 2\r\nLine 3"},
		{"cr", "Line 1\rLine 2\rLine 3"},
		{"lf_trailing", "Line 1\nLine 2\n"},
		{"crlf_trailing", "Line 1\r\nLine 2\r\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := bridgetest.RoundTrip(t, pool, cfg, filterClass,
				[]byte(tt.input), "test.txt", mimeType, nil)
			require.NotEmpty(t, result.Output, "roundtrip should produce output")
		})
	}
}
