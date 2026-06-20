package math_test

import (
	"testing"

	math "github.com/neokapi/neokapi/core/math"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A namespace-less OMML fragment (as captured from a .docx) mixing math runs
// with two <m:nor/> prose runs: x  where  y  otherwise.
const norOMML = `<m:oMath>` +
	`<m:r><m:t>x</m:t></m:r>` +
	`<m:r><m:rPr><m:nor/></m:rPr><m:t>where</m:t></m:r>` +
	`<m:r><m:t>y</m:t></m:r>` +
	`<m:r><m:rPr><m:nor/></m:rPr><m:t>otherwise</m:t></m:r>` +
	`</m:oMath>`

func TestNorTexts(t *testing.T) {
	assert.Equal(t, []string{"where", "otherwise"}, math.NorTexts([]byte(norOMML)))
	// Pure math has no prose spans.
	assert.Empty(t, math.NorTexts([]byte(`<m:oMath><m:r><m:t>x+y</m:t></m:r></m:oMath>`)))
}

// NorSpans returns each prose span's text plus a byte range that slices the
// exact CharData out of the original fragment — the contract the docx
// sub-skeleton relies on for a byte-exact splice.
func TestNorSpans(t *testing.T) {
	spans := math.NorSpans([]byte(norOMML))
	require.Len(t, spans, 2)
	assert.Equal(t, "where", spans[0].Text)
	assert.Equal(t, "where", norOMML[spans[0].Start:spans[0].End], "range slices the span's text")
	assert.Equal(t, "otherwise", spans[1].Text)
	assert.Equal(t, "otherwise", norOMML[spans[1].Start:spans[1].End])
	// Offsets are monotonic and in range.
	assert.Less(t, spans[0].End, spans[1].Start)
	assert.LessOrEqual(t, spans[1].End, len(norOMML))
	// Pure math has no spans.
	assert.Empty(t, math.NorSpans([]byte(`<m:oMath><m:r><m:t>x+y</m:t></m:r></m:oMath>`)))
}

// nil / all-empty translations leave the OMML byte-identical.
func TestSpliceNorTextByteExactNoop(t *testing.T) {
	assert.Equal(t, norOMML, string(math.SpliceNorText([]byte(norOMML), nil)))
	assert.Equal(t, norOMML, string(math.SpliceNorText([]byte(norOMML), []string{"", ""})))
	// A translation equal to the original is also a no-op.
	assert.Equal(t, norOMML, string(math.SpliceNorText([]byte(norOMML), []string{"where", "otherwise"})))
}

// Translations replace only the corresponding nor spans; math text and all other
// bytes (tags, rPr, nor markers) are preserved exactly.
func TestSpliceNorTextReplaces(t *testing.T) {
	out := string(math.SpliceNorText([]byte(norOMML), []string{"der", "ellers"}))
	assert.Contains(t, out, `<m:rPr><m:nor/></m:rPr><m:t>der</m:t>`)
	assert.Contains(t, out, `<m:rPr><m:nor/></m:rPr><m:t>ellers</m:t>`)
	// Math runs untouched.
	assert.Contains(t, out, `<m:r><m:t>x</m:t></m:r>`)
	assert.Contains(t, out, `<m:r><m:t>y</m:t></m:r>`)
	assert.NotContains(t, out, "where")
	assert.NotContains(t, out, "otherwise")

	// A short slice translates only the spans it covers.
	out2 := string(math.SpliceNorText([]byte(norOMML), []string{"der"}))
	assert.Contains(t, out2, `<m:t>der</m:t>`)
	assert.Contains(t, out2, `<m:t>otherwise</m:t>`, "uncovered span stays original")
}

// The replacement text is XML-escaped.
func TestSpliceNorTextEscapes(t *testing.T) {
	out := string(math.SpliceNorText([]byte(norOMML), []string{"a < b & c", ""}))
	assert.Contains(t, out, `<m:t>a &lt; b &amp; c</m:t>`)
}

// Round-trip via the converter still sees the translated prose.
func TestSpliceNorThenConvert(t *testing.T) {
	out := math.SpliceNorText([]byte(norOMML), []string{"WHERE", "OTHERWISE"})
	m, err := math.FromOMML(out)
	require.NoError(t, err)
	assert.Equal(t, "WHERE OTHERWISE", m.TranslatableText())
}
