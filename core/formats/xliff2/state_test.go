package xliff2_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/formats/xliff2"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeStatefulBlock writes one block with the given target status to an XLIFF 2
// document (the scratch-build path) and returns the serialized output.
func writeStatefulBlock(t *testing.T, status model.TargetStatus) string {
	t.Helper()
	buf := &bytes.Buffer{}
	w := xliff2.NewWriter()
	require.NoError(t, w.SetOutputWriter(buf))

	layer := &model.Layer{
		ID: "file-f1", Name: "f1", Format: "xliff2", Locale: "en", IsMultilingual: true,
		Properties: map[string]string{"target-language": "fr"},
	}
	block := &model.Block{ID: "u1", Translatable: true, Source: []model.Run{{Text: &model.TextRun{Text: "Hello"}}}}
	span := []model.Span{{ID: "s1", Range: model.RunRange{StartRun: 0, EndRun: 1}}}
	block.SetSegmentation(nil, span)
	block.SetTargetRuns(model.LocaleFrench, []model.Run{{Text: &model.TextRun{Text: "Bonjour"}}})
	tgtKey := model.Variant(model.LocaleFrench)
	block.SetSegmentation(&tgtKey, span)
	block.StampTargetProvenance(model.LocaleFrench, status, model.Origin{Kind: model.OriginHuman})

	parts := make(chan *model.Part, 2)
	parts <- &model.Part{Type: model.PartLayerStart, Resource: layer}
	parts <- &model.Part{Type: model.PartBlock, Resource: block}
	close(parts)
	require.NoError(t, w.Write(context.Background(), parts))
	return buf.String()
}

// TestWriteXLIFF2_TargetState verifies the writer emits the segment `state` from
// the target's lifecycle status on the scratch-build path (e.g. kapi extract).
func TestWriteXLIFF2_TargetState(t *testing.T) {
	assert.Contains(t, writeStatefulBlock(t, model.TargetStatusReviewed), `state="reviewed"`)
	assert.Contains(t, writeStatefulBlock(t, model.TargetStatusSignedOff), `state="final"`)
	assert.Contains(t, writeStatefulBlock(t, model.TargetStatusDraft), `state="translated"`,
		"a draft is a translation awaiting review")
	// An unset status emits no state attribute (XLIFF defaults to initial).
	assert.NotContains(t, writeStatefulBlock(t, model.TargetStatusNew), `state=`)
}

// TestXLIFF2_StateRoundTrip closes the loop: a status written out is read back.
func TestXLIFF2_StateRoundTrip(t *testing.T) {
	output := writeStatefulBlock(t, model.TargetStatusSignedOff)

	reader := xliff2.NewReader()
	require.NoError(t, reader.Open(t.Context(), testutil.RawDocFromString(output, model.LocaleEnglish)))
	defer reader.Close()
	blocks := testutil.CollectBlocks(t, reader.Read(t.Context()))

	require.Len(t, blocks, 1)
	tgt := blocks[0].Target(model.LocaleFrench)
	require.NotNil(t, tgt)
	assert.Equal(t, model.TargetStatusSignedOff, tgt.Status, "signed-off → final → signed-off")
}

const statefulXLIFF2 = `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="2.0" xmlns="urn:oasis:names:tc:xliff:document:2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="u1"><segment state="reviewed"><source>Hello</source><target>Bonjour</target></segment></unit>
    <unit id="u2"><segment state="final"><source>Bye</source><target>Au revoir</target></segment></unit>
    <unit id="u3"><segment state="translated"><source>Yes</source><target>Oui</target></segment></unit>
    <unit id="u4"><segment><source>No</source><target>Non</target></segment></unit>
  </file>
</xliff>`

// TestReadXLIFF2_TargetState verifies that the XLIFF 2 segment `state` is mapped
// onto the target's lifecycle status, so coverage and ship gates can see review
// progress that arrived over the interchange.
func TestReadXLIFF2_TargetState(t *testing.T) {
	ctx := t.Context()
	reader := xliff2.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(statefulXLIFF2, model.LocaleEnglish)))
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 4)

	status := func(b *model.Block) model.TargetStatus {
		tgt := b.Target(model.LocaleFrench)
		require.NotNil(t, tgt)
		return tgt.Status
	}
	assert.Equal(t, model.TargetStatusReviewed, status(blocks[0]), "state=reviewed")
	assert.Equal(t, model.TargetStatusSignedOff, status(blocks[1]), "state=final → signed-off")
	assert.Equal(t, model.TargetStatusTranslated, status(blocks[2]), "state=translated")
	assert.Equal(t, model.TargetStatusNew, status(blocks[3]), "no state → unset (presence baseline applies)")
}
