package messageformat_test

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/formats/messageformat"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadMalformedRejected feeds broken, truncated, and stray-brace patterns
// and asserts the reader surfaces a clean error without panicking.
//
// The MessageFormat reader is a StreamingReader: Open only stores the document
// and parsing happens per line in Read, so a syntax error surfaces on the Read
// channel (PartResult.Error) rather than from Open. Each error must carry the
// format's diagnostic prefix so callers can tell the failure came from
// MessageFormat parsing. The error is never swallowed — the file runner and the
// subfilter driver both check PartResult.Error.
func TestReadMalformedRejected(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{
			// Picker truncated mid-branch: opening branch brace, no body close,
			// no closing brace for the plural expression.
			name:  "truncated plural picker",
			input: "{count, plural, one {",
		},
		{
			// Unterminated selectordinal expression.
			name:  "truncated selectordinal picker",
			input: "{count, selectordinal, one {#st}",
		},
		{
			// Unterminated select with no closing brace.
			name:  "unterminated select",
			input: "{gender, select, male {x} ",
		},
		{
			// Unbalanced open brace: argument never closed.
			name:  "unbalanced open brace",
			input: "Hello {name",
		},
		{
			// A run of open braces, each opening an unterminated argument.
			name:  "many open braces",
			input: strings.Repeat("{", 64),
		},
		{
			// Stray closing brace at the top level.
			name:  "stray close brace",
			input: "Hello }",
		},
		{
			// A run of stray closing braces.
			name:  "many close braces",
			input: strings.Repeat("}", 64),
		},
		{
			// The deprecated CHOICE format is explicitly rejected.
			name:  "deprecated choice",
			input: "{0,choice,0#none|1#one|1<many}",
		},
		{
			// Argument with a type list but no branch bodies.
			name:  "plural without branches",
			input: "{count, plural,",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			reader := messageformat.NewReader()
			defer reader.Close()

			// Streaming reader: Open only stores the document.
			require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(tt.input, model.LocaleEnglish)))

			var readErr error
			require.NotPanics(t, func() {
				for res := range reader.Read(ctx) {
					if res.Error != nil {
						readErr = res.Error
						break
					}
				}
			})
			require.Error(t, readErr, "malformed MessageFormat input must be rejected on the Read channel")
			assert.Contains(t, readErr.Error(), "messageformat:",
				"error should be attributed to the messageformat reader")
		})
	}
}

// TestReadTolerantInputs feeds inputs the parser deliberately tolerates as
// literal text — binary bytes, control characters, garbage, and large but
// in-bounds documents — and asserts the full Open→Read cycle never panics and
// never silently swallows a parse error. Patterns with no MessageFormat syntax
// (no unmatched braces) are valid literal messages, so Open succeeds and Read
// emits blocks cleanly.
func TestReadTolerantInputs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{
			// NUL, control bytes, and invalid UTF-8 — no brace syntax, so the
			// line is treated as opaque literal text.
			name:  "binary garbage",
			input: "\x00\x01\x02\xff\xfe\xfd garbage \x07\x08",
		},
		{
			// Pure invalid UTF-8.
			name:  "invalid utf8",
			input: "\xc3\x28\xa0\xa1 broken bytes",
		},
		{
			// Random-looking ASCII with no MessageFormat markup.
			name:  "ascii noise",
			input: "asd;lfkj!@#$%^&*()_+=-`~[]\\|;:\",.<>/?",
		},
		{
			// Many lines, each a plain literal: stresses the per-line loop with
			// a large overall document (~600KB) that stays within the scanner's
			// per-line token limit.
			name:  "many literal lines",
			input: strings.Repeat("line of text\n", 50000),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			reader := messageformat.NewReader()
			defer reader.Close()

			var openErr error
			require.NotPanics(t, func() {
				openErr = reader.Open(ctx, testutil.RawDocFromString(tt.input, model.LocaleEnglish))
			})
			require.NoError(t, openErr, "tolerant input should open without error")

			require.NotPanics(t, func() {
				for result := range reader.Read(ctx) {
					// A tolerant input must never surface a swallowed parse
					// error on the channel: it either rejects in Open or
					// streams cleanly.
					require.NoError(t, result.Error,
						"Read must not surface an error for tolerant input")
				}
			})
		})
	}
}

// TestReadVeryLargeLine feeds a single literal line far larger than the old
// bufio.Scanner token limit (64KB). The streaming reader reads lines via a
// bufio window with no per-line token cap — the total read is bounded by the
// shared safeio byte budget (1 GiB) instead — so an over-long literal line is
// read and emitted as one message block, not rejected, truncated, or corrupted.
// It must not panic and must not surface a spurious error.
func TestReadVeryLargeLine(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := messageformat.NewReader()
	defer reader.Close()

	// ~4MB on a single line, well past the old bufio.MaxScanTokenSize but far
	// under the safeio byte budget.
	input := strings.Repeat("the quick brown fox ", 200000)

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))

	blocks := 0
	require.NotPanics(t, func() {
		for res := range reader.Read(ctx) {
			require.NoError(t, res.Error, "an over-long literal line must read cleanly")
			if res.Part != nil && res.Part.Type == model.PartBlock {
				blocks++
			}
		}
	})
	assert.Equal(t, 1, blocks, "the whole line is one literal message block")
}

// TestReadNilDocumentNoPanic verifies Open rejects a nil document without
// panicking. (TestReadNilDocument in reader_test.go already asserts the error;
// this guards the no-panic contract specifically.)
func TestReadNilDocumentNoPanic(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := messageformat.NewReader()

	var err error
	require.NotPanics(t, func() {
		err = reader.Open(ctx, nil)
	})
	require.Error(t, err)
}

// TestReadNilReader verifies Open rejects a document whose underlying reader is
// nil without panicking.
func TestReadNilReader(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := messageformat.NewReader()

	var err error
	require.NotPanics(t, func() {
		err = reader.Open(ctx, &model.RawDocument{SourceLocale: model.LocaleEnglish})
	})
	require.Error(t, err)
}
