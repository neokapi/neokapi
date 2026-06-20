package math_test

import (
	"strings"
	"testing"

	math "github.com/neokapi/neokapi/core/math"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func conv(t *testing.T, omml string) *math.Math {
	t.Helper()
	m, err := math.FromOMML([]byte(omml))
	require.NoError(t, err)
	require.NotNil(t, m)
	return m
}

// A simple fraction a/b.
func TestFraction(t *testing.T) {
	omml := `<m:oMath xmlns:m="` + nsM + `"><m:f><m:num><m:r><m:t>a</m:t></m:r></m:num>` +
		`<m:den><m:r><m:t>b</m:t></m:r></m:den></m:f></m:oMath>`
	m := conv(t, omml)
	assert.Equal(t, `\frac{a}{b}`, m.ToLaTeX())
	assert.Contains(t, m.ToMathML(), "<mfrac><mi>a</mi><mi>b</mi></mfrac>")
	assert.False(t, m.Block)
}

// Superscript with a compound base: (x+a)^n.
func TestSuperscriptDelimited(t *testing.T) {
	omml := `<m:oMath xmlns:m="` + nsM + `">` +
		`<m:sSup><m:e><m:d><m:e><m:r><m:t>x+a</m:t></m:r></m:e></m:d></m:e>` +
		`<m:sup><m:r><m:t>n</m:t></m:r></m:sup></m:sSup></m:oMath>`
	m := conv(t, omml)
	assert.Equal(t, `\left(x+a\right)^{n}`, m.ToLaTeX())
	mml := m.ToMathML()
	assert.Contains(t, mml, "<msup>")
	assert.Contains(t, mml, `<mo fence="true">(</mo>`)
	assert.Contains(t, mml, "<mi>x</mi><mo>+</mo><mi>a</mi>")
}

// n-ary summation with limits: ∑_{k=0}^{n} k.
func TestNarySum(t *testing.T) {
	omml := `<m:oMath xmlns:m="` + nsM + `"><m:nary><m:naryPr><m:chr m:val="∑"/></m:naryPr>` +
		`<m:sub><m:r><m:t>k=0</m:t></m:r></m:sub><m:sup><m:r><m:t>n</m:t></m:r></m:sup>` +
		`<m:e><m:r><m:t>k</m:t></m:r></m:e></m:nary></m:oMath>`
	m := conv(t, omml)
	assert.Equal(t, `\sum_{k=0}^{n}k`, m.ToLaTeX())
	assert.Contains(t, m.ToMathML(), "<munderover><mo>∑</mo>")
}

// Square root and a binomial (noBar fraction).
func TestRadicalAndBinomial(t *testing.T) {
	sqrt := conv(t, `<m:oMath xmlns:m="`+nsM+`"><m:rad><m:radPr><m:degHide m:val="1"/></m:radPr>`+
		`<m:deg/><m:e><m:r><m:t>2</m:t></m:r></m:e></m:rad></m:oMath>`)
	assert.Equal(t, `\sqrt{2}`, sqrt.ToLaTeX())
	assert.Contains(t, sqrt.ToMathML(), "<msqrt><mn>2</mn></msqrt>")

	binom := conv(t, `<m:oMath xmlns:m="`+nsM+`"><m:f><m:fPr><m:type m:val="noBar"/></m:fPr>`+
		`<m:num><m:r><m:t>n</m:t></m:r></m:num><m:den><m:r><m:t>k</m:t></m:r></m:den></m:f></m:oMath>`)
	assert.Equal(t, `{n\atop k}`, binom.ToLaTeX())
	assert.Contains(t, binom.ToMathML(), `<mfrac linethickness="0">`)
}

// Block equation (<m:oMathPara>) sets Block and MathML display="block".
func TestBlockEquation(t *testing.T) {
	m := conv(t, `<m:oMathPara xmlns:m="`+nsM+`"><m:oMath><m:r><m:t>E=mc</m:t></m:r>`+
		`<m:sSup><m:e><m:r><m:t></m:t></m:r></m:e><m:sup><m:r><m:t>2</m:t></m:r></m:sup></m:sSup></m:oMath></m:oMathPara>`)
	assert.True(t, m.Block)
	assert.Contains(t, m.ToMathML(), `display="block"`)
	assert.Contains(t, m.ToLaTeX(), "E=mc")
}

// Normal-text (<m:nor/>) runs are the translatable prose; math glyphs are not.
func TestTranslatableNormalText(t *testing.T) {
	omml := `<m:oMath xmlns:m="` + nsM + `">` +
		`<m:r><m:t>x</m:t></m:r>` +
		`<m:r><m:rPr><m:nor/></m:rPr><m:t>for all</m:t></m:r>` +
		`<m:r><m:t>x</m:t></m:r></m:oMath>`
	m := conv(t, omml)
	assert.Equal(t, "for all", m.TranslatableText(), "only the m:nor run is translatable prose")
	assert.Contains(t, m.ToLaTeX(), `\text{for all}`)
	// The math letters are NOT in the translatable surface.
	assert.NotContains(t, m.TranslatableText(), "x")
}

// Unsupported / malformed input degrades gracefully, never panics or errors.
func TestGracefulFallback(t *testing.T) {
	m, err := math.FromOMML([]byte(`<m:oMath xmlns:m="` + nsM + `"><m:weird><m:r><m:t>z</m:t></m:r></m:weird></m:oMath>`))
	require.NoError(t, err)
	assert.NotNil(t, m)
	empty, err := math.FromOMML([]byte(`not xml at all <<<`))
	_ = err // malformed XML may error; must not panic
	_ = empty
}

const nsM = "http://schemas.openxmlformats.org/officeDocument/2006/math"

// Sanity: the canonical equation.docx body converts without panic and yields
// recognizable LaTeX/MathML structure.
func TestCanonicalEquationSmoke(t *testing.T) {
	m := conv(t, canonicalOMML)
	latex := m.ToLaTeX()
	// The binomial (n k) is a noBar fraction → \atop, not \frac.
	for _, want := range []string{`\left(`, `^{n}`, `\sum`, `\atop`} {
		assert.Contains(t, latex, want, "latex should contain %q", want)
	}
	mml := m.ToMathML()
	assert.True(t, strings.HasPrefix(mml, "<math"))
	assert.Contains(t, mml, "<msup>")
	assert.Contains(t, mml, `<mfrac linethickness="0">`)
}

// A trimmed slice of equation.docx's OMML: (x+a)^n = ∑_{k=0}^{n} (n/k) ...
const canonicalOMML = `<m:oMath xmlns:m="http://schemas.openxmlformats.org/officeDocument/2006/math">` +
	`<m:sSup><m:e><m:d><m:e><m:r><m:t>x+a</m:t></m:r></m:e></m:d></m:e><m:sup><m:r><m:t>n</m:t></m:r></m:sup></m:sSup>` +
	`<m:r><m:t>=</m:t></m:r>` +
	`<m:nary><m:naryPr><m:chr m:val="∑"/></m:naryPr><m:sub><m:r><m:t>k=0</m:t></m:r></m:sub>` +
	`<m:sup><m:r><m:t>n</m:t></m:r></m:sup><m:e><m:d><m:e>` +
	`<m:f><m:fPr><m:type m:val="noBar"/></m:fPr><m:num><m:r><m:t>n</m:t></m:r></m:num><m:den><m:r><m:t>k</m:t></m:r></m:den></m:f>` +
	`</m:e></m:d></m:e></m:nary></m:oMath>`
