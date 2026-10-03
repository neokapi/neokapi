package tools_test

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/model"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A model reply can name a placeholder the source never had. The translation
// still lands, with the placeholder the model invented kept as an empty code
// for the placeholder checks to flag; one malformed reply does not fail the
// translate stage and the run with it.
func TestAITranslate_AnInventedPlaceholderDoesNotFailTheBlock(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.TranslateFunc = func(_ context.Context, req aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
		return &aiprovider.TranslateResponse{Translation: `Cliquez <x id="1"/>ici<x id="/1"/> <x id="2/"/>`, Model: "test-model"}, nil
	}
	tl := tools.NewAITranslateTool(mock, singleBlockConfig())

	block := model.NewRunsBlock("tu1", []model.Run{
		model.TextR("Click "),
		model.PcOpenR(model.PcOpenRun{ID: "1", Type: "link:hyperlink", Data: `<a href="/x">`}),
		model.TextR("here"),
		model.PcCloseR(model.PcCloseRun{ID: "1", Type: "link:hyperlink", Data: "</a>"}),
	})
	block.SourceLocale = model.LocaleEnglish
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)
	require.NoError(t, tl.Process(t.Context(), in, out))
	close(out)

	got := (<-out).Resource.(*model.Block)
	runs := got.TargetRuns(model.LocaleFrench)
	require.NotEmpty(t, runs)
	assert.Equal(t, "Cliquez ici ", model.RunsText(runs))
	require.NotNil(t, runs[1].PcOpen)
	assert.Equal(t, `<a href="/x">`, runs[1].PcOpen.Data, "the source's link keeps its native form")
	last := runs[len(runs)-1]
	require.NotNil(t, last.Ph, "the invented placeholder is kept for the checks to find")
	assert.Equal(t, "2", last.Ph.ID)
	assert.Empty(t, last.Ph.Data)
	tgt, ok := got.Edition(model.Variant(model.LocaleFrench))
	require.True(t, ok)
	assert.Equal(t, model.Status(model.TargetStatusDraft), tgt.Status)
}
