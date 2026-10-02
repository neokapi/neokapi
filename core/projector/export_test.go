package projector

import "testing"

// SetCheckpointPartSizes lowers the sizes at which a checkpoint keeps a table
// in parts, for the length of a test, so a small history shows the shape a
// large one takes.
func SetCheckpointPartSizes(t *testing.T, inline, part int) {
	t.Helper()
	oldInline, oldPart := inlineTableBytes, partBytes
	inlineTableBytes, partBytes = inline, part
	t.Cleanup(func() { inlineTableBytes, partBytes = oldInline, oldPart })
}
