package tex_test

import (
	"errors"
	"testing"

	"github.com/neokapi/neokapi/core/formats/tex"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadMalformedInput feeds truncated, unterminated, and garbage TeX at the
// reader and asserts it never panics. TeX has no strict grammar gate — Okapi's
// TEXFilter and this reader are tolerant string scanners that extract whatever
// translatable text they can find — so malformed *content* does not produce a
// PartResult.Error; the contract for these inputs is "drain cleanly, no panic".
// The reader-error path (a genuine I/O failure surfacing on the channel) is
// covered separately by TestReadReaderError.
func TestReadMalformedInput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{
			// \begin{document} with no matching \end{document}: the body runs
			// to EOF and must not trip the scanner.
			name:  "unclosed begin document",
			input: "\\begin{document}\nHello world.",
		},
		{
			// An environment opened but never closed.
			name:  "unclosed environment",
			input: "\\begin{document}\n\\begin{abstract}\nSome abstract text.\n\\end{document}",
		},
		{
			// Unterminated inline math: a lone $ with no closing $ to EOF.
			name:  "unterminated inline math",
			input: "\\begin{document}\nThe formula $E = mc^2 keeps going forever",
		},
		{
			// Unterminated display math: $$ with no closing $$.
			name:  "unterminated display math",
			input: "\\begin{document}\n$$\\int_0^1 f(x)\\,dx",
		},
		{
			// A command with an unterminated brace argument.
			name:  "unterminated brace argument",
			input: "\\begin{document}\n\\section{Introduction\nbody text",
		},
		{
			// A lone trailing backslash at EOF (no command name follows).
			name:  "lone backslash at eof",
			input: "\\begin{document}\nText then a stray backslash \\",
		},
		{
			// A backslash immediately at EOF with nothing else.
			name:  "only backslash",
			input: "\\",
		},
		{
			// Raw binary / control bytes mixed with NULs — must not corrupt
			// the scanner or panic on invalid UTF-8.
			name:  "raw binary bytes",
			input: "\\begin{document}\n\x00\x01\x02\xff\xfe text \x00 more\n\\end{document}",
		},
		{
			// Garbage that looks like commands but is structurally nonsense.
			name:  "garbage commands",
			input: "}}}{{{ \\}{ \\\\\\ $$$ %%% &&&^^^ ~~~",
		},
		{
			// Deeply nested unbalanced braces — exercises the brace matcher
			// without a closing partner.
			name:  "deeply unbalanced braces",
			input: "\\begin{document}\n\\textbf{\\emph{\\texttt{never closed",
		},
		{
			// Empty input.
			name:  "empty",
			input: "",
		},
		{
			// Whitespace only.
			name:  "whitespace only",
			input: "   \n\t  \n",
		},
		{
			// A comment line that never ends with a newline (runs to EOF).
			name:  "unterminated comment",
			input: "\\begin{document}\nText % a comment that never ends",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			reader := tex.NewReader()
			require.NotPanics(t, func() {
				err := reader.Open(ctx, testutil.RawDocFromString(tt.input, model.LocaleEnglish))
				require.NoError(t, err)
			})
			defer reader.Close()

			// The tolerant scanner must drain the entire channel without
			// panicking. Any PartResult.Error is acceptable but not required;
			// what matters is that the reader neither panics nor wedges.
			require.NotPanics(t, func() {
				for result := range reader.Read(ctx) {
					_ = result
				}
			})
		})
	}
}

// errReader is an io.Reader that always fails. It exercises the reader-error
// path so we can assert the failure surfaces on the channel as a clean
// PartResult.Error rather than panicking or being swallowed.
type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, errors.New("simulated read failure")
}

// TestReadReaderError verifies that a failure reading the document body surfaces
// as a PartResult.Error on the channel (via Reader.readContent's io.ReadAll
// error path) rather than panicking or being silently dropped.
func TestReadReaderError(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := tex.NewReader()
	doc := testutil.RawDocFromReader(errReader{}, "test://broken.tex", model.LocaleEnglish)

	require.NotPanics(t, func() {
		err := reader.Open(ctx, doc)
		require.NoError(t, err)
	})
	defer reader.Close()

	var foundError bool
	require.NotPanics(t, func() {
		for result := range reader.Read(ctx) {
			if result.Error != nil {
				foundError = true
			}
		}
	})
	assert.True(t, foundError, "expected a clean error when the document reader fails")
}

// TestReadNilDocument verifies Open rejects a nil document without panicking.
func TestReadNilDocument(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := tex.NewReader()
	require.NotPanics(t, func() {
		err := reader.Open(ctx, nil)
		require.Error(t, err)
	})
}

// TestReadNilReader verifies Open rejects a document whose Reader field is nil
// without panicking. RawDocument with a nil Reader is the other half of the
// guard in Reader.Open.
func TestReadNilReader(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := tex.NewReader()
	doc := &model.RawDocument{
		URI:          "test://no-reader.tex",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       nil,
	}
	require.NotPanics(t, func() {
		err := reader.Open(ctx, doc)
		require.Error(t, err)
	})
}
