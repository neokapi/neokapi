package main

import (
	"cmp"
	"slices"
)

// Mean and median are both reported. A mean over document conversion is easy to
// move with one pathological file, and a median hides a converter that is fine
// on most documents and destroys a few. Neither alone answers "would I trust
// this on my corpus".

func meanOf(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	var sum float64
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}

func medianOf(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := slices.Clone(xs)
	slices.Sort(s)
	mid := len(s) / 2
	if len(s)%2 == 1 {
		return s[mid]
	}
	return (s[mid-1] + s[mid]) / 2
}

func medianInt(xs []int64) int64 {
	if len(xs) == 0 {
		return 0
	}
	s := slices.Clone(xs)
	slices.Sort(s)
	mid := len(s) / 2
	if len(s)%2 == 1 {
		return s[mid]
	}
	return (s[mid-1] + s[mid]) / 2
}

// sortMissing orders lost words by how much was lost, then by the word, so a
// rerun over the same corpus produces identical bytes.
func sortMissing(m []MissingWord) {
	slices.SortFunc(m, func(a, b MissingWord) int {
		if c := cmp.Compare(b.Times, a.Times); c != 0 {
			return c
		}
		return cmp.Compare(a.Word, b.Word)
	})
}
