package ts_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/formats/ts"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// noLanguageNumerus is a numerus message in a file that names no language.
const noLanguageNumerus = `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.0">
<context>
    <name>Ctx</name>
    <message numerus="yes">
        <source>Some files</source>
        <translation>
            <numerusform>Eine Datei</numerusform>
            <numerusform>Viele Dateien</numerusform>
        </translation>
    </message>
</context>
</TS>
`

// Read with no source locale, a file that names no language files its
// translation under no language. The spans that divide that translation into
// its numerus forms go with it: the source stays one unsegmented segment, and
// the writer divides the translation into the forms it read.
func TestNoLanguageNumerusFormsStayOnTheTranslation(t *testing.T) {
	ctx := t.Context()
	reader := ts.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(noLanguageNumerus, "")))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	require.NoError(t, reader.Close())

	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 1)
	b := blocks[0]

	assert.Equal(t, "Some files", b.SourceText())
	assert.Nil(t, b.SourceSegmentation(), "the source carries no segmentation")
	assert.Equal(t, 1, b.SourceSegmentCount())
	assert.Empty(t, b.Overlays, "no overlay names the source")

	assert.Equal(t, "Eine DateiViele Dateien", model.RunsText(b.TargetRuns("")))
	seg := b.TargetSegmentation("")
	require.NotNil(t, seg, "the numerus forms divide the translation under no language")
	require.Len(t, seg.Spans, 2)
	assert.Equal(t, "Eine Datei", model.RunsText(seg.Spans[0].Range.ExtractRuns(b.TargetRuns(""))))
	assert.Equal(t, "Viele Dateien", model.RunsText(seg.Spans[1].Range.ExtractRuns(b.TargetRuns(""))))

	var buf bytes.Buffer
	w := ts.NewWriter()
	require.NoError(t, w.SetOutputWriter(&buf))
	require.NoError(t, w.Write(ctx, testutil.PartsToChannel(parts)))
	require.NoError(t, w.Close())
	out := buf.String()
	assert.Contains(t, out, "<numerusform>Eine Datei</numerusform>")
	assert.Contains(t, out, "<numerusform>Viele Dateien</numerusform>")
	assert.Contains(t, out, "<source>Some files</source>")
}
