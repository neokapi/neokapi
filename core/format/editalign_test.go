package format

import (
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lcsLen is the length of a longest common subsequence of a and b, by the
// textbook table.
func lcsLen(a, b string) int {
	t := make([][]int, len(a)+1)
	for i := range t {
		t[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			switch {
			case a[i] == b[j]:
				t[i][j] = t[i+1][j+1] + 1
			case t[i+1][j] >= t[i][j+1]:
				t[i][j] = t[i+1][j]
			default:
				t[i][j] = t[i][j+1]
			}
		}
	}
	return t[0][0]
}

// assertAlignment checks that match pairs equal elements in increasing order
// and pairs as many as a longest common subsequence holds.
func assertAlignment(t *testing.T, a, b string, match []int) {
	t.Helper()
	require.Len(t, match, len(a))
	paired, last := 0, -1
	for i, j := range match {
		if j < 0 {
			continue
		}
		require.Greater(t, j, last, "pairs increase: %q %q %v", a, b, match)
		require.Equal(t, a[i], b[j], "pairs equal elements: %q %q %v", a, b, match)
		last = j
		paired++
	}
	assert.Equal(t, lcsLen(a, b), paired, "a shortest edit keeps a longest common subsequence: %q %q", a, b)
}

func matchStrings(a, b string) []int {
	return MatchEdit(len(a), len(b), func(i, j int) bool { return a[i] == b[j] })
}

// MatchEdit keeps a longest common subsequence of the two sequences, whatever
// the edit: the cases a writer meets and random sequences over small alphabets.
func TestMatchEditKeepsALongestCommonSubsequence(t *testing.T) {
	for _, tc := range [][2]string{
		{"", ""}, {"a", ""}, {"", "a"}, {"a", "b"}, {"ab", "ba"},
		{"We utilize the widget", "We use the widget"},
		{"utilize", "use"}, {"abcabba", "cbabac"}, {"xaxbx", "ab"},
	} {
		assertAlignment(t, tc[0], tc[1], matchStrings(tc[0], tc[1]))
	}
	r := rand.New(rand.NewPCG(1, 2))
	word := func(n int, alphabet string) string {
		var b strings.Builder
		for range n {
			b.WriteByte(alphabet[r.IntN(len(alphabet))])
		}
		return b.String()
	}
	for range 2000 {
		alphabet := "abc"[:1+r.IntN(3)]
		a, b := word(r.IntN(30), alphabet), word(r.IntN(30), alphabet)
		assertAlignment(t, a, b, matchStrings(a, b))
	}
}

// A one-word edit of a long value keeps every other element, at any length.
func TestMatchEditAlignsAShortEditOfALongValue(t *testing.T) {
	long := strings.Repeat(`café and \/ `, 8000)
	a := long + "We utilize it. " + long
	b := long + "We use it. " + long
	match := matchStrings(a, b)
	kept := 0
	for _, j := range match {
		if j >= 0 {
			kept++
		}
	}
	assert.Equal(t, len(b)-len("e"), kept, "only the edited word's letters are not kept")
}
