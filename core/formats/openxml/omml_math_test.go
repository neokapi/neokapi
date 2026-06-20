package openxml

import (
	"os"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// findFormulaBlock returns the first RoleFormula block in the part stream.
func findFormulaBlock(parts []*model.Part) *model.Block {
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		if b, ok := p.Resource.(*model.Block); ok && b.SemanticRole() == model.RoleFormula {
			return b
		}
	}
	return nil
}

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

// A standalone display equation (an equation-only paragraph) — previously
// dropped to skeleton with no block — now also surfaces a detached
// non-translatable RoleFormula block carrying the portable math, so
// cross-format export renders it. Fixture: equation.docx ((x+a)^n = ∑…).
func TestOMMLStandaloneSurfaced(t *testing.T) {
	blk := findFormulaBlock(readFile(t, "testdata/math_block.docx"))
	require.NotNil(t, blk, "standalone equation should surface as a RoleFormula block")
	assert.False(t, blk.Translatable, "math is non-translatable")
	require.Len(t, blk.Source, 1)
	ph := blk.Source[0].Ph
	require.NotNil(t, ph, "the formula block carries an OMML placeholder run")
	assert.True(t, strings.HasPrefix(ph.Equiv, "$$"), "display math wrapped in $$: %q", ph.Equiv)
	assert.Contains(t, ph.Disp, `\sum`, "bare LaTeX carries the summation")
	assert.Contains(t, ph.Data, "<m:oMath", "opaque OMML preserved for docx round-trip")
}

// Disabling surfacing yields no formula block for a standalone equation.
func TestOMMLStandaloneOpaqueWhenDisabled(t *testing.T) {
	blk := findFormulaBlock(readFileWithConfig(t, "testdata/math_block.docx", func(c *Config) {
		c.SetExtractNonTranslatableContent(false)
	}))
	assert.Nil(t, blk, "no RoleFormula block when surfacing is off")
}

// The detached formula block is NOT skeleton-referenced, so surfacing has zero
// effect on the docx→docx output: the round-trip is byte-identical whether math
// surfacing is on or off. (This fixture does not itself round-trip byte-exact to
// source — the writer normalizes it — but my change must not alter that output.)
func TestOMMLStandaloneRoundtripUnaffected(t *testing.T) {
	input, err := os.ReadFile("testdata/math_block.docx")
	require.NoError(t, err)
	on := roundtripUntranslatedConfig(t, input, "math_block.docx", nil)
	off := roundtripUntranslatedConfig(t, input, "math_block.docx", func(c *Config) {
		c.SetExtractNonTranslatableContent(false)
	})
	assert.Equal(t, off, on, "math surfacing must not change the docx round-trip output")
}
