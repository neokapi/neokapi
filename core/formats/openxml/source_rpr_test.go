package openxml

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests cover the faithful per-run rPr machinery (#592): the
// reader's run-property parsing (parseRunProps / parseRunPropsFromRaw)
// that feeds the per-run rPr sidecar, and the per-paragraph
// intersection (commonRPrChildren in source_rpr.go) the writer prepends
// on every emitted <w:r>. They were previously colocated with the now-
// deleted Word Style Optimisation tests but exercise the always-on
// faithful path, not WSO.

func TestParseRunProps_StripsDefaultValuedRtl(t *testing.T) {
	// rtl is a WpmlToggleRunProperty (RunPropertyFactory.java:219).
	// Toggle properties default to "true" per ECMA-376-1 §17.3.2, so
	// `<w:rtl w:val="0"/>` is a no-op that upstream Okapi strips at
	// parse time via RunProperties.minified() (RunParser.java:280-294 +
	// RunProperties.java:497-540). minifyRPrChildren mirrors that
	// pre-WSO step in native, so the rtl child never reaches
	// rPrChildren and the writer never re-emits it.
	//
	// Without this, redundant `<w:rtl w:val="0"/>` rPrs round-trip
	// into the per-run sidecar (reordered-zip.docx fixture).
	src := `<w:rPr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:rtl w:val="0"/></w:rPr>`
	dec := xml.NewDecoder(strings.NewReader(src))
	_, err := dec.Token()
	require.NoError(t, err)
	props, err := parseRunProps(dec, false, nil)
	require.NoError(t, err)
	for _, c := range props.rPrChildren {
		if c.name == "rtl" {
			t.Fatalf("rtl should be stripped by minified(), got %q", c.xml)
		}
	}
}

func TestParseRunProps_KeepsExplicitRtlTrue(t *testing.T) {
	// A bare `<w:rtl/>` (or explicit `w:val="1"` / `"true"`) is the
	// actual on-toggle and must travel through to the writer. Only the
	// no-op default (false-equivalent values) gets minified out.
	src := `<w:rPr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:rtl/></w:rPr>`
	dec := xml.NewDecoder(strings.NewReader(src))
	_, err := dec.Token()
	require.NoError(t, err)
	props, err := parseRunProps(dec, false, nil)
	require.NoError(t, err)
	found := false
	for _, c := range props.rPrChildren {
		if c.name == "rtl" {
			found = true
			break
		}
	}
	assert.True(t, found, "bare <w:rtl/> must be preserved (toggle defaults to true)")
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
	props, err := parseRunProps(dec, false, nil)
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
	// common rPr. See 1080-1.docx for the original repro (paragraph
	// whose only run rPr is <w:lang w:val="en-US"/>).
	src := `<w:rPr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:lang w:val="en-US"/><w:noProof/><w:rStyle w:val="X"/></w:rPr>`
	dec := xml.NewDecoder(strings.NewReader(src))
	_, err := dec.Token()
	require.NoError(t, err)
	props, err := parseRunProps(dec, false, nil)
	require.NoError(t, err)
	names := make([]string, 0, len(props.rPrChildren))
	for _, c := range props.rPrChildren {
		names = append(names, c.name)
	}
	assert.Equal(t, []string{"rStyle"}, names,
		"lang and noProof must be skipped from rPrChildren capture")
}

func TestParseRunPropsFromRaw_PreservesNoProofInStrict(t *testing.T) {
	// The drawing-bearing run in 859.docx carries
	// `<w:rPr><w:noProof/><w:lang w:eastAsia="ru-RU"/></w:rPr>`.
	// parseRunPropsFromRaw is called with strict=true and re-hydrates
	// the rPrXML against the strict-namespace binding. With Fix #2
	// (runprops.go strict gate on noProof) both children should be
	// preserved in props.rPrChildren so the writer can emit them.
	rpr := `<w:rPr><w:noProof/><w:lang w:eastAsia="ru-RU"/></w:rPr>`
	props, err := parseRunPropsFromRaw(rpr, false, true, nil)
	require.NoError(t, err)
	names := make([]string, 0, len(props.rPrChildren))
	for _, c := range props.rPrChildren {
		names = append(names, c.name)
	}
	assert.Equal(t, []string{"noProof", "lang"}, names,
		"strict-namespace noProof + lang must both be preserved")
}

func TestParseRunPropsFromRaw_PreservesNoProofInStrict_OpenClose(t *testing.T) {
	// captureRawElement re-emits an empty element in OPEN-CLOSE form
	// (`<w:noProof></w:noProof>`) rather than self-closing. Make sure
	// the strict gate fires on this form too.
	rpr := `<w:rPr><w:noProof></w:noProof><w:lang w:eastAsia="ru-RU"></w:lang></w:rPr>`
	props, err := parseRunPropsFromRaw(rpr, false, true, nil)
	require.NoError(t, err)
	names := make([]string, 0, len(props.rPrChildren))
	for _, c := range props.rPrChildren {
		names = append(names, c.name)
	}
	assert.Equal(t, []string{"noProof", "lang"}, names,
		"strict-namespace noProof + lang must be preserved in open-close form too")
}

func TestParseRunPropsFromRaw_PreservesNoProofInStrict_Aggressive(t *testing.T) {
	// AggressiveCleanup defaults to TRUE in DefaultConfig (config.go:86),
	// and the aggressive branch in parseRunProps strips noProof. That
	// strip must also be gated on the transitional WPML namespace —
	// otherwise strict-OOXML noProof is dropped at the parser even
	// though the dedicated noProof strip below is correctly gated.
	rpr := `<w:rPr><w:noProof></w:noProof><w:lang w:eastAsia="ru-RU"></w:lang></w:rPr>`
	props, err := parseRunPropsFromRaw(rpr, true, true, nil)
	require.NoError(t, err)
	names := make([]string, 0, len(props.rPrChildren))
	for _, c := range props.rPrChildren {
		names = append(names, c.name)
	}
	assert.Equal(t, []string{"noProof", "lang"}, names,
		"strict-namespace noProof must survive aggressive cleanup")
}

func TestParseRunProps_PreservesLangNoProofInStrict(t *testing.T) {
	// For Strict OOXML documents (xmlns="http://purl.oclc.org/ooxml/
	// wordprocessingml/main") upstream Okapi's RunSkippableElements
	// QName for lang/noProof binds to the TRANSITIONAL URI only
	// (Namespaces.WordProcessingML.getQName, Namespaces.java:26 +
	// SkippableElement.java:207). Strict-namespace lang/noProof are
	// preserved on the run rPr — the native reader must mirror that.
	// 859.docx is the canonical fixture: a strict-OOXML drawing-bearing
	// run whose `<w:rPr><w:noProof/><w:lang w:eastAsia="ru-RU"/></w:rPr>`
	// must round-trip on the wire.
	src := `<w:rPr xmlns:w="http://purl.oclc.org/ooxml/wordprocessingml/main"><w:lang w:eastAsia="ru-RU"/><w:noProof/></w:rPr>`
	dec := xml.NewDecoder(strings.NewReader(src))
	_, err := dec.Token()
	require.NoError(t, err)
	props, err := parseRunProps(dec, false, nil)
	require.NoError(t, err)
	names := make([]string, 0, len(props.rPrChildren))
	for _, c := range props.rPrChildren {
		names = append(names, c.name)
	}
	assert.Equal(t, []string{"lang", "noProof"}, names,
		"lang and noProof must be preserved in strict-OOXML namespace")
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
	// This guards against lifting rPr into the per-run sidecar's common
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

func TestCommonRPrChildren_RFontsAttributeSubset(t *testing.T) {
	// Heterogeneous rFonts — same value but different attribute subsets
	// across runs (gettysburg_en.docx pattern). Mirrors upstream Okapi
	// RunFonts.canBeMerged + RunFonts.merge (RunFonts.java lines 190-247
	// and 267-315): RunMerger fuses adjacent runs whose rFonts are
	// mergeable. Native does not run RunMerger, so commonRPrChildren
	// approximates the upstream merge as the per-attribute intersection
	// of every run's rFonts. Per ECMA-376-1 §17.3.2.26 the rFonts
	// attributes (ascii, hAnsi, cs, eastAsia, *Theme, hint) are
	// independent and an rFonts may carry any subset.
	runs := []textRun{
		{text: "A", props: runProps{rPrChildren: []rPrChild{
			{name: "rFonts", xml: `<w:rFonts w:ascii="DejaVu Serif" w:cs="DejaVu Serif" w:hAnsi="DejaVu Serif"/>`},
			{name: "b", xml: `<w:b/>`},
		}}},
		{text: "B", props: runProps{rPrChildren: []rPrChild{
			{name: "rFonts", xml: `<w:rFonts w:ascii="DejaVu Serif" w:cs="DejaVu Serif" w:eastAsia="DejaVu Serif" w:hAnsi="DejaVu Serif"/>`},
			{name: "b", xml: `<w:b/>`},
		}}},
	}
	common := commonRPrChildren(runs)
	require.Len(t, common, 2)
	// Pick out by name; order is implementation-defined.
	byName := map[string]rPrChild{}
	for _, c := range common {
		byName[c.name] = c
	}
	require.Contains(t, byName, "b")
	require.Contains(t, byName, "rFonts")
	// b survives unchanged.
	assert.Equal(t, `<w:b/>`, byName["b"].xml)
	// rFonts is the per-attribute intersection: ascii + cs + hAnsi
	// (eastAsia present in only one run is dropped).
	assert.Contains(t, byName["rFonts"].xml, `w:ascii="DejaVu Serif"`)
	assert.Contains(t, byName["rFonts"].xml, `w:cs="DejaVu Serif"`)
	assert.Contains(t, byName["rFonts"].xml, `w:hAnsi="DejaVu Serif"`)
	assert.NotContains(t, byName["rFonts"].xml, "eastAsia")
}

func TestCommonRPrChildren_RFontsValueDisagreementDrops(t *testing.T) {
	// When runs disagree on a shared rFonts attribute value, that
	// attribute drops out of the intersection. If nothing remains,
	// rFonts is excluded from the common set entirely (and stays on
	// each run).
	runs := []textRun{
		{text: "A", props: runProps{rPrChildren: []rPrChild{
			{name: "rFonts", xml: `<w:rFonts w:ascii="Arial"/>`},
		}}},
		{text: "B", props: runProps{rPrChildren: []rPrChild{
			{name: "rFonts", xml: `<w:rFonts w:ascii="Times"/>`},
		}}},
	}
	common := commonRPrChildren(runs)
	assert.Empty(t, common, "rFonts with disagreeing ascii drops; nothing else common")
}
