package rtf_test

import (
	"errors"
	"io"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/rtf"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// malformedInputs collects broken, truncated, garbage, binary, and otherwise
// degenerate RTF documents. The RTF tokenizer is intentionally permissive — it
// walks untrusted bytes best-effort and never rejects "invalid" markup — so the
// contract here is robustness: tokenize(), parseControlWord(), and
// skipUnicodeFallback() must drain the full Part stream without panicking and
// without surfacing a spurious error, no matter how truncated or garbled the
// input is. Each case targets a boundary in those byte-walking routines.
var malformedInputs = []struct {
	name  string
	input string
}{
	{
		name:  "empty",
		input: "",
	},
	{
		name:  "only whitespace",
		input: "   \r\n\t  \n",
	},
	{
		// A bare backslash at EOF: parseControlWord hits io.EOF on its first
		// ReadByte and must fall back to literal "\" without panicking.
		name:  "lone backslash at eof",
		input: `\`,
	},
	{
		// Open brace, control word, then EOF — no matching close brace. The
		// group never ends; emitParts must still drain and flush cleanly.
		name:  "truncated header no close brace",
		input: `{\rtf1\ansi\deff0`,
	},
	{
		// Truncated mid-paragraph: text with no closing \par or '}'.
		name:  "truncated mid paragraph",
		input: `{\rtf1\ansi\deff0\pard Hello world without a close`,
	},
	{
		// More '}' than '{': emitParts must guard depth underflow (depth > 0).
		name:  "unbalanced extra close braces",
		input: `{\rtf1 text}}}}`,
	},
	{
		// More '{' than '}': groups stay open at EOF; the group stack must
		// unwind without index-out-of-range.
		name:  "unbalanced extra open braces",
		input: `{{{{\rtf1 text`,
	},
	{
		// Control word with no name and a digit param at EOF.
		name:  "control word param at eof",
		input: `{\rtf1\fs`,
	},
	{
		// A control word that runs straight into EOF after its keyword, so
		// readControlWordFrom's numeric-param loop hits io.EOF.
		name:  "bare control word at eof",
		input: `{\rtf1\pard`,
	},
	{
		// \uN with no ANSI fallback character following it — skipUnicodeFallback
		// must hit io.EOF on its first ReadByte and return, not panic.
		name:  "unicode no fallback at eof",
		input: `{\rtf1\pard Caf\u233`,
	},
	{
		// \uN followed by a truncated \'HH fallback (only one hex digit before
		// EOF) — skipUnicodeFallback's hex branch must survive a short read.
		name:  "unicode truncated hex fallback",
		input: `{\rtf1\pard X\u233\'e`,
	},
	{
		// \uN whose fallback is a control word running into EOF — the keyword
		// branch of skipUnicodeFallback must terminate on io.EOF.
		name:  "unicode control word fallback at eof",
		input: `{\rtf1\pard X\u233\bin`,
	},
	{
		// \uN whose fallback is an unbalanced "{" group at EOF — the balanced
		// group skip must terminate on io.EOF rather than loop forever.
		name:  "unicode group fallback unbalanced",
		input: `{\rtf1\pard X\u233{never closes`,
	},
	{
		// A hex escape \'HH truncated to a single digit before EOF.
		name:  "hex escape truncated at eof",
		input: `{\rtf1\pard Caf\'e`,
	},
	{
		// A hex escape with non-hex digits — ParseUint fails and the bytes are
		// surfaced as literal text, no panic.
		name:  "hex escape non hex digits",
		input: `{\rtf1\pard \'zz tail}`,
	},
	{
		// \u with no number and no following char (control word "u" at EOF).
		name:  "u control word no digits at eof",
		input: `{\rtf1\pard \u`,
	},
	{
		// \u followed by a sign then EOF, exercising the negative-number path.
		name:  "u negative number at eof",
		input: `{\rtf1\pard \u-`,
	},
	{
		// Raw garbage punctuation and brackets that are not valid RTF at all.
		name:  "garbage punctuation",
		input: "definitely not rtf :: {[<>]}@#$%^&*\\",
	},
	{
		// NUL and high bytes interleaved with control words — the byte walker
		// must treat them as opaque text without choking on invalid UTF-8.
		name:  "binary bytes",
		input: "{\\rtf1\x00\x01\xff\xfe\\pard te\x00xt\xc3\x28\\par}",
	},
	{
		// A backslash immediately before EOF inside an escaped-group skip — the
		// '\' branch of skipUnicodeFallback's group walker must survive.
		name:  "unicode group fallback backslash at eof",
		input: `{\rtf1\pard X\u233{a\`,
	},
	{
		// Nested skip-destination group (fonttbl) truncated mid-way.
		name:  "truncated font table",
		input: `{\rtf1{\fonttbl{\f0 Times New`,
	},
	{
		// A control word with an enormous numeric param — Atoi may overflow;
		// the reader must not panic on the resulting value.
		name:  "huge control word param",
		input: `{\rtf1\pard \fs999999999999999999999999 text\par}`,
	},
	{
		// \uN with an enormous number — rune conversion must stay in range.
		name:  "huge unicode value",
		input: `{\rtf1\pard 香99999999999999? text\par}`,
	},
}

// TestReadMalformedDoesNotPanic feeds every malformed document through the
// reader and asserts the full Part stream drains without a panic and without a
// spurious error. The RTF tokenizer accepts arbitrary bytes, so no parse error
// is expected — but if one ever surfaces we want to see it rather than silently
// swallow it. Run with -race to also catch data races in the streaming
// goroutine that drives tokenize()/emitParts().
func TestReadMalformedDoesNotPanic(t *testing.T) {
	t.Parallel()
	for _, tt := range malformedInputs {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			reader := rtf.NewReader()
			require.NotPanics(t, func() {
				err := reader.Open(ctx, testutil.RawDocFromString(tt.input, model.LocaleEnglish))
				require.NoError(t, err)
			})
			defer reader.Close()

			require.NotPanics(t, func() {
				for result := range reader.Read(ctx) {
					assert.NoError(t, result.Error, "unexpected error draining malformed input")
				}
			})
		})
	}
}

// TestReadMalformedWithSkeleton re-runs the malformed corpus with a skeleton
// store attached. The skeleton path slices the raw input by the byte offsets
// recorded for each text token (readContent's textRef walk), so truncated or
// garbage input — where token byte ranges can butt up against EOF — is the most
// likely place an out-of-range slice could panic.
func TestReadMalformedWithSkeleton(t *testing.T) {
	t.Parallel()
	for _, tt := range malformedInputs {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			reader := rtf.NewReader()
			store, err := format.NewSkeletonStore()
			require.NoError(t, err)
			defer store.Close()
			reader.SetSkeletonStore(store)
			require.NotPanics(t, func() {
				err := reader.Open(ctx, testutil.RawDocFromString(tt.input, model.LocaleEnglish))
				require.NoError(t, err)
			})
			defer reader.Close()

			require.NotPanics(t, func() {
				for result := range reader.Read(ctx) {
					assert.NoError(t, result.Error, "unexpected error draining malformed input")
				}
			})
		})
	}
}

// errReader is an io.ReadCloser that emits an optional prefix then fails,
// simulating an I/O failure partway through the document.
type errReader struct {
	prefix []byte
	off    int
	err    error
}

func (e *errReader) Read(p []byte) (int, error) {
	if e.off < len(e.prefix) {
		n := copy(p, e.prefix[e.off:])
		e.off += n
		return n, nil
	}
	return 0, e.err
}

func (e *errReader) Close() error { return nil }

// TestReadReaderError verifies that an I/O failure while reading the document
// body surfaces as a clean error on the result channel (PartResult.Error)
// rather than panicking or being silently dropped. This is the genuine
// error-surfacing path for the RTF reader: readContent reads the whole document
// with io.ReadAll up front, so a failing reader is the one input that produces
// a PartResult.Error (the tokenizer itself never rejects content).
func TestReadReaderError(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	wantErr := errors.New("simulated disk failure")
	doc := &model.RawDocument{
		URI:          "test://broken",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       &errReader{prefix: []byte(`{\rtf1\ansi\pard Partial `), err: wantErr},
	}

	reader := rtf.NewReader()
	require.NotPanics(t, func() {
		require.NoError(t, reader.Open(ctx, doc))
	})
	defer reader.Close()

	var foundError bool
	require.NotPanics(t, func() {
		for result := range reader.Read(ctx) {
			if result.Error != nil {
				foundError = true
				assert.ErrorIs(t, result.Error, wantErr,
					"reader error should wrap the underlying I/O failure")
			}
		}
	})
	assert.True(t, foundError, "expected a clean error when the underlying reader fails")
}

// TestReadNilReader verifies Open rejects a document whose Reader is nil without
// panicking. (The nil-document case is covered by TestReadNilDocument in
// reader_test.go.)
func TestReadNilReader(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := rtf.NewReader()
	doc := &model.RawDocument{
		URI:          "test://input",
		SourceLocale: model.LocaleEnglish,
		Reader:       nil,
	}
	var err error
	require.NotPanics(t, func() {
		err = reader.Open(ctx, doc)
	})
	require.Error(t, err)
}

// Ensure errReader satisfies io.ReadCloser at compile time.
var _ io.ReadCloser = (*errReader)(nil)
