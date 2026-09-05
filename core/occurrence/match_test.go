package occurrence

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatcherFind(t *testing.T) {
	tests := []struct {
		name string
		term string
		text string
		want []string // the matched substrings, in order
	}{
		{
			name: "plain match",
			term: "memory",
			text: "The memory holds it.",
			want: []string{"memory"},
		},
		{
			name: "case folds both ways",
			term: "Content Memory",
			text: "the CONTENT memory and the content Memory",
			want: []string{"CONTENT memory", "content Memory"},
		},
		{
			name: "multi-word across any whitespace run",
			term: "content memory",
			text: "the content  memory, the content\nmemory, the content\tmemory",
			want: []string{"content  memory", "content\nmemory", "content\tmemory"},
		},
		{
			name: "words must be separated by whitespace",
			term: "content memory",
			text: "contentmemory is one word",
			want: nil,
		},
		{
			name: "a word boundary is required at the start",
			term: "AI",
			text: "again and again",
			want: nil,
		},
		{
			name: "a word boundary is required at the end",
			term: "term",
			text: "terminology is not a term",
			want: []string{"term"},
		},
		{
			name: "punctuation is a boundary",
			term: "memory",
			text: "(memory), memory. memory!",
			want: []string{"memory", "memory", "memory"},
		},
		{
			name: "digits continue a word",
			term: "v1",
			text: "v1 and v12",
			want: []string{"v1"},
		},
		{
			name: "underscore continues a word",
			term: "id",
			text: "id and block_id",
			want: []string{"id"},
		},
		{
			name: "a term ending in punctuation needs no boundary there",
			term: "e.g.",
			text: "see e.g. the guide",
			want: []string{"e.g."},
		},
		{
			// The boundary rule already keeps "aa" out of "aaaa"; a script
			// with no boundaries is where non-overlap has to carry itself.
			name: "matches of one term never overlap",
			term: "語語",
			text: "語語語語",
			want: []string{"語語", "語語"},
		},
		{
			name: "a repeated term inside a word is not a use of it",
			term: "aa",
			text: "aaaa",
			want: nil,
		},
		{
			name: "scripts without word separators skip the boundary rule",
			term: "用語",
			text: "これは日本語の用語です。",
			want: []string{"用語"},
		},
		{
			name: "an accented word is one word",
			term: "café",
			text: "café and cafés",
			want: []string{"café"},
		},
		{
			name: "empty term matches nothing",
			term: "   ",
			text: "anything",
			want: nil,
		},
		{
			name: "empty text matches nothing",
			term: "memory",
			text: "",
			want: nil,
		},
		{
			name: "term absent",
			term: "widget",
			text: "nothing of the sort",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newMatcher(tt.term)
			var got []string
			for _, s := range m.find(tt.text) {
				got = append(got, tt.text[s.start:s.end])
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

// Two terms may both describe the same words. Reporting one and hiding the
// other would answer the narrower question without saying so.
func TestOverlappingTermsAreIndependent(t *testing.T) {
	text := "The content memory recycles wording."
	short := newMatcher("memory").find(text)
	long := newMatcher("content memory").find(text)

	assert.Len(t, short, 1)
	assert.Len(t, long, 1)
	assert.Less(t, long[0].start, short[0].start, "the longer term starts earlier")
	assert.Equal(t, long[0].end, short[0].end, "and they end together")
}

func TestSnippet(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		start, end int
		want       string
	}{
		{
			name:  "short text is carried whole",
			text:  "The memory holds it.",
			start: 4, end: 10,
			want: "The memory holds it.",
		},
		{
			name:  "whitespace collapses, including across lines",
			text:  "The\n\tmemory   holds it.",
			start: 5, end: 11,
			want: "The memory holds it.",
		},
		{
			name: "a long text is elided at both ends",
			text: "aaaaaaaaaa bbbbbbbbbb cccccccccc dddddddddd eeeeeeeeee ffffffffff MATCH " +
				"gggggggggg hhhhhhhhhh iiiiiiiiii jjjjjjjjjj kkkkkkkkkk llllllllll",
			start: 65, end: 70,
			want: "…bbbb cccccccccc dddddddddd eeeeeeeeee ffffffffff MATCH " +
				"gggggggggg hhhhhhhhhh iiiiiiiiii jjjjjjjjjj kk…",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, snippet(tt.text, tt.start, tt.end))
		})
	}
}

func TestSnippetRejectsBadRanges(t *testing.T) {
	assert.Empty(t, snippet("short", 2, 99))
	assert.Empty(t, snippet("short", -1, 2))
	assert.Empty(t, snippet("short", 4, 2))
}

func TestSnippetRendersTheFirstUseInContext(t *testing.T) {
	text := "Drag a Widget onto the board, then drag another widget beside it."
	got := Snippet(text, "widget")
	assert.Contains(t, got, "Widget onto the board")
	assert.True(t, strings.HasPrefix(got, "Drag a Widget"), "the first use, not the second: %q", got)

	assert.Empty(t, Snippet(text, "gadget"), "a term the text does not use has no snippet")
	assert.Empty(t, Snippet(text, "  "), "an empty term has no snippet")
	assert.Empty(t, Snippet("again and again", "AI"), "word boundaries hold, as they do for the join")
}
