package vtt_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/vtt"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// drainVTT runs Open+Read inside require.NotPanics and drains the result
// channel, returning whether any clean PartResult.Error surfaced and the number
// of translatable blocks emitted. A panic in the reader goroutine fails the
// test with a clear message instead of crashing the run, and draining to
// completion proves the reader does not hang.
//
// The VTT reader is a lenient subtitle parser: its line-oriented scanner
// best-effort parses cues rather than rejecting malformed structure (see
// reader.go's parseCues / readContentSimple — neither emits an error). The
// contract these tests enforce is therefore robustness: no panic, no hang, and
// either a clean error or a sane best-effort parse.
func drainVTT(t *testing.T, input string) (foundError bool, blocks int) {
	t.Helper()
	ctx := t.Context()
	reader := vtt.NewReader()

	require.NotPanics(t, func() {
		// Open only validates the document/reader; any parse handling
		// happens during Read.
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
// non-UTF-8 input to the VTT reader and asserts it stays robust: no panic, no
// hang (the channel drains to completion), and no crash. Because the reader is
// a lenient best-effort parser, whether a given input surfaces a clean error or
// parses leniently is an implementation detail we do not over-assert here — the
// single contract is robustness.
//
// Run with -race to surface any data race in the reader goroutine that drives
// the result channel.
func TestReadMalformedDoesNotPanic(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{
			// No "WEBVTT" header at all: the first line is consumed as the
			// header regardless, then the remainder is parsed as cues.
			name:  "missing webvtt header",
			input: "00:00:01.000 --> 00:00:04.000\nHello world\n",
		},
		{
			// Header present but cue timecode is malformed (no "-->"): the
			// line is treated as a cue identifier rather than a timecode.
			name:  "malformed cue timestamp",
			input: "WEBVTT\n\n00:00:01,000 00:00:04,000\nHello world\n",
		},
		{
			// Timecode arrow present but the numbers are garbage.
			name:  "garbage timestamp numbers",
			input: "WEBVTT\n\nNOTATIME --> ALSONOT\nSome text\n",
		},
		{
			// Cue identifier with no following timecode line, then EOF:
			// the timecode stays empty and a block is still emitted.
			name:  "truncated cue after identifier",
			input: "WEBVTT\n\ncue-1\n",
		},
		{
			// Cue with a timecode but no text, truncated at EOF.
			name:  "truncated cue after timecode",
			input: "WEBVTT\n\n00:00:01.000 --> 00:00:04.000\n",
		},
		{
			// Truncated mid-timecode line with no newline at EOF.
			name:  "truncated mid timecode no newline",
			input: "WEBVTT\n\n00:00:01.000 --> 00:00",
		},
		{
			// Pure garbage / control bytes with no VTT structure.
			name:  "garbage bytes",
			input: "@@@ %%% ^^^\x00\x01\x02\x03\x07\x7f",
		},
		{
			// Invalid UTF-8: a lone continuation byte and a truncated
			// multi-byte sequence. Go's string handling tolerates these as
			// replacement runes; the reader must not panic.
			name:  "invalid utf8 bytes",
			input: "WEBVTT\n\n00:00:01.000 --> 00:00:04.000\n\xff\xfe\xc3\x28\n",
		},
		{
			// Binary-looking payload mislabelled as VTT.
			name:  "binary garbage",
			input: "WEBVTT\n\n\x00\x00\x00\x00\xde\xad\xbe\xef\n\x10\x11\x12\n",
		},
		{
			// Empty input: nothing to parse.
			name:  "empty input",
			input: "",
		},
		{
			// Only a header, nothing else.
			name:  "header only",
			input: "WEBVTT",
		},
		{
			// Only whitespace / blank lines.
			name:  "blank lines only",
			input: "\n\n\n\n",
		},
		{
			// CRLF line endings throughout (Windows-authored file).
			name:  "crlf line endings",
			input: "WEBVTT\r\n\r\n00:00:01.000 --> 00:00:04.000\r\nHello world\r\n",
		},
		{
			// Mixed CRLF and LF line endings in one file.
			name:  "mixed crlf and lf",
			input: "WEBVTT\r\n\n00:00:01.000 --> 00:00:04.000\nLine one\r\nLine two\n",
		},
		{
			// A lone CR with no LF: splitRawLines keys on '\n', so the whole
			// thing is one line. Must not panic.
			name:  "lone carriage returns",
			input: "WEBVTT\r00:00:01.000 --> 00:00:04.000\rHello\r",
		},
		{
			// Cue text immediately follows the timecode with no blank line
			// separators anywhere.
			name:  "no blank line separators",
			input: "WEBVTT\n00:00:01.000 --> 00:00:04.000\nHello\n00:00:05.000 --> 00:00:08.000\nWorld\n",
		},
		{
			// Lone byte-order mark with no content.
			name:  "lone bom",
			input: "\uFEFF",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// The whole contract is that this does not panic or hang.
			require.NotPanics(t, func() {
				drainVTT(t, tt.input)
			})
		})
	}
}

// TestReadMalformedSkeletonDoesNotPanic exercises the same malformed inputs
// through the skeleton-tracking code path (readContentSkeleton), which is a
// distinct parser from the simple path and additionally reads the full input
// via io.ReadAll. It must be equally robust.
//
// Run with -race to surface any data race in the reader goroutine.
func TestReadMalformedSkeletonDoesNotPanic(t *testing.T) {
	t.Parallel()
	inputs := []struct {
		name  string
		input string
	}{
		{"missing header", "00:00:01.000 --> 00:00:04.000\nHi\n"},
		{"truncated after identifier", "WEBVTT\n\ncue-1\n"},
		{"truncated after timecode", "WEBVTT\n\n00:00:01.000 --> 00:00:04.000\n"},
		{"truncated mid timecode no newline", "WEBVTT\n\n00:00:01.000 --> 00:00"},
		{"garbage bytes", "@@@ %%% ^^^\x00\x01\x02\x7f"},
		{"invalid utf8", "WEBVTT\n\n00:00:01.000 --> 00:00:04.000\n\xff\xfe\xc3\x28\n"},
		{"binary garbage", "WEBVTT\n\n\x00\x00\xde\xad\n"},
		{"empty input", ""},
		{"header only", "WEBVTT"},
		{"blank lines only", "\n\n\n\n"},
		{"crlf", "WEBVTT\r\n\r\n00:00:01.000 --> 00:00:04.000\r\nHello\r\n"},
		{"mixed crlf lf", "WEBVTT\r\n\n00:00:01.000 --> 00:00:04.000\nA\r\nB\n"},
		{"lone cr", "WEBVTT\r00:00:01.000 --> 00:00:04.000\rHello\r"},
		{"lone bom", "\uFEFF"},
	}

	for _, in := range inputs {
		t.Run(in.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			reader := vtt.NewReader()
			store, err := format.NewSkeletonStore()
			require.NoError(t, err)
			defer store.Close()
			reader.SetSkeletonStore(store)

			require.NotPanics(t, func() {
				err := reader.Open(ctx, testutil.RawDocFromString(in.input, model.LocaleEnglish))
				require.NoError(t, err)
			})
			defer reader.Close()

			require.NotPanics(t, func() {
				for range reader.Read(ctx) {
					// Drain to completion to prove no hang.
				}
			})
		})
	}
}

// TestReadNilReader verifies Open rejects a document whose Reader is nil
// without panicking, and rejects a nil document outright.
func TestReadNilReader(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	require.NotPanics(t, func() {
		reader := vtt.NewReader()
		err := reader.Open(ctx, &model.RawDocument{SourceLocale: model.LocaleEnglish})
		assert.Error(t, err, "nil Reader should be rejected")
	})

	require.NotPanics(t, func() {
		reader := vtt.NewReader()
		err := reader.Open(ctx, nil)
		assert.Error(t, err, "nil document should be rejected")
	})
}
