package partjoin

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestJoinKeepsPlainPartsAsTheyAre pins that parts holding neither the
// separator nor the escape join to the bytes of the plain join, so an
// identity built from them does not move.
func TestJoinKeepsPlainPartsAsTheyAre(t *testing.T) {
	parts := []string{"docs/a.md", "tu1", "Save the file"}
	assert.Equal(t, strings.Join(parts, "\x00"), NUL(parts...))
	assert.Equal(t, "a:b", Join(':', '%', "a", "b"))
	assert.Equal(t, "a:b:c", JoinTail(':', '%', "a", "b:c"))
	assert.Empty(t, NUL())
	assert.Equal(t, "x", NUL("x"))
}

// TestJoinKeepsPartsApart pins that two different lists of parts never join
// to one string, where the plain join gave them one.
func TestJoinKeepsPartsApart(t *testing.T) {
	cases := [][2][]string{
		{{"a\x00b", "c"}, {"a", "b\x00c"}},
		{{"a\x01", "b"}, {"a", "\x01b"}},
		{{"a\x010", "b"}, {"a\x00", "0b"}},
		{{"", "a"}, {"a", ""}},
		{{"a\x00"}, {"a", ""}},
	}
	for _, c := range cases {
		assert.NotEqual(t, NUL(c[0]...), NUL(c[1]...), "%q and %q", c[0], c[1])
	}
	assert.NotEqual(t, JoinTail(':', '%', "a:b", "c"), JoinTail(':', '%', "a", "b:c"))
	assert.NotEqual(t, JoinTail(':', '%', "a", "%0x"), JoinTail(':', '%', "a", "\x00x"))
}

// TestEscapedForms pins the escaped form of each character.
func TestEscapedForms(t *testing.T) {
	assert.Equal(t, "a\x010b\x011c", NUL("a\x00b\x01c"))
	assert.Equal(t, "a%0b%1c", Escape("a:b%c", ':', '%'))
	assert.Equal(t, "a%0b:c%1", JoinTail(':', '%', "a:b", "c%"))
}
