package check

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
