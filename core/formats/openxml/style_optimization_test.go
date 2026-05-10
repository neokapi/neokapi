package openxml

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindParagraphs_simple(t *testing.T) {
	src := []byte(`<w:body><w:p><w:r><w:t>hello</w:t></w:r></w:p></w:body>`)
	got := findParagraphs(src)
	assert.Len(t, got, 1)
	assert.Equal(t, "<w:p><w:r><w:t>hello</w:t></w:r></w:p>", string(src[got[0].start:got[0].end]))
}

func TestFindRuns_basic(t *testing.T) {
	src := []byte(`<w:p><w:r><w:t>a</w:t></w:r><w:r><w:t>b</w:t></w:r></w:p>`)
	got := findRuns(src)
	assert.Len(t, got, 2)
}

func TestParseRunPropElements_basic(t *testing.T) {
	src := []byte(`<w:rPr><w:rFonts w:ascii="Arial"/><w:b/></w:rPr>`)
	got := parseRunPropElements(src)
	assert.Len(t, got, 2)
	assert.Equal(t, "rFonts", got[0].name)
	assert.Equal(t, "b", got[1].name)
}

func TestOptimizeWMLPart_MultipleRunsCommonProps(t *testing.T) {
	// Two runs with the same rFonts — common prop should be extracted
	// into a synthesised style. Mirrors the 1437-color-exclusion fixture
	// shape (multi-run paragraphs where Okapi factors out a common rPr
	// shape into a paragraph style).
	src := []byte(`<w:body><w:p><w:r><w:rPr><w:rFonts w:ascii="Arial"/></w:rPr><w:t>a</w:t></w:r><w:r><w:rPr><w:rFonts w:ascii="Arial"/></w:rPr><w:t>b</w:t></w:r></w:p></w:body>`)
	existing := map[string]bool{}
	counters := map[string]int{}
	syn := map[string]synthesisedStyle{}
	var ids []string
	got := optimizeWMLPart(src, existing, counters, syn, &ids)
	assert.Contains(t, string(got), "NF974E24F-Normal1")
	assert.Len(t, ids, 1)
}

func TestOptimizeWMLPart_SingleRun_Optimised(t *testing.T) {
	// Post-#592, 1-run paragraphs are also optimised, mirroring upstream
	// Okapi StyleOptimisation.Default.applyTo (StyleOptimisation.java
	// line 98 bypasses only when chunks.size() <= 2 == 0 runs). The
	// writer now preserves per-source-run rPr on every emitted <w:r>
	// (#592 — see source_rpr.go), so the run carries the same rPr
	// payload Okapi sees and the optimisation premise — common props
	// across rendered runs — holds for 1-run paragraphs too.
	src := []byte(`<w:body><w:p><w:r><w:rPr><w:b/></w:rPr><w:t>a</w:t></w:r></w:p></w:body>`)
	existing := map[string]bool{}
	counters := map[string]int{}
	syn := map[string]synthesisedStyle{}
	var ids []string
	got := optimizeWMLPart(src, existing, counters, syn, &ids)
	assert.Contains(t, string(got), "NF974E24F-Normal1")
	assert.Len(t, ids, 1)
}

func TestOptimizeWMLPart_SingleRun_RStyle_Bypassed(t *testing.T) {
	// rStyle (character style reference) is in the WSO exclusion list
	// (mirrors upstream WordDocument.java construction of
	// StyleOptimisation.Default with Collections.singletonList(rStyle)).
	// A 1-run paragraph whose only rPr child is rStyle must NOT be
	// optimised — the rStyle stays on the run.
	src := []byte(`<w:body><w:p><w:r><w:rPr><w:rStyle w:val="Emphasis"/></w:rPr><w:t>a</w:t></w:r></w:p></w:body>`)
	existing := map[string]bool{}
	counters := map[string]int{}
	syn := map[string]synthesisedStyle{}
	var ids []string
	got := optimizeWMLPart(src, existing, counters, syn, &ids)
	assert.NotContains(t, string(got), "NF974E24F")
	assert.Len(t, ids, 0)
	// Bypass keeps rStyle on the run.
	assert.Contains(t, string(got), `<w:rStyle w:val="Emphasis"/>`)
}

func TestOptimizeWMLPart_SingleRun_Rtl_Bypassed(t *testing.T) {
	// rtl (run-level RTL direction marker) is also in the WSO exclusion
	// list — observed parity behaviour in reordered-zip.docx (Okapi
	// keeps <w:rtl> on the run rather than lifting it into a synthesised
	// paragraph style). #592.
	src := []byte(`<w:body><w:p><w:r><w:rPr><w:rtl w:val="0"/></w:rPr><w:t>a</w:t></w:r></w:p></w:body>`)
	existing := map[string]bool{}
	counters := map[string]int{}
	syn := map[string]synthesisedStyle{}
	var ids []string
	got := optimizeWMLPart(src, existing, counters, syn, &ids)
	assert.NotContains(t, string(got), "NF974E24F")
	assert.Len(t, ids, 0)
	assert.Contains(t, string(got), `<w:rtl w:val="0"/>`)
}

