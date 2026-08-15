package csv_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	csvfmt "github.com/neokapi/neokapi/core/formats/csv"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// drainCSV reads a fixed CSV input fully and reports whether a clean error
// surfaced on the result channel. It is the shared body for both the
// encoding/csv path (skeleton == nil) and the hand-rolled
// splitRawLines/splitRawCells skeleton path (skeleton != nil). It must never
// panic; that is asserted by the caller via require.NotPanics.
func drainCSV(t *testing.T, input string, withSkeleton bool) (sawError bool) {
	t.Helper()
	ctx := t.Context()

	reader := csvfmt.NewReader()
	var store *format.SkeletonStore
	if withSkeleton {
		var err error
		store, err = format.NewSkeletonStore()
		require.NoError(t, err)
		reader.SetSkeletonStore(store)
	}

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	for result := range reader.Read(ctx) {
		if result.Error != nil {
			sawError = true
		}
	}
	require.NoError(t, reader.Close())
	if store != nil {
		require.NoError(t, store.Close())
	}
	return sawError
}

// TestReadMalformedNoPanic feeds a battery of broken, truncated, garbage, and
// invalid-UTF-8 inputs through BOTH reader paths and asserts each one drains
// without panicking. Go's encoding/csv runs with LazyQuotes and an unbounded
// FieldsPerRecord, so it tolerates most of these inputs rather than erroring —
// the hard contract for malformed CSV is therefore "never panic, always drain
// cleanly". The skeleton path additionally drives the hand-rolled
// splitRawLines/splitRawCells scanner, which must survive dangling quotes,
// embedded newlines, and binary bytes without crashing.
func TestReadMalformedNoPanic(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{
			// A bare double quote in the middle of an unquoted field.
			name:  "bare quote mid-field",
			input: "a,b\"c,d\ne,f,g\n",
		},
		{
			// An opening quote with no closing quote: the field runs to EOF.
			name:  "dangling unterminated quote",
			input: "\"unterminated value, with comma\n1,2,3\n",
		},
		{
			// A row truncated mid-record (no trailing newline, open quote).
			name:  "truncated open quote at eof",
			input: "id,text\n1,\"abc",
		},
		{
			// Embedded newline inside a quoted field — the test2cols.csv class
			// the upstream roundtrip test explicitly excludes. It must still
			// parse and extract without panicking on either path.
			name:  "embedded newline in quoted field",
			input: "id,text\n1,\"line one\nline two\"\n2,plain\n",
		},
		{
			// A quote immediately after the closing quote of a quoted field.
			name:  "extra data after closing quote",
			input: "id,text\n1,\"abc\"def\n",
		},
		{
			// Invalid UTF-8 bytes inside a cell. CSV is byte-oriented, so the
			// reader must carry the bytes through without choking.
			name:  "invalid utf8 bytes",
			input: "id,text\n1,\xff\xfe\xfa value\n",
		},
		{
			// Raw binary / control-byte garbage, including NUL bytes.
			name:  "binary control-byte garbage",
			input: string([]byte{0x00, 0x01, 0x02, 0xff, '\n', 0x7f, ',', 0x00, '\n'}),
		},
		{
			// A giant single field built from invalid + control bytes, to
			// exercise buffer growth in both the csv reader and the raw scanner.
			name:  "giant invalid blob",
			input: "id,text\n1," + strings.Repeat("\xff\x00\xfe x,\n\"", 4096),
		},
		{
			// Only delimiters and quotes, no real content.
			name:  "delimiters and quotes only",
			input: ",\",\",\"\"\"\n,,,\n",
		},
		{
			// Empty input.
			name:  "empty input",
			input: "",
		},
		{
			// Lone carriage returns and mismatched CRLF.
			name:  "stray carriage returns",
			input: "a\rb,c\r\nd,\"e\rf\"\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.NotPanics(t, func() {
				drainCSV(t, tt.input, false)
			}, "encoding/csv path must not panic on malformed input")
			require.NotPanics(t, func() {
				drainCSV(t, tt.input, true)
			}, "skeleton (splitRawLines/splitRawCells) path must not panic on malformed input")
		})
	}
}

// errReader returns one short valid prefix and then a hard read error, to force
// the io.ReadAll inside readContent to fail. It lets us assert that read
// failures surface as PartResult.Error on the channel rather than being
// swallowed.
type errReader struct{ done bool }

func (e *errReader) Read(p []byte) (int, error) {
	if !e.done {
		e.done = true
		n := copy(p, []byte("id,text\n"))
		return n, nil
	}
	return 0, errors.New("simulated read failure")
}

func (e *errReader) Close() error { return nil }

