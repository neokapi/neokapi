package check

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Text a source writes as program syntax is not prose: inline code, a fenced
// block, a kapi command it quotes or lists as an example, and a flag name. A
// target keeps those as written, so "check" inside `kapi check` is not a use of
// the term "check". The same word in the prose around them still is.
func TestTermText_CodeAndCommandSpansAreNotTerms(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		uses    []string
		notUses []string
	}{
		{
			name:    "inline code",
			in:      "Run `kapi check` before you commit.",
			uses:    []string{"run", "commit"},
			notUses: []string{"kapi", "check"},
		},
		{
			name:    "a fenced block",
			in:      "Run this:\n\n```\nkapi check --ship\n```\n",
			uses:    []string{"run"},
			notUses: []string{"check", "ship"},
		},
		{
			name:    "a single-quoted command",
			in:      "A pre-commit hook runs 'kapi check --staged' before each commit.",
			uses:    []string{"hook", "commit"},
			notUses: []string{"check", "staged"},
		},
		{
			name:    "a double-quoted command",
			in:      `Run "kapi check" again.`,
			uses:    []string{"run", "again"},
			notUses: []string{"check"},
		},
		{
			name:    "a quoted command wrapped onto the next line",
			in:      "The loop for an agent: edit, run 'kapi check --diff-against\nHEAD', repair.",
			uses:    []string{"loop", "repair"},
			notUses: []string{"check", "diff", "against"},
		},
		{
			name:    "example lines",
			in:      "Examples:\n  kapi check --staged\n  $ kapi check docs/guide.md",
			uses:    []string{"examples"},
			notUses: []string{"check", "staged", "guide"},
		},
		{
			name:    "a comment after an example command is prose",
			in:      "Examples:\n  kapi tools list   # list every tool",
			uses:    []string{"every", "tool"},
			notUses: []string{"tools"},
		},
		{
			name:    "a flag name",
			in:      "Pass --diff-range to check a range of commits.",
			uses:    []string{"pass", "check", "range", "commits"},
			notUses: []string{"diff"},
		},
		{
			name:    "an apostrophe before a quoted command",
			in:      "The project's 'kapi check' run and the team's review.",
			uses:    []string{"project", "run", "review"},
			notUses: []string{"check"},
		},
		{
			name: "a quoted phrase is prose",
			in:   "Choose 'check spelling' in the menu.",
			uses: []string{"check", "spelling"},
		},
		{
			name: "a sentence about kapi is prose",
			in:   "kapi checks every file it reads.",
			uses: []string{"kapi", "checks", "file"},
		},
		{
			name: "a double hyphen in prose is punctuation",
			in:   "Run the check -- then commit.",
			uses: []string{"check", "commit"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TermText(tt.in)
			assert.Len(t, got, len(tt.in))
			for _, term := range tt.uses {
				assert.True(t, ContainsTerm(got, term), "%q uses %q: %q", tt.in, term, got)
			}
			for _, term := range tt.notUses {
				assert.False(t, ContainsTerm(got, term), "%q does not use %q: %q", tt.in, term, got)
			}
		})
	}
}

// A match after a masked span indexes the original text, including when the
// span holds characters of more than one byte.
func TestTermText_OffsetsAfterAMaskedSpanAreUnchanged(t *testing.T) {
	src := "Run `kapi check --name café` or 'kapi check', then check the file."
	got := TermText(src)
	require.Len(t, got, len(src), "an offset into the projection is an offset into the text")

	hits := FindTerm(got, "check")
	require.Len(t, hits, 1, "only the prose check is a use: %q", got)
	assert.Equal(t, strings.LastIndex(src, "check"), hits[0][0])
	assert.Equal(t, "check", src[hits[0][0]:hits[0][1]])

	file := FindTerm(got, "file")
	require.Len(t, file, 1)
	assert.Equal(t, strings.Index(src, "file"), file[0][0])
}

// The compass catalog's own string: the argument names are program syntax, and
// a vessel rule must not fire on them.
func TestTermText_ArgumentNamesAreNotTerms(t *testing.T) {
	src := "{vessel} is alongside until {until}."
	got := TermText(src)

	assert.Len(t, got, len(src), "an offset into the projection is an offset into the text")
	assert.False(t, ContainsTerm(got, "vessel"), "the argument {vessel} is not a use of the term: %q", got)
	assert.Len(t, FindTerm(got, "until"), 1, "the prose word counts and the argument {until} does not")
	assert.True(t, ContainsTerm(got, "alongside"))
}

func TestTermText(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		uses    []string
		notUses []string
	}{
		{
			name: "a term in text",
			in:   "No vessel is alongside.",
			uses: []string{"vessel", "alongside"},
		},
		{
			name:    "typed argument",
			in:      "Declared draught {draught, number} m against {available} m.",
			uses:    []string{"draught"},
			notUses: []string{"available", "number"},
		},
		{
			name:    "the text of a plural branch",
			in:      "{count, plural, one {# berth} other {# berths}} at this terminal.",
			uses:    []string{"berth", "berths", "terminal"},
			notUses: []string{"count", "plural", "other"},
		},
		{
			name:    "the text of a select branch",
			in:      "{rank, select, master {The master boards} other {Nobody boards}}",
			uses:    []string{"master boards", "nobody"},
			notUses: []string{"rank", "select"},
		},
		{
			name: "a brace that never closes is text",
			in:   "Use {unclosed brace for the vessel",
			uses: []string{"unclosed", "vessel"},
		},
		{
			name:    "i18next double braces",
			in:      "{{vessel}} is alongside",
			uses:    []string{"alongside"},
			notUses: []string{"vessel"},
		},
		{
			name:    "printf and positional styles",
			in:      "%s vessels and %1$d berths",
			uses:    []string{"vessels", "berths"},
			notUses: []string{"s", "d"},
		},
		{
			name:    "a term cannot bridge a placeholder",
			in:      "content {kind} memory",
			uses:    []string{"content", "memory"},
			notUses: []string{"content memory", "kind"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TermText(tt.in)
			assert.Len(t, got, len(tt.in))
			for _, term := range tt.uses {
				assert.True(t, ContainsTerm(got, term), "%q uses %q", tt.in, term)
			}
			for _, term := range tt.notUses {
				assert.False(t, ContainsTerm(got, term), "%q does not use %q", tt.in, term)
			}
		})
	}
}

func TestTermText_TextWithoutPlaceholdersIsUnchanged(t *testing.T) {
	for _, s := range []string{"", "No vessel is alongside.", "Berth #4, 50% full"} {
		assert.Equal(t, s, TermText(s))
	}
}
