package tools

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSubtitleFilter verifies the subtitle-filter keeps only timing-anchored,
// non-geometry cues (spoken subtitles) and drops on-screen frame OCR (which is
// geometry-anchored) and untimed blocks, while passing non-Block parts through.
func TestSubtitleFilter(t *testing.T) {
	cue := model.NewBlock("c1", "spoken line")
	cue.SetTiming(&model.TimingAnnotation{StartMS: 0, EndMS: 1000})

	frame := model.NewBlock("f1", "on-screen text")
	frame.SetTiming(&model.TimingAnnotation{StartMS: 0, EndMS: 1000})
	frame.SetGeometry(&model.GeometryAnnotation{BBox: model.Rect{X: 0, Y: 0, W: 10, H: 10}})

	untimed := model.NewBlock("u1", "metadata")

	in := make(chan *model.Part, 8)
	out := make(chan *model.Part, 8)
	in <- &model.Part{Type: model.PartLayerStart, Resource: &model.Layer{ID: "doc"}}
	in <- &model.Part{Type: model.PartBlock, Resource: cue}
	in <- &model.Part{Type: model.PartBlock, Resource: frame}
	in <- &model.Part{Type: model.PartBlock, Resource: untimed}
	in <- &model.Part{Type: model.PartLayerEnd, Resource: &model.Layer{ID: "doc"}}
	close(in)

	require.NoError(t, NewSubtitleFilterTool(nil).Process(t.Context(), in, out))
	close(out)

	var keptBlocks []string
	var layerParts int
	for p := range out {
		switch p.Type {
		case model.PartBlock:
			keptBlocks = append(keptBlocks, p.Resource.(*model.Block).ID)
		case model.PartLayerStart, model.PartLayerEnd:
			layerParts++
		}
	}
	assert.Equal(t, []string{"c1"}, keptBlocks, "only the timed, non-geometry cue is kept")
	assert.Equal(t, 2, layerParts, "non-Block parts pass through unchanged")
}
