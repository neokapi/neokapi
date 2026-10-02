package protoconvert_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/protoconvert"
)

// A block read from an en-US to en-US file holds a target under its source
// language. Both wire forms carry the source as the source and the target as a
// target, and bring them back the same way.
func TestSameLanguageTargetCrossesThePluginWire(t *testing.T) {
	block := func() *model.Block {
		b := model.NewBlock("b1", "colour source")
		b.SourceLocale = "en-US"
		b.SetTargetText("en-US", "colour target")
		return b
	}

	check := func(t *testing.T, got *model.Block) {
		t.Helper()
		src, ok := got.Edition(model.EditionKey{})
		require.True(t, ok)
		assert.Equal(t, "colour source", model.RunsText(src.Runs))
		assert.Equal(t, "colour target", got.TargetText("en-US"))
	}

	t.Run("block message", func(t *testing.T) {
		msg := protoconvert.BlockToProto(block())
		require.Len(t, msg.Source, 1)
		assert.Equal(t, "colour source", model.RunsText(protoconvert.ProtoToRuns(msg.Source[0].Runs)))
		got := protoconvert.ProtoToBlock(msg)
		got.SourceLocale = "en-US" // the block message carries no locale
		check(t, got)
	})

	t.Run("content block", func(t *testing.T) {
		cb := protoconvert.PartToContentBlock(&model.Part{Type: model.PartBlock, Resource: block()})
		require.Len(t, cb.Source, 1)
		got := protoconvert.ContentBlockToPart(cb).Resource.(*model.Block)
		got.SourceLocale = "en-US"
		check(t, got)
	})
}

// A block with an empty source sends no source segment, and a message with no
// targets comes back as a block holding its source edition alone.
func TestEmptyEditionsCrossThePluginWire(t *testing.T) {
	msg := protoconvert.BlockToProto(&model.Block{ID: "b1"})
	assert.Empty(t, msg.Source)
	assert.Empty(t, msg.Targets)

	got := protoconvert.ProtoToBlock(msg)
	assert.Equal(t, []model.EditionKey{{}}, got.Editions())
	assert.NotNil(t, got.Properties)

	part := protoconvert.ContentBlockToPart(protoconvert.PartToContentBlock(&model.Part{Type: model.PartBlock, Resource: &model.Block{ID: "b2"}}))
	assert.Equal(t, []model.EditionKey{{}}, part.Resource.(*model.Block).Editions())
}
