package venue

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	contentv1 "github.com/neokapi/neokapi/core/proto/content/v1"
	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
)

// A block read from an en-US to en-US file holds a target under its source
// language. The sync wire carries the source, with its status, as the source
// and the target as a target, and the decoded block holds both the same way.
func TestBlockRoundTrip_SameLanguageTarget(t *testing.T) {
	b := model.NewBlock("b1", "colour source")
	b.SourceLocale = "en-US"
	b.SourceStatus = model.SourceStatusEstablished
	b.SetTargetVariant(model.Variant("en-US"), &model.Target{
		Runs:   []model.Run{model.TextR("colour target")},
		Status: model.TargetStatusTranslated,
		Origin: model.Origin{Kind: model.OriginHuman},
	})

	sb := BlockToProto(b, "en.xlf")
	require.Len(t, sb.Source, 1)
	assert.Equal(t, string(model.SourceStatusEstablished), sb.Properties[propSourceStatus])
	require.Contains(t, sb.Targets, "en-US")

	got, err := ProtoToBlock(sb)
	require.NoError(t, err)
	src, ok := got.Edition(model.EditionKey{})
	require.True(t, ok)
	assert.Equal(t, "colour source", model.RunsText(src.Runs))
	assert.Equal(t, model.Status(model.SourceStatusEstablished), src.Status)
	tgt := got.Target("en-US")
	require.NotNil(t, tgt)
	assert.Equal(t, "colour target", model.RunsText(tgt.Runs))
	assert.Equal(t, model.TargetStatusTranslated, tgt.Status)
	assert.Equal(t, model.OriginHuman, tgt.Origin.Kind)
	_, edited := got.SourceAsRead()
	assert.False(t, edited, "a decoded block holds its source as read")
}

// A target filed under a key that is not canonical (nb_NO) is listed by
// Editions under its canonical key, which Edition may not reach. The encoder
// never sends such an edition as an empty target, which a receiver would store
// in place of the translation.
func TestBlockToProto_UnreachableEditionIsNotSentEmpty(t *testing.T) {
	b := model.NewBlock("b1", "Hello")
	b.SourceLocale = "en-US"
	b.Targets[model.VariantKey{Locale: "nb_NO"}] = &model.Target{Runs: []model.Run{model.TextR("Hei")}}
	b.SetTargetVariant(model.Variant("fr-FR"), &model.Target{Runs: []model.Run{model.TextR("Bonjour")}})

	sb := BlockToProto(b, "item")

	require.Contains(t, sb.Targets, "fr-FR")
	for key, list := range sb.Targets {
		require.Len(t, list.Segments, 1, key)
		assert.NotEmpty(t, list.Segments[0].Runs, "target %s goes out empty", key)
	}
	if list, ok := sb.Targets["nb-NO"]; ok {
		got, err := ProtoToBlock(&pb.SyncBlock{Targets: map[string]*pb.SyncSegmentList{"nb-NO": list}})
		require.NoError(t, err)
		assert.Equal(t, "Hei", got.TargetText("nb-NO"))
	}
}

// A target key that names no language names no target. BlockToProto never
// sends one, and ProtoToBlock stores none: the block holds its source alone.
func TestProtoToBlock_KeyWithNoLanguageIsNoTarget(t *testing.T) {
	got, err := ProtoToBlock(&pb.SyncBlock{
		SourceText: "Hello",
		Targets: map[string]*pb.SyncSegmentList{
			"": {Segments: []*contentv1.SegmentMessage{runsToSegment("", []model.Run{model.TextR("Hei")})}},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, []model.EditionKey{{}}, got.Editions())
	assert.Empty(t, got.TargetLocales())
	assert.Equal(t, "Hello", got.SourceText())
}
