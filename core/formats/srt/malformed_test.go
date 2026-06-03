package srt_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/srt"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// malformedSRTInputs collects broken/truncated/garbage/non-UTF-8 SRT payloads.
// SRT is a forgiving, line-oriented format: its state-machine parser tolerates
// structurally incomplete files (a truncated cue, a stray timecode, leading
// blank lines, etc.) rather than rejecting them. The contract these inputs
// verify is therefore robustness — the reader must never panic and must never
// fabricate a spurious channel error for input it has chosen to tolerate.
var malformedSRTInputs = []struct {
	name  string
	input string
}{
	{
		// Cue truncated mid-text at EOF: sequence + timecode present, one text
		// line, but no trailing blank line. The state machine must flush the
		// dangling entry without panicking.
		name:  "truncated cue at eof",
		input: "1\n00:00:01,000 --> 00:00:04,000\nHello",
	},
	{
		// Sequence + timecode with no text and no terminator — the entry never
		// reaches stateText content, so it yields no block.
		name:  "truncated after timecode",
		input: "1\n00:00:01,000 --> 00:00:04,000\n",
	},
	{
		// A bare timecode-looking line with no preceding sequence. The first
		// non-blank line is consumed as the "sequence", the timecode line as the
		// "timecode", leaving no text — a degenerate but non-panicking parse.
		name:  "timecode only fragment",
		input: "00:00:01,000 --> 00:00:04,000\n",
	},
	{
		// A lone sequence number with nothing after it.
		name:  "sequence only fragment",
		input: "42\n",
	},
	{
		// Arbitrary garbage that is valid UTF-8 but not SRT structure at all.
		name:  "garbage text",
		input: "%%% not a subtitle ::: {[<>]} \t\t lorem ipsum",
	},
	{
		// Invalid UTF-8 byte sequences embedded in otherwise SRT-shaped content.
		// Go strings carry arbitrary bytes, so this exercises the byte path.
		name:  "non-utf8 bytes",
		input: "1\n00:00:01,000 --> 00:00:04,000\n\xff\xfe\x00 broken \xc3\x28\n",
	},
	{
		// UTF-8 BOM prefixing the first sequence number. The BOM is not stripped
		// by the parser; it must still not panic.
		name:  "utf8 bom prefix",
		input: "\xef\xbb\xbf1\n00:00:01,000 --> 00:00:04,000\nHello\n",
	},
	{
		// UTF-16 LE BOM followed by garbage — wholly non-UTF-8.
		name:  "utf16 bom garbage",
		input: "\xff\xfe1\x00\n\x00",
	},
	{
		// Nothing but blank lines (mix of LF and CRLF). Every line is a
		// separator; no entry is ever produced.
		name:  "only blank lines",
		input: "\n\r\n\n\r\n",
	},
	{
		// Leading and interspersed stray blank lines around a single valid cue.
		name:  "stray blank lines around cue",
		input: "\n\n\n1\n00:00:01,000 --> 00:00:04,000\nHello\n\n\n\n",
	},
	{
		// Carriage-return-only line endings (old Mac style). ReadString('\n')
		// never splits, so the whole file arrives as one line.
		name:  "cr only line endings",
		input: "1\r00:00:01,000 --> 00:00:04,000\rHello\r",
	},
	{
		// A single NUL byte.
		name:  "lone nul byte",
		input: "\x00",
	},
}

// TestReadMalformedSimple feeds malformed/truncated/garbage/non-UTF-8 input
// through the simple (no-skeleton) read path and asserts the reader never panics
// and never surfaces a spurious channel error for tolerated input. Run with
// -race to catch unsynchronized access in the reader goroutine.
func TestReadMalformedSimple(t *testing.T) {
	t.Parallel()
	for _, tt := range malformedSRTInputs {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			reader := srt.NewReader()

			require.NotPanics(t, func() {
				err := reader.Open(ctx, testutil.RawDocFromString(tt.input, model.LocaleEnglish))
				require.NoError(t, err)
			})
			defer reader.Close()

			require.NotPanics(t, func() {
				for result := range reader.Read(ctx) {
					// SRT's line parser tolerates these inputs; it must not
					// emit a spurious error for content it accepted.
					assert.NoError(t, result.Error, "unexpected channel error for tolerated SRT input")
				}
			})
		})
	}
}

// TestReadMalformedSkeleton feeds the same malformed inputs through the
// byte-preserving skeleton read path (a distinct parser in reader.go) and
// asserts it likewise never panics or surfaces a spurious channel error.
func TestReadMalformedSkeleton(t *testing.T) {
	t.Parallel()
	for _, tt := range malformedSRTInputs {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()

			store, err := format.NewSkeletonStore()
			require.NoError(t, err)
			defer store.Close()

			reader := srt.NewReader()
			reader.SetSkeletonStore(store)

			require.NotPanics(t, func() {
				err := reader.Open(ctx, testutil.RawDocFromString(tt.input, model.LocaleEnglish))
				require.NoError(t, err)
			})
			defer reader.Close()

			require.NotPanics(t, func() {
				for result := range reader.Read(ctx) {
					assert.NoError(t, result.Error, "unexpected channel error for tolerated SRT input")
				}
			})
		})
	}
}

// TestOpenNilDocumentAndReader verifies Open rejects both a nil document and a
// document with a nil reader, returning an error rather than panicking later.
func TestOpenNilDocumentAndReader(t *testing.T) {
	t.Parallel()

	t.Run("nil document", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		reader := srt.NewReader()
		require.NotPanics(t, func() {
			err := reader.Open(ctx, nil)
			require.Error(t, err)
		})
	})

	t.Run("nil reader", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		reader := srt.NewReader()
		doc := &model.RawDocument{URI: "test://nil-reader", SourceLocale: model.LocaleEnglish}
		require.NotPanics(t, func() {
			err := reader.Open(ctx, doc)
			require.Error(t, err)
		})
	})
}
