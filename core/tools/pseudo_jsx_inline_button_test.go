package tools_test

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The shape @neokapi/i18n-react emits for a sentence with a button in it: one
// block, the button a paired code, and the button's label an ordinary
// translatable run inside the pair. A code span differs on the last point, and
// TestPseudoTranslateKeepsAJSXCodeSpan covers that side.
//
// Both halves of the sentence must come out accented in the qps probe, because
// a label that stayed English is exactly the symptom the probe exists to show,
// and the pair must survive so `tx()` can re-attach the button around the
// translated label.
func TestPseudoTranslateAJSXButtonInProse(t *testing.T) {
	t.Parallel()
	cfg := &tools.PseudoConfig{Prefix: "[", Suffix: "]", TargetLocale: "qps"}
	tl := tools.NewPseudoTranslateTool(cfg)

	block := model.NewBlock("tu1", "")
	block.SetSourceRuns([]model.Run{
		{Text: &model.TextRun{Text: "Press "}},
		{PcOpen: &model.PcOpenRun{ID: "0", Type: "jsx:element", SubType: "button", Data: `<button type="button">`, Equiv: "=m0"}},
		{Text: &model.TextRun{Text: "Try it live"}},
		{PcClose: &model.PcCloseRun{ID: "0", Type: "jsx:element", SubType: "button", Data: "</button>", Equiv: "=m0"}},
		{Text: &model.TextRun{Text: " to list the registered formats."}},
	})

	result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: block})
	runs := result.Resource.(*model.Block).TargetRuns("qps")

	var opens, closes, protected int
	for _, r := range runs {
		switch {
		case r.Text != nil && r.Text.NoTranslate:
			protected++
		case r.PcOpen != nil:
			opens++
			assert.Equal(t, "button", r.PcOpen.SubType)
			assert.Equal(t, "=m0", r.PcOpen.Equiv, "the equiv tx() binds the element to")
			assert.Equal(t, `<button type="button">`, r.PcOpen.Data, "the opening tag is carried verbatim")
		case r.PcClose != nil:
			closes++
			assert.Equal(t, "=m0", r.PcClose.Equiv)
		}
	}
	require.Equal(t, 1, opens, "the button is still a paired code: %v", runs)
	require.Equal(t, 1, closes)
	assert.Zero(t, protected, "a button label is prose, so nothing here is protected")

	flat := model.FlattenRuns(runs)
	assert.NotContains(t, flat, "Press", "the prose before the button is pseudo-translated")
	assert.NotContains(t, flat, "Try it live", "and so is the label inside it")
	assert.NotContains(t, flat, "registered", "and so is the prose after it")
	assert.True(t, strings.Contains(flat, "š"), "accented throughout: %q", flat)

	// The element structure a writer splices back: the tags the author wrote,
	// with the accented label between them.
	rendered := model.RenderRunsWithData(runs)
	assert.Contains(t, rendered, `<button type="button">`)
	assert.Contains(t, rendered, "</button>")
}
