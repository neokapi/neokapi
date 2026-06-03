package json_test

import (
	"strings"
	"testing"

	jsonfmt "github.com/neokapi/neokapi/core/formats/json"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readAll drains the reader's result channel, returning whether any clean
// PartResult.Error surfaced and the number of translatable blocks emitted. It
// is wrapped in require.NotPanics by callers so a panic fails the test with a
// clear message instead of crashing the run.
func readAll(t *testing.T, input string) (foundError bool, blocks int) {
	t.Helper()
	ctx := t.Context()
	reader := jsonfmt.NewReader()

	require.NotPanics(t, func() {
		// Open only validates the document/reader; parse errors surface
		// later, during Read.
		err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
		require.NoError(t, err)
	})
	defer reader.Close()

	require.NotPanics(t, func() {
		for result := range reader.Read(ctx) {
			if result.Error != nil {
				foundError = true
			}
			if result.Part != nil && result.Part.Type == model.PartBlock {
				blocks++
			}
		}
	})
	return foundError, blocks
}

// TestReadMalformedSurfacesError feeds inputs the JSON scanner genuinely cannot
// tokenize and asserts the parse error surfaces cleanly on the result channel
// (PartResult.Error) rather than panicking or being silently swallowed.
//
// Run with -race to catch any data race in the reader goroutine that drives the
// channel.
func TestReadMalformedSurfacesError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{
			// Truncated object: the value string is opened but never closed,
			// so the scanner reports an unterminated string.
			name:  "truncated object unterminated string",
			input: `{"appTitle": "Flutter`,
		},
		{
			// Pure garbage bytes: the first character is neither whitespace,
			// a structural token, a number, a string, nor a JSON5 bare
			// identifier start, so the scanner rejects it.
			name:  "garbage bytes",
			input: "@@@ %%% ^^^",
		},
		{
			// Invalid \u escape: the four characters after \u are not hex.
			name:  "invalid unicode escape",
			input: `{"k": "\uZZZZ"}`,
		},
		{
			// Incomplete \u escape: fewer than four hex digits remain.
			name:  "incomplete unicode escape",
			input: `{"k": "\u12"`,
		},
		{
			// Dangling backslash: the input ends mid escape sequence.
			name:  "dangling escape at eof",
			input: `{"k": "\`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			foundError, _ := readAll(t, tt.input)
			assert.True(t, foundError, "expected a clean error for malformed JSON input")
		})
	}
}

// TestReadLenientInputsDoNotPanic feeds inputs the scanner deliberately
// tolerates. The JSON reader's scanner is a permissive tokenizer (not a
// validating parser): it accepts JSON5 niceties and recovers from unbalanced
// structure rather than rejecting it (see scanner.go and reader.go's walk
// functions). These inputs must therefore not panic and not race; whether they
// surface an error or parse leniently is an implementation detail we do not
// over-assert here. The single contract is robustness: no panic.
//
// Run with -race to surface any data race in the reader goroutine.
func TestReadLenientInputsDoNotPanic(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "empty input",
			input: "",
		},
		{
			// A lone byte-order mark with no following content. The scanner
			// treats U+FEFF as leading whitespace, so this reduces to empty.
			name:  "lone bom",
			input: "\uFEFF",
		},
		{
			// Unbalanced opening braces with no matching close.
			name:  "unbalanced open braces",
			input: "{{{{",
		},
		{
			// Unbalanced closing braces with nothing opened.
			name:  "unbalanced close braces",
			input: "}}}}",
		},
		{
			// Truncated array: closing bracket missing.
			name:  "truncated array",
			input: `["a", "b"`,
		},
		{
			// Truncated object that ends on structural tokens (no open
			// string), so the tokenizer reaches EOF without an error.
			name:  "truncated object after comma",
			input: `{"a": 1, `,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.NotPanics(t, func() {
				readAll(t, tt.input)
			})
		})
	}
}

// TestReadDeeplyNestedDoesNotPanic feeds a deeply nested object to exercise the
// recursive walkTokenValue path. Go grows goroutine stacks on demand, so this
// must complete without a stack-overflow panic.
//
// Run with -race to surface any data race in the reader goroutine.
func TestReadDeeplyNestedDoesNotPanic(t *testing.T) {
	t.Parallel()
	const depth = 5000
	input := strings.Repeat(`{"k":`, depth) + `"deep value"` + strings.Repeat(`}`, depth)

	var foundError bool
	var blocks int
	require.NotPanics(t, func() {
		foundError, blocks = readAll(t, input)
	})
	// The input is well-formed JSON, so it parses without error and yields the
	// single innermost translatable string.
	assert.False(t, foundError, "well-formed deeply nested JSON should not error")
	assert.Equal(t, 1, blocks, "expected the innermost value as the sole block")
}

// TestReadNilReader verifies Open rejects a document whose Reader is nil
// without panicking. (The nil-*document* case is covered by
// TestReadNilDocument in reader_test.go.)
func TestReadNilReader(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := jsonfmt.NewReader()
	require.NotPanics(t, func() {
		err := reader.Open(ctx, &model.RawDocument{SourceLocale: model.LocaleEnglish})
		require.Error(t, err)
	})
}
