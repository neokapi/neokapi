package safeio_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/safeio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLimitedReader(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		input     string
		max       int64
		wantBytes string
		wantErr   bool
	}{
		{name: "under limit", input: "hello", max: 100, wantBytes: "hello"},
		{name: "exactly at limit", input: "hello", max: 5, wantBytes: "hello"},
		{name: "over limit by one", input: "hello!", max: 5, wantBytes: "", wantErr: true},
		{name: "well over limit", input: strings.Repeat("x", 1000), max: 10, wantErr: true},
		{name: "empty input under limit", input: "", max: 10, wantBytes: ""},
		{name: "zero max disables cap", input: "anything goes", max: 0, wantBytes: "anything goes"},
		{name: "negative max disables cap", input: "anything", max: -1, wantBytes: "anything"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lr := safeio.NewLimitedReader(strings.NewReader(tt.input), tt.max)
			got, err := io.ReadAll(lr)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, safeio.ErrByteBudget)
				var le *safeio.LimitError
				assert.ErrorAs(t, err, &le)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantBytes, string(got))
		})
	}
}

// TestLimitedReader_OneBytePerRead verifies the budget holds even when the
// underlying reader hands back a single byte at a time (the boundary +1 trick
// must not over- or under-count across many small reads).
func TestLimitedReader_OneBytePerRead(t *testing.T) {
	t.Parallel()
	lr := safeio.NewLimitedReader(iotest1ByteReader(strings.NewReader("0123456789")), 10)
	got, err := io.ReadAll(lr)
	require.NoError(t, err)
	assert.Equal(t, "0123456789", string(got))

	lr = safeio.NewLimitedReader(iotest1ByteReader(strings.NewReader("0123456789X")), 10)
	_, err = io.ReadAll(lr)
	require.Error(t, err)
	assert.ErrorIs(t, err, safeio.ErrByteBudget)
}

func TestLimitedReader_StickyError(t *testing.T) {
	t.Parallel()
	lr := safeio.NewLimitedReader(strings.NewReader("toolong"), 3)
	_, err1 := lr.Read(make([]byte, 100))
	require.ErrorIs(t, err1, safeio.ErrByteBudget)
	// A subsequent Read returns the same sticky error, never a different state.
	_, err2 := lr.Read(make([]byte, 100))
	require.ErrorIs(t, err2, safeio.ErrByteBudget)
}

func TestBudgetReader(t *testing.T) {
	t.Parallel()
	b := safeio.DefaultBudget().WithMaxBytes(4)
	_, err := io.ReadAll(b.Reader(strings.NewReader("12345")))
	require.ErrorIs(t, err, safeio.ErrByteBudget)

	got, err := io.ReadAll(b.Reader(strings.NewReader("123")))
	require.NoError(t, err)
	assert.Equal(t, "123", string(got))
}

func TestLimitedWriter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		writes   []string
		max      int64
		wantOut  string
		wantErr  bool
		wantTrip int // index of write that should trip (only checked if wantErr)
	}{
		{name: "under limit", writes: []string{"ab", "cd"}, max: 100, wantOut: "abcd"},
		{name: "exactly at limit", writes: []string{"abcde"}, max: 5, wantOut: "abcde"},
		{name: "single write over limit", writes: []string{"abcdef"}, max: 5, wantOut: "abcde", wantErr: true},
		{name: "second write trips", writes: []string{"abc", "defg"}, max: 5, wantOut: "abcde", wantErr: true},
		{name: "zero max disables cap", writes: []string{"abcdef"}, max: 0, wantOut: "abcdef"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			lw := safeio.NewLimitedWriter(&buf, tt.max)
			var err error
			for _, w := range tt.writes {
				if _, err = lw.Write([]byte(w)); err != nil {
					break
				}
			}
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, safeio.ErrByteBudget)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantOut, buf.String())
		})
	}
}

func TestBudgetCompose(t *testing.T) {
	t.Parallel()
	base := safeio.DefaultBudget()
	assert.Equal(t, safeio.DefaultMaxBytes, base.MaxBytes)
	assert.Equal(t, safeio.DefaultMaxDepth, base.MaxDepth)

	got := base.WithMaxBytes(10).WithMaxDepth(3).WithZip(safeio.ZipLimits{MaxEntries: 7})
	assert.Equal(t, int64(10), got.MaxBytes)
	assert.Equal(t, 3, got.MaxDepth)
	assert.Equal(t, 7, got.Zip.MaxEntries)
	// Original is unchanged (value semantics).
	assert.Equal(t, safeio.DefaultMaxBytes, base.MaxBytes)
}

func TestLimitErrorMessage(t *testing.T) {
	t.Parallel()
	lr := safeio.NewLimitedReader(strings.NewReader("toolong"), 3)
	_, err := io.ReadAll(lr)
	require.Error(t, err)
	var le *safeio.LimitError
	require.ErrorAs(t, err, &le)
	assert.Equal(t, int64(3), le.Limit)
	assert.Contains(t, err.Error(), "byte budget exceeded")
	assert.True(t, errors.Is(err, safeio.ErrByteBudget))
}

// iotest1ByteReader returns a reader that yields at most one byte per Read,
// mimicking iotest.OneByteReader without importing testing/iotest into the
// table above.
func iotest1ByteReader(r io.Reader) io.Reader { return &oneByteReader{r} }

type oneByteReader struct{ r io.Reader }

func (o *oneByteReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return o.r.Read(p[:1])
}
