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

// A translation filed under no language crosses the plugin wire as the entry
// with the empty locale: its segmentation as that entry's segments, and its
// other overlays with a variant message that is present and empty. None of it
// lands on the source.
func TestATranslationUnderNoLanguageCrossesThePluginWireWithItsOverlays(t *testing.T) {
	span := func(id string, from, to int) model.Span {
		return model.Span{ID: id, Range: model.SpanAnchor(model.RunPos{Run: from}, model.RunPos{Run: to})}
	}
	block := func() *model.Block {
		b := model.NewBlock("b1", "Some files")
		b.SetTargetRuns("", []model.Run{model.TextR("Eine Datei"), model.TextR("Viele Dateien")})
		b.SetTargetSegmentation("", []model.Span{span("n0", 0, 1), span("n1", 1, 2)})
		b.SetUnlabelledOverlays(append(b.UnlabelledOverlays(),
			model.Overlay{Type: model.OverlayTerm, Spans: []model.Span{span("t1", 1, 2)}}))
		return b
	}
	check := func(t *testing.T, got *model.Block) {
		t.Helper()
		assert.Equal(t, "Some files", got.SourceText())
		assert.Nil(t, got.SourceSegmentation(), "the translation's segments are not the source's")
		assert.Empty(t, got.Overlays, "no overlay lands on the source")
		assert.Equal(t, "Eine DateiViele Dateien", model.RunsText(got.TargetRuns("")))
		seg := got.TargetSegmentation("")
		require.NotNil(t, seg)
		require.Len(t, seg.Spans, 2)
		assert.Equal(t, []string{"n0", "n1"}, []string{seg.Spans[0].ID, seg.Spans[1].ID})
		var terms []model.Overlay
		for _, o := range got.UnlabelledOverlays() {
			if o.Type == model.OverlayTerm {
				terms = append(terms, o)
			}
		}
		require.Len(t, terms, 1)
		assert.Equal(t, "t1", terms[0].Spans[0].ID)
	}

	t.Run("block message", func(t *testing.T) {
		msg := protoconvert.BlockToProto(block())
		var entry *pb.TargetEntry
		for _, te := range msg.Targets {
			if te.Locale == "" {
				entry = te
			}
		}
		require.NotNil(t, entry)
		assert.Len(t, entry.Segments, 2, "the translation travels in its segments")
		require.Len(t, msg.Overlays, 1)
		require.NotNil(t, msg.Overlays[0].Variant, "the term overlay names the translation under no language")
		check(t, protoconvert.ProtoToBlock(msg))
	})
	t.Run("content block", func(t *testing.T) {
		cb := protoconvert.PartToContentBlock(&model.Part{Type: model.PartBlock, Resource: block()})
		check(t, protoconvert.ContentBlockToPart(cb).Resource.(*model.Block))
	})
}

// An overlay message whose variant is present and empty sits on the
// translation filed under no language, as it always has on this wire; one with
// no variant sits on the source.
func TestAnEmptyVariantMessageNamesTheTranslationUnderNoLanguage(t *testing.T) {
	term := func(variant *pb.VariantMessage) *pb.OverlayMessage {
		return &pb.OverlayMessage{Type: string(model.OverlayTerm), Variant: variant, Spans: []*pb.SpanMessage{{Id: "t1"}}}
	}
	got := protoconvert.ProtoToBlock(&pb.BlockMessage{
		Id:       "b1",
		Overlays: []*pb.OverlayMessage{term(&pb.VariantMessage{}), term(nil)},
	})
	require.Len(t, got.Overlays, 1)
	assert.True(t, got.Overlays[0].OnSource())
	require.Len(t, got.UnlabelledOverlays(), 1)
	assert.Equal(t, model.OverlayTerm, got.UnlabelledOverlays()[0].Type)
}
