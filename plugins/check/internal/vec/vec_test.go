package vec

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCosine(t *testing.T) {
	a := []float32{1, 0, 0}
	assert.InDelta(t, 1.0, Cosine(a, a), 1e-9, "identical → 1")
	assert.InDelta(t, 0.0, Cosine(a, []float32{0, 1, 0}), 1e-9, "orthogonal → 0")
	assert.InDelta(t, -1.0, Cosine(a, []float32{-1, 0, 0}), 1e-9, "opposite → -1")
	assert.Equal(t, 0.0, Cosine(a, []float32{1, 0}), "length mismatch → 0")
	assert.Equal(t, 0.0, Cosine(a, []float32{0, 0, 0}), "zero vector → 0")
}

func TestL2Normalize(t *testing.T) {
	out := L2Normalize([]float32{3, 4})
	var n float64
	for _, x := range out {
		n += float64(x) * float64(x)
	}
	assert.InDelta(t, 1.0, math.Sqrt(n), 1e-6, "unit length")
	assert.InDelta(t, 0.6, out[0], 1e-6)
	assert.InDelta(t, 0.8, out[1], 1e-6)

	zero := L2Normalize([]float32{0, 0})
	assert.Equal(t, []float32{0, 0}, zero, "zero vector unchanged")
}

func TestMeanPool(t *testing.T) {
	hidden := [][]float32{{1, 1}, {3, 3}, {9, 9}}
	// mask drops the third token: mean of (1,1) and (3,3) = (2,2).
	out := MeanPool(hidden, []int64{1, 1, 0})
	assert.Equal(t, []float32{2, 2}, out)

	// all-masked → zero vector of the right width.
	assert.Equal(t, []float32{0, 0}, MeanPool(hidden, []int64{0, 0, 0}))
}
