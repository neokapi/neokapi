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
	b.SetEditionStatus(model.EditionKey{}, model.Status(model.SourceStatusEstablished))
	b.SetTargetRuns("en-US", nil)
	b.SetEdition(model.Variant("en-US"), model.Edition{
		Runs:   []model.Run{model.TextR("colour target")},
		Status: model.Status(model.TargetStatusTranslated),
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
	tgt, ok := got.Edition(model.Variant("en-US"))
	require.True(t, ok)
	assert.Equal(t, "colour target", model.RunsText(tgt.Runs))
	assert.Equal(t, model.Status(model.TargetStatusTranslated), tgt.Status)
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
	b.FileTargetAsSpelled(model.EditionKey{Locale: "nb_NO"}, model.Edition{Runs: []model.Run{model.TextR("Hei")}})
	b.SetEdition(model.Variant("fr-FR"), model.Edition{Runs: []model.Run{model.TextR("Bonjour")}})

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

// A target key that names no language is filed as a target under no language,
// with its status, origin and score, and the source stays as the wire sent it.
// No edition accessor lists that target, so BlockToProto sends none back.
func TestProtoToBlock_KeyWithNoLanguageIsATarget(t *testing.T) {
	sent := model.Edition{
		Runs:   []model.Run{model.TextR("Hei")},
		Status: model.Status(model.TargetStatusTranslated),
		Origin: model.Origin{Kind: model.OriginAI, ContextFingerprint: "fp-zero"},
		Score:  0.5,
	}
	got, err := ProtoToBlock(&pb.SyncBlock{
		SourceText:   "Hello",
		SourceLocale: "en-US",
		Targets: map[string]*pb.SyncSegmentList{
			"": {Segments: []*contentv1.SegmentMessage{targetToSegment(sent)}},
		},
	})
	require.NoError(t, err)

	tgt, ok := got.TargetEdition("")
	require.True(t, ok, "the target filed under no language")
	assert.Equal(t, sent, tgt)
	assert.Equal(t, []model.LocaleID{""}, got.TargetLocales())
	src, ok := got.Edition(model.EditionKey{})
	require.True(t, ok)
	assert.Equal(t, "Hello", model.RunsText(src.Runs))
	_, edited := got.SourceAsRead()
	assert.False(t, edited, "a decoded block holds its source as read")
	assert.Equal(t, []model.EditionKey{{Locale: "en-US"}}, got.Editions())

	assert.NotContains(t, BlockToProto(got, "item").Targets, "")
}
