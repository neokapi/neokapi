package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pluralMessage is an ICU message whose plural's other branch holds a sentence
// break: "You have {n, plural, one {one item. Remove it?} other {many items.
// Remove them?}} Thanks."
func pluralMessage() *model.Block {
	plural := model.Run{Plural: &model.PluralRun{Pivot: "n", Forms: map[model.PluralForm][]model.Run{
		model.PluralOne:   {model.TextR("one item. Remove it?")},
		model.PluralOther: {model.TextR("many items. Remove them?")},
	}}}
	b := model.NewRunsBlock("b2", []model.Run{model.TextR("You have "), plural, model.TextR(" Thanks.")})
	b.SourceLocale = "en"
	return b
}

// A sentence break inside a plural branch has no run position at the top
// level of the message, so the sentences around it cannot be segments of it.
// The segmentation tool leaves such a block unsegmented rather than fail the
// flow over it, and it segments the blocks around it as before.
func TestSegmentationTool_AMessageWithAPluralDoesNotFailTheFlow(t *testing.T) {
	tl := tools.NewSegmentationTool(&tools.SegmentationConfig{})
	in := make(chan *model.Part, 2)
	out := make(chan *model.Part, 2)
	plain := model.NewBlock("b1", "One sentence. Another sentence.")
	in <- &model.Part{Type: model.PartBlock, Resource: plain}
	in <- &model.Part{Type: model.PartBlock, Resource: pluralMessage()}
	close(in)

	require.NoError(t, tl.Process(t.Context(), in, out))
	close(out)

	var blocks []*model.Block
	for p := range out {
		blocks = append(blocks, p.Resource.(*model.Block))
	}
	require.Len(t, blocks, 2)
	assert.Equal(t, 2, blocks[0].SourceSegmentCount())
	for _, o := range blocks[1].Overlays {
		for _, s := range o.Spans {
			assert.True(t, s.Range.Resolves(blocks[1].Source), "no span of %s points into the plural", o.Type)
		}
	}
}

// An entity a detector found inside a plural's other branch is left out; the
// entity beside it is written.
func TestView_AnEntityInsideAPluralIsLeftOut(t *testing.T) {
	b := pluralMessage()
	text := model.RunsText(b.Source)
	bt := &tool.BaseTool{ToolName: "ner"}
	bt.Annotate = func(v tool.BlockView) error {
		v.AddOverlay(model.Overlay{Type: model.OverlayEntity, Spans: []model.Span{
			{ID: "e1", Range: model.RangeAnchorForBytes(v.SourceRuns(), 9, 19)},                    // "many items"
			{ID: "e2", Range: model.RangeAnchorForBytes(v.SourceRuns(), len(text)-7, len(text)-1)}, // "Thanks"
		}})
		return nil
	}
	_, err := bt.ApplyContext(t.Context(), &model.Part{Type: model.PartBlock, Resource: b})
	require.NoError(t, err)
	assert.Nil(t, b.OverlaySpan(model.OverlayEntity, "e1"))
	sp := b.OverlaySpan(model.OverlayEntity, "e2")
	require.NotNil(t, sp)
	assert.Equal(t, "Thanks", model.RunsText(sp.Range.ExtractRuns(b.Source)))
}
