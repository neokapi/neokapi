package xliff2_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/formats/xliff2"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
