package main

import (
	"math"
	"sort"
)

// computeStats calculates descriptive statistics from a slice of float64 values.
func computeStats(values []float64) Stats {
	if len(values) == 0 {
		return Stats{}
	}

	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	n := float64(len(sorted))
	sum := 0.0
	for _, v := range sorted {
		sum += v
	}
	mean := sum / n

	// Variance
	varSum := 0.0
	for _, v := range sorted {
		d := v - mean
		varSum += d * d
	}
	stddev := 0.0
	if len(sorted) > 1 {
		stddev = math.Sqrt(varSum / (n - 1))
	}

	return Stats{
		Mean:   round2(mean),
		Median: round2(percentile(sorted, 50)),
		P5:     round2(percentile(sorted, 5)),
		P95:    round2(percentile(sorted, 95)),
		Stddev: round2(stddev),
		Min:    round2(sorted[0]),
		Max:    round2(sorted[len(sorted)-1]),
	}
}

// percentile returns the p-th percentile of a sorted slice using linear interpolation.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 1 {
		return sorted[0]
	}
	rank := (p / 100.0) * float64(len(sorted)-1)
	lower := int(rank)
	upper := lower + 1
	if upper >= len(sorted) {
		return sorted[len(sorted)-1]
	}
	frac := rank - float64(lower)
	return sorted[lower] + frac*(sorted[upper]-sorted[lower])
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
