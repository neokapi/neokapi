package tools_test

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The shape @neokapi/i18n-react emits for a paragraph that mentions a format id
// or a flag: the sentence is one block, the `<code>` element is a paired code,
// and what it holds is a protected text run.
//
// The qps probe locale is where a reader sees whether that held. A command that
// came out accented is a command nobody can type, and the docs site is the
// surface it reaches, so the two halves are asserted together here rather than
// each side asserting its own half.
func TestPseudoTranslateKeepsAJSXCodeSpan(t *testing.T) {
	t.Parallel()
	cfg := &tools.PseudoConfig{Prefix: "[", Suffix: "]", TargetLocale: "qps"}
	tl := tools.NewPseudoTranslateTool(cfg)

	block := model.NewBlock("tu1", "")
	block.SetSourceRuns([]model.Run{
		{Text: &model.TextRun{Text: "Say "}},
		{PcOpen: &model.PcOpenRun{ID: "0", Type: "jsx:element", SubType: "code", Data: "<code>", Equiv: "=m0"}},
		{Text: &model.TextRun{Text: "json", NoTranslate: true}},
		{PcClose: &model.PcCloseRun{ID: "0", Type: "jsx:element", SubType: "code", Data: "</code>", Equiv: "=m0"}},
		{Text: &model.TextRun{Text: " for the faithful readers."}},
	})

	result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: block})
	runs := result.Resource.(*model.Block).TargetRuns("qps")

	var protected, opens, closes int
	for _, r := range runs {
		switch {
		case r.Text != nil && r.Text.NoTranslate:
			protected++
			assert.Equal(t, "json", r.Text.Text, "the code span reads as the author wrote it")
		case r.PcOpen != nil:
			opens++
		case r.PcClose != nil:
			closes++
		}
	}
	require.Equal(t, 1, protected, "exactly one protected run, still marked: %v", runs)
	// The paired code survives, so the compiled dictionary entry still carries
	// the `{=m0}` markers `tx()` re-attaches the element with.
	assert.Equal(t, 1, opens)
	assert.Equal(t, 1, closes)

	flat := model.FlattenRuns(runs)
	assert.Contains(t, flat, "json", "the command is typeable")
	assert.NotContains(t, flat, "Say", "the prose before it is pseudo-translated")
	assert.NotContains(t, flat, "faithful", "and so is the prose after it")
	assert.True(t, strings.Contains(flat, "š"), "accented prose: %q", flat)
}
