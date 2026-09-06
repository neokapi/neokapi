package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuiltFenceIsLongEnoughForItsContent is the #2487 reproducer.
// CommonMark 4.5 closes a fence on a run at least as long as the opener, so a
// three-backtick opener around content holding a three-backtick line ended the
// block after its first line: the code block lost its tail, the rest became a
// paragraph, and a stray opener was left at the end.
func TestRebuiltFenceIsLongEnoughForItsContent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		written string
	}{
		{"four-backtick fence", " ````\n0\n```\n0", "````\n0\n```\n0\n````\n"},
		{"closed four-backtick fence", "````\n0\n```\n0\n````\n", "````\n0\n```\n0\n````\n"},
		{"tilde fence holding backticks", "~~~~\n0\n```\n0\n~~~~\n", "````\n0\n```\n0\n````\n"},
		{"tilde run inside a fence", "~~~~\n0\n~~~\n0\n~~~~\n", "```\n0\n~~~\n0\n```\n"},
		{"five backticks inside", "``````\na\n`````\n``````\n", "``````\na\n`````\n``````\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := roundtrip(t, tc.input)
			assert.Equal(t, tc.written, out)
			require.Len(t, readBlocks(t, out), 1, "the code block did not stay one block")
			assert.Equal(t, out, roundtrip(t, out), "the rebuild is not idempotent")
		})
	}
}

// TestOrdinaryFenceStaysThreeBackticks pins the floor: content with no backtick
// run keeps the three-backtick spelling.
func TestOrdinaryFenceStaysThreeBackticks(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "```js\na\n```\n", roundtrip(t, "```js\na\n```\n"))
}
