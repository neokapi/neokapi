package llm

import (
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArgmax(t *testing.T) {
	assert.Equal(t, 2, argmax([]float32{0.1, 0.2, 0.9, 0.3}))
	assert.Equal(t, 0, argmax([]float32{5, 1, 1}))
	assert.Equal(t, 0, argmax(nil))
}

func TestSoftmaxSumsToOne(t *testing.T) {
	p := softmax([]float32{1, 2, 3}, 1)
	var sum float64
	for _, v := range p {
		assert.GreaterOrEqual(t, v, 0.0)
		sum += v
	}
	assert.InDelta(t, 1.0, sum, 1e-9)
	// Monotonic: larger logit → larger probability.
	assert.Less(t, p[0], p[1])
	assert.Less(t, p[1], p[2])
}

func TestSoftmaxTemperatureFlattens(t *testing.T) {
	hot := softmax([]float32{1, 2, 3}, 0.1)
	warm := softmax([]float32{1, 2, 3}, 10)
	// Lower temperature concentrates mass on the top token.
	assert.Greater(t, hot[2], warm[2])
}

func TestSoftmaxStableWithLargeLogits(t *testing.T) {
	p := softmax([]float32{1000, 1001, 1002}, 1)
	for _, v := range p {
		assert.False(t, math.IsNaN(v), "no overflow → NaN")
	}
}

func TestSampleGreedyWhenTemperatureZero(t *testing.T) {
	logits := []float32{0.1, 3.0, 0.2}
	// Greedy ignores rng entirely; nil rng must be safe.
	for i := 0; i < 5; i++ {
		assert.Equal(t, 1, sample(logits, 0, 0, nil))
	}
}

func TestSampleNucleusRestrictsToTop(t *testing.T) {
	// One token dominates; with a tight nucleus, sampling must always pick it.
	logits := []float32{10, 0, 0, 0}
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 50; i++ {
		assert.Equal(t, 0, sample(logits, 1.0, 0.5, r))
	}
}

func TestSampleDrawsFromDistribution(t *testing.T) {
	// Two equally likely tokens, full nucleus: both should appear over many draws.
	logits := []float32{1, 1}
	r := rand.New(rand.NewSource(42))
	counts := map[int]int{}
	for i := 0; i < 200; i++ {
		counts[sample(logits, 1.0, 0, r)]++
	}
	require.Len(t, counts, 2)
	assert.Greater(t, counts[0], 20)
	assert.Greater(t, counts[1], 20)
}