// TestReadIOErrorSurfaces asserts that a failing underlying reader produces a
// clean error on the result channel (rather than silently truncating) on BOTH
// the encoding/csv path and the skeleton path, and that neither path panics.
func TestReadIOErrorSurfaces(t *testing.T) {
	t.Parallel()
	for _, withSkeleton := range []bool{false, true} {
		name := "csv-path"
		if withSkeleton {
			name = "skeleton-path"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()

			reader := csvfmt.NewReader()
			var store *format.SkeletonStore
			if withSkeleton {
				var err error
				store, err = format.NewSkeletonStore()
				require.NoError(t, err)
				reader.SetSkeletonStore(store)
			}

			doc := &model.RawDocument{
				URI:          "test://io-error",
				SourceLocale: model.LocaleEnglish,
				Encoding:     "UTF-8",
				Reader:       &errReader{},
			}
			require.NoError(t, reader.Open(ctx, doc))

			var sawError bool
			require.NotPanics(t, func() {
				for result := range reader.Read(ctx) {
					if result.Error != nil {
						sawError = true
					}
				}
			})
			require.NoError(t, reader.Close())
			if store != nil {
				require.NoError(t, store.Close())
			}

			assert.True(t, sawError,
				"a failing underlying reader must surface PartResult.Error on the channel, not be swallowed")
		})
	}
}

// TestOpenRejectsNilDocument verifies Open rejects a nil document and a document
// with a nil reader without panicking.
func TestOpenRejectsNilDocument(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	t.Run("nil document", func(t *testing.T) {
		t.Parallel()
		reader := csvfmt.NewReader()
		require.NotPanics(t, func() {
			err := reader.Open(ctx, nil)
			require.Error(t, err)
		})
	})

	t.Run("nil reader", func(t *testing.T) {
		t.Parallel()
		reader := csvfmt.NewReader()
		doc := &model.RawDocument{
			URI:          "test://nil-reader",
			SourceLocale: model.LocaleEnglish,
			Encoding:     "UTF-8",
			Reader:       nil,
		}
		require.NotPanics(t, func() {
			err := reader.Open(ctx, doc)
			require.Error(t, err)
		})
	})
}

// writeBlocks runs the writer's non-skeleton path over blocks carrying the
// given properties, and returns the output alongside the error.
func writeBlocks(t *testing.T, props ...map[string]string) (string, error) {
	t.Helper()
	var out strings.Builder
	w := csvfmt.NewWriter()
	w.Output = &out

	ch := make(chan *model.Part, len(props)+1)
	for i, p := range props {
		block := model.NewBlock(fmt.Sprintf("b%d", i), fmt.Sprintf("cell %d", i))
		block.Properties = p
		ch <- &model.Part{Type: model.PartBlock, Resource: block}
	}
	close(ch)
	err := w.Write(t.Context(), ch)
	return out.String(), err
}

// TestWriteRejectsCellsWithoutGridPosition pins the writer's half of the same
// rule the reader tests state: content that cannot be placed must say so.
//
// Row and column are how a cell finds its place in the rebuilt grid, and data
// rows are 1-based — so a cell that arrives without them, or with a value that
// is not an index, used to default to row 0 and fall outside the emit loop
// entirely. Every such cell vanished, and the writer returned an empty file
// with a nil error: the exact outcome of directing a non-tabular document at a
// .csv output, where no block carries a grid position at all.
func TestWriteRejectsCellsWithoutGridPosition(t *testing.T) {
	t.Parallel()

	placed := map[string]string{"row": "1", "column": "0"}

	cases := map[string]struct {
		props   []map[string]string
		wantErr string
	}{
		"no properties at all": {
			props:   []map[string]string{nil, nil},
			wantErr: "no column property",
		},
		"row missing": {
			props:   []map[string]string{{"column": "2"}},
			wantErr: "no row property",
		},
		"column missing": {
			props:   []map[string]string{{"row": "3"}},
			wantErr: "no column property",
		},
		"row not a number": {
			props:   []map[string]string{{"row": "seven", "column": "0"}},
			wantErr: `row="seven"`,
		},
		"column not a number": {
			props:   []map[string]string{{"row": "1", "column": "1e3"}},
			wantErr: `column="1e3"`,
		},
		"negative row": {
			props:   []map[string]string{{"row": "-1", "column": "0"}},
			wantErr: "row=-1",
		},
		"one bad cell among good ones": {
			props:   []map[string]string{placed, {"column": "1"}, placed},
			wantErr: "no row property",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			out, err := writeBlocks(t, tc.props...)
			require.Error(t, err, "an unplaceable cell must not be silently dropped")
			assert.Contains(t, err.Error(), tc.wantErr, "the error says which coordinate is wrong")
			assert.Empty(t, out, "nothing is written when the grid cannot be rebuilt")
		})
	}
}

// TestWritePlacedCellsRoundTrip is the companion assertion: coordinates the
// reader records are still accepted, so the check above rejects only what it
// was written to reject.
func TestWritePlacedCellsRoundTrip(t *testing.T) {
	t.Parallel()

	out, err := writeBlocks(t,
		map[string]string{"column": "0", "header": "true"},
		map[string]string{"column": "1", "header": "true"},
		map[string]string{"row": "1", "column": "0"},
		map[string]string{"row": "1", "column": "1"},
	)
	require.NoError(t, err)
	assert.Equal(t, "cell 0,cell 1\ncell 2,cell 3\n", out)
}
