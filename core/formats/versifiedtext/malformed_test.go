package versifiedtext_test

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/versifiedtext"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readAll drains the reader's result channel, returning whether any clean
// PartResult.Error surfaced and the number of translatable blocks emitted. It is
// wrapped in require.NotPanics by callers so a panic fails the test with a clear
// message instead of crashing the run.
//
// The versifiedtext reader is a best-effort line tokenizer: every non-blank
// line becomes a Block (a verse line when it matches versePattern, otherwise a
// plain line) and blank lines become stanza-break Data parts. There is no
// validating parser, so it has no parse-error path of its own — the only error
// it can surface is a bufio.Scanner failure (e.g. bufio.ErrTooLong on a line
// that exceeds the scanner's max token size). The contract this file exercises
// is therefore robustness: malformed/garbage/truncated input must never panic,
// hang, or race, and any error that does arise must surface cleanly on the
// channel rather than crash the goroutine.
func readAll(t *testing.T, input string) (foundError bool, blocks int) {
	t.Helper()
	ctx := t.Context()
	reader := versifiedtext.NewReader()

	require.NotPanics(t, func() {
		// Open only validates the document/reader; line scanning happens
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

// readAllSkeleton drains the reader's result channel with a skeleton store
// attached, exercising the readContentSkeleton path (which slices the verse
// prefix out of the matched line: prefix := content[:len(content)-len(text)]).
// That path is structurally distinct from readContentSimple, so malformed input
// must be robust there too. Returns whether any clean error surfaced and the
// number of translatable blocks emitted.
func readAllSkeleton(t *testing.T, input string) (foundError bool, blocks int) {
	t.Helper()
	ctx := t.Context()
	reader := versifiedtext.NewReader()

	require.NotPanics(t, func() {
		store, err := format.NewSkeletonStore()
		require.NoError(t, err)
		t.Cleanup(func() { _ = store.Close() })
		reader.SetSkeletonStore(store)

		err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
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

// malformedInputs are inputs that stress the reader's verse-reference matching
// and line handling: broken verse markers, missing/garbled numbering, binary
// bytes, invalid UTF-8, and stanza-break edge cases. Because the reader treats
// anything it cannot parse as a verse marker as a plain translatable line, none
// of these should error — but, crucially, none should panic, hang, or race.
var malformedInputs = []struct {
	name  string
	input string
}{
	{
		// A lone verse-marker backslash command with no number and no text.
		// versePattern requires \d+ after \v, so this fails to match and the
		// whole line is emitted as a plain Block.
		name:  "verse marker without number",
		input: `\v`,
	},
	{
		// \v followed by spaces but no digits: the marker is malformed, so the
		// line falls through to a plain Block rather than panicking.
		name:  "verse marker spaces no digits",
		input: "\\v   ",
	},
	{
		// A verse number far larger than any int — the reader keeps it as a
		// string in Properties["verse"], so no integer overflow is possible.
		name:  "absurdly large verse number",
		input: `\v999999999999999999999999999999 Genesis`,
	},
	{
		// Bare dot with no leading number: does not match the numeric marker
		// branch (which needs \d+), so it becomes a plain line.
		name:  "leading dot no number",
		input: ". orphaned punctuation",
	},
	{
		// A number immediately followed by text with no separator — the
		// numeric branch needs [.\s] after the digits, so "12abc" matches
		// nothing and stays a plain line.
		name:  "number glued to text",
		input: "12abc no separator",
	},
	{
		// Pure garbage / control bytes that are not whitespace, digits, or a
		// verse command. Treated as one plain translatable line.
		name:  "garbage bytes",
		input: "@@@ \x00\x01\x02 %%% ^^^",
	},
	{
		// Raw NUL bytes interleaved with text. bufio.Scanner splits on \n and
		// keeps NULs in the token; strings ops tolerate them without panic.
		name:  "embedded nul bytes",
		input: "line one\x00\x00\nline\x00two",
	},
	{
		// Invalid UTF-8 byte sequences (lone continuation bytes, truncated
		// multibyte starts). Go's string/regexp/bufio handle these as bytes
		// without panicking.
		name:  "invalid utf8",
		input: "\\v1 \xff\xfe\x80\xbf broken text\n\\v2 \xc3\x28 more",
	},
	{
		// A truncated multibyte sequence at the very end of input.
		name:  "truncated utf8 at eof",
		input: "\\v1 caf\xc3",
	},
	{
		// Many consecutive blank lines — each becomes its own stanza-break
		// Data part. Stresses the stanza-break path without emitting blocks.
		name:  "many blank lines",
		input: "\n\n\n\n\n",
	},
	{
		// Whitespace-only lines (spaces/tabs) are treated as blank by
		// strings.TrimSpace and become stanza breaks, not blocks.
		name:  "whitespace only lines",
		input: "   \n\t\t\n \t \n",
	},
	{
		// CRLF interleaved with bare CR and LF, plus a stray verse marker, to
		// stress line-ending splitting.
		name:  "mixed line endings",
		input: "\\v1 a\r\n\\v2 b\r\rc\n\rd",
	},
	{
		// A verse marker with a leading BOM before the backslash: the BOM is
		// part of the line content, so the marker does not match and the line
		// stays plain.
		name:  "bom before verse marker",
		input: "\uFEFF\\v1 In the beginning",
	},
}

// TestReadMalformedDoesNotPanic feeds malformed/garbage/truncated input through
// the simple reader path and asserts robustness: no panic, no hang, no race
// (run with -race). Whether a given input surfaces an error or parses leniently
// is an implementation detail we do not over-assert; the single contract here
// is that the reader never crashes.
func TestReadMalformedDoesNotPanic(t *testing.T) {
	t.Parallel()
	for _, tt := range malformedInputs {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.NotPanics(t, func() {
				readAll(t, tt.input)
			})
		})
	}
}

// TestReadMalformedSkeletonDoesNotPanic feeds the same malformed inputs through
// the skeleton-store reader path (readContentSkeleton), which performs the
// prefix-slice computation when a verse marker matches. This guards against an
// out-of-range slice or other crash on the skeleton path specifically.
func TestReadMalformedSkeletonDoesNotPanic(t *testing.T) {
	t.Parallel()
	for _, tt := range malformedInputs {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.NotPanics(t, func() {
				readAllSkeleton(t, tt.input)
			})
		})
	}
}

// TestReadEmptyInputNoBlocks verifies the degenerate empty-input case: no
// panic, no error, and no translatable blocks (only the layer start/end).
func TestReadEmptyInputNoBlocks(t *testing.T) {
	t.Parallel()
	foundError, blocks := readAll(t, "")
	assert.False(t, foundError, "empty input should not surface an error")
	assert.Equal(t, 0, blocks, "empty input should produce no blocks")
}

// TestReadLoneBlankLineNoBlocks verifies a single blank line yields a
// stanza-break Data part but no translatable Block, and never errors or panics.
func TestReadLoneBlankLineNoBlocks(t *testing.T) {
	t.Parallel()
	foundError, blocks := readAll(t, "\n")
	assert.False(t, foundError, "a lone blank line should not surface an error")
	assert.Equal(t, 0, blocks, "a lone blank line is a stanza break, not a block")
}

// TestReadOverlongLineSurfacesScannerError feeds a single line far larger than
// bufio.Scanner's default max token size (64 KiB). The simple reader path uses
// bufio.Scanner, so scanner.Err() returns bufio.ErrTooLong, which the reader
// must surface cleanly on the channel as a PartResult.Error rather than panic
// or hang. This is the one genuine error path the reader has.
func TestReadOverlongLineSurfacesScannerError(t *testing.T) {
	t.Parallel()
	// 1 MiB single line with no newline — well past bufio's 64 KiB token cap.
	input := "\\v1 " + strings.Repeat("a", 1<<20)
	foundError, _ := readAll(t, input)
	assert.True(t, foundError, "an over-long line should surface a clean scanner error")
}

// TestReadOverlongLineSkeletonDoesNotPanic feeds the same over-long line through
// the skeleton path (which uses bufio.Reader.ReadString rather than
// bufio.Scanner, so it has no token-size cap). The contract here is only
// robustness: it must not panic, hang, or race on a very large single line.
func TestReadOverlongLineSkeletonDoesNotPanic(t *testing.T) {
	t.Parallel()
	input := "\\v1 " + strings.Repeat("a", 1<<20)
	require.NotPanics(t, func() {
		readAllSkeleton(t, input)
	})
}
