package protoconvert_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/protoconvert"
	pb "github.com/neokapi/neokapi/core/proto/content/v1"
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
	assert.Equal(t, []model.EditionKey{{}}, got.EditionKeys())
	assert.NotNil(t, got.Properties)

	part := protoconvert.ContentBlockToPart(protoconvert.PartToContentBlock(&model.Part{Type: model.PartBlock, Resource: &model.Block{ID: "b2"}}))
	assert.Equal(t, []model.EditionKey{{}}, part.Resource.(*model.Block).EditionKeys())
}

// Every edition other than the first native one crosses the plugin wire as a
// target under its key's text form: an edition of a tone or a channel keeps
// its key, a same-language channel edition never lands in the source, and a
// translation filed under no language travels under the empty locale. Both
// wire forms bring each back as the edition it left as.
func TestEveryEditionCrossesThePluginWireUnderItsKey(t *testing.T) {
	short := model.EditionKey{Locale: "en", Channel: "short"}
	formal := model.EditionKey{Locale: "fr", Tone: "formal"}
	block := func() *model.Block {
		b := model.NewBlock("b1", "Read the guide")
		b.SourceLocale = "en"
		b.SetEdition(short, model.Edition{Runs: []model.Run{model.TextR("Read")}})
		b.SetEdition(formal, model.Edition{Runs: []model.Run{model.TextR("Veuillez lire le guide")}})
		b.SetTargetText("fr", "Lis le guide")
		b.SetTargetText("", "under no language")
		return b
	}
	wantEntries := map[string]string{
		"en;channel=short": "Read",
		"fr":               "Lis le guide",
		"fr;tone=formal":   "Veuillez lire le guide",
		"":                 "under no language",
	}

	check := func(t *testing.T, source []*pb.SegmentMessage, targets []*pb.TargetEntry, got *model.Block) {
		t.Helper()
		require.Len(t, source, 1)
		assert.Equal(t, "Read the guide", model.RunsText(protoconvert.ProtoToRuns(source[0].Runs)), "the first native edition is the source")
		entries := map[string]string{}
		for _, te := range targets {
			require.Len(t, te.Segments, 1, te.Locale)
			entries[te.Locale] = model.RunsText(protoconvert.ProtoToRuns(te.Segments[0].Runs))
		}
		assert.Equal(t, wantEntries, entries)

		got.SourceLocale = "en" // the wire carries no locale
		want := block()
		assert.Equal(t, want.EditionKeys(), got.EditionKeys())
		for _, k := range want.EditionKeys() {
			we, _ := want.Edition(k)
			ge, ok := got.Edition(k)
			require.True(t, ok, "%+v", k)
			assert.Equal(t, model.RunsText(we.Runs), model.RunsText(ge.Runs), "%+v", k)
		}
		assert.Equal(t, "under no language", got.TargetText(""))
	}

	t.Run("block message", func(t *testing.T) {
		msg := protoconvert.BlockToProto(block())
		check(t, msg.Source, msg.Targets, protoconvert.ProtoToBlock(msg))
	})
	t.Run("content block", func(t *testing.T) {
		cb := protoconvert.PartToContentBlock(&model.Part{Type: model.PartBlock, Resource: block()})
		check(t, cb.Source, cb.Targets, protoconvert.ContentBlockToPart(cb).Resource.(*model.Block))
	})
}

// A segmented channel edition keeps its segments across the wire, filed on
// that edition and never on the source.
func TestASegmentedChannelEditionCrossesThePluginWire(t *testing.T) {
	short := model.EditionKey{Locale: "en", Channel: "short"}
	b := model.NewBlock("b1", "One. Two.")
	b.SourceLocale = "en"
	b.SetEdition(short, model.Edition{Runs: []model.Run{model.TextR("One. "), model.TextR("Two.")}})
	b.SetSegmentation(short, []model.Span{
		{ID: "s1", Range: model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 1})},
		{ID: "s2", Range: model.SpanAnchor(model.RunPos{Run: 1}, model.RunPos{Run: 2})},
	})

	got := protoconvert.ProtoToBlock(protoconvert.BlockToProto(b))
	seg := got.SegmentationFor(short)
	require.NotNil(t, seg)
	assert.Len(t, seg.Spans, 2)
	assert.Nil(t, got.SourceSegmentation(), "the channel edition's segments are not the source's")
}
