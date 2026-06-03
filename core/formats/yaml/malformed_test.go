package yaml_test

import (
	"testing"

	yamlfmt "github.com/neokapi/neokapi/core/formats/yaml"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readAll drains the reader's result channel, returning whether any clean
// PartResult.Error surfaced and the number of translatable blocks emitted. It
// is wrapped in require.NotPanics by callers so a panic fails the test with a
// clear message instead of crashing the run. YAML is text, so the input is fed
// through testutil.RawDocFromString.
func readAll(t *testing.T, input string) (foundError bool, blocks int) {
	t.Helper()
	ctx := t.Context()
	reader := yamlfmt.NewReader()

	require.NotPanics(t, func() {
		// Open only validates the document/reader; the YAML decoder runs
		// during Read, so parse errors surface on the result channel.
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

// TestReadMalformedSurfacesError feeds inputs the yaml.v3 decoder genuinely
// cannot parse and asserts the parse error surfaces cleanly on the result
// channel (PartResult.Error) rather than panicking or being silently swallowed.
// The reader wraps yaml.v3's streaming Decoder.Decode and forwards any non-EOF
// error as PartResult{Error: ...} (see readContent in reader.go).
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
			// A child key dedented to an inconsistent column: the decoder
			// reports "did not find expected key".
			name:  "bad indentation",
			input: "parent:\n  a: 1\n b: 2\n",
		},
		{
			// Double-quoted scalar opened but never closed before EOF.
			name:  "unterminated double quote",
			input: "key: \"unterminated value\n",
		},
		{
			// Single-quoted scalar opened but never closed before EOF.
			name:  "unterminated single quote",
			input: "key: 'unterminated value\n",
		},
		{
			// Flow sequence opened with '[' and never closed.
			name:  "unclosed flow sequence",
			input: "key: [a, b, c\n",
		},
		{
			// Flow mapping opened with '{' and never closed.
			name:  "unclosed flow mapping",
			input: "key: {a: 1, b: 2\n",
		},
		{
			// Tab used for block indentation, which YAML forbids: the
			// decoder reports a character that cannot start any token.
			name:  "tab indentation",
			input: "parent:\n\tchild: value\n",
		},
		{
			// Alias referencing an anchor that was never defined.
			name:  "undefined alias",
			input: "key: *undefined\n",
		},
		{
			// Anchor introducer '&' with no following name.
			name:  "invalid anchor syntax",
			input: "key: &\n",
		},
		{
			// Non-YAML garbage at the document root.
			name:  "garbage bytes",
			input: "@@@ %%% ^^^\n",
		},
		{
			// Raw control bytes, which the scanner rejects outright.
			name:  "garbage control bytes",
			input: "\x00\x01\x02\xff\xfe",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			foundError, _ := readAll(t, tt.input)
			assert.True(t, foundError, "expected a clean error for malformed YAML input")
		})
	}
}

// TestReadLenientInputsDoNotPanic feeds inputs that yaml.v3 deliberately
// tolerates rather than rejecting. The contract here is robustness only: the
// reader must not panic, not hang, and not race — whether such input surfaces
// an error or parses leniently is a decoder detail we do not over-assert.
//
// Run with -race to surface any data race in the reader goroutine.
func TestReadLenientInputsDoNotPanic(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{
			// yaml.v3 accepts repeated keys, keeping the last value, so this
			// parses without error.
			name:  "duplicate keys",
			input: "a: 1\na: 2\n",
		},
		{
			// Empty input decodes to EOF immediately with no document.
			name:  "empty input",
			input: "",
		},
		{
			// Whitespace-only input also reduces to an empty stream.
			name:  "whitespace only",
			input: "   \n\t\n  \n",
		},
		{
			// Comments-only input yields no nodes.
			name:  "comments only",
			input: "# just a comment\n# and another\n",
		},
		{
			// A document separator with no following content.
			name:  "bare document separator",
			input: "---\n",
		},
		{
			// Truncated multi-document stream: the first document is complete,
			// the second is just a separator.
			name:  "truncated multi document",
			input: "a: 1\n---\n",
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

// TestReadDeeplyNestedDoesNotPanic feeds a deeply nested mapping to exercise the
// recursive walkNode path. Go grows goroutine stacks on demand, so a well-formed
// deeply nested document must complete without a stack-overflow panic. (yaml.v3
// imposes its own nesting/aliasing limits; if it rejects the depth, the error
// must still surface cleanly rather than panic — hence the relaxed assertion.)
//
// Run with -race to surface any data race in the reader goroutine.
func TestReadDeeplyNestedDoesNotPanic(t *testing.T) {
	t.Parallel()
	const depth = 2000
	var b []byte
	for i := range depth {
		for range i {
			b = append(b, ' ')
		}
		b = append(b, 'k', ':', '\n')
	}
	// Innermost value.
	for range depth {
		b = append(b, ' ')
	}
	b = append(b, []byte("v: deep value\n")...)

	require.NotPanics(t, func() {
		readAll(t, string(b))
	})
}

// TestReadNilReader verifies Open rejects a document whose Reader is nil without
// panicking, returning a clean error instead.
func TestReadNilReader(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := yamlfmt.NewReader()
	require.NotPanics(t, func() {
		err := reader.Open(ctx, &model.RawDocument{SourceLocale: model.LocaleEnglish})
		require.Error(t, err)
	})
}
