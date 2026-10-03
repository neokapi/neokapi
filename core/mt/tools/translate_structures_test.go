package tools_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/mt/tools"
	mtprovider "github.com/neokapi/neokapi/providers/mt"
)

// A block holding a select is translated a branch at a time, and its target
// holds the select with every case translated; a branch with an inline code
// goes through semantic HTML and keeps it.
func TestMTTranslateToolKeepsEveryBranch(t *testing.T) {
	mock := newMock("test-mt")
	var sources []string
	mock.translateFn = func(_ context.Context, req mtprovider.TranslateRequest) (*mtprovider.TranslateResponse, error) {
		sources = append(sources, req.Source)
		return &mtprovider.TranslateResponse{Translation: "[fr] " + req.Source}, nil
	}
	name := model.PhR(model.PlaceholderRun{ID: "p1", Type: "icu", Data: "{name}", Equiv: "{name}"})
	block := model.NewRunsBlock("tu1", []model.Run{{Select: &model.SelectRun{Pivot: "gender", Cases: map[string][]model.Run{
		"female": {model.TextR("She replied")},
		"other":  {model.TextR("They replied to "), name},
	}}}})
	tl := tools.NewMTTranslateTool(mock, tools.MTTranslateConfig{SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench})
	got := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: block}).Resource.(*model.Block)

	runs := got.TargetRuns(model.LocaleFrench)
	require.Len(t, runs, 1)
	require.NotNil(t, runs[0].Select, "the select is still a select")
	assert.Equal(t, []model.Run{model.TextR("[fr] She replied")}, runs[0].Select.Cases["female"])
	other := runs[0].Select.Cases["other"]
	require.Len(t, other, 2)
	assert.Equal(t, "[fr] They replied to ", other[0].Text.Text)
	require.NotNil(t, other[1].Ph)
	assert.Equal(t, "{name}", other[1].Ph.Data, "the code keeps its native form")
	assert.Len(t, sources, 2, "one call per case; nothing around the select to translate")
	assert.Equal(t, model.TargetStatusDraft, got.Target(model.LocaleFrench).Status)
}
