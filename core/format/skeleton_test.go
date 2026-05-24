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
	require.NoError(t, store.WriteText([]byte("<html><body><p>")))
	require.NoError(t, store.WriteRef("tu1"))
	require.NoError(t, store.WriteText([]byte("</p><p>")))
	require.NoError(t, store.WriteRef("tu2"))
	require.NoError(t, store.WriteText([]byte("</p></body></html>")))

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

	require.NoError(t, store.WriteText([]byte("<p>")))
	require.NoError(t, store.WriteRef("tu1"))
	require.NoError(t, store.WriteText([]byte("</p>")))
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

	require.NoError(t, store.WriteText([]byte{}))
	require.NoError(t, store.WriteRef("tu1"))
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
	require.NoError(t, store.WriteText(bigData))
	require.NoError(t, store.Flush())

	e, err := store.Next()
	require.NoError(t, err)
	assert.Equal(t, SkeletonText, e.Type)
	assert.Equal(t, bigData, e.Data)
}
