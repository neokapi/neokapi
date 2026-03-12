//go:build integration

package doxygen

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

// ---------------------------------------------------------------------------
// Roundtrip tests — snippet level
// ---------------------------------------------------------------------------

// okapi: DoxygenFilterTest#testOutputSimpleLine (roundtrip facet)
func TestRoundTrip_SimpleLine(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	input := []byte("/// A simple line comment\n")
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, input, "test.h", mimeType, nil)
}

// okapi: DoxygenFilterTest#testOutputOneLiner (roundtrip facet)
func TestRoundTrip_OneLiner(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	input := []byte("int x; ///< A one-liner comment\n")
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, input, "test.h", mimeType, nil)
}

// okapi: DoxygenFilterTest#testOutputMultipleLines (roundtrip facet)
func TestRoundTrip_MultipleLines(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	input := []byte("/// First line\n/// Second line\n/// Third line\n")
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, input, "test.h", mimeType, nil)
}

// okapi: DoxygenFilterTest#testOutputJavadocMultipleLines (roundtrip facet)
func TestRoundTrip_JavadocMultipleLines(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	input := []byte("/**\n * First line\n * Second line\n * Third line\n */\n")
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, input, "test.h", mimeType, nil)
}

// ---------------------------------------------------------------------------
// Roundtrip tests — full files
// ---------------------------------------------------------------------------

// okapi: RoundTripDoxygenIT (header files)
func TestRoundTrip_TestFilesH(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okf_doxygen/*.h", mimeType, nil)
}

// okapi: RoundTripDoxygenIT (Python files)
func TestRoundTrip_TestFilesPy(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okf_doxygen/*.py", mimeType, nil)
}

// ---------------------------------------------------------------------------
// Roundtrip tests — comment style variants
// ---------------------------------------------------------------------------

// okapi: RoundTripDoxygenIT (inline snippet variants)
func TestRoundTrip_CommentStyles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	tests := []struct {
		name  string
		input string
	}{
		{"triple_slash", "/// Triple slash comment\n"},
		{"exclamation", "//! Exclamation comment\n"},
		{"javadoc_single", "/** Javadoc single line */\n"},
		{"javadoc_multi", "/**\n * Javadoc multi line\n */\n"},
		{"qt_single", "/*! Qt single line */\n"},
		{"qt_multi", "/*!\n  Qt multi line.\n*/\n"},
		{"trailing_comment", "int x; ///< Trailing comment\n"},
		{"trailing_qt", "int x; /*!< Trailing Qt comment */\n"},
		{"code_with_comment", "int y = 0;\n/// Doxygen comment\nvoid f();\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
				[]byte(tt.input), "test.h", mimeType, nil)
		})
	}
}

// ---------------------------------------------------------------------------
// DoxygenWriterTest — writer tests
// ---------------------------------------------------------------------------

// okapi: DoxygenWriterTest#testOutputMultilineComment
func TestWriter_MultilineComment(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	// Multi-line triple-slash Doxygen comments should roundtrip correctly.
	input := "/// Line one\n/// Line two\n/// Line three\n"
	result := bridgetest.RoundTrip(t, pool, cfg, filterClass,
		[]byte(input), "test.h", mimeType, nil)
	require.NotEmpty(t, result.Output, "writer should produce output for multiline comment")
}

// okapi: DoxygenWriterTest#testOutputJavadocComment
func TestWriter_JavadocComment(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	// Javadoc-style multi-line comments should roundtrip correctly.
	input := "/**\n * Javadoc line one\n * Javadoc line two\n */\n"
	result := bridgetest.RoundTrip(t, pool, cfg, filterClass,
		[]byte(input), "test.h", mimeType, nil)
	require.NotEmpty(t, result.Output, "writer should produce output for Javadoc comment")
}
