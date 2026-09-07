package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestUntranslatableDefinitionReplaysItsBytes is the #2507 reproducer. A
// definition with nothing to translate was rebuilt from the parser's resolved
// label, destination and title whenever the scanner could not place its parts,
// and the rebuild spelled one space after the colon whatever the source had.
func TestUntranslatableDefinitionReplaysItsBytes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"control character destination", "[a]:\x06"},
		{"no space after the colon", "[a]:/x\n"},
		{"two spaces after the colon", "[a]:  /x\n"},
		{"tab after the colon", "[a]:\t/x\n"},
		{"title padded from the destination", "[a]: /x   'T'\n"},
		{"angle-bracketed destination", "[a]: </x y>\n"},
		{"definition spread over lines", "[a]:\n /x\n 'T'\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
		})
	}
}
