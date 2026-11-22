package json_test

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
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
		// The rest tokenize cleanly and break the grammar.
		{name: "member with no value (#3616)", input: `{"a": "one", "b": }`},
		{name: "key with no colon", input: `{"a" "b"}`},
		{name: "members with no comma", input: `{"a": "b" "c": "d"}`},
		{name: "elements with no comma", input: `["a" "b"]`},
		{name: "comma before any member", input: `{, "a": "b"}`},
		{name: "two commas in an array", input: `["a",, "b"]`},
		{name: "stray close bracket in an object", input: `{"a": "b", ] }`},
		{name: "unbalanced open braces", input: "{{{{"},
		{name: "unbalanced close braces", input: "}}}}"},
		{name: "truncated array", input: `["a", "b"`},
		{name: "truncated object after comma", input: `{"a": 1, `},
		{name: "a second top-level value", input: `{"a": "b"} {"c": "d"}`},
		{name: "fuzz crasher with no colons", input: `{"a""""a""0"0`},
		{name: "unterminated block comment", input: `/*A`},
		{name: "unterminated block comment after a value", input: `{"a": "b"} /* open`},
		{name: "unterminated nested block comment", input: `/* /* */ {"a": "b"}`},
		{name: "unterminated html comment", input: `<!-- {"a": "b"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			foundError, _ := readAll(t, tt.input)
			assert.True(t, foundError, "expected a clean error for malformed JSON input")
		})
	}
}

// TestReadMalformedErrorNamesThePosition checks that a grammar error names
// what was expected and the byte offset of the token found instead, on both the
// buffered path and the streaming path a skeleton round-trip reads through.
func TestReadMalformedErrorNamesThePosition(t *testing.T) {
	t.Parallel()
	const input = `{"a": "one", "b": }`
	want := `expected a value, found "}" at position 18`

	reader := jsonfmt.NewReader()
	require.NoError(t, reader.Open(t.Context(), testutil.RawDocFromString(input, model.LocaleEnglish)))
	require.ErrorContains(t, firstError(t, reader), want)

	streaming := jsonfmt.NewReader()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	streaming.SetSkeletonStore(store)
	require.NoError(t, streaming.Open(t.Context(), testutil.RawDocFromString(input, model.LocaleEnglish)))
	require.ErrorContains(t, firstError(t, streaming), want)
}

func firstError(t *testing.T, reader *jsonfmt.Reader) error {
	t.Helper()
	defer reader.Close()
	var first error
	for result := range reader.Read(t.Context()) {
		if result.Error != nil && first == nil {
			first = result.Error
		}
	}
	return first
}

// TestReadAcceptsJSON5 checks that the grammar keeps the JSON5 forms the
// scanner reads: comments, a trailing comma, bare keys, single quotes, and an
// empty document.
func TestReadAcceptsJSON5(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		input  string
		blocks int
	}{
		{name: "empty input", input: "", blocks: 0},
		{name: "lone bom", input: "\uFEFF", blocks: 0},
		{name: "whitespace and a comment only", input: "  // nothing\n", blocks: 0},
		{name: "trailing comma in an object", input: `{"a": "b",}`, blocks: 1},
		{name: "trailing comma in an array", input: `{"a": ["b", "c",]}`, blocks: 2},
		{name: "bare key and single quotes", input: `{a: 'b'}`, blocks: 1},
		{name: "comments between tokens", input: "{/* c */ \"a\" /* c */ : // c\n \"b\" # c\n}", blocks: 1},
		{name: "empty containers", input: `{"a": {}, "b": []}`, blocks: 0},
		{name: "top-level scalar", input: `"just text"`, blocks: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			foundError, blocks := readAll(t, tt.input)
			assert.False(t, foundError)
			assert.Equal(t, tt.blocks, blocks)
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
