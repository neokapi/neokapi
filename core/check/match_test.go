package check

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFindTerm_WordBoundary(t *testing.T) {
	// The canonical false positive: "use" must not match inside "user".
	assert.False(t, ContainsTerm("the user clicked", "use"), `"use" in "user"`)
	assert.False(t, ContainsTerm("abuser", "use"))
	assert.False(t, ContainsTerm("used", "use"))
	assert.True(t, ContainsTerm("I use it", "use"))
	assert.True(t, ContainsTerm("(use)", "use"))
	assert.True(t, ContainsTerm("use.", "use"))
}

func TestFindTerm_CaseInsensitive(t *testing.T) {
	hits := FindTerm("Leverage our Synergy", "leverage")
	assert.Len(t, hits, 1)
	assert.Equal(t, [2]int{0, 8}, hits[0])
}

func TestFindTerm_MultiWord(t *testing.T) {
	assert.True(t, ContainsTerm("open the Acme Cloud dashboard", "Acme Cloud"))
	assert.False(t, ContainsTerm("open the AcmeCloud dashboard", "Acme Cloud"))
}

func TestFindTerm_PunctuationTermRelaxesBoundary(t *testing.T) {
	// A term whose edges are non-word runes still matches when adjacent to word
	// characters — e.g. placeholders.
	assert.True(t, ContainsTerm("you have {count} items", "{count}"))
	assert.True(t, ContainsTerm("written in C++ today", "C++"))
}

func TestFindTerm_ByteOffsetsWithMultibytePrefix(t *testing.T) {
	// "café " is 6 bytes (é = 2). "leverage" then starts at byte 6.
	hits := FindTerm("café leverage", "leverage")
	assert.Len(t, hits, 1)
	assert.Equal(t, 6, hits[0][0])
	assert.Equal(t, 14, hits[0][1])
	assert.Equal(t, "leverage", "café leverage"[hits[0][0]:hits[0][1]])
}

func TestFindTerm_MultipleHitsAndEmpty(t *testing.T) {
	assert.Len(t, FindTerm("go go go", "go"), 3)
	assert.Nil(t, FindTerm("anything", ""))
}
