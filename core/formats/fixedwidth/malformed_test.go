package fixedwidth_test

import (
	"errors"
	"testing"

	"github.com/neokapi/neokapi/core/formats/fixedwidth"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadMalformedInput feeds truncated, garbage, and otherwise broken
// fixed-width input and asserts that Open/Read never panic. Fixed-width is a
// plain-text columnar format, so there is no "syntactically invalid" content:
// any byte stream is a valid (if ragged) sequence of lines. The contract here
// is purely "do not panic on ragged/garbage input" — out-of-range columns,
// truncated lines, and binary bytes must all be handled gracefully.
func TestReadMalformedInput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{
			// Line far shorter than the declared column layout.
			name:  "truncated line",
			input: "ab\n",
		},
		{
			// No content at all between the column boundaries.
			name:  "only newlines",
			input: "\n\n\n",
		},
		{
			// Last line has no trailing newline.
			name:  "no trailing newline",
			input: "id001Hello World",
		},
		{
			// Embedded NUL and control bytes; still a single decodable line.
			name:  "binary garbage",
			input: "id001\x00\x01\x02\x7f garbage",
		},
		{
			// Bare carriage returns with no line feed.
			name:  "bare carriage returns",
			input: "id001Hello\rid002World\r",
		},
		{
			// Invalid UTF-8 byte sequence in the translatable column.
			name:  "invalid utf-8",
			input: "id001\xff\xfe\xfd bad bytes  \n",
		},
		{
			// Empty input.
			name:  "empty",
			input: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			reader := fixedwidth.NewReader()
			cfg := reader.Config().(*fixedwidth.Config)
			cfg.Columns = []fixedwidth.ColumnDef{
				{Name: "id", Start: 0, Width: 5, Translatable: false},
				{Name: "text", Start: 5, Width: 15, Translatable: true},
			}

			require.NotPanics(t, func() {
				err := reader.Open(ctx, testutil.RawDocFromString(tt.input, model.LocaleEnglish))
				require.NoError(t, err)
			})
			defer reader.Close()

			// Draining the channel must not panic and must not surface an error
			// for any of these inputs — none of them represent a read failure.
			require.NotPanics(t, func() {
				for result := range reader.Read(ctx) {
					require.NoError(t, result.Error)
				}
			})
		})
	}
}

// TestReadNilDocument verifies Open rejects a nil document without panicking.
func TestReadNilDocument(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := fixedwidth.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}

// TestReadNilReader verifies Open rejects a document whose Reader is nil
// without panicking — the reader must not dereference a nil io.Reader later.
func TestReadNilReader(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := fixedwidth.NewReader()
	doc := &model.RawDocument{
		URI:          "test://input",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       nil,
	}
	err := reader.Open(ctx, doc)
	require.Error(t, err)
}

// errReader is an io.Reader that always fails. It models a transport/IO error
// (truncated stream, disk failure) mid-read so we can prove the reader surfaces
// the failure on the result channel rather than silently swallowing it.
type errReader struct{}

func (errReader) Read(_ []byte) (int, error) {
	return 0, errors.New("simulated read failure")
}

// TestReadErrorSurfacesOnChannel asserts that an underlying reader error is
// reported as a PartResult.Error rather than being swallowed. This guards the
// default (non-skeleton) scan path, where a bufio.Scanner error would otherwise
// be discarded.
func TestReadErrorSurfacesOnChannel(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := fixedwidth.NewReader()
	cfg := reader.Config().(*fixedwidth.Config)
	cfg.Columns = []fixedwidth.ColumnDef{
		{Name: "text", Start: 0, Width: 10, Translatable: true},
	}

	doc := testutil.RawDocFromReader(errReader{}, "test://input", model.LocaleEnglish)
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
	assert.True(t, foundError, "expected the underlying read failure to surface on the channel")
}
