package model_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newVocabRegistry(t *testing.T) *model.VocabularyRegistry {
	t.Helper()
	reg := model.NewVocabularyRegistry()
	require.NoError(t, reg.LoadDefaults())
	return reg
}

func TestRunsSemanticHTML(t *testing.T) {
	reg := newVocabRegistry(t)

	runs := []model.Run{
		{Text: &model.TextRun{Text: "Click "}},
		{PcOpen: &model.PcOpenRun{ID: "1", Type: "fmt:bold", Data: `<b class="x">`}},
		{Text: &model.TextRun{Text: "here"}},
		{PcClose: &model.PcCloseRun{ID: "1", Type: "fmt:bold", Data: "</b>"}},
		{Text: &model.TextRun{Text: " for info"}},
	}

	assert.Equal(t, "Click <b>here</b> for info", model.RunsSemanticHTML(runs, reg))
}

func TestRunsSemanticHTMLPlaceholder(t *testing.T) {
	reg := newVocabRegistry(t)

	runs := []model.Run{
		{Text: &model.TextRun{Text: "Line one"}},
		{Ph: &model.PlaceholderRun{ID: "1", Type: "struct:break", Data: "<br/>"}},
		{Text: &model.TextRun{Text: "Line two"}},
	}

	assert.Equal(t, "Line one<br/>Line two", model.RunsSemanticHTML(runs, reg))
}

func TestRunsSemanticHTMLUnknownType(t *testing.T) {
	reg := newVocabRegistry(t)

	runs := []model.Run{
		{Text: &model.TextRun{Text: "Hello "}},
		{PcOpen: &model.PcOpenRun{ID: "1", Type: "custom:foo", Data: "<x>"}},
		{Text: &model.TextRun{Text: "world"}},
		{PcClose: &model.PcCloseRun{ID: "1", Type: "custom:foo", Data: "</x>"}},
	}

	assert.Equal(t, `Hello <span data-type="custom:foo">world</span>`,
		model.RunsSemanticHTML(runs, reg))
}

func TestRunsSemanticHTMLNoRuns(t *testing.T) {
	reg := newVocabRegistry(t)
	assert.Empty(t, model.RunsSemanticHTML(nil, reg))
}

func TestRunsSemanticHTMLPlainText(t *testing.T) {
	reg := newVocabRegistry(t)
	runs := []model.Run{{Text: &model.TextRun{Text: "plain text"}}}
	assert.Equal(t, "plain text", model.RunsSemanticHTML(runs, reg))
}

func TestParseRunsSemanticHTML(t *testing.T) {
	reg := newVocabRegistry(t)

	sourceRuns := []model.Run{
		{PcOpen: &model.PcOpenRun{ID: "1", Type: "fmt:bold", Data: `<b class="emphasis">`, Disp: "[B]"}},
		{PcClose: &model.PcCloseRun{ID: "1", Type: "fmt:bold", Data: "</b>"}},
	}

	response := "Cliquez <b>ici</b>"

	runs := model.ParseRunsSemanticHTML(response, sourceRuns, reg)
	assert.Equal(t, "Cliquez ici", model.FlattenRuns(runs))
	require.Len(t, runs, 4)

	require.NotNil(t, runs[1].PcOpen)
	assert.Equal(t, "fmt:bold", runs[1].PcOpen.Type)
	assert.Equal(t, `<b class="emphasis">`, runs[1].PcOpen.Data)
	assert.Equal(t, "[B]", runs[1].PcOpen.Disp)

	require.NotNil(t, runs[3].PcClose)
	assert.Equal(t, "</b>", runs[3].PcClose.Data)
}

func TestParseRunsSemanticHTMLWithPlaceholder(t *testing.T) {
	reg := newVocabRegistry(t)

	sourceRuns := []model.Run{
		{Ph: &model.PlaceholderRun{ID: "1", Type: "struct:break", Data: "<br/>"}},
	}

	response := "Line one<br/>Line two"

	runs := model.ParseRunsSemanticHTML(response, sourceRuns, reg)

	var phs int
	var textParts []string
	for _, r := range runs {
		switch {
		case r.Text != nil:
			textParts = append(textParts, r.Text.Text)
		case r.Ph != nil:
			phs++
			assert.Equal(t, "<br/>", r.Ph.Data)
		}
	}
	assert.Equal(t, 1, phs)
	require.Len(t, textParts, 2)
	assert.Equal(t, "Line one", textParts[0])
	assert.Equal(t, "Line two", textParts[1])
}

func TestParseRunsSemanticHTMLAssignsIDsToExtraTags(t *testing.T) {
	reg := newVocabRegistry(t)

	sourceRuns := []model.Run{
		{PcOpen: &model.PcOpenRun{ID: "1", Type: "fmt:bold", Data: `<b>`}},
		{PcClose: &model.PcCloseRun{ID: "1", Type: "fmt:bold", Data: "</b>"}},
	}

	// MT response added an extra <i>…</i> the source did not contain.
	response := "<i>Hola</i> <b>mundo</b>"

	runs := model.ParseRunsSemanticHTML(response, sourceRuns, reg)

	var ids []string
	for _, r := range runs {
		switch {
		case r.PcOpen != nil:
			ids = append(ids, "open:"+r.PcOpen.ID)
		case r.PcClose != nil:
			ids = append(ids, "close:"+r.PcClose.ID)
		}
	}
	require.Len(t, ids, 4)
	// The source-matched <b>/</b> pair carries id="1"; the synthetic
	// <i>/</i> pair gets a fresh sequential id.
	assert.Contains(t, ids, "open:1")
	assert.Contains(t, ids, "close:1")
}
