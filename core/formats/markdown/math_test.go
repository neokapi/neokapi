package markdown_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Inline <math> inside a paragraph rides as an inline placeholder run whose Data
// carries the full MathML markup, tagged distinctly (fmt:math / md:math-inline)
// so editors and markdown preview can recognize and render it — and the document
// round-trips byte-exact.
func TestInlineMathTaggedForEditors(t *testing.T) {
	input := "The area is <math><mi>r</mi></math> per unit.\n"
	parts := readParts(t, input)

	var mathRun *model.PcOpenRun
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		for _, run := range p.Resource.(*model.Block).Source {
			if run.PcOpen != nil && run.PcOpen.Type == "fmt:math" {
				mathRun = run.PcOpen
			}
		}
	}
	require.NotNil(t, mathRun, "inline <math> should surface as a fmt:math placeholder run")
	assert.Equal(t, "md:math-inline", mathRun.SubType)
	assert.Contains(t, mathRun.Data, "<math>", "the MathML markup is carried on the run for rendering")
	assert.Contains(t, mathRun.Data, "<mi>r</mi>")

	assert.Equal(t, input, roundtripWithSkeleton(t, input), "round-trip stays byte-exact")
}
