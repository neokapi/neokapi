package xliff2_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// markdownBlock is a Markdown paragraph as the Markdown reader builds it: a
// bold span and a link whose native form is Markdown syntax.
func markdownBlock() *model.Block {
	b := model.NewRunsBlock("tu2", []model.Run{
		model.TextR("Connect by "),
		model.PcOpenR(model.PcOpenRun{ID: "1", Type: "fmt:bold", Data: "**"}),
		model.TextR("video appointment"),
		model.PcCloseR(model.PcCloseRun{ID: "1", Type: "fmt:bold", Data: "**"}),
		model.TextR(" with the "),
		model.PcOpenR(model.PcOpenRun{ID: "2", Type: "link:hyperlink", Data: "["}),
		model.TextR("Harbor app"),
		model.PcCloseR(model.PcCloseRun{ID: "2", Type: "link:hyperlink", Data: "](https://harbor.example/app)"}),
		model.TextR("."),
	})
	b.SourceLocale = "en"
	return b
}

// A block read from another format is written with its inline codes as XLIFF
// codes, their native form in the unit's originalData, so a translator's tool
// shows them as tags and a read of the file gives back the same codes. Written
// as text, the markup would come back as text, and a merge would refuse every
// translation of the block for the codes it lost.
func TestForeignCodesAreWrittenAsXLIFFCodes(t *testing.T) {
	b := markdownBlock()
	b.SetTarget("nb", &model.Target{Runs: []model.Run{
		model.TextR("Koble til med "),
		model.PcOpenR(model.PcOpenRun{ID: "1", Type: "fmt:bold", Data: "**"}),
		model.TextR("videotime"),
		model.PcCloseR(model.PcCloseRun{ID: "1", Type: "fmt:bold", Data: "**"}),
		model.TextR(" i "),
		model.PcOpenR(model.PcOpenRun{ID: "2", Type: "link:hyperlink", Data: "["}),
		model.TextR("Harbor-appen"),
		model.PcCloseR(model.PcCloseRun{ID: "2", Type: "link:hyperlink", Data: "](https://harbor.example/app)"}),
		model.TextR("."),
	}})
	out := writeBlocksIn(t, "nb", b)
	assert.Contains(t, out, `<source>Connect by <pc id="1" dataRefStart="d1" dataRefEnd="d1">video appointment</pc> with the <pc id="2" dataRefStart="d2" dataRefEnd="d3">Harbor app</pc>.</source>`)
	assert.Contains(t, out, `<data id="d3">](https://harbor.example/app)</data>`)
	assert.NotContains(t, out, "**video", "the markup is not text")

	back := readOneBlock(t, out)
	assert.Equal(t, model.RunsPlaceholderText(b.Source), model.RunsPlaceholderText(back.Source),
		"the codes read back as the codes they were")
	tgt := back.Target("nb")
	require.NotNil(t, tgt)
	assert.Equal(t, `Koble til med <x id="1"/>videotime<x id="/1"/> i <x id="2"/>Harbor-appen<x id="/2"/>.`, model.RunsPlaceholderText(tgt.Runs))
}
