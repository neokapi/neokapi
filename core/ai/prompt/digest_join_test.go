package prompt

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDigestKeepsItsBytes pins that a context whose parts hold neither NUL
// nor U+0001, and whose neighbours before the block do not start with "|",
// digests the bytes it always has.
func TestDigestKeepsItsBytes(t *testing.T) {
	c := Context{
		Before:     []string{"one", "two"},
		After:      []string{"three"},
		Prior:      &PriorVersion{Source: "s", Target: "t"},
		Rejections: []Rejection{{Text: "draft", Reasons: []string{"tone", "terms"}, Note: "too casual"}},
	}
	want := hashString("rejected\x00draft\x00tone,terms\x00too casual\x00s\x00t\x00one\x00two\x00|three\x00")
	assert.Equal(t, want, c.Digest())
}

// TestDigestKeepsNeighboursApart pins that contexts whose neighbours split
// differently digest apart: around a NUL, and around the "|" that ends the
// neighbours before the block. Before, each pair digested alike, so a
// translation cached under one context was served under the other.
func TestDigestKeepsNeighboursApart(t *testing.T) {
	assert.NotEqual(t,
		Context{Before: []string{"a\x00b"}}.Digest(),
		Context{Before: []string{"a", "b"}}.Digest())
	assert.NotEqual(t,
		Context{Before: []string{"|a"}, After: []string{"b"}}.Digest(),
		Context{After: []string{"a", "|b"}}.Digest())
}