func TestInsertPStyle_OpenCloseFormStripped(t *testing.T) {
	// captureRawElement re-emits self-closing pStyle elements in
	// open/close form ("<w:pStyle ...></w:pStyle>"). insertPStyle must
	// strip the existing element regardless of which form the source
	// uses — otherwise a paragraph that already has <w:pStyle> ends up
	// with TWO pStyle children (the new synthesised one, then the
	// preserved original). Pre-#592 this never bit because the writer
	// dropped per-source rPr and WSO never fired on these paragraphs.
	// See gettysburg_en.docx for the original repro fixture.
	src := []byte(`<w:pPr><w:pStyle w:val="style0"></w:pStyle><w:jc w:val="center"/></w:pPr>`)
	got := insertPStyle(src, "NF974E24F-style01")
	assert.Equal(t,
		`<w:pPr><w:pStyle w:val="NF974E24F-style01"/><w:jc w:val="center"/></w:pPr>`,
		string(got),
		"open/close <w:pStyle> form must be stripped before inserting the synthesised id",
	)
}

func TestParseRunProps_PreservesNonToggleChildren(t *testing.T) {
	// #592: parseRunProps must capture every non-toggle <w:rPr> child as
	// a writer-friendly serialisation in props.rPrChildren so the writer
	// can re-emit it on every <w:r>. Toggle children (b, i, u, strike,
	// vertAlign, vanish) are intentionally excluded because the writer
	// reconstructs them from PcOpen/PcClose runs.
	src := `<w:rPr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:rStyle w:val="Emphasis"/><w:rFonts w:ascii="Arial"/><w:color w:val="FF0000"/><w:sz w:val="24"/><w:b/><w:i/></w:rPr>`
	dec := xml.NewDecoder(strings.NewReader(src))
	// Skip past the opening <w:rPr> token.
	_, err := dec.Token()
	require.NoError(t, err)
	props, err := parseRunProps(dec, false)
	require.NoError(t, err)
	// Toggles parsed into struct fields.
	assert.True(t, props.bold)
	assert.True(t, props.italic)
	// Non-toggles kept verbatim in source order.
	names := make([]string, 0, len(props.rPrChildren))
	for _, c := range props.rPrChildren {
		names = append(names, c.name)
	}
	assert.Equal(t, []string{"rStyle", "rFonts", "color", "sz"}, names)
	// Each child uses the "w:" element prefix the writer needs.
	assert.Equal(t, `<w:rStyle w:val="Emphasis"/>`, props.rPrChildren[0].xml)
	assert.Equal(t, `<w:color w:val="FF0000"/>`, props.rPrChildren[2].xml)
}

func TestParseRunProps_SkipsLangNoProof(t *testing.T) {
	// lang and noProof are stripped by upstream RunSkippableElements
	// (RunSkippableElements.java lines 50-62). The native reader must
	// match — otherwise these elements leak into the per-paragraph
	// common rPr and get lifted into a synthesised pStyle that Okapi
	// did not generate. See 1080-1.docx for the original repro
	// (paragraph whose only run rPr is <w:lang w:val="en-US"/>).
	src := `<w:rPr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:lang w:val="en-US"/><w:noProof/><w:rStyle w:val="X"/></w:rPr>`
	dec := xml.NewDecoder(strings.NewReader(src))
	_, err := dec.Token()
	require.NoError(t, err)
	props, err := parseRunProps(dec, false)
	require.NoError(t, err)
	names := make([]string, 0, len(props.rPrChildren))
	for _, c := range props.rPrChildren {
		names = append(names, c.name)
	}
	assert.Equal(t, []string{"rStyle"}, names,
		"lang and noProof must be skipped from rPrChildren capture")
}

func TestCommonRPrChildren_IntersectionAcrossRuns(t *testing.T) {
	// #592: commonRPrChildren computes the per-paragraph intersection
	// of source-run rPr children, mirroring upstream Okapi
	// StyleOptimisation.commonRunPropertiesOf
	// (StyleOptimisation.java lines 204-237). Children present and
	// equal across every text-bearing source run survive; the rest
	// drop out.
	runs := []textRun{
		{
			text: "A",
			props: runProps{rPrChildren: []rPrChild{
				{name: "rFonts", xml: `<w:rFonts w:ascii="Arial"/>`},
				{name: "color", xml: `<w:color w:val="FF0000"/>`},
				{name: "sz", xml: `<w:sz w:val="24"/>`},
			}},
		},
		{
			text: "B",
			props: runProps{rPrChildren: []rPrChild{
				{name: "rFonts", xml: `<w:rFonts w:ascii="Arial"/>`},
				{name: "color", xml: `<w:color w:val="00FF00"/>`}, // differs
				{name: "sz", xml: `<w:sz w:val="24"/>`},
			}},
		},
	}
	common := commonRPrChildren(runs)
	names := make([]string, 0, len(common))
	for _, c := range common {
		names = append(names, c.name)
	}
	assert.Equal(t, []string{"rFonts", "sz"}, names)
}

func TestCommonRPrChildren_RunWithoutRPrClearsCommon(t *testing.T) {
	// Per upstream Okapi behaviour (StyleOptimisation.java lines 224-228):
	// if any run carries an empty rPr, the common-property set is cleared.
	// This guards against lifting rPr into a synthesised paragraph style
	// when at least one run has direct heterogeneous formatting.
	runs := []textRun{
		{text: "A", props: runProps{rPrChildren: []rPrChild{
			{name: "rFonts", xml: `<w:rFonts w:ascii="Arial"/>`},
		}}},
		{text: "B", props: runProps{}}, // no rPr at all
	}
	common := commonRPrChildren(runs)
	assert.Empty(t, common)
}
