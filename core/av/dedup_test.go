package av

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func solidPNG(t *testing.T, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := range 32 {
		for x := range 32 {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

// halfPNG is black on the left half, white on the right — a clearly different
// content hash from a solid image.
func halfPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := range 32 {
		for x := range 32 {
			if x < 16 {
				img.Set(x, y, color.Black)
			} else {
				img.Set(x, y, color.White)
			}
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func TestAHashDistinguishesContent(t *testing.T) {
	white, err := aHashBytes(solidPNG(t, color.White))
	require.NoError(t, err)
	black, err := aHashBytes(solidPNG(t, color.Black))
	require.NoError(t, err)
	half, err := aHashBytes(halfPNG(t))
	require.NoError(t, err)

	// Two solid frames look identical (small distance); the half/half frame is
	// clearly different.
	assert.LessOrEqual(t, hammingDistance(white, black), 1)
	assert.Greater(t, hammingDistance(half, white), 16)
}

func TestDedupKeep(t *testing.T) {
	a, b := uint64(0), uint64(0xFFFFFFFFFFFFFFFF)
	// Persistent frame (a a a) then a change (b b) then back (a).
	keep := dedupKeep([]uint64{a, a, a, b, b, a}, 5)
	assert.Equal(t, []int{0, 3, 5}, keep)

	// All identical → only the first kept.
	assert.Equal(t, []int{0}, dedupKeep([]uint64{a, a, a, a}, 5))

	// Empty.
	assert.Nil(t, dedupKeep(nil, 5))
}
