package paraplaintext_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/formats/paraplaintext"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadMalformedInput feeds truncated, garbage, binary, and control-character
// payloads through the reader. Plain text imposes no syntax, so the reader must
// accept any byte sequence without panicking: it neither rejects the input nor
// loses parts, it simply treats the bytes as paragraph content.
func TestReadMalformedInput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "raw binary bytes",
			input: "\x00\x01\x02\x03\xff\xfe\xfd",
		},
		{
			name:  "embedded nul bytes",
			input: "First para\x00with nul\n\nSecond\x00para",
		},
		{
			name:  "invalid utf-8 sequences",
			input: "\xc3\x28\xa0\xa1 broken utf8 \xf0\x28\x8c\x28",
		},
		{
			name:  "control characters",
			input: "text\x07\x08\x0b\x0c with control chars",
		},
		{
			name:  "lone carriage returns",
			input: "line one\rline two\r\rline three",
		},
		{
			name:  "truncated multibyte at end",
			input: "valid prefix \xe2\x82", // start of € (U+20AC) with the final byte chopped
		},
		{
			name:  "only nul bytes",
			input: "\x00\x00\x00\x00",
		},
		{
			name:  "mixed newlines and garbage",
			input: "\x00\n\n\xff\xff\n\n\x01\x02",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			reader := paraplaintext.NewReader()

			require.NotPanics(t, func() {
				err := reader.Open(ctx, testutil.RawDocFromString(tt.input, model.LocaleEnglish))
				require.NoError(t, err)
			})
			defer reader.Close()

			// Plain text never produces a parse error; the contract is only that
			// Read drains cleanly without panicking and surfaces no Error on the
			// channel for byte payloads it cannot syntactically reject.
			require.NotPanics(t, func() {
				for result := range reader.Read(ctx) {
					require.NoError(t, result.Error)
				}
			})
		})
	}
}

// TestReadEmptyInput verifies that an empty document drains cleanly with no
// blocks and no error, exercising the early-return path in readContent.
func TestReadEmptyInput(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := paraplaintext.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString("", model.LocaleEnglish)))
	defer reader.Close()

	var blocks []*model.Block
	require.NotPanics(t, func() {
		blocks = testutil.CollectBlocks(t, reader.Read(ctx))
	})
	assert.Empty(t, blocks)
}

// failingReader returns an error after yielding some bytes, simulating a
// truncated or otherwise broken stream (e.g. a closed socket mid-read).
type failingReader struct {
	data []byte
	pos  int
	err  error
}

func (f *failingReader) Read(p []byte) (int, error) {
	if f.pos >= len(f.data) {
		return 0, f.err
	}
	n := copy(p, f.data[f.pos:])
	f.pos += n
	return n, nil
}

func (f *failingReader) Close() error { return nil }

// TestReadStreamError verifies that an underlying read failure surfaces as a
// clean PartResult.Error on the channel rather than being silently swallowed or
// causing a panic.
func TestReadStreamError(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := paraplaintext.NewReader()

	doc := &model.RawDocument{
		URI:          "test://broken",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       &failingReader{data: []byte("partial paragraph"), err: errors.New("boom: stream truncated")},
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
	assert.True(t, foundError, "expected a clean error when the underlying reader fails")
}

// TestOpenNilReader verifies Open rejects a document whose Reader is nil without
// panicking. (A nil document is covered by TestReadNilDocument in reader_test.go.)
func TestOpenNilReader(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := paraplaintext.NewReader()

	doc := &model.RawDocument{URI: "test://no-reader", SourceLocale: model.LocaleEnglish}
	var err error
	require.NotPanics(t, func() {
		err = reader.Open(ctx, doc)
	})
	require.Error(t, err)
}

// TestReadHugeSingleParagraph feeds a large contiguous payload with no paragraph
// breaks to confirm the reader handles big single blocks without panicking.
func TestReadHugeSingleParagraph(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := paraplaintext.NewReader()

	input := strings.Repeat("word ", 200000) // ~1MB, no blank-line separators
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	defer reader.Close()

	var blocks []*model.Block
	require.NotPanics(t, func() {
		blocks = testutil.CollectBlocks(t, reader.Read(ctx))
	})
	require.Len(t, blocks, 1)
	assert.Equal(t, strings.TrimRight(input, " "), strings.TrimRight(blocks[0].SourceText(), " "))
}

// failingReaderImmediate returns an error before any bytes are produced, the
// degenerate truncation case.
type failingReaderImmediate struct{ err error }

func (f *failingReaderImmediate) Read([]byte) (int, error) { return 0, f.err }
func (f *failingReaderImmediate) Close() error             { return nil }

// TestReadImmediateStreamError verifies a reader that errors on the very first
// read still produces a clean channel error rather than panicking.
func TestReadImmediateStreamError(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := paraplaintext.NewReader()

	doc := &model.RawDocument{
		URI:          "test://immediate-fail",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       &failingReaderImmediate{err: io.ErrUnexpectedEOF},
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
	assert.True(t, foundError, "expected a clean error when the underlying reader fails immediately")
}
