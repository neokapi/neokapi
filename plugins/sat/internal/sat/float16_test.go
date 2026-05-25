package sat

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFloat16RoundTripExact(t *testing.T) {
	// Values exactly representable in float16.
	exact := []float32{0, 1, 2, 3, 0.5, -0.5, -1, 1024, -2048, 0.0009765625}
	for _, v := range exact {
		bits := float32ToFloat16Bits(v)
		got := float16BitsToFloat32(bits)
		assert.Equal(t, v, got, "round-trip of %v", v)
	}
}

func TestFloat16KnownBitPatterns(t *testing.T) {
	assert.Equal(t, uint16(0x3C00), float32ToFloat16Bits(1.0))
	assert.Equal(t, uint16(0x0000), float32ToFloat16Bits(0.0))
	assert.Equal(t, uint16(0xBC00), float32ToFloat16Bits(-1.0))
	assert.Equal(t, uint16(0x4000), float32ToFloat16Bits(2.0))

	assert.Equal(t, float32(1.0), float16BitsToFloat32(0x3C00))
	assert.Equal(t, float32(2.0), float16BitsToFloat32(0x4000))
}

func TestFloat16Approx(t *testing.T) {
	// Values not exactly representable round to nearest within half precision.
	for _, v := range []float32{0.1, 0.25, 3.14159, -2.71828, 100.5} {
		bits := float32ToFloat16Bits(v)
		got := float16BitsToFloat32(bits)
		// float16 has ~3 decimal digits; tolerance scales with magnitude.
		tol := math.Abs(float64(v)) * 0.01
		if tol < 0.001 {
			tol = 0.001
		}
		assert.InDelta(t, v, got, tol, "approx round-trip of %v", v)
	}
}

func TestFloat16InfNaN(t *testing.T) {
	posInf := float32ToFloat16Bits(float32(math.Inf(1)))
	assert.Equal(t, uint16(0x7C00), posInf)
	assert.True(t, math.IsInf(float64(float16BitsToFloat32(0x7C00)), 1))

	negInf := float32ToFloat16Bits(float32(math.Inf(-1)))
	assert.Equal(t, uint16(0xFC00), negInf)

	assert.True(t, math.IsNaN(float64(float16BitsToFloat32(0x7E00))))
}

func TestOnesFloat16Mask(t *testing.T) {
	buf := onesFloat16Mask(3)
	require.Len(t, buf, 6)
	for i := range 3 {
		assert.Equal(t, uint16(0x3C00), binary.LittleEndian.Uint16(buf[i*2:]))
	}
}

func TestDecodeFloat16Logits(t *testing.T) {
	// Encode [0.0, 1.0, -1.0] as float16 LE bytes then decode.
	buf := make([]byte, 6)
	binary.LittleEndian.PutUint16(buf[0:], float32ToFloat16Bits(0.0))
	binary.LittleEndian.PutUint16(buf[2:], float32ToFloat16Bits(1.0))
	binary.LittleEndian.PutUint16(buf[4:], float32ToFloat16Bits(-1.0))
	got := decodeFloat16Logits(buf)
	assert.Equal(t, []float32{0, 1, -1}, got)
}
