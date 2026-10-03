package host

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// A returned translation that drops a code of its source is refused as
// guard/codes_changed, naming the codes as a read shows them, rather than as
// the first finding of a gate.
func TestCodesChanged(t *testing.T) {
	source := []model.Run{
		model.TextR("Connect by "),
		model.PcOpenR(model.PcOpenRun{ID: "1", Type: "fmt:bold", Data: "**"}),
		model.TextR("video appointment"),
		model.PcCloseR(model.PcCloseRun{ID: "1", Type: "fmt:bold", Data: "**"}),
		model.TextR("."),
	}
	assert.Nil(t, codesChanged(source, []model.Run{
		model.TextR("Koble til med "),
		model.PcOpenR(model.PcOpenRun{ID: "1"}),
		model.TextR("videotime"),
		model.PcCloseR(model.PcCloseRun{ID: "1"}),
		model.TextR("."),
	}), "a translation that keeps every code passes")

	e := codesChanged(source, []model.Run{model.TextR("Koble til med **videotime**.")})
	require.NotNil(t, e)
	assert.Equal(t, change.CodeGuard, e.Code)
	assert.Equal(t, change.SubcodeCodesChanged, e.Subcode)
	assert.Equal(t, `<x id="1"/> <x id="/1"/>`, e.Expected, "in the order the codes stand in the source")
	assert.Empty(t, e.Found)
	assert.Contains(t, e.Error(), `guard (codes_changed): expected <x id="1"/> <x id="/1"/>, found none`)
	assert.NotContains(t, e.Error(), "pc-close")

	assert.Nil(t, codesChanged(source, append(append([]model.Run{}, source...), model.PhR(model.PlaceholderRun{ID: "9"}))),
		"a code the translation adds is the change service's to judge")
	e = codesChanged(source, append(append([]model.Run{}, source[:1]...), model.TextR("videotime."), model.PhR(model.PlaceholderRun{ID: "9"})))
	require.NotNil(t, e)
	assert.Equal(t, `<x id="9/"/>`, e.Found, "beside a dropped code, an added one is named too")
}

// A gate refusal of a merged unit names every failing finding the commit
// check reported for its edition.
func TestEveryFinding(t *testing.T) {
	at := change.Ref{Doc: "docs/a.md", Block: "p", Edition: model.EditionKey{Locale: "nb"}}
	other := change.Ref{Doc: "docs/a.md", Block: "h", Edition: model.EditionKey{Locale: "nb"}}
	e := &change.Error{Code: change.CodeGateFailed, Message: "the edit introduces 2 failing findings"}
	docs := []change.DocResult{{Doc: "docs/a.md", Findings: []change.Finding{
		{Rule: "terms.vocabulary", Message: "say overview page", Fails: true, At: &at},
		{Rule: "placeholder.placeholder", Message: "a code is missing", Fails: true, At: &at},
		{Rule: "terms.vocabulary", Message: "advisory", Fails: false, At: &at},
		{Rule: "terms.vocabulary", Message: "elsewhere", Fails: true, At: &other},
	}}}
	got := everyFinding(e, docs, at)
	assert.Equal(t, "2 failing findings: say overview page; a code is missing", got.Message)
	assert.Equal(t, change.CodeGateFailed, got.Code)
}
