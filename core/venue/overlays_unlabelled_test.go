package venue

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// unlabelledOverlaysJSON is a stored overlays column holding a source
// segmentation and the numerus segmentation of a translation filed under no
// language, the second under an explicit empty "variant", the shape the
// overlays of such a translation are stored in.
const unlabelledOverlaysJSON = `[` +
	`{"type":"segmentation","spans":[{"id":"s1","range":{"kind":"range","end":{"run":1}}}]},` +
	`{"type":"segmentation","variant":"","spans":[` +
	`{"id":"n0","range":{"kind":"range","end":{"run":1}}},` +
	`{"id":"n1","range":{"kind":"range","start":{"run":1},"end":{"run":2}}}]}]`

// An overlay with an explicit empty "variant" sits on the translation filed
// under no language. The block codec files it there and writes it back in the
// same shape; the list codec, which has no place for it, leaves it out rather
// than reading it as the source's.
func TestOverlaysOnATranslationUnderNoLanguage(t *testing.T) {
	t.Run("block codec", func(t *testing.T) {
		b := model.NewBlock("b1", "Some files")
		require.NoError(t, UnmarshalBlockOverlays(b, []byte(unlabelledOverlaysJSON)))
		require.NotNil(t, b.SourceSegmentation())
		assert.Len(t, b.SourceSegmentation().Spans, 1)
		seg := b.TargetSegmentation("")
		require.NotNil(t, seg, "the numerus spans sit on the translation under no language")
		assert.Len(t, seg.Spans, 2)

		data, err := MarshalBlockOverlays(b)
		require.NoError(t, err)
		assert.JSONEq(t, unlabelledOverlaysJSON, string(data))
	})

	t.Run("list codec", func(t *testing.T) {
		overlays, err := UnmarshalOverlays([]byte(unlabelledOverlaysJSON))
		require.NoError(t, err)
		require.Len(t, overlays, 1)
		assert.Len(t, overlays[0].Spans, 1, "only the source's segmentation is read")
		assert.True(t, overlays[0].OnSource())
	})

	t.Run("a block with neither", func(t *testing.T) {
		data, err := MarshalBlockOverlays(model.NewBlock("b1", "x"))
		require.NoError(t, err)
		assert.Equal(t, "[]", string(data))
	})
}
