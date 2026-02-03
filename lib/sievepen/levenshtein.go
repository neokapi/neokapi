package sievepen

// LevenshteinDistance calculates the edit distance between two strings.
// It counts the minimum number of single-character edits (insertions,
// deletions, or substitutions) required to change one string into the other.
func LevenshteinDistance(a, b string) int {
	runesA := []rune(a)
	runesB := []rune(b)

	lenA := len(runesA)
	lenB := len(runesB)

	if lenA == 0 {
		return lenB
	}
	if lenB == 0 {
		return lenA
	}

	// Use two rows to reduce memory: previous and current.
	prev := make([]int, lenB+1)
	curr := make([]int, lenB+1)

	for j := 0; j <= lenB; j++ {
		prev[j] = j
	}

	for i := 1; i <= lenA; i++ {
		curr[0] = i
		for j := 1; j <= lenB; j++ {
			cost := 1
			if runesA[i-1] == runesB[j-1] {
				cost = 0
			}
			curr[j] = min3(
				prev[j]+1,      // deletion
				curr[j-1]+1,    // insertion
				prev[j-1]+cost, // substitution
			)
		}
		prev, curr = curr, prev
	}

	return prev[lenB]
}

// LevenshteinRatio returns a similarity ratio between 0.0 and 1.0.
// A ratio of 1.0 means the strings are identical, and 0.0 means
// they are completely different. The ratio is calculated as:
//
//	1.0 - (distance / max(len(a), len(b)))
func LevenshteinRatio(a, b string) float64 {
	if a == b {
		return 1.0
	}

	runesA := []rune(a)
	runesB := []rune(b)
	maxLen := len(runesA)
	if len(runesB) > maxLen {
		maxLen = len(runesB)
	}

	if maxLen == 0 {
		return 1.0
	}

	dist := LevenshteinDistance(a, b)
	return 1.0 - float64(dist)/float64(maxLen)
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
