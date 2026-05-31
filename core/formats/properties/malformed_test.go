package properties_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/formats/properties"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/require"
)

// TestReadMalformedInput feeds a range of broken, truncated, and garbage
// inputs and asserts the reader neither panics nor wedges. Java Properties is
// a line-oriented text format with no structural grammar to violate, so the
// reader is lenient by design: truncated values, invalid escape sequences, and
// even raw binary bytes are treated as opaque text and pass through gracefully
// rather than surfacing a PartResult.Error. The contract this test pins is
// therefore "never panic, always drain the channel to completion" — if a
// future change makes any of these inputs panic (or deadlock the Read
// goroutine), this test fails loudly.
//
// Run under the race detector to catch any data race between the Read
// goroutine and the channel consumer on malformed input.
func TestReadMalformedInput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{
			// Key with no separator and a value that is simply cut off at
			// EOF (no terminator). The whole line is the key; value is empty.
			name:  "truncated key only no separator",
			input: "some.text.key",
		},
		{
			// A key=value whose value is truncated mid-token at EOF.
			name:  "truncated value at eof",
			input: "greeting.text=Hello, Wor",
		},
		{
			// Lone trailing backslash continuation at EOF: the line promises
			// a continuation line that never arrives. readLogicalLines must
			// flush the dangling continuation instead of looping forever.
			name:  "lone trailing backslash continuation at eof",
			input: "wrapped.text=line one \\",
		},
		{
			// Continuation backslash at EOF with no value content before it.
			name:  "bare backslash line at eof",
			input: "\\",
		},
		{
			// Invalid \uXXXX: too few hex digits ("\u00" — only two). The
			// decoder must keep the sequence verbatim, not read past the
			// end of the string.
			name:  "invalid unicode escape too short",
			input: "label.text=ab\\u00",
		},
		{
			// Invalid \uXXXX: non-hex digits ("\uZZZZ"). parseHexRune must
			// reject it and the literal backslash be preserved.
			name:  "invalid unicode escape non hex",
			input: "label.text=val\\uZZZZend",
		},
		{
			// A \u at the very end of input, with nothing after it.
			name:  "dangling unicode escape at eof",
			input: "label.text=tail\\u",
		},
		{
			// A lone trailing backslash with no following character (escape
			// that runs off the end of the value).
			name:  "dangling backslash escape at eof",
			input: "label.text=ends.with\\",
		},
		{
			// Raw garbage / non-UTF-8 binary bytes as the value. The reader
			// scans bytes, so this must pass through without panicking on
			// invalid runes.
			name:  "garbage binary bytes",
			input: "bin.text=\xff\xfe\x00\x80\x01garbage\x7f",
		},
		{
			// Garbage bytes spanning what looks like a key/separator too.
			name:  "garbage bytes whole line",
			input: "\x00\xff=\xfe\x80\\u\x00",
		},
		{
			// NUL bytes interleaved with a continuation marker at EOF.
			name:  "nul bytes with continuation at eof",
			input: "k.text=a\x00b \\",
		},
		{
			// Empty input — no lines at all.
			name:  "empty input",
			input: "",
		},
		{
			// Whitespace-only input.
			name:  "whitespace only",
			input: "   \t  ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			reader := properties.NewReader()
			require.NotPanics(t, func() {
				err := reader.Open(ctx, testutil.RawDocFromString(tt.input, model.LocaleEnglish))
				require.NoError(t, err)
			})
			defer reader.Close()

			// Drain the full channel. The reader is lenient, so we don't
			// require an error — only that nothing panics and the channel
			// closes (the loop terminates). A swallowed panic in the Read
			// goroutine would propagate here; a deadlock would hang the
			// per-test timeout.
			require.NotPanics(t, func() {
				for result := range reader.Read(ctx) {
					// If the reader ever does surface an error for malformed
					// input, that is acceptable too — we just must not panic.
					_ = result.Error
				}
			})
		})
	}
}

// Note: Open's rejection of a nil document is covered by TestReadNilDocument
// in reader_test.go. The nil-Reader case below is the additional guard that a
// nil document does not cover.

// TestReadNilReader verifies Open rejects a document whose Reader is nil
// (an otherwise-populated RawDocument) without panicking. This is distinct
// from a nil document: the reader must guard the embedded Reader too, or
// readLogicalLines would dereference nil.
func TestReadNilReader(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := properties.NewReader()
	doc := &model.RawDocument{
		URI:          "test://nil-reader",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       nil,
	}
	require.NotPanics(t, func() {
		err := reader.Open(ctx, doc)
		require.Error(t, err)
	})
}
