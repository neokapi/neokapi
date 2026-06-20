package llm

import (
	"math"
	"math/rand"
	"sort"
)

// argmax returns the index of the largest logit. Used for greedy decoding
// (temperature <= 0).
func argmax(logits []float32) int {
	best := 0
	bestV := float32(math.Inf(-1))
	for i, v := range logits {
		if v > bestV {
			bestV = v
			best = i
		}
	}
	return best
}

// softmax converts logits to probabilities, applying temperature (>0). It is
// numerically stable (subtracts the max before exponentiating).
func softmax(logits []float32, temperature float64) []float64 {
	if temperature <= 0 {
		temperature = 1
	}
	maxV := math.Inf(-1)
	for _, v := range logits {
		if float64(v) > maxV {
			maxV = float64(v)
		}
	}
	out := make([]float64, len(logits))
	var sum float64
	for i, v := range logits {
		e := math.Exp((float64(v) - maxV) / temperature)
		out[i] = e
		sum += e
	}
	if sum == 0 {
		return out
	}
	for i := range out {
		out[i] /= sum
	}
	return out
}

// sample picks the next token id from logits.
//
//   - temperature <= 0: greedy (argmax), ignoring topP and r.
//   - temperature > 0:  temperature softmax, optionally restricted to the
//     smallest set of tokens whose cumulative probability reaches topP (nucleus
//     sampling) when 0 < topP < 1, then drawn from r.
//
// r must be non-nil when temperature > 0.
func sample(logits []float32, temperature, topP float64, r *rand.Rand) int {
	if temperature <= 0 || len(logits) == 0 {
		return argmax(logits)
	}
	probs := softmax(logits, temperature)

	// Rank token indices by descending probability.
	idx := make([]int, len(probs))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return probs[idx[a]] > probs[idx[b]] })

	// Nucleus restriction: keep the top tokens until cumulative mass >= topP.
	keep := len(idx)
	if topP > 0 && topP < 1 {
		var cum float64
		for k, id := range idx {
			cum += probs[id]
			if cum >= topP {
				keep = k + 1
				break
			}
		}
	}

	// Renormalize the kept mass and draw.
	var kept float64
	for k := 0; k < keep; k++ {
		kept += probs[idx[k]]
	}
	if kept <= 0 {
		return idx[0]
	}
	target := r.Float64() * kept
	var cum float64
	for k := 0; k < keep; k++ {
		cum += probs[idx[k]]
		if target < cum {
			return idx[k]
		}
	}
	return idx[keep-1]
}
