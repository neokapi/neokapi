package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A recipe is reviewed. The diff `kapi add` leaves behind has to be the size of
// the decision it took — a write that also re-spaces the file and rewrites a
// binding nobody touched is how a reviewer learns to stop reading recipe diffs.
//
// The fixture is samples/northsea because it is authored the way a recipe is:
// blank lines between the collections, and a `defaults.voice` written in the
// long form the loader also accepts as a bare path.

// soleInsertion splits two documents by the lines they share at each end.
// before's middle is what the write removed or rewrote; after's middle is what
// it added.
func soleInsertion(before, after string) (removed, added []string) {
	b := strings.Split(before, "\n")
	a := strings.Split(after, "\n")
	head := 0
	for head < len(b) && head < len(a) && b[head] == a[head] {
		head++
	}
	tail := 0
	for tail < len(b)-head && tail < len(a)-head && b[len(b)-1-tail] == a[len(a)-1-tail] {
		tail++
	}
	return b[head : len(b)-tail], a[head : len(a)-tail]
}

func TestAdd_WritesOnlyTheLinesItAdds(t *testing.T) {
	a := processOnlyApp(t)
	recipe, _ := northseaCopy(t)
	before := readFile(t, recipe)

	// A pattern the recipe already tracks is the whole verb's no-op, and it
	// must reach the file as nothing at all.
	out, err := runCLI(t, NewAddCmd(a), "docs/**/*.md", "--project", recipe)
	require.NoError(t, err, out)
	assert.Equal(t, before, readFile(t, recipe),
		"adding a pattern already tracked rewrote the recipe")

	out, err = runCLI(t, NewAddCmd(a), "support/**/*.md",
		"--project", recipe, "--name", "northsea-support", "--channel", "northsea/docs")
	require.NoError(t, err, out)

	after := readFile(t, recipe)
	removed, added := soleInsertion(before, after)
	assert.Empty(t, removed, "the write rewrote lines the add did not ask for")

	block := strings.Join(added, "\n")
	assert.Contains(t, block, "- name: northsea-support", "the add must land")
	assert.Contains(t, block, "path: support/**/*.md")
	assert.True(t, strings.HasPrefix(strings.TrimSpace(block), "- name: northsea-support"),
		"every line the write added belongs to the new collection, and this one does not: %q", block)

	assert.Contains(t, after, "  voice:\n    profile_file: .kapi/voice.yaml\n",
		"a binding the add never touched keeps the form it was authored in")
	assert.Contains(t, after, "\n\ncollections:\n", "the blank line above a section survives the write")
	assert.Contains(t, after, "\n\n  # Compass interface strings. One block per key.\n",
		"and so does the one that separates two collections")
}
