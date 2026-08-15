package check

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// TestTermMatcher_Rule pins the one rule every checker matches a term by. It is
// the same rule the terms-store lookup and the occurrence graph use, so a word
// is a hit for the whole gate or for none of it.
func TestTermMatcher_Rule(t *testing.T) {
	tests := []struct {
		name string
		term string
		text string
		want []string // the matched substrings, in order
	}{
		{
			name: "an underscore continues a word",
			term: "mooring",
			text: "the mooring_id field names the mooring",
			want: []string{"mooring"},
		},
		{
			name: "a suffixed inflection is a different word",
			term: "mooring",
			text: "hides moorings whose draught is too great",
			want: nil,
		},
		{
			name: "a multi-word term matches across any run of whitespace",
			term: "content memory",
			text: "the content\nmemory and the content  memory",
			want: []string{"content\nmemory", "content  memory"},
		},
		{
			name: "scripts without word separators skip the boundary rule",
			term: "用語",
			text: "これは日本語の用語です。",
			want: []string{"用語"},
		},
		{
			name: "matches of one term never overlap",
			term: "語語",
			text: "語語語語",
			want: []string{"語語", "語語"},
		},
		{
			name: "digits continue a word",
			term: "v1",
			text: "v1 and v12",
			want: []string{"v1"},
		},
		{
			name: "a term ending in punctuation needs no boundary there",
			term: "e.g.",
			text: "see e.g. the guide",
			want: []string{"e.g."},
		},
		{
			name: "a term of whitespace matches nothing",
			term: "   ",
			text: "anything",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, h := range FindTerm(tt.text, tt.term) {
				got = append(got, tt.text[h[0]:h[1]])
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestTermMatcher_OffsetsSurviveAWideningFold: a handful of runes lowercase to a
// different number of bytes, and there the folded copy's offsets no longer
// address the original. Reported ranges must index the text as given.
func TestTermMatcher_OffsetsSurviveAWideningFold(t *testing.T) {
	// U+0130 (LATIN CAPITAL LETTER I WITH DOT ABOVE) is 2 bytes and lowercases
	// to 3, so every offset after it shifts in the folded copy.
	text := "İstanbul and the berth"
	hits := FindTerm(text, "berth")
	require.Len(t, hits, 1)
	assert.Equal(t, "berth", text[hits[0][0]:hits[0][1]])
}

// TestPreparedText_SharedAcrossMatchers: one text, many terms — the fold is paid
// once, and every matcher reports offsets into the original.
func TestPreparedText_SharedAcrossMatchers(t *testing.T) {
	text := "Review the source code carefully"
	p := PrepareText(text)

	long := NewTermMatcher("Source Code").FindIn(p)
	require.Len(t, long, 1)
	assert.Equal(t, "source code", text[long[0][0]:long[0][1]])

	short := NewTermMatcher("code").FindIn(p)
	require.Len(t, short, 1)
	assert.Equal(t, "code", text[short[0][0]:short[0][1]])
	assert.Equal(t, text, p.Text())
}

// TestCaseSensitiveTermMatcher: a vocabulary that distinguishes the product from
// the instrument gets a matcher that does too.
func TestCaseSensitiveTermMatcher(t *testing.T) {
	m := NewCaseSensitiveTermMatcher("Compass")
	assert.Len(t, m.Find("Compass shows the compass"), 1)
	assert.Empty(t, NewCaseSensitiveTermMatcher("compass").Find("Compass alone"))
	assert.False(t, m.Empty())
	assert.True(t, NewTermMatcher("  ").Empty())
}
