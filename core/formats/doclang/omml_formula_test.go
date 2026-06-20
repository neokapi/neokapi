package doclang_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
)

// A RoleFormula block sourced from a foreign format (an OMML equation surfaced by
// the OpenXML reader as a placeholder run carrying bare LaTeX in Disp) is written
// as <formula>LaTeX</formula> — DocLang mandates bare LaTeX (no $…$ wrapping),
// so the writer uses Disp, not the markdown-wrapped Equiv.
func TestFormulaBlockFromForeignMath(t *testing.T) {
	blk := model.NewBlock("b1", "")
	blk.Translatable = false
	blk.Type = "math"
	blk.SetSemanticRole(model.RoleFormula, 0)
	blk.Source = []model.Run{{Ph: &model.PlaceholderRun{
		ID:    "c1",
		Type:  "opaque-para-child",
		Data:  "<m:oMath><m:r><m:t>x</m:t></m:r></m:oMath>",
		Equiv: `$$\int_a^b x\,dx$$`, // markdown form — must NOT leak into DocLang
		Disp:  `\int_a^b x\,dx`,     // bare LaTeX — the DocLang form
	}}}

	out := string(renderParts(t, []*model.Part{{Type: model.PartBlock, Resource: blk}}))
	assert.Contains(t, out, `<formula>\int_a^b x\,dx</formula>`, "formula carries bare LaTeX")
	assert.NotContains(t, out, "$$", "no markdown math delimiters leak into DocLang")
}
