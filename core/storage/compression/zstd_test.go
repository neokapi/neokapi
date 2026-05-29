package compression

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompressDecompress(t *testing.T) {
	pool := NewPool(nil)

	original := []byte(`{"blocks":[{"id":"b1","text":"Hello world","translatable":true}]}`)
	compressed := pool.Compress(original)

	// Compressed should be smaller (or at least not larger for small inputs).
	assert.NotEmpty(t, compressed)

	decompressed, err := pool.Decompress(compressed)
	require.NoError(t, err)
	assert.Equal(t, original, decompressed)
}

func TestCompressDecompress_Large(t *testing.T) {
	pool := NewPool(nil)

	// Create repetitive data (simulates translation blocks).
	var original []byte
	for range 1000 {
		original = append(original, []byte(`{"id":"block-id","source_text":"Click on last point or press Escape or Enter to finish","translatable":true,"properties":{"context":"toolbar"}},`)...)
	}

	compressed := pool.Compress(original)

	// Repetitive data should compress well.
	ratio := float64(len(compressed)) / float64(len(original))
	assert.Less(t, ratio, 0.1, "compression ratio should be < 10% for repetitive data")

	decompressed, err := pool.Decompress(compressed)
	require.NoError(t, err)
	assert.Equal(t, original, decompressed)
}

func TestPoolReuse(t *testing.T) {
	pool := NewPool(nil)

	// Multiple compress/decompress cycles should work (pool reuse).
	for range 10 {
		data := []byte("test data iteration")
		compressed := pool.Compress(data)
		decompressed, err := pool.Decompress(compressed)
		require.NoError(t, err)
		assert.Equal(t, data, decompressed)
	}
}
