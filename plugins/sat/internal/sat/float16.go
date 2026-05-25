package sat

import (
	"encoding/binary"
	"math"
)

// The SaT ONNX exports use IEEE-754 half-precision (float16) for the
// attention_mask input and the logits output. The Go onnxruntime binding has
// no native float16 tensor type, so we feed/read those tensors as raw bytes
// (CustomDataTensor) and convert with the helpers here. input_ids stays int64.
//
// This file is build-tag-free so the conversion math is unit-testable without
// the ONNX runtime; the cgo-typed tensor wrappers live in engine_onnx.go.

// float32ToFloat16Bits converts a float32 to its IEEE-754 half-precision bit
// pattern. Handles normal, subnormal, zero, inf and NaN. Rounds to nearest,
// ties to even.
func float32ToFloat16Bits(f float32) uint16 {
	b := math.Float32bits(f)
	sign := uint16((b >> 16) & 0x8000)
	exp := int32((b>>23)&0xFF) - 127 + 15 // rebias 127 -> 15
	mant := b & 0x7FFFFF

	switch {
	case (b & 0x7FFFFFFF) == 0: // +/-0
		return sign
	case ((b >> 23) & 0xFF) == 0xFF: // inf or NaN
		if mant != 0 {
			return sign | 0x7E00 // NaN
		}
		return sign | 0x7C00 // inf
	case exp >= 0x1F: // overflow -> inf
		return sign | 0x7C00
	case exp <= 0: // subnormal or underflow
		if exp < -10 {
			return sign // too small -> signed zero
		}
		mant |= 0x800000 // restore implicit leading 1
		shift := uint32(14 - exp)
		half := uint16(mant >> shift)
		// Round to nearest, ties to even.
		if (mant>>(shift-1))&1 != 0 {
			half++
		}
		return sign | half
	default:
		half := sign | uint16(exp<<10) | uint16(mant>>13)
		// Round to nearest, ties to even.
		if mant&0x1000 != 0 {
			half++
		}
		return half
	}
}

// float16BitsToFloat32 converts an IEEE-754 half-precision bit pattern to
// float32.
func float16BitsToFloat32(h uint16) float32 {
	sign := uint32(h&0x8000) << 16
	exp := uint32(h>>10) & 0x1F
	mant := uint32(h & 0x3FF)

	switch exp {
	case 0:
		if mant == 0 {
			return math.Float32frombits(sign) // signed zero
		}
		// Subnormal: normalize.
		e := uint32(0)
		for mant&0x400 == 0 {
			mant <<= 1
			e++
		}
		mant &= 0x3FF
		exp32 := 127 - 15 - e + 1
		return math.Float32frombits(sign | (exp32 << 23) | (mant << 13))
	case 0x1F:
		if mant == 0 {
			return math.Float32frombits(sign | 0x7F800000) // inf
		}
		return math.Float32frombits(sign | 0x7FC00000) // NaN
	default:
		exp32 := exp - 15 + 127
		return math.Float32frombits(sign | (exp32 << 23) | (mant << 13))
	}
}

// onesFloat16Mask returns n float16 1.0 values packed little-endian (1.0 ==
// 0x3C00), suitable as an attention_mask byte buffer.
func onesFloat16Mask(n int) []byte {
	buf := make([]byte, n*2)
	for i := range n {
		binary.LittleEndian.PutUint16(buf[i*2:], 0x3C00)
	}
	return buf
}

// decodeFloat16Logits decodes a float16 byte buffer into float32 values.
func decodeFloat16Logits(buf []byte) []float32 {
	out := make([]float32, len(buf)/2)
	for i := range out {
		out[i] = float16BitsToFloat32(binary.LittleEndian.Uint16(buf[i*2:]))
	}
	return out
}
