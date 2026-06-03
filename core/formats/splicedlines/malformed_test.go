package splicedlines_test

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/formats/splicedlines"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readAll drains the reader's result channel, returning whether any clean
// PartResult.Error surfaced and the number of translatable blocks emitted. It
// is wrapped in require.NotPanics by callers so a panic fails the test with a
// clear message instead of crashing the run.
//
// The splicedlines reader is a byte-oriented, fully lenient logical-line
// tokenizer: it joins lines ending in `\` into a single Block and best-effort
// parses any byte sequence as text. It only surfaces a PartResult.Error on a
// genuine non-EOF I/O error from the underlying reader (reader.go's read loop),
// never on "malformed" content — there is no content grammar to violate. So
// every input below is expected to parse cleanly (foundError == false); the
// contract these tests guard is robustness: no panic, no hang, no race.
func readAll(t *testing.T, input string) (foundError bool, blocks int) {
	t.Helper()
	ctx := t.Context()
	reader := splicedlines.NewReader()

	require.NotPanics(t, func() {
		// Open only validates the document/reader; any parse work happens
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

// TestReadMalformedDoesNotPanic feeds malformed, garbage, truncated, and
// otherwise hostile byte sequences and asserts the reader handles each
// gracefully: no panic, no hang, no race, and a clean (error-free) drain. The
// reader has no content grammar — it tokenizes logical lines — so the
// best-effort contract is that none of these inputs should error or crash.
//
// Run with -race to surface any data race in the reader goroutine that drives
// the channel.
func TestReadMalformedDoesNotPanic(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{
			// Trailing line-continuation with no following line: the file
			// ends mid-continuation, so the post-loop EOF flush must handle
			// the dangling accumulator (and tag it trailing-splicer) rather
			// than indexing past the slice.
			name:  "trailing splicer at eof",
			input: "Line 1\\",
		},
		{
			// Trailing continuation followed only by a bare newline, so the
			// accumulator's final entry has an empty content but a line
			// ending — exercises the same EOF-flush path with a non-empty
			// final line ending.
			name:  "trailing splicer then newline",
			input: "Line 1\\\n",
		},
		{
			// A line that is *only* the continuation marker: stripped to
			// empty content, then accumulated. The next (final) empty flush
			// must not panic on the all-blank accumulator.
			name:  "lone continuation marker",
			input: "\\",
		},
		{
			// Two consecutive lone continuation markers feeding EOF: the
			// accumulator holds two empty-content lines whose joined text is
			// blank, exercising the empty-data branch of flushBlock.
			name:  "double lone continuation marker",
			input: "\\\n\\",
		},
		{
			// Mixed / garbage non-text bytes. ReadString is byte-oriented and
			// must pass these through untouched without choking.
			name:  "garbage bytes",
			input: "@@@ \x00\x01\x02 %%% \x7f^^^",
		},
		{
			// Invalid UTF-8: lone continuation bytes and an unfinished
			// multibyte sequence. The reader works on bytes, not runes, so
			// it must not panic decoding these.
			name:  "invalid utf-8",
			input: "abc\xff\xfe\xc3\x28 def",
		},
		{
			// Invalid UTF-8 immediately before a continuation marker, so the
			// splicer detection and accumulation runs over non-UTF-8 bytes.
			name:  "invalid utf-8 before splicer",
			input: "\xff\xfe\\\nmore",
		},
		{
			// Bare CR with no LF: not recognized as a line ending, so the CR
			// stays part of the line content. Must not be mistaken for CRLF.
			name:  "bare carriage returns",
			input: "Line 1\rLine 2\rLine 3",
		},
		{
			// CRLF and LF interleaved with a continuation across the styles.
			name:  "mixed crlf and lf with continuation",
			input: "A\\\r\nB\\\nC\r\nD",
		},
		{
			// NUL bytes embedded mid-line, including adjacent to a splicer.
			name:  "embedded nul bytes",
			input: "a\x00b\\\nc\x00d",
		},
		{
			// A run of backslashes at end of line. Only the final one is the
			// splicer; the rest stay in content. Tests TrimSuffix semantics.
			name:  "multiple trailing backslashes",
			input: "path\\\\\\\nnext",
		},
		{
			// Whitespace-only continuation chain ending at EOF: every line is
			// blank, so the joined text trims to empty and the empty-data
			// branch runs at EOF flush.
			name:  "whitespace-only continuation chain",
			input: "   \\\n\t\\\n  ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			foundError, _ := readAll(t, tt.input)
			assert.False(t, foundError,
				"the lenient splicedlines reader should best-effort parse this input without error")
		})
	}
}

// TestReadEmptyInputIsRobust verifies the degenerate empty-document case under
// the panic guard: no lines at all, so the reader emits only the layer
// start/end and yields no blocks, without panicking. (TestReadEmpty in
// reader_test.go asserts the same emptiness; this adds the no-panic contract.)
func TestReadEmptyInputIsRobust(t *testing.T) {
	t.Parallel()
	var foundError bool
	var blocks int
	require.NotPanics(t, func() {
		foundError, blocks = readAll(t, "")
	})
	assert.False(t, foundError, "empty input should not error")
	assert.Equal(t, 0, blocks, "empty input yields no translatable blocks")
}

// TestReadManyContinuationsDoesNotPanic feeds a very long continuation chain to
// exercise the accumulator-growth path. The accumulator is a flat slice (no
// recursion), so this must complete without panicking or hanging.
//
// Run with -race to surface any data race in the reader goroutine.
func TestReadManyContinuationsDoesNotPanic(t *testing.T) {
	t.Parallel()
	const lines = 20000
	// Each line ends in `\` so they all splice into a single block, with a
	// final non-continuation line to terminate the chain.
	input := strings.Repeat("x\\\n", lines) + "end"

	var foundError bool
	var blocks int
	require.NotPanics(t, func() {
		foundError, blocks = readAll(t, input)
	})
	assert.False(t, foundError, "a long continuation chain should parse without error")
	assert.Equal(t, 1, blocks, "the whole chain joins into one block")
}

// TestReadNilReader verifies Open rejects a document whose Reader is nil
// without panicking, surfacing a clean error instead. (The nil-*document* case
// is covered by TestReadNilDocument in reader_test.go.)
func TestReadNilReader(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := splicedlines.NewReader()
	require.NotPanics(t, func() {
		err := reader.Open(ctx, &model.RawDocument{SourceLocale: model.LocaleEnglish})
		require.Error(t, err)
	})
}
