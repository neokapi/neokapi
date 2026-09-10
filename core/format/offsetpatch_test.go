package format

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOffsetWriterUsesOriginalByteCoordinates(t *testing.T) {
	source := []byte("Å one two end")
	plan := OffsetPatchPlan{
		Format: "markdown", Snapshot: SourceSnapshot(source), Patches: []OffsetPatch{
			{Start: 7, End: 10, Before: "two", Replacement: "second, longer passage"},
			{Start: 3, End: 6, Before: "one", Replacement: "first"},
		},
	}
	result, err := ApplyOffsetPatches(source, plan)
	require.NoError(t, err)
	assert.Equal(t, "Å first second, longer passage end", string(result))
	assert.Equal(t, "Å one two end", string(source), "source remains immutable")
	assert.Equal(t, 7, plan.Patches[0].Start, "writer does not sort the caller's plan in place")
	var preview bytes.Buffer
	require.NoError(t, WriteOffsetPatches(&preview, source, plan))
	assert.Equal(t, result, preview.Bytes())
}

func TestOffsetWriterRejectsInvalidPlansBeforeWriting(t *testing.T) {
	source := []byte("first second")
	cases := map[string]OffsetPatchPlan{
		"stale source": {Format: "markdown", Snapshot: SourceSnapshot([]byte("old")), Patches: []OffsetPatch{
			{Start: 0, End: 5, Before: "first", Replacement: "new"},
		}},
		"stale range": {Format: "markdown", Snapshot: SourceSnapshot(source), Patches: []OffsetPatch{
			{Start: 0, End: 5, Before: "other", Replacement: "new"},
		}},
		"overlap": {Format: "html", Snapshot: SourceSnapshot(source), Patches: []OffsetPatch{
			{Start: 0, End: 5, Before: "first", Replacement: "new"},
			{Start: 4, End: 6, Before: "t ", Replacement: "more"},
		}},
		"negative": {Format: "markdown", Snapshot: SourceSnapshot(source), Patches: []OffsetPatch{
			{Start: -1, End: 0, Replacement: "new"},
		}},
		"outside": {Format: "markdown", Snapshot: SourceSnapshot(source), Patches: []OffsetPatch{
			{Start: 0, End: 99, Replacement: "new"},
		}},
		"wrong part": {Format: "html", Snapshot: SourceSnapshot(source), Patches: []OffsetPatch{
			{Entry: "word/document.xml", Start: 0, End: 5, Before: "first", Replacement: "new"},
		}},
	}
	for name, plan := range cases {
		t.Run(name, func(t *testing.T) {
			var output bytes.Buffer
			require.Error(t, WriteOffsetPatches(&output, source, plan))
			assert.Empty(t, output.Bytes())
			assert.Equal(t, "first second", string(source))
		})
	}
}
