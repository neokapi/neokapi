package doxygen_test

import (
	"errors"
	"io"
	"testing"

	doxygen "github.com/neokapi/neokapi/core/formats/doxygen"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadMalformedInput feeds truncated, unterminated, and garbage sources
// through the Doxygen reader and asserts it never panics while draining the
// result channel. The Doxygen format is line-oriented and recovers from
// unterminated comments by consuming to EOF — there is no "syntax error" to
// surface for these inputs — so the contract under test is robustness (no
// panic, channel closes cleanly) rather than a forced parse error.
func TestReadMalformedInput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{
			// Block comment opened with /** but never closed (no */).
			// parseBlockComment consumes the remaining lines to EOF.
			name:  "truncated javadoc block comment",
			input: "/**\n * Summary line\n * Detail line without a closing delimiter",
		},
		{
			// Qt-style /*! opened, then the file ends mid-comment.
			name:  "interrupted qt block comment at EOF",
			input: "int x;\n/*! Brief description\n * more text",
		},
		{
			// Python triple-quoted docstring opened with """ but never closed.
			// parseDocstring scans to EOF without finding the closing """.
			name:  "unterminated python docstring",
			input: "def f():\n    \"\"\"Docstring body\n    second line never closed",
		},
		{
			// Single line that only opens a docstring with no body or close.
			name:  "bare opening docstring quotes",
			input: "\"\"\"",
		},
		{
			// Opening block delimiter with nothing after it.
			name:  "bare opening block delimiter",
			input: "/**",
		},
		{
			// Trailing-comment markers with no preceding code and no body.
			name:  "dangling trailing markers",
			input: "///<\n/*!<",
		},
		{
			// Random ASCII garbage that resembles neither code nor comments.
			name:  "ascii garbage",
			input: "definitely not doxygen :: {[<>]} ***/// /*!*/ \"\"\"",
		},
		{
			// Binary / control bytes embedded in the source, including a NUL
			// and an invalid UTF-8 lead byte, to exercise the byte-splitter
			// and marker scans on non-text input.
			name:  "binary garbage bytes",
			input: "\x00\x01\xff/**\xfe\x00 broken\x07\n\xc3\x28 */",
		},
		{
			// Empty source: no lines, no comments.
			name:  "empty input",
			input: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			reader := doxygen.NewReader()
			require.NotPanics(t, func() {
				err := reader.Open(ctx, testutil.RawDocFromString(tt.input, model.LocaleEnglish))
				require.NoError(t, err)
			})
			defer reader.Close()

			// Draining the channel must not panic and must terminate (the
			// channel is closed when readContent returns). Any surfaced
			// error is tolerated here — what matters is that the reader does
			// not crash on these inputs.
			require.NotPanics(t, func() {
				for result := range reader.Read(ctx) {
					_ = result
				}
			})
		})
	}
}

// errReader is an io.ReadCloser whose Read always fails. It exercises the
// reader's io.ReadAll error path, which is the only place the Doxygen reader
// surfaces a PartResult.Error.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("simulated read failure") }
func (errReader) Close() error             { return nil }

// TestReadIOErrorSurfacesOnChannel verifies that an underlying read failure is
// reported as a clean PartResult.Error on the channel rather than panicking or
// being silently swallowed.
func TestReadIOErrorSurfacesOnChannel(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := doxygen.NewReader()
	doc := &model.RawDocument{
		URI:          "test://broken",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       errReader{},
	}
	require.NoError(t, reader.Open(ctx, doc))
	defer reader.Close()

	var foundError bool
	require.NotPanics(t, func() {
		for result := range reader.Read(ctx) {
			if result.Error != nil {
				foundError = true
			}
		}
	})
	assert.True(t, foundError, "expected the read failure to surface on the channel")
}

// TestOpenRejectsNilDocument verifies Open returns an error (not a panic) for a
// nil document.
func TestOpenRejectsNilDocument(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := doxygen.NewReader()
	var err error
	require.NotPanics(t, func() { err = reader.Open(ctx, nil) })
	require.Error(t, err)
}

// TestOpenRejectsNilReader verifies Open returns an error (not a panic) for a
// document whose Reader is nil.
func TestOpenRejectsNilReader(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := doxygen.NewReader()
	doc := &model.RawDocument{
		URI:          "test://no-reader",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       nil,
	}
	var err error
	require.NotPanics(t, func() { err = reader.Open(ctx, doc) })
	require.Error(t, err)
}

// TestReopenAfterClose verifies that reusing a reader after Close — opening a
// fresh document and draining it — does not panic and yields content.
func TestReopenAfterClose(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := doxygen.NewReader()

	// First open + drain a closed/exhausted reader, then Close.
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString("/** first */", model.LocaleEnglish)))
	require.NotPanics(t, func() {
		for range reader.Read(ctx) {
		}
	})
	require.NoError(t, reader.Close())

	// Re-open with a new document; this must not panic and must produce a
	// translatable block from the fresh source.
	var blocks []*model.Block
	require.NotPanics(t, func() {
		require.NoError(t, reader.Open(ctx, testutil.RawDocFromString("/** second comment */", model.LocaleEnglish)))
		blocks = testutil.CollectBlocks(t, reader.Read(ctx))
	})
	assert.NotEmpty(t, blocks, "expected the reopened reader to extract content")
}

var _ io.ReadCloser = errReader{}
