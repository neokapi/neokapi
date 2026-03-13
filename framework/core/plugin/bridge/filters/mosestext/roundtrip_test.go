//go:build integration

package mosestext

import (
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/require"
)

// readTestFile reads a file from disk and returns the content bytes.
func readTestFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// okapi: MosesTextFilterTest#testDoubleExtraction (event-level roundtrip)
func TestRoundTrip_Simple(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	input := []byte("Hello world\nThis is a test.\n")
	bridgetest.AssertRoundTrip(t, pool, cfg, filterClass, input, "test.txt", mimeType, nil)
}

// okapi: MosesTextFilterTest#testDoubleExtraction (all test files)
func TestRoundTrip_TestFiles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okapi/filters/mosestext/src/test/resources/*.txt", mimeType, nil)
}

// okapi: MosesTextFilterTest#testLineBreaks_CR (roundtrip)
// okapi: MosesTextFilterTest#testineBreaks_CRLF (roundtrip)
// okapi: MosesTextFilterTest#testLineBreaks_LF (roundtrip)
func TestRoundTrip_LineEndings(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	tests := []struct {
		name  string
		input string
	}{
		{"lf", "Line 1\nLine 2\n"},
		{"crlf", "Line 1\r\nLine 2\r\n"},
		{"cr", "Line 1\rLine 2\r"},
		{"lf_no_trailing", "Line 1\nLine 2"},
		{"crlf_no_trailing", "Line 1\r\nLine 2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := bridgetest.RoundTrip(t, pool, cfg, filterClass,
				[]byte(tt.input), "test.txt", mimeType, nil)
			require.NotEmpty(t, result.Output, "roundtrip should produce output")
		})
	}
}

// okapi: MosesTextFilterTest#testCode1 (roundtrip inline codes)
func TestRoundTrip_InlineCodes(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	tests := []struct {
		name  string
		input string
	}{
		{"x_tag", "Text <x id='1'/>\n"},
		{"g_tag", "<g id='2'>Text</g> <x id='1'/>\n"},
		{"nested_g", "<g id='1'>Text</g><x id='2'/><g id='3'>t2<x id='4'/><g id='5'>t3</g></g>\n"},
		{"bx_ex", "<bx id='1'/>T1<x id='2'/>T2<ex id='3'/>\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
				[]byte(tt.input), "test.txt", mimeType, nil)
		})
	}
}

// okapi: MosesTextFilterTest#testFromFile (roundtrip individual files)
func TestRoundTrip_IndividualFiles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	files := []string{"Test01.txt", "Test02.txt"}
	for _, f := range files {
		t.Run(f, func(t *testing.T) {
			path := tdDir + "/okapi/filters/mosestext/src/test/resources/" + f
			content, err := readTestFile(path)
			require.NoError(t, err)
			bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
				content, path, mimeType, nil)
		})
	}
}

// okapi: MosesTextFilterTest#testLiterals (roundtrip entity handling)
func TestRoundTrip_Literals(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	input := []byte("&lt;=lt, &gt;=gt, &quot;=quot, &apos;=apos, &amp;=amp\n")
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
		input, "test.txt", mimeType, nil)
}

// okapi: MosesTextFilterTest#testEntry (roundtrip mrk segments)
func TestRoundTrip_MrkSegments(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	tests := []struct {
		name  string
		input string
	}{
		{"simple_mrk", "<mrk mtype=\"seg\">Marked text</mrk>\n"},
		{"mrk_with_lb", "Text 1.\n<mrk mtype=\"seg\">Text 2\nText 3.</mrk>\n"},
		{"mixed_mrk_and_plain", "<mrk mtype=\"seg\">Segment 1</mrk>\nPlain line\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
				[]byte(tt.input), "test.txt", mimeType, nil)
		})
	}
}
