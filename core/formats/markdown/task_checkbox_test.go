package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTaskCheckboxSpellingComesFromSource covers the checkbox of an item with
// nothing after it. With no text sibling to bound the spelling, the canonical
// form was used, and it carries a trailing space the source does not have:
// "- [ ]" came back as "- [ ] ".
func TestTaskCheckboxSpellingComesFromSource(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"empty item", "- [ ]\n"},
		{"empty item with a star marker", "* [ ]\n"},
		{"empty checked item", "- [x]\n"},
		{"empty item ending in a hard break", "- [ ]  \n"},
		{"empty item with a tab", "- [ ]\t\n"},
		{"item with text", "- [ ] a task\n"},
		{"item opening with a code span", "- [ ] `config.go` and more\n"},
		{"item opening with a link", "- [x] [a](b)\n"},
		{"two items", "- [ ] a\n- [x] b\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
		})
	}
}
