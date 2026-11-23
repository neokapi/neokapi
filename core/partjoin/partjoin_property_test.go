package partjoin

import (
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// split decodes a join back to its parts, as Join writes them: a bare sep
// ends a part, esc+'0' is sep and esc+'1' is esc. When tail is set the last
// part keeps its seps, as JoinTail writes it, and n names the part count.
func split(t *testing.T, s string, sep, esc byte, tail bool, n int) []string {
	t.Helper()
	var parts []string
	var cur strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == esc:
			require.Less(t, i+1, len(s), "an escape ends the join")
			i++
			switch s[i] {
			case '0':
				cur.WriteByte(sep)
			case '1':
				cur.WriteByte(esc)
			default:
				t.Fatalf("escape followed by %q", s[i])
			}
		case c == sep && !(tail && len(parts) == n-1):
			parts = append(parts, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
	}
	return append(parts, cur.String())
}

// randomPart builds a part from an alphabet weighted towards the characters
// the join treats specially, so a separator or an escape lands at every
// position, at either end, alone and doubled.
func randomPart(r *rand.Rand, sep, esc byte) string {
	alphabet := []byte{sep, esc, '0', '1', 'a', ':', '%', 0x00, 0x01}
	b := make([]byte, r.IntN(6))
	for i := range b {
		b[i] = alphabet[r.IntN(len(alphabet))]
	}
	return string(b)
}

// TestJoinDecodesToItsParts checks, over many random lists, that every join
// decodes back to the parts it was made from, so no two lists of one length
// share a join.
func TestJoinDecodesToItsParts(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for _, f := range []struct {
		sep, esc byte
	}{{NULSep, NULEscape}, {':', '%'}, {'|', NULEscape}} {
		seen := map[string][]string{}
		for range 20000 {
			n := 1 + r.IntN(4)
			parts := make([]string, n)
			for i := range parts {
				parts[i] = randomPart(r, f.sep, f.esc)
			}
			j := Join(f.sep, f.esc, parts...)
			assert.Equal(t, parts, split(t, j, f.sep, f.esc, false, n), "Join(%q)", parts)
			jt := JoinTail(f.sep, f.esc, parts...)
			assert.Equal(t, parts, split(t, jt, f.sep, f.esc, true, n), "JoinTail(%q)", parts)
			if prev, ok := seen[j]; ok {
				assert.Equal(t, prev, parts, "two lists share the join %q", j)
			}
			seen[j] = parts
		}
	}
}

// TestJoinEdgeCases checks the lists a random draw may miss.
func TestJoinEdgeCases(t *testing.T) {
	cases := [][]string{
		{""},
		{"", ""},
		{"\x01"},
		{"\x01", "\x01"},
		{"\x00"},
		{"\x00\x01", "\x01\x00"},
		{"\x010", ""},
		{"", "", ""},
	}
	for _, parts := range cases {
		assert.Equal(t, parts, split(t, NUL(parts...), NULSep, NULEscape, false, len(parts)), "%q", parts)
	}
	// The one pair the part count tells apart, and nothing else does.
	assert.Equal(t, NUL(), NUL(""))
	assert.NotEqual(t, NUL(""), NUL("", ""))
}

// TestAppendAndWriteMatchNUL checks that AppendNUL and WriteNUL write the
// bytes NUL writes for the same parts.
func TestAppendAndWriteMatchNUL(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	for range 2000 {
		parts := make([]string, 1+r.IntN(4))
		for i := range parts {
			parts[i] = randomPart(r, NULSep, NULEscape)
		}
		assert.Equal(t, NUL(parts...), AppendNUL(NUL(parts[0]), parts[1:]...))
		var b strings.Builder
		WriteNUL(&b, parts...)
		assert.Equal(t, NUL(parts...)+"\x00", b.String())
		var j strings.Builder
		WriteNULJoined(&j, NUL(parts...))
		assert.Equal(t, b.String(), j.String())
	}
}
