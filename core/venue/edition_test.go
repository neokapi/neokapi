package venue

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
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
