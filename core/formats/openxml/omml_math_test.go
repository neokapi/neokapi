package openxml

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// findOMathPh returns the first OMML placeholder run across all blocks.
func findOMathPh(parts []*model.Part) *model.PlaceholderRun {
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b, ok := p.Resource.(*model.Block)
		if !ok {
			continue
		}
		for _, r := range b.Source {
			if r.Ph != nil && r.Ph.SubType == SubTypeOMath {
				return r.Ph
			}
		}
	}
	return nil
}

// ommlToMathEquiv converts an OMML fragment to ($/$$-wrapped, bare) LaTeX.
func TestOMMLToMathEquiv(t *testing.T) {
	inline := `<m:oMath xmlns:m="http://schemas.openxmlformats.org/officeDocument/2006/math">` +
		`<m:f><m:num><m:r><m:t>a</m:t></m:r></m:num><m:den><m:r><m:t>b</m:t></m:r></m:den></m:f></m:oMath>`
	equiv, disp := ommlToMathEquiv(inline)
	assert.Equal(t, `$\frac{a}{b}$`, equiv)
	assert.Equal(t, `\frac{a}{b}`, disp)

	block := `<m:oMathPara xmlns:m="http://schemas.openxmlformats.org/officeDocument/2006/math">` +
		`<m:oMath><m:r><m:t>x</m:t></m:r></m:oMath></m:oMathPara>`
	eq, _ := ommlToMathEquiv(block)
	assert.True(t, strings.HasPrefix(eq, "$$") && strings.HasSuffix(eq, "$$"), "block math uses $$: %q", eq)

	e, d := ommlToMathEquiv("not omml")
	assert.Empty(t, e)
	assert.Empty(t, d)
}

// End-to-end: an inline OMML equation in a text paragraph is surfaced with a
// portable LaTeX rendering in the placeholder's Equiv (markdown $…$) and bare
// LaTeX in Disp, while the opaque OMML stays in Ph.Data for byte-exact docx
// round-trip. Fixture: a "Here is a math equation—an integral: ∫…" paragraph.
func TestOMMLSurfacedInline(t *testing.T) {
	ph := findOMathPh(readFile(t, "testdata/math_inline.docx"))
	require.NotNil(t, ph, "the inline-math paragraph should yield an OMML placeholder run")

	assert.Contains(t, ph.Data, "<m:oMath", "raw OMML stays in Ph.Data for round-trip")
	require.NotEmpty(t, ph.Equiv, "Equiv carries the cross-format math rendering")
	assert.True(t, strings.HasPrefix(ph.Equiv, "$"), "Equiv wrapped in markdown math delimiters: %q", ph.Equiv)
	assert.Contains(t, ph.Disp, `\int`, "bare LaTeX carries the integral: %q", ph.Disp)
	assert.NotContains(t, ph.Disp, "$", "Disp is bare LaTeX (no delimiters)")
}

// With non-translatable-content surfacing disabled (Okapi-faithful config), the
// equation stays opaque — no LaTeX is produced, the OMML is still preserved.
func TestOMMLOpaqueWhenDisabled(t *testing.T) {
	ph := findOMathPh(readFileWithConfig(t, "testdata/math_inline.docx", func(c *Config) {
		c.SetExtractNonTranslatableContent(false)
	}))
	require.NotNil(t, ph)
	assert.Contains(t, ph.Data, "<m:oMath", "raw OMML still preserved")
	assert.Empty(t, ph.Equiv, "no cross-format LaTeX when surfacing is off")
	assert.Empty(t, ph.Disp)
}
