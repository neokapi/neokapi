package format

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSkeletonStore_RoundTrip(t *testing.T) {
	store, err := NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()

	// Write entries.
	store.WriteText([]byte("<html><body><p>"))
	store.WriteRef("tu1")
	store.WriteText([]byte("</p><p>"))
	store.WriteRef("tu2")
	store.WriteText([]byte("</p></body></html>"))

	// Flush and read back.
	require.NoError(t, store.Flush())

	e1, err := store.Next()
	require.NoError(t, err)
	assert.Equal(t, SkeletonText, e1.Type)
	assert.Equal(t, []byte("<html><body><p>"), e1.Data)

	e2, err := store.Next()
	require.NoError(t, err)
	assert.Equal(t, SkeletonRef, e2.Type)
	assert.Equal(t, []byte("tu1"), e2.Data)

	e3, err := store.Next()
	require.NoError(t, err)
	assert.Equal(t, SkeletonText, e3.Type)
	assert.Equal(t, []byte("</p><p>"), e3.Data)

	e4, err := store.Next()
	require.NoError(t, err)
	assert.Equal(t, SkeletonRef, e4.Type)
	assert.Equal(t, []byte("tu2"), e4.Data)

	e5, err := store.Next()
	require.NoError(t, err)
	assert.Equal(t, SkeletonText, e5.Type)
	assert.Equal(t, []byte("</p></body></html>"), e5.Data)

	_, err = store.Next()
	assert.ErrorIs(t, err, io.EOF)
}

func TestMemorySkeletonStore_RoundTrip(t *testing.T) {
	// Memory-backed store: identical contract to the file-backed one, but
	// usable where there's no filesystem (e.g. the js/wasm build).
	store := NewMemorySkeletonStore()
	defer store.Close()

	store.WriteText([]byte("<p>"))
	store.WriteRef("tu1")
	store.WriteText([]byte("</p>"))
	assert.Equal(t, 3, store.EntriesWritten())

	require.NoError(t, store.Flush())

	e1, err := store.Next()
	require.NoError(t, err)
	assert.Equal(t, SkeletonText, e1.Type)
	assert.Equal(t, []byte("<p>"), e1.Data)

	e2, err := store.Next()
	require.NoError(t, err)
	assert.Equal(t, SkeletonRef, e2.Type)
	assert.Equal(t, []byte("tu1"), e2.Data)

	e3, err := store.Next()
	require.NoError(t, err)
	assert.Equal(t, SkeletonText, e3.Type)
	assert.Equal(t, []byte("</p>"), e3.Data)

	_, err = store.Next()
	assert.ErrorIs(t, err, io.EOF)
}

func TestSkeletonStore_EmptyTextSkipped(t *testing.T) {
	store, err := NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()

	store.WriteText([]byte{})
	store.WriteRef("tu1")
	require.NoError(t, store.Flush())

	e1, err := store.Next()
	require.NoError(t, err)
	assert.Equal(t, SkeletonRef, e1.Type)

	_, err = store.Next()
	assert.ErrorIs(t, err, io.EOF)
}

func TestSkeletonStore_ReadBeforeFlush(t *testing.T) {
	store, err := NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()

	_, err = store.Next()
	require.Error(t, err)
}

func TestSkeletonStore_LargeData(t *testing.T) {
	store, err := NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()

	bigData := make([]byte, 100_000)
	for i := range bigData {
		bigData[i] = byte(i % 256)
	}
	store.WriteText(bigData)
	require.NoError(t, store.Flush())

	e, err := store.Next()
	require.NoError(t, err)
	assert.Equal(t, SkeletonText, e.Type)
	assert.Equal(t, bigData, e.Data)
}

func TestSkeletonPair_RoundTrip(t *testing.T) {
	cases := []struct {
		name string
		a    string
		b    string
	}{
		{name: "both_empty"},
		{name: "empty_first", b: "\n      "},
		{name: "empty_second", a: "Book a demonstration"},
		{name: "typical", a: "Compass holds the plan. Tidewatch watches.", b: "\n  Compass holds the plan.\n  Tidewatch watches.\n"},
		{name: "first_half_holds_a_length_prefix", a: "\x00\x00\x00\x05abc", b: "\x00\x00\x00\x09"},
		{name: "nul_and_high_bytes", a: "a\x00b\xff", b: "\x00\xfe"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, b, ok := DecodeSkeletonPair(EncodeSkeletonPair([]byte(tc.a), []byte(tc.b)))
			require.True(t, ok)
			assert.Equal(t, tc.a, string(a))
			assert.Equal(t, tc.b, string(b))
		})
	}
}

func TestSkeletonPair_TruncatedPayloadIsNotFatal(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{name: "nil"},
		{name: "short_header", data: []byte{0, 0, 1}},
		{name: "length_past_end", data: []byte{0, 0, 0, 9, 'a', 'b'}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, ok := DecodeSkeletonPair(tc.data)
			assert.False(t, ok)
		})
	}
}

func TestSkeletonStore_OriginalAndTrimmedEntries(t *testing.T) {
	store, err := NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()

	store.WriteOriginal([]byte("Hello world"), []byte("\n  Hello world\n"))
	store.WriteRef("tu1")
	store.WriteTrimmed([]byte("Hello world"), []byte("\n  "))
	// An empty trim is not an entry: there is nothing to restore.
	store.WriteTrimmed([]byte("Hello world"), nil)
	require.NoError(t, store.Flush())

	e, err := store.Next()
	require.NoError(t, err)
	assert.Equal(t, SkeletonOriginal, e.Type)
	rendered, original, ok := DecodeSkeletonPair(e.Data)
	require.True(t, ok)
	assert.Equal(t, "Hello world", string(rendered))
	assert.Equal(t, "\n  Hello world\n", string(original))

	e, err = store.Next()
	require.NoError(t, err)
	assert.Equal(t, SkeletonRef, e.Type)

	e, err = store.Next()
	require.NoError(t, err)
	assert.Equal(t, SkeletonTrimmed, e.Type)
	rendered, trimmed, ok := DecodeSkeletonPair(e.Data)
	require.True(t, ok)
	assert.Equal(t, "Hello world", string(rendered))
	assert.Equal(t, "\n  ", string(trimmed))

	_, err = store.Next()
	assert.ErrorIs(t, err, io.EOF)
}
