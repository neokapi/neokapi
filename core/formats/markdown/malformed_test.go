package markdown_test

import (
	"errors"
	"io"
	"testing"

	"github.com/neokapi/neokapi/core/formats/markdown"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// malformedInputs collects broken, truncated, garbage, binary, and otherwise
// degenerate documents. goldmark is intentionally lenient — almost any byte
// sequence is "valid" markdown — so the contract here is robustness: the reader
// must drain the full Part stream without panicking and without surfacing a
// spurious error, regardless of how malformed the input is.
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
		input: "   \n\t  \n",
	},
	{
		name:  "garbage punctuation",
		input: "definitely not markdown :: {[<>]}@#$%^&*",
	},
	{
		name:  "binary bytes",
		input: "\x00\x01\x02\xff\xfe\x00garbage\x00\x1b[2J",
	},
	{
		name:  "invalid utf8",
		input: "# Heading \xc3\x28 \xa0\xa1 invalid continuation",
	},
	{
		name:  "truncated fenced code block",
		input: "# Title\n\n```go\nfunc main() {\n",
	},
	{
		name:  "truncated heading no newline",
		input: "# Unterminated heading without trailing newline",
	},
	{
		name:  "unbalanced inline emphasis",
		input: "Some **bold without close and _italic [link](http://x",
	},
	{
		name:  "broken front matter no close",
		input: "---\ntitle: My Doc\nauthor: Jane\n\n# Body never reaches a closing delimiter",
	},
	{
		name:  "front matter only opening delimiter",
		input: "---\n",
	},
	{
		name:  "front matter crlf no close",
		input: "---\r\ntitle: Windows\r\n# body",
	},
	{
		name:  "front matter colon-less lines",
		input: "---\njust some text with no colon\nmore text\n---\n# Body\n",
	},
	{
		name:  "front matter empty values",
		input: "---\nkey1:\nkey2: \n: orphan value\n---\nBody\n",
	},
	{
		name:  "broken html block unclosed tag",
		input: "<div class=\"box\">\n<span>text without closing tags\n\nA paragraph.",
	},
	{
		name:  "broken html comment",
		input: "<!-- unterminated comment\n\nMore text.",
	},
	{
		name:  "malformed table",
		input: "| a | b\n|---\n| 1 | 2 | 3 | extra |\nbroken",
	},
	{
		name:  "dangling link reference",
		input: "See [the docs][missing-ref] for details.\n\n[orphan]: ",
	},
	{
		name:  "deeply nested lists no content",
		input: "- \n  - \n    - \n      - \n",
	},
	{
		name:  "only delimiters",
		input: "######\n***\n---\n```\n```\n",
	},
	{
		name:  "null bytes interleaved with markdown",
		input: "# He\x00ading\n\n- item\x00 one\n- it\x00em two\n",
	},
}

// TestReadMalformedDoesNotPanic feeds every malformed document through the
// reader and asserts the full Part stream drains without a panic and without a
// spurious error. Run with -race to also catch data races in the streaming
// goroutine.
func TestReadMalformedDoesNotPanic(t *testing.T) {
	t.Parallel()
	for _, tt := range malformedInputs {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			reader := markdown.NewReader()
			require.NotPanics(t, func() {
				err := reader.Open(ctx, testutil.RawDocFromString(tt.input, model.LocaleEnglish))
				require.NoError(t, err)
			})
			defer reader.Close()

			require.NotPanics(t, func() {
				for result := range reader.Read(ctx) {
					// goldmark accepts arbitrary bytes, so no parse error is
					// expected; if one ever surfaces we still want to see it
					// rather than silently swallow it.
					assert.NoError(t, result.Error, "unexpected error draining malformed input")
				}
			})
		})
	}
}

// TestReadMalformedFrontMatterTranslated re-runs the malformed corpus with
// front-matter translation enabled. This exercises the value-emitting code path
// (handleFrontMatter → emitFrontMatterBlocks) that splits YAML lines on colons
// and rewrites skeleton spans — the most likely place a broken front-matter
// block could panic on an unexpected delimiter layout.
func TestReadMalformedFrontMatterTranslated(t *testing.T) {
	t.Parallel()
	for _, tt := range malformedInputs {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			reader := markdown.NewReader()
			reader.MarkdownConfig().TranslateFrontMatter = true
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

// errReader is an io.ReadCloser that returns an error after emitting an optional
// prefix, simulating an I/O failure mid-document.
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
// error-surfacing path for the markdown reader, since goldmark itself does not
// reject malformed content.
func TestReadReaderError(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	wantErr := errors.New("simulated disk failure")
	doc := &model.RawDocument{
		URI:          "test://broken",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       &errReader{prefix: []byte("# Partial heading\n\nSome "), err: wantErr},
	}

	reader := markdown.NewReader()
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

// Note: the nil-document case (Open(ctx, nil) must return an error) is covered
// by TestReadNilDocument in reader_test.go.

// TestReadNilReader verifies Open rejects a document whose Reader is nil
// without panicking.
func TestReadNilReader(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := markdown.NewReader()
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
