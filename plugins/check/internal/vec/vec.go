// Package vec holds the small, pure-Go vector operations the checker needs to
// turn model output into a similarity score: L2 normalization and cosine
// similarity. It has no native dependency, so it builds and unit-tests without
// the ONNX runtime.
package vec

import "math"

// L2Normalize returns a unit-length copy of v. A zero vector is returned
// unchanged.
func L2Normalize(v []float32) []float32 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	norm := math.Sqrt(sum)
	if norm == 0 {
		out := make([]float32, len(v))
		copy(out, v)
		return out
	}
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = float32(float64(x) / norm)
	}
	return out
}

// Cosine returns the cosine similarity of a and b in [-1,1]. Mismatched lengths
// or a zero vector yield 0.
func Cosine(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// MeanPool averages the per-token hidden states over the positions where the
// attention mask is 1 — the standard sentence-embedding pooling for e5-style
// models. hidden is [seqLen][hiddenSize]; mask is [seqLen].
func MeanPool(hidden [][]float32, mask []int64) []float32 {
	if len(hidden) == 0 {
		return nil
	}
	dim := len(hidden[0])
	out := make([]float32, dim)
	var n float64
	for i, row := range hidden {
		if i < len(mask) && mask[i] == 0 {
			continue
		}
		for j := 0; j < dim && j < len(row); j++ {
			out[j] += row[j]
		}
		n++
	}
	if n == 0 {
		return out
	}
	for j := range out {
		out[j] = float32(float64(out[j]) / n)
	}
	return out
}
